package sysd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

// Version 当前 sboxkit 版本（由 main 设置）。运行时资源——服务单元、bootstrap DNS、
// Web 面板、内核与规则集、运行时配置——都由部署它们的版本生成，部署成功后把该版本记入
// state/runtime.version。
var Version = "dev"

// 可在测试中替换。
var (
	runtimeInstalled = IsInstalled
	runtimeActive    = IsActive
	runtimeSync      = SyncAndRestart
)

// ReconcileRuntime 当前版本与记录的部署版本不同（自更新、apt 升级或换用新安装包后首次
// 运行）时整体重新同步一次运行时资源，之后不再打扰。只读状态目录里的记录，不读 root 专属
// 的运行时目录，因此日常启动无需授权。服务未安装或已暂停时不动；同步失败不记版本，下次
// 启动再试。
func ReconcileRuntime(p paths.Paths, name string) {
	if name == "" {
		name = DefaultName
	}
	if !RuntimeNeedsReconcile(p, name) {
		return
	}
	deployed := deployedRuntimeVersion(p)
	if deployed == "" {
		deployed = "?"
	}
	execx.Info(fmt.Sprintf(i18n.T("sboxkit 版本已变化（%s → %s），同步全部运行时资源（服务单元、Web 面板、内核与规则集、运行时配置）…"), deployed, Version))
	if err := runtimeSync(p, name); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("同步运行时资源失败，下次启动时重试：%v"), err))
	}
}

// RuntimeNeedsReconcile 报告 ReconcileRuntime 此刻是否会重新同步（只读）：系统模式的
// 管理界面据此决定启动时是否需要提权。
func RuntimeNeedsReconcile(p paths.Paths, name string) bool {
	if name == "" {
		name = DefaultName
	}
	return runtimeInstalled(name) && runtimeActive(name) && deployedRuntimeVersion(p) != Version
}

func deployedRuntimeVersion(p paths.Paths) string {
	raw, err := os.ReadFile(p.RuntimeVersion)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// recordRuntimeVersion 部署成功后记下当前版本（0644：以 root 运行的定时更新写入后，
// 发起交互的普通用户仍可读取）。
func recordRuntimeVersion(p paths.Paths) error {
	return atomicWriteFile(p.RuntimeVersion, []byte(Version+"\n"), 0o644)
}

// recordRuntimeVersionBestEffort 记录失败只提示：资源已部署，最坏情况是下次启动多同步一次。
func recordRuntimeVersionBestEffort(p paths.Paths) {
	if err := recordRuntimeVersion(p); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("记录运行时资源版本失败：%v"), err))
	}
}
