// 更改配置全流程，拆成两个独立主菜单入口（对应 flows/modify.py）：
//
// ModifyConfig「配置变更」——订阅增删改 / 切换 / 刷新、部署设置、自定义分流
// 叠加，改动缓冲在事务里，esc「保存并退出」才提交，^R 回退本次会话全部改动，
// 需要重启服务才能生效。定制层字段分组（部署设置 / 自定义分流叠加）直接是
// 本菜单下的平级项，不再经过多余的「编辑定制层」中间层。
// ModifyRuntime「运行时管理」——节点实时切换 / 内核更新 / 服务重启 / 网络自愈 /
// 更新定时器，均为即时生效的系统操作（各自按需处理重启，无需你事后再单独
// 重启一次）。
package flows

import (
	"errors"
	"fmt"
	"os"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
	"github.com/Trilives/sboxkit/internal/txn"
)

var modifyConfigOptions = []string{
	"订阅管理（增 / 删 / 改名 / 切换 / 刷新）",
	"部署设置（TUN / 面板 / 下载）",
	"自定义分流叠加（AI / 流媒体 / 地区组）",
}

// 顺序按常用程度排列：切节点/查看服务状态是日常操作，自愈/定时器属一次性
// 设置项，排最后。临时切换与固定切换拆成两项：前者不写盘/不重启，后者才会
// 写盘并可选重启。
var modifyRuntimeOptions = []string{
	"节点切换",
	"固定节点",
	"服务设置",
	"更新",
	"网络自愈设置",
	"每周更新定时器",
}

// ModifyConfig 配置变更会话（需重启生效）：订阅管理 + 定制层字段分组编辑。
// 改动缓冲在事务里，esc 保存并退出才提交，^R 回退并退出则整体撤销。
func ModifyConfig(p paths.Paths) error {
	return modifySession(p, "配置变更", modifyConfigOptions, []func() error{
		func() error { return subscriptionsMenu(p) },
		func() error {
			return editFieldGroupFlow(p, "部署设置（TUN / 面板 / 下载）", config.DeploymentFields)
		},
		func() error {
			return editFieldGroupFlow(p, "自定义分流叠加（AI / 流媒体 / 地区组）", config.OverlayFields)
		},
	})
}

// ModifyRuntime 运行时管理会话（即时生效）：节点切换 / 内核更新 / sboxkit 自
// 更新 / 服务设置 / 网络自愈 / 更新定时器，均为即时生效的系统操作。
func ModifyRuntime(p paths.Paths, currentVersion string) error {
	return modifySession(p, "运行时管理", modifyRuntimeOptions, []func() error{
		func() error { return NodeSwitchLive(p, p.ConfigFile, "") },
		func() error { return NodeSelect(p, p.ConfigFile, "") },
		func() error { return serviceSettings(p) },
		func() error { return updateMenuFlow(p, currentVersion) },
		func() error { return resilienceMenuFlow() },
		func() error { return timerMenuFlow() },
	})
}

// modifySession 更改配置的公共会话骨架：快照 + 回退钩子 + 菜单循环。
func modifySession(p paths.Paths, title string, options []string, handlers []func() error) error {
	return txn.Run(i18n.T(title), func(session *txn.Transaction) error {
		// 会话开始即快照配置类路径，使任意改动都能被 ^R 统一回退
		for _, path := range []string{p.ConfigFile, p.ActiveFile, p.CustomizeFile, p.Subscriptions} {
			if err := session.Snapshot(path); err != nil {
				return err
			}
		}
		// 回退发生在文件还原之后（LIFO，最先登记 → 最后执行）：把运行中的服务对齐回退后的配置
		session.AddUndo(i18n.T("同步服务到回退后的配置"), func() error { resyncService(p); return nil })

		translated := make([]string, len(options))
		for i, o := range options {
			translated[i] = i18n.T(o)
		}
		idx := 0
		for {
			i, err := tui.Select(i18n.T(title), translated,
				tui.SelectOpts{BackLabel: i18n.T("回退并退出"), SaveLabel: i18n.T("保存并退出"), Initial: idx})
			if err != nil {
				if errors.Is(err, errs.ErrSaveExit) {
					return nil // esc = 保存并退出 → 事务提交
				}
				return err // 主菜单 ^R → 回退整个会话
			}
			idx = i
			if err := handlers[i](); err != nil {
				if errors.Is(err, errs.ErrSaveExit) {
					return nil // 子菜单选了「保存并退出」→ 提交整个会话
				}
				if errors.Is(err, errs.ErrCancelled) {
					continue // 单个操作中途取消 → 回主菜单，会话改动仍在缓冲中
				}
				execx.Error(err.Error()) // 单个操作失败不终结会话
			}
		}
	})
}

func resyncService(p paths.Paths) {
	if sysd.IsInstalled(sysd.DefaultName) && fileExists(p.ConfigFile) {
		if err := sysd.SyncAndRestart(p, sysd.DefaultName); err != nil {
			execx.Warn(fmt.Sprintf(i18n.T("服务同步失败：%v"), err))
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// --------------------------------------------------------------------------
// 订阅管理
// --------------------------------------------------------------------------

func subscriptionsMenu(p paths.Paths) error {
	act := 0
	for {
		subs := subscription.ListAll(p)
		activeName := ""
		if active := subscription.GetActive(p); active != nil {
			activeName = active.Name
		}
		execx.Header(i18n.T("订阅管理"))
		if len(subs) == 0 {
			fmt.Println(i18n.T("  • （暂无订阅）"))
		}
		for _, s := range subs {
			line := fmt.Sprintf(i18n.T("%s  [%s, %d 节点]"), s.Name, s.SourceType, s.LastNodeCount)
			if s.Name == activeName {
				line += i18n.T("  ← 生效")
			}
			fmt.Println("  • " + line)
		}
		// 顺序按常用程度排列：切换/刷新已有订阅最常用，新增/导入次之，改名/删除最少见。
		a, err := tui.Select(i18n.T("订阅操作"),
			[]string{i18n.T("切换生效订阅"), i18n.T("刷新订阅"), i18n.T("添加订阅"), i18n.T("本地文件覆盖"), i18n.T("重命名"), i18n.T("删除订阅")},
			tui.SelectOpts{BackLabel: i18n.T("返回上层"), Initial: act})
		if err != nil {
			return nil // 返回上层菜单（改动仍在会话缓冲中）
		}
		act = a
		ops := []func() error{
			func() error { return subSwitch(p) },
			func() error { return subRefresh(p) },
			func() error { return subAdd(p) },
			func() error { return importConfigFlow(p) },
			func() error { return subRename(p) },
			func() error { return subRemove(p) },
		}
		if err := ops[a](); err != nil {
			if errors.Is(err, errs.ErrCancelled) {
				continue
			}
			execx.Error(err.Error())
		}
	}
}

// maybeNodeSelect 订阅链接变化后，提示是否进入「切换 / 固定节点」。
func maybeNodeSelect(p paths.Paths) error {
	ok, err := tui.Confirm(i18n.T("订阅已更新，是否现在切换 / 固定节点？"), false)
	if err != nil || !ok {
		return err
	}
	return NodeSelect(p, p.ConfigFile, "")
}

func subAdd(p paths.Paths) error {
	info, err := askNewSubscription()
	if err != nil {
		return err
	}
	if info == nil {
		execx.Warn(i18n.T("订阅链接留空，已取消添加。"))
		return nil
	}
	setActive := subscription.GetActive(p) == nil
	if !setActive {
		setActive, err = tui.Confirm(i18n.T("设为生效订阅？"), true)
		if err != nil {
			return err
		}
	}
	if _, err := subscription.Add(p, info.Name, info.URL, info.SourceType, info.ApplyOverlay, setActive); err != nil {
		return err
	}
	if setActive {
		return maybeNodeSelect(p)
	}
	return nil
}

func pickSub(p paths.Paths, prompt string) (string, error) {
	subs := subscription.ListAll(p)
	if len(subs) == 0 {
		execx.Warn(i18n.T("暂无订阅。"))
		return "", nil
	}
	names := make([]string, len(subs))
	for i, s := range subs {
		names[i] = s.Name
	}
	idx, err := tui.Select(prompt, names, tui.SelectOpts{})
	if err != nil {
		return "", err
	}
	return names[idx], nil
}

func subSwitch(p paths.Paths) error {
	name, err := pickSub(p, i18n.T("切换到哪个订阅"))
	if err != nil || name == "" {
		return err
	}
	if err := subscription.Switch(p, name); err != nil {
		return err
	}
	return maybeNodeSelect(p)
}

func subRefresh(p paths.Paths) error {
	name, err := pickSub(p, i18n.T("刷新哪个订阅"))
	if err != nil || name == "" {
		return err
	}
	active := subscription.GetActive(p)
	if _, err := subscription.Refresh(p, name); err != nil {
		return err
	}
	if active != nil && active.Name == name {
		return maybeNodeSelect(p)
	}
	return nil
}

func subRename(p paths.Paths) error {
	name, err := pickSub(p, i18n.T("重命名哪个订阅"))
	if err != nil || name == "" {
		return err
	}
	newName, err := tui.Ask(i18n.T("新名称"), tui.AskOpts{AllowEmpty: false})
	if err != nil {
		return err
	}
	return subscription.Rename(p, name, newName)
}

func subRemove(p paths.Paths) error {
	name, err := pickSub(p, i18n.T("删除哪个订阅"))
	if err != nil || name == "" {
		return err
	}
	ok, err := tui.Confirm(fmt.Sprintf(i18n.T("确认删除订阅「%s」？"), name), false)
	if err != nil || !ok {
		return err
	}
	return subscription.RemoveSub(p, name)
}

// --------------------------------------------------------------------------
// 其它
// --------------------------------------------------------------------------

func editFieldGroupFlow(p paths.Paths, title string, fields []string) error {
	changed, err := EditFieldGroup(p, title, fields)
	if err != nil {
		return err
	}
	active := subscription.GetActive(p)
	if changed && active != nil {
		ok, err := tui.Confirm(i18n.T("立即用本地原文重新生成生效订阅并重启？（不重新拉取链接）"), true)
		if err != nil {
			return err
		}
		if ok {
			_, err = subscription.Rebuild(p, active.Name)
			return err
		}
	}
	return nil
}

// updateMenuFlow 聚合内核 / geo 数据 / sboxkit 自身三个独立更新入口，各自单独
// 触发、互不牵连。Web 面板内置在 sboxkit 二进制里（见 internal/uiassets），
// 随每次订阅生成自动物化，不需要单独的更新入口。
func updateMenuFlow(p paths.Paths, currentVersion string) error {
	options := []string{i18n.T("内核"), i18n.T("geo 数据"), i18n.T("sboxkit 自身")}
	handlers := []func() error{
		func() error { return updateCoreOnly(p) },
		func() error { return updateGeoOnly(p) },
		func() error { return SelfUpdateFlow(p, currentVersion) },
	}
	idx := 0
	for {
		i, err := tui.Select(i18n.T("更新"), options, tui.SelectOpts{BackLabel: i18n.T("返回上层"), Initial: idx})
		if err != nil {
			return nil
		}
		idx = i
		if err := handlers[i](); err != nil {
			if errors.Is(err, errs.ErrCancelled) {
				continue
			}
			execx.Error(err.Error())
		}
	}
}

func updateCoreOnly(p paths.Paths) error {
	ensureGithubToken(p)
	f, s := kernel.NewFetcher(p)
	if _, err := kernel.UpdateCore(p, f, s, true); err != nil {
		return err
	}
	return syncRestartIfInstalled(p)
}

func updateGeoOnly(p paths.Paths) error {
	ensureGithubToken(p)
	f, s := kernel.NewFetcher(p)
	if err := kernel.UpdateGeodata(p, f, s, true); err != nil {
		return err
	}
	return syncRestartIfInstalled(p)
}

func syncRestartIfInstalled(p paths.Paths) error {
	if fileExists(p.ConfigFile) && sysd.IsInstalled(sysd.DefaultName) {
		return sysd.SyncAndRestart(p, sysd.DefaultName)
	}
	return nil
}
