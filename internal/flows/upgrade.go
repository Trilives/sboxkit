// 组件升级（内核 / 规则集 / sboxkit 自身）的组合校验与应用：新组件先暂存或准备好，
// 校验「生成器 + 内核 + 规则集 + 生效订阅」组合以及现有生效配置在新组合下能否启动；
// 不兼容时说明所需版本与影响，并询问是否同时更新相关组件或重新生成配置。全部通过后
// 才在一个事务里换入组件、写入重新生成的配置并同步服务，失败整体回退；用户拒绝、
// 无人值守或升级不可用时放弃本次更新，原组合继续运行。
package flows

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/deploy"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/selfupdate"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
	"github.com/Trilives/sboxkit/internal/txn"
)

// upgrade 一次待应用的升级；nil 字段表示该组件不变。
type upgrade struct {
	assets *kernel.Staged
	tool   *selfupdate.Prepared
}

// upgradeMode interactive=false 时凡需用户决定的情形一律放弃；sync=false 时不碰服务。
type upgradeMode struct {
	interactive bool
	sync        bool
}

// verdict 升级后组合的校验结果。
type verdict struct {
	report      compat.Report // 生成器为生效订阅重新生成配置的校验结论
	regenerated []byte        // 兼容时重新生成的配置
	deployed    error         // 现有生效配置在新内核 / 规则集下的校验错误
}

// 可在测试中替换。
var (
	confirmUpgrade  = tui.Confirm
	toolCompatCheck = func(t *selfupdate.Prepared, p paths.Paths, b compat.Bundle) (compat.Report, []byte, error) {
		return t.CompatCheck(p, b)
	}
	prepareTool = func(p paths.Paths) (*selfupdate.Prepared, bool, error) {
		return selfupdate.Prepare(p, Version, selfupdate.Stable)
	}
	stageCore = func(p paths.Paths, s *kernel.Staged) error { return s.StageCore(p, true) }
)

func refusedUpgrade() error {
	return errors.New(i18n.T("已放弃本次更新：原 sboxkit / 内核 / 规则集组合保持不变，服务继续运行。"))
}

// runUpgrade 校验并应用 u；无论结果如何都清理暂存（已换入的组件不受影响）。
func runUpgrade(p paths.Paths, u *upgrade, mode upgradeMode) error {
	defer func() {
		u.assets.Discard()
		u.tool.Discard()
	}()
	regenerated, err := resolveUpgrade(p, u, mode)
	if err != nil {
		return err
	}
	return applyUpgrade(p, u, regenerated, mode)
}

func (u *upgrade) bundle(p paths.Paths) compat.Bundle {
	if u.assets != nil {
		return u.assets.Bundle(p)
	}
	return compat.StateBundle(p)
}

// resolveUpgrade 校验组合；生成器不兼容时询问是否补充升级另一组件并重新校验。
// 返回需要写入的重新生成配置（nil 表示沿用现有生效配置）。
func resolveUpgrade(p paths.Paths, u *upgrade, mode upgradeMode) ([]byte, error) {
	for {
		v, err := verifyUpgrade(p, u)
		if err != nil {
			return nil, err
		}
		if v.report.Error != "" {
			execx.Warn(v.report.Explain())
			extended, err := offerComplement(p, u, v.report, mode)
			if err != nil {
				// 补充组件不可用（如下载失败）：保留原因，并说明原组合不变。
				return nil, errors.Join(err, refusedUpgrade())
			}
			if !extended {
				return nil, refusedUpgrade()
			}
			continue
		}
		if err := acceptUnverified(u, v.report, mode); err != nil {
			return nil, err
		}
		return decideRegenerate(u, v, mode)
	}
}

func verifyUpgrade(p paths.Paths, u *upgrade) (verdict, error) {
	b := u.bundle(p)
	var v verdict
	if u.assets != nil && fileExists(p.ConfigFile) {
		raw, err := os.ReadFile(p.ConfigFile)
		if err != nil {
			return v, err
		}
		execx.Info(i18n.T("用即将部署的内核与规则集校验当前生效配置…"))
		if err := compat.Check(p, raw, b); err != nil && !errors.Is(err, compat.ErrKernelMissing) {
			v.deployed = err
		}
	}
	if u.tool == nil {
		execx.Info(i18n.T("用当前生成器为生效订阅重新生成配置并校验…"))
		v.report, v.regenerated = checkGenerator(p, b)
		return v, nil
	}
	execx.Info(fmt.Sprintf(i18n.T("用 sboxkit %s 的生成器为生效订阅重新生成配置并校验…"), u.tool.Version))
	report, cfg, err := toolCompatCheck(u.tool, p, b)
	if errors.Is(err, selfupdate.ErrNoCompatCheck) {
		report = compat.Report{Generator: u.tool.Version, Unverified: fmt.Sprintf(
			i18n.T("sboxkit %s 不支持兼容性自检，无法确认它生成的配置能被当前内核接受。"), u.tool.Version)}
		return verdict{report: report, deployed: v.deployed}, nil
	}
	if err != nil {
		return v, err
	}
	v.report, v.regenerated = report, cfg
	return v, nil
}

// offerComplement 生成器与内核不兼容时提议补齐另一侧：内核低于生成器要求时提议更新
// 内核；否则只换内核 / 规则集则提议更新 sboxkit，只换 sboxkit 则提议更新内核。
// 返回 true 表示已加入新组件、需要重新校验。
func offerComplement(p paths.Paths, u *upgrade, r compat.Report, mode upgradeMode) (bool, error) {
	if !mode.interactive {
		return false, nil
	}
	kernelStaged := u.assets != nil && u.assets.Kernel != ""
	switch {
	case r.NeedsNewerKernel() && !kernelStaged:
		return offerKernelUpdate(p, u)
	case u.tool == nil:
		return offerToolUpdate(p, u)
	case !kernelStaged:
		return offerKernelUpdate(p, u)
	}
	return false, nil
}

func offerToolUpdate(p paths.Paths, u *upgrade) (bool, error) {
	ok, err := confirmUpgrade(i18n.T("是否同时把 sboxkit 更新到最新稳定版（使用新版生成器）？"), true)
	if err != nil || !ok {
		return false, err
	}
	tool, latest, err := prepareTool(p)
	if err != nil {
		return false, err
	}
	if latest {
		execx.Warn(i18n.T("sboxkit 已是最新稳定版，暂无可用的新生成器。"))
		return false, nil
	}
	u.tool = tool
	return true, nil
}

func offerKernelUpdate(p paths.Paths, u *upgrade) (bool, error) {
	ok, err := confirmUpgrade(i18n.T("是否同时把 sing-box 内核更新到最新版？"), true)
	if err != nil || !ok {
		return false, err
	}
	if u.assets == nil {
		staged, err := kernel.NewStaging(p)
		if err != nil {
			return false, err
		}
		u.assets = staged
	}
	if err := stageCore(p, u.assets); err != nil {
		return false, err
	}
	if current := stateKernelVersion(p); current != "" && current == strings.TrimPrefix(u.assets.Version, "v") {
		execx.Warn(fmt.Sprintf(i18n.T("sing-box 内核已是最新版（%s），暂无可用的升级。"), current))
		return false, nil
	}
	return true, nil
}

// acceptUnverified 新版生成器无法自检时由用户决定（默认否）；内核侧无法重新生成时仅提示，
// 现有生效配置仍按新组合校验。
func acceptUnverified(u *upgrade, r compat.Report, mode upgradeMode) error {
	if r.Unverified == "" {
		return nil
	}
	execx.Warn(r.Unverified)
	if u.tool == nil {
		return nil
	}
	if !mode.interactive {
		return refusedUpgrade()
	}
	ok, err := confirmUpgrade(i18n.T("仍要应用这次更新吗？"), false)
	if err != nil {
		return err
	}
	if !ok {
		return refusedUpgrade()
	}
	return nil
}

// decideRegenerate 现有生效配置不被新组合接受时，只有重新生成（经用户同意）才能继续；
// 仅更新 sboxkit 时可选用新版生成器重新生成。
func decideRegenerate(u *upgrade, v verdict, mode upgradeMode) ([]byte, error) {
	if v.deployed != nil {
		execx.Warn(fmt.Sprintf(i18n.T("当前生效配置不被即将部署的内核与规则集接受：%v"), v.deployed) +
			"\n" + i18n.T("影响：直接应用会使服务在下次同步或重启时无法启动。"))
		if v.regenerated == nil || !mode.interactive {
			return nil, refusedUpgrade()
		}
		ok, err := confirmUpgrade(fmt.Sprintf(i18n.T("是否同时重新生成生效订阅「%s」的配置（已通过校验）并应用？"), v.report.Subscription), true)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, refusedUpgrade()
		}
		return v.regenerated, nil
	}
	if u.tool == nil || v.regenerated == nil || !mode.interactive || !mode.sync || !sysd.IsInstalled(sysd.DefaultName) {
		return nil, nil
	}
	ok, err := confirmUpgrade(fmt.Sprintf(i18n.T("用新版生成器重新生成生效订阅「%s」的配置并重启服务？"), v.report.Subscription), false)
	if err != nil || !ok {
		return nil, err
	}
	return v.regenerated, nil
}

// applyUpgrade 在一个事务里换入组件、写入重新生成的配置并同步服务；失败整体回退。
func applyUpgrade(p paths.Paths, u *upgrade, regenerated []byte, mode upgradeMode) error {
	active := subscription.GetActive(p)
	snapshot := []string{p.ConfigFile}
	if regenerated != nil && active != nil {
		snapshot = append(snapshot, p.SubscriptionDir(active.Name))
	}
	err := deploy.Apply(p, deploy.Change{
		Name:     i18n.T("应用更新"),
		Snapshot: snapshot,
		Sync:     mode.sync && fileExists(p.ConfigFile) && (u.assets != nil || regenerated != nil),
		Commit: func(t *txn.Transaction) error {
			return commitUpgrade(p, t, u, active, regenerated)
		},
	})
	if err != nil {
		return err
	}
	if u.tool != nil {
		u.tool.Finish(p)
	}
	return nil
}

func commitUpgrade(p paths.Paths, t *txn.Transaction, u *upgrade, active *subscription.Subscription, regenerated []byte) error {
	if u.assets != nil {
		restore, err := u.assets.Commit(p)
		if err != nil {
			return err
		}
		t.AddUndo(i18n.T("还原内核与规则集"), restore)
	}
	if u.tool != nil {
		undo, err := u.tool.Activate(p)
		if err != nil {
			return err
		}
		t.AddUndo(i18n.T("切回原版本 sboxkit"), undo)
	}
	if regenerated != nil && active != nil {
		return subscription.StoreGenerated(p, active.Name, regenerated)
	}
	return nil
}

func stateKernelVersion(p paths.Paths) string {
	raw, err := os.ReadFile(p.SingBoxVersion)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSpace(string(raw)), "v")
}
