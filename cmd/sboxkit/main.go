// sboxkit：在 Linux 上交互式部署 / 管理 sing-box。
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/flows"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
)

// version 由构建注入：-ldflags "-X main.version=..."。
var version = "dev"

func usageText() string {
	return i18n.T("用法: sboxkit [init|modify|nettest|uninstall|update|pause|resume|version]\n不带参数则进入交互式主菜单。")
}

func main() {
	p := paths.Detect()
	setupLanguage(p)
	setupLogging(p)
	args := os.Args[1:]
	if len(args) == 0 {
		exitIfErr(flows.EnsureStateRoot(p))
		os.Exit(interactive(p))
	}
	switch args[0] {
	case "init", "modify", "uninstall", "update", "pause", "resume":
		exitIfErr(flows.EnsureStateRoot(p))
	}
	switch args[0] {
	case "version", "--version", "-v":
		fmt.Println("sboxkit " + version)
	case "-h", "--help", "help":
		fmt.Println(usageText())
	case "init":
		if lerr := flows.PickLanguage(p); lerr != nil {
			execx.Error(lerr.Error())
		}
		exitFlow(flows.Init(p))
	case "modify":
		exitFlow(flows.ModifyConfig(p))
	case "nettest":
		exitFlow(flows.Nettest())
	case "uninstall":
		exitFlow(flows.Uninstall(p))
	case "update":
		os.Exit(runUpdate(p))
	case "pause":
		exitFlow(sysd.Pause(sysd.DefaultName))
	case "resume":
		exitFlow(sysd.Resume(sysd.DefaultName))
	case "healthcheck":
		os.Exit(sysd.RunHealthcheck(args[1:]))
	default:
		fmt.Fprintf(os.Stderr, i18n.T("未知子命令: %s\n%s\n"), args[0], usageText())
		os.Exit(2)
	}
}

// setupLanguage 确定界面语言：SBOXKIT_LANG 环境变量 > customize.json 里的
// language 字段 > 默认英文。config.Load 在状态目录不存在时安全返回默认值。
func setupLanguage(p paths.Paths) {
	if v := os.Getenv("SBOXKIT_LANG"); v == "zh" {
		i18n.SetLang(i18n.ZH)
		return
	} else if v == "en" {
		i18n.SetLang(i18n.EN)
		return
	}
	if config.Str(config.Load(p), "language") == "zh" {
		i18n.SetLang(i18n.ZH)
	}
}

// setupLogging customize.json 的 enable_log=true 时，把 execx 的输出额外写入
// 日志文件（超限自动裁剪旧内容，见 internal/execx/log.go）。
func setupLogging(p paths.Paths) {
	if !config.Bool(config.Load(p), "enable_log") {
		return
	}
	if err := execx.EnableLog(execx.LogPath(p.State), 0); err != nil {
		execx.Warn(i18n.T("日志启用失败：") + err.Error())
	}
}

func exitIfErr(err error) {
	if err != nil {
		execx.Error(err.Error())
		os.Exit(1)
	}
}

func exitFlow(err error) {
	if err != nil && !errors.Is(err, errs.ErrCancelled) {
		execx.Error(err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

// switchLabel 主菜单服务开关项标签：随主服务当前状态变化。
func switchLabel() string {
	if !sysd.IsInstalled(sysd.DefaultName) {
		return i18n.T("暂停 / 启动服务")
	}
	if sysd.IsActive(sysd.DefaultName) {
		return i18n.T("暂停服务 ⏸")
	}
	return i18n.T("启动服务 ▶")
}

func interactive(p paths.Paths) int {
	// 服务不存在（含已停止但仍注册的情况，IsInstalled 只看单元文件是否存在）时，
	// 先选语言（确保后续这句问询本身、以及整个初始化流程都以用户选定的语言展示），
	// 再询问是否现在进入初始化；主菜单不再需要单独的「初始化」入口，但是否进入由用户选择。
	if !sysd.IsInstalled(sysd.DefaultName) {
		if lerr := flows.PickLanguage(p); lerr != nil {
			execx.Error(lerr.Error())
		}
		ok, err := tui.Confirm(i18n.T("未检测到已注册的服务，是否现在进行初始化？"), true)
		if err == nil && ok {
			if ierr := flows.Init(p); ierr != nil && !errors.Is(ierr, errs.ErrCancelled) {
				execx.Error(ierr.Error())
			}
		}
	}
	idx := 0
	for {
		// 顺序按常用程度排列：运行时管理/配置变更/网络测试等日常操作在前，
		// 暂停/启动服务其次，卸载这类低频/破坏性操作放最后。
		options := []string{
			i18n.T("运行时管理"),
			i18n.T("配置变更"),
			i18n.T("工具"),
			switchLabel(),
			i18n.T("语言 / Language"),
			i18n.T("卸载"),
		}
		i, err := tui.Select(i18n.T("sing-box 部署系统"), options, tui.SelectOpts{BackLabel: i18n.T("退出"), Initial: idx})
		if err != nil {
			fmt.Println(i18n.T("再见。"))
			return 0
		}
		idx = i
		var aerr error
		switch i {
		case 0:
			aerr = flows.ModifyRuntime(p, version)
		case 1:
			aerr = flows.ModifyConfig(p)
		case 2:
			aerr = flows.ToolsMenu(p)
		case 3:
			aerr = flows.ServiceToggleFlow(p)
		case 4:
			aerr = flows.PickLanguage(p)
		case 5:
			aerr = flows.Uninstall(p)
		}
		if aerr != nil && !errors.Is(aerr, errs.ErrCancelled) {
			execx.Error(aerr.Error())
		}
	}
}

// runUpdate 非交互全量更新（每周定时器的执行目标）：内核+geo 强制更新 →
// 服务同步重启。
func runUpdate(p paths.Paths) int {
	if _, err := kernel.DownloadAll(p, kernel.Options{Force: true}); err != nil {
		execx.Error(err.Error())
		return 1
	}
	if sysd.IsInstalled(sysd.DefaultName) {
		if err := sysd.SyncAndRestart(p, sysd.DefaultName); err != nil {
			execx.Error(err.Error())
			return 1
		}
	}
	return 0
}
