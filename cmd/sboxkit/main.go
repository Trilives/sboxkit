// sboxkit：在 Linux 上交互式部署 / 管理 sing-box。
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Trilives/sboxkit/internal/bootstrapdns"
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
	return i18n.T("用法: sboxkit [init|modify|nettest|diagnose|uninstall|update|assets-update|pause|resume|version]\n不带参数则进入交互式主菜单。")
}

func main() {
	flows.Version = version
	sysd.Version = version
	p := paths.Detect()
	args, err := parseResume(os.Args[1:])
	exitIfErr(err)
	cmd := ""
	if len(args) > 0 {
		cmd = args[0]
	}
	isRoot := execx.IsRoot()
	// 普通用户读状态不构成提权风险；root 要等状态目录收归之后才读其中的 customize.json：
	// 收归前它可能是普通用户埋下的 FIFO 或符号链接。
	setupLanguage(p, p.UserMode || !isRoot)
	if reason, ok := elevationReason(p, cmd, isRoot); ok {
		exitIfErr(elevate(reason, args))
	}
	trusted := prepareState(p, cmd, isRoot)
	if !p.UserMode && isRoot && trusted {
		setupLanguage(p, true)
	}
	setupLogging(p, trusted)
	if len(args) == 0 {
		os.Exit(interactive(p))
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
		exitFlow(flows.Nettest(p))
	case "diagnose":
		os.Exit(runDiagnose(p, args[1:], os.Stdout, os.Stderr, flows.Diagnose))
	case "uninstall":
		exitFlow(flows.Uninstall(p))
	case "update":
		os.Exit(runUpdate(p))
	case "assets-update", "portable-update": // portable-update：旧版本名称，保留兼容
		os.Exit(runAssetsUpdate(p))
	case "pause":
		exitFlow(sysd.Pause(sysd.DefaultName))
	case "resume":
		exitFlow(sysd.Resume(sysd.DefaultName))
	case "healthcheck":
		if !p.UserMode && isRoot && !trusted {
			os.Exit(1) // 状态目录没能收归 root（原因已提示）：不以 root 读取其中的文件
		}
		os.Exit(sysd.RunHealthcheck(args[1:]))
	case "bootstrap-dns":
		os.Exit(bootstrapdns.Run(args[1:]))
	case "compat-check": // 内部：自更新前由旧版本调用，校验本版本生成器与指定内核的组合
		os.Exit(flows.RunCompatCheck(p, args[1:], os.Stderr))
	default:
		fmt.Fprintf(os.Stderr, i18n.T("未知子命令: %s\n%s\n"), args[0], usageText())
		os.Exit(2)
	}
}

// setupLanguage 确定界面语言：SBOXKIT_LANG 环境变量 > customize.json 里的
// language 字段 > 默认英文。readConfig=false 时只看环境变量。config.Load 在状态
// 目录不存在时安全返回默认值。
func setupLanguage(p paths.Paths, readConfig bool) {
	if v := os.Getenv("SBOXKIT_LANG"); v == "zh" {
		i18n.SetLang(i18n.ZH)
		return
	} else if v == "en" {
		i18n.SetLang(i18n.EN)
		return
	}
	if !readConfig {
		return
	}
	if config.Str(config.Load(p), "language") == "zh" {
		i18n.SetLang(i18n.ZH)
	}
}

// setupLogging customize.json 的 enable_log=true 时，把 execx 的输出额外写入
// 日志文件（超限自动裁剪旧内容，见 internal/execx/log.go）。trusted=false 表示
// 状态目录未确认可安全写入（见 prepareState），不写日志。
func setupLogging(p paths.Paths, trusted bool) {
	if !trusted {
		return
	}
	cfg := config.Load(p)
	if !cfg.EnableFileLog {
		return
	}
	if err := execx.EnableLog(execx.LogPath(p.State), int64(cfg.LogMaxMB)*1024*1024, p.SecretFileMode()); err != nil {
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

// firstRunDeps 把 maybeOfferFirstRunInit 依赖的外部调用抽成可注入字段，
// 便于单测覆盖「先选语言、再询问是否初始化」这个调用顺序，不必真的跑 TUI/systemd。
type firstRunDeps struct {
	isInstalled  func(string) bool
	pickLanguage func(paths.Paths) error
	confirm      func(string, bool) (bool, error)
	initFlow     func(paths.Paths) error
	reportError  func(string)
}

func defaultFirstRunDeps() firstRunDeps {
	return firstRunDeps{
		isInstalled:  sysd.IsInstalled,
		pickLanguage: flows.PickLanguage,
		confirm:      tui.Confirm,
		initFlow:     flows.Init,
		reportError:  execx.Error,
	}
}

// maybeOfferFirstRunInit 服务不存在（含已停止但仍注册的情况，IsInstalled 只看
// 单元文件是否存在）时，先选语言（确保后续这句问询本身、以及整个初始化流程都
// 以用户选定的语言展示），再询问是否现在进入初始化；主菜单不再需要单独的
// 「初始化」入口，但是否进入由用户选择。
func maybeOfferFirstRunInit(p paths.Paths, deps firstRunDeps) {
	if deps.isInstalled(sysd.DefaultName) {
		return
	}
	if err := deps.pickLanguage(p); err != nil {
		deps.reportError(err.Error())
		return
	}
	ok, err := deps.confirm(i18n.T("未检测到已注册的服务，是否现在进行初始化？"), true)
	if err == nil && ok {
		if ierr := deps.initFlow(p); ierr != nil && !errors.Is(ierr, errs.ErrCancelled) {
			deps.reportError(ierr.Error())
		}
	}
}

func interactive(p paths.Paths) int {
	// 升级后首次运行：运行时资源（单元、面板、内核与规则集、运行时配置）仍由旧版本生成，
	// 按版本记录整体同步一次；版本未变时不读运行时目录、不请求授权。
	sysd.ReconcileRuntime(p, sysd.DefaultName)
	// 用户模式下定时更新可能因 TUN 无法无人值守授权而推迟切换内核
	sysd.WarnPendingCore(p, sysd.DefaultName)
	if err := sysd.ReconcileResilience(p, sysd.DefaultName); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("修复网络自愈失败（主服务不受影响）：%v"), err))
	}
	maybeOfferFirstRunInit(p, defaultFirstRunDeps())
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
		i, err := flows.MenuSelect(i18n.T("sing-box 部署系统"), options, flows.MainMenuKeys, tui.SelectOpts{BackLabel: i18n.T("退出"), Initial: idx})
		if err != nil {
			fmt.Println(i18n.T("再见。"))
			return 0
		}
		idx = i
		var aerr error
		switch i {
		case 0:
			aerr = flows.ModifyRuntime(p)
		case 1:
			aerr = gated(p, flows.EntryConfig, func() error { return flows.ModifyConfig(p) })
		case 2:
			aerr = flows.ToolsMenu(p, version)
		case 3:
			aerr = gated(p, flows.EntryToggle, func() error { return flows.ServiceToggleFlow(p) })
		case 4:
			aerr = gated(p, flows.EntryLanguage, func() error { return flows.PickLanguage(p) })
		case 5:
			aerr = gated(p, flows.EntryUninstall, func() error { return flows.Uninstall(p) })
		}
		if aerr != nil && !errors.Is(aerr, errs.ErrCancelled) {
			execx.Error(aerr.Error())
		}
	}
}

// gated 系统模式下以普通用户运行时先提权（直接进入 entry），再执行 run。
func gated(p paths.Paths, entry flows.Entry, run func() error) error {
	if err := flows.RequireRoot(p, entry); err != nil {
		return err
	}
	return run()
}

// runUpdate 非交互全量更新（每周定时器的执行目标）：内核 + geo 最新版先暂存，
// 与当前生成器、生效订阅和生效配置的组合校验通过后才换入并同步重启服务；
// 不兼容时放弃更新、原组合继续运行，并以非零状态退出。
func runUpdate(p paths.Paths) int {
	opts := kernel.StageOptions{Core: true, Geo: true, Force: true}
	if err := flows.UpdateAssets(p, opts, false, true); err != nil {
		execx.Error(err.Error())
		return 1
	}
	return 0
}

// runAssetsUpdate updates managed assets but deliberately never touches
// systemd (formerly `portable-update`, still accepted as an alias). The same
// compatibility check as runUpdate guards the state, because the next service
// sync or restart picks the new kernel and rule sets up. sboxkit itself remains
// available through Tools > Update.
func runAssetsUpdate(p paths.Paths) int {
	opts := kernel.StageOptions{Core: true, Geo: true, Force: true}
	if err := flows.UpdateAssets(p, opts, false, false); err != nil {
		execx.Error(err.Error())
		return 1
	}
	return 0
}
