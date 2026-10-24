package main

import (
	"fmt"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/flows"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
)

// 系统模式的权限模型：状态目录只有 root 能写、管理员组可读，root 运行的服务与定时器
// 因此只读写 root 属主的文件。改动状态或服务的子命令启动即经 sudo 提权；管理界面先以
// 调用用户运行，选择改动操作时才提权（见 flows.RequireRoot）。

// writeCommands 改动状态或服务的子命令：需要可写状态目录（不存在时创建，系统模式下收归 root）。
var writeCommands = map[string]bool{
	"init": true, "modify": true, "uninstall": true, "update": true,
	"assets-update": true, "portable-update": true, "pause": true, "resume": true,
}

// stateReaders 由 root 定时器运行、读取 root 独占文件（customize.json、生效配置）的入口。
var stateReaders = map[string]bool{"healthcheck": true}

// launchCheck 可在测试中替换。
var launchCheck = flows.LaunchNeedsRoot

// elevationReason 系统模式下以普通用户运行 cmd 时需要先提权的原因；不需要时 ok=false。
// healthcheck 由 root 的 watchdog 单元运行，从不自行提权。
func elevationReason(p paths.Paths, cmd string, isRoot bool) (string, bool) {
	if p.UserMode || isRoot {
		return "", false
	}
	if writeCommands[cmd] {
		return i18n.T("该命令会改动系统服务或配置"), true
	}
	if cmd == "" {
		return launchCheck(p)
	}
	return "", false
}

// elevate 经 sudo 以 root 重新执行当前命令，成功时不返回。
func elevate(reason string, args []string) error {
	exe, err := sysd.StableExecutablePath()
	if err != nil {
		return err
	}
	execx.Info(fmt.Sprintf(i18n.T("%s：正在通过 sudo 以 root 重新启动 sboxkit…"), reason))
	return execx.ReexecAsRoot(exe, args)
}

// parseResume 识别提权重启附带的 --resume=<入口>：返回去掉该参数后的参数表。
func parseResume(args []string) ([]string, error) {
	if len(args) == 0 || !strings.HasPrefix(args[0], flows.ResumeFlag) {
		return args, nil
	}
	entry, ok := flows.ParseEntry(strings.TrimPrefix(args[0], flows.ResumeFlag))
	if !ok || len(args) > 1 {
		return nil, fmt.Errorf(i18n.T("无效的内部参数：%s"), strings.Join(args, " "))
	}
	flows.Resume(entry)
	return nil, nil
}

// prepareState 按入口准备状态目录，返回本进程能否放心写入其中（决定是否写日志、
// 何时读取 customize.json）。系统模式下 root 运行的入口先把整棵树收归 root：旧版本
// 把目录交给了调用用户，升级后首个 root 入口（含每 2 分钟一次的 watchdog）即完成迁移。
// 以普通用户运行的管理界面只读状态，不写日志。
func prepareState(p paths.Paths, cmd string, isRoot bool) bool {
	if writeCommands[cmd] || (cmd == "" && (p.UserMode || isRoot)) {
		exitIfErr(flows.EnsureStateRoot(p))
		return true
	}
	if p.UserMode {
		return true
	}
	if !isRoot || !stateReaders[cmd] {
		return false
	}
	if err := flows.SecureSystemState(p); err != nil {
		execx.Warn(err.Error())
		return false
	}
	return true
}
