// 服务注册 / 同步的前置阶段：前置检查、提权、暂存运行时配置，并用即将部署的内核与
// 规则集执行 `sing-box check`。全部通过后才覆盖运行时文件、停止或替换旧服务；
// 这一阶段的失败以 PrecheckError 返回，调用方据此判断运行时无需回退。
package sysd

import (
	"encoding/json"
	"os"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

// PrecheckError 服务注册 / 同步在改动运行时之前失败：运行时文件、单元与运行中的
// 服务都保持原样。
type PrecheckError struct{ Err error }

func (e *PrecheckError) Error() string { return e.Err.Error() }
func (e *PrecheckError) Unwrap() error { return e.Err }

func precheckFailed(err error) error {
	if err == nil {
		return nil
	}
	return &PrecheckError{Err: err}
}

// prepareRuntime 完成前置阶段并返回校验通过的暂存配置（调用方负责删除）。校验
// 不需要 root，先于提权执行：不兼容的配置不会触发 sudo 询问。checkConflict 用于
// 首次注册：拒绝与运行中的系统级同名服务争抢端口。
func prepareRuntime(p paths.Paths, name, reason string, checkConflict bool) (runtimePaths, string, error) {
	if err := preflight(p); err != nil {
		return runtimePaths{}, "", precheckFailed(err)
	}
	rt := rtPathsFor(scopeFor(p), name)
	// 随后写入的单元会执行 sboxkit 自身，路径不可信时在改动运行时之前拒绝（见 service_exe.go）。
	if _, err := serviceExecutable(rt.Scope); err != nil {
		return runtimePaths{}, "", precheckFailed(err)
	}
	staged, err := stageRuntimeConfig(p, rt)
	if err != nil {
		return runtimePaths{}, "", precheckFailed(err)
	}
	if err := admitRuntime(p, rt, name, reason, staged, checkConflict); err != nil {
		os.Remove(staged)
		return runtimePaths{}, "", precheckFailed(err)
	}
	if rt, err = withRuntimeIdentity(p, rt, staged); err != nil {
		os.Remove(staged)
		return runtimePaths{}, "", precheckFailed(err)
	}
	return rt, staged, nil
}

// withRuntimeIdentity 按暂存配置决定系统单元的运行身份，非特权运行时一并列出需代为登记的
// TUN DNS；仍以 root 运行时说明原因。
func withRuntimeIdentity(p paths.Paths, rt runtimePaths, staged string) (runtimePaths, error) {
	raw, err := os.ReadFile(staged)
	if err != nil {
		return rt, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return rt, err
	}
	identity, reason := chooseIdentity(p, rt.Scope, doc)
	if reason != "" {
		execx.Info(reason)
	}
	rt.Identity = identity
	if identity == identityDynamic {
		if rt.TunDNS, err = tunDNSLinks(doc); err != nil {
			return rt, err
		}
	}
	return rt, nil
}

func admitRuntime(p paths.Paths, rt runtimePaths, name, reason, staged string, checkConflict bool) error {
	if err := validateStaged(p, rt, staged); err != nil {
		return err
	}
	if err := rt.Scope.ensurePrivilege(reason); err != nil {
		return err
	}
	if checkConflict {
		return checkScopeConflict(rt.Scope, name)
	}
	return nil
}

// validateStaged 用部署后服务实际运行的内核与 state 规则集（deployRuntimeAssets 部署的
// 正是这两份）校验暂存配置。
func validateStaged(p paths.Paths, rt runtimePaths, staged string) error {
	raw, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	kernel, err := deployKernel(p, rt, staged)
	if err != nil {
		return err
	}
	execx.Info(i18n.T("用即将部署的内核校验配置…"))
	return compat.Check(p, raw, compat.Bundle{Kernel: kernel, Geosite: p.GeositeCN, GeoIP: p.GeoIPCN})
}

// deployKernel 本次部署后服务运行的内核：用户模式推迟切换（coreDefer）时仍是运行时旧内核。
func deployKernel(p paths.Paths, rt runtimePaths, staged string) (string, error) {
	action, _, err := planRuntimeCore(p, rt, staged)
	if err != nil {
		return "", err
	}
	if action == coreDefer {
		return rt.Bin, nil
	}
	return p.SingBoxBin, nil
}
