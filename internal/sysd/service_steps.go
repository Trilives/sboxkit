package sysd

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

// installRuntimeConfig 把暂存配置装入运行时并用运行时内核校验；校验失败发生在
// 替换单元之前，旧服务保持原样。
func installRuntimeConfig(rt runtimePaths, staged string) error {
	s := rt.Scope
	if _, err := s.run([]string{"install", "-m", runtimeConfigMode, staged, rt.Config}, "", nil); err != nil {
		return err
	}
	if _, err := s.run([]string{rt.Bin, "check", "-c", rt.Config}, i18n.T("校验配置"), nil); err != nil {
		return err
	}
	return ensureNetCapabilities(rt)
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
