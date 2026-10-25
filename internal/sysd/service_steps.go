package sysd

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

// deployRuntime 部署内核、规则集、面板与已校验的暂存配置（prepareRuntime 之后调用）。
func deployRuntime(p paths.Paths, rt runtimePaths, staged string) error {
	if err := deployRuntimeAssets(p, rt, staged); err != nil {
		return err
	}
	// UI（内置静态面板，见 internal/uiassets；缺失也不影响内核运行）
	if err := syncUIRuntime(rt); err != nil {
		execx.Warn(i18n.T("刷新内置 Web 面板资源失败：") + err.Error())
	}
	return installRuntimeConfig(rt, staged)
}

// installRuntimeConfig 把已校验的暂存配置装入运行时，并按需授予网络能力；用户模式启用 TUN 时
// 还要安装代为登记系统 DNS 的系统单元（见 tundns_user.go）。
func installRuntimeConfig(rt runtimePaths, staged string) error {
	if _, err := rt.Scope.run([]string{"install", "-m", runtimeConfigMode, staged, rt.Config}, "", nil); err != nil {
		return err
	}
	if err := ensureNetCapabilities(rt); err != nil {
		return err
	}
	return ensureUserTunDNS(rt)
}

// writeMainUnit 渲染并写入主服务单元，随后 daemon-reload。
func writeMainUnit(name string, rt runtimePaths) error {
	unitText, err := renderUnit(name, rt)
	if err != nil {
		return err
	}
	if err := rt.Scope.write(rt.Unit, unitText, "0644", i18n.T("写服务单元")); err != nil {
		return err
	}
	_, err = rt.Scope.run(rt.Scope.systemctl("daemon-reload"), "", nil)
	return err
}

// checkScopeConflict 用户模式服务与系统模式同名服务会争抢 mixed / 面板 / relay 端口。
// 系统服务正在运行时，用户服务启动后只会绑定失败并反复重启，因此直接拒绝安装；
// 已安装但未运行时仅提示。用户级安装无权替用户停掉系统服务。
func checkScopeConflict(s unitScope, name string) error {
	if !s.User {
		return nil
	}
	systemUnit := unitScope{}.unitFile(name + ".service")
	if !fileExists(systemUnit) {
		return nil
	}
	return scopeConflict(systemUnit, unitActive(unitScope{}, name+".service"))
}

func scopeConflict(systemUnit string, active bool) error {
	if active {
		return fmt.Errorf(i18n.T("系统模式服务 %s 正在运行，会占用用户模式服务所需的端口；请先在系统模式 sboxkit 中暂停或卸载（sudo sboxkit pause）后再试。"), systemUnit)
	}
	execx.Warn(fmt.Sprintf(i18n.T("检测到系统级服务 %s，它与用户模式用户级服务会争抢相同端口；请先在系统版 sboxkit 中暂停或卸载。"), systemUnit))
	return nil
}
