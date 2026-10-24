package flows

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
)

// 系统模式的提权模型：状态只有 root 能写、管理员组可读。管理界面先以调用用户运行，
// 浏览菜单、实时切换节点、网络测试、诊断、日志与信息都不需要密码；第一次选择会改动
// 状态或服务的操作时，经 sudo 以 root 重新启动 sboxkit 并直接进入该操作（--resume），
// 此后整个会话保持 root。

// Entry 提权后可直接进入的菜单项，以菜单路径表示；只接受下列固定值。
type Entry string

const (
	EntryConfig         Entry = "config"
	EntryToggle         Entry = "toggle"
	EntryLanguage       Entry = "language"
	EntryUninstall      Entry = "uninstall"
	EntryPinNode        Entry = "runtime/pin"
	EntryServiceRestart Entry = "runtime/service/restart"
	EntryServiceReload  Entry = "runtime/service/reload"
	EntryResilience     Entry = "runtime/resilience"
	EntryTimer          Entry = "runtime/timer"
	EntryUpdate         Entry = "tools/update"
)

var knownEntries = []Entry{
	EntryConfig, EntryToggle, EntryLanguage, EntryUninstall, EntryPinNode,
	EntryServiceRestart, EntryServiceReload, EntryResilience, EntryTimer, EntryUpdate,
}

// ResumeFlag 提权重启时附加的内部参数：--resume=<Entry>。
const ResumeFlag = "--resume="

// 各菜单选项的路径名，与选项一一对应。
var (
	MainMenuKeys    = []string{"runtime", "config", "tools", "toggle", "language", "uninstall"}
	runtimeMenuKeys = []string{"switch", "pin", "service", "resilience", "timer"}
	serviceMenuKeys = []string{"status", "restart", "reload"}
	toolMenuKeys    = []string{"nettest", "diagnose", "logs", "files", "info", "update", "usage"}
)

// ParseEntry 校验 --resume 的取值。
func ParseEntry(value string) (Entry, bool) {
	for _, e := range knownEntries {
		if string(e) == value {
			return e, true
		}
	}
	return "", false
}

// resumePath 待恢复的菜单路径，逐级被菜单消费。
var resumePath []string

// Resume 记录提权后要直接进入的菜单项（main 解析 --resume 后调用）。
func Resume(e Entry) { resumePath = strings.Split(string(e), "/") }

// takeResume 路径的下一段是本菜单的某个选项时消费它并返回其下标；对不上时放弃恢复，
// 回到正常交互。
func takeResume(keys []string) (int, bool) {
	if len(resumePath) == 0 {
		return 0, false
	}
	for i, key := range keys {
		if key == resumePath[0] {
			resumePath = resumePath[1:]
			return i, true
		}
	}
	resumePath = nil
	return 0, false
}

// MenuSelect 菜单选择：有待恢复的提权入口时直接返回对应选项，否则交给 tui.Select。
func MenuSelect(title string, options, keys []string, opts tui.SelectOpts) (int, error) {
	if i, ok := takeResume(keys); ok {
		return i, nil
	}
	return tui.Select(title, options, opts)
}

// canManage 当前进程能否改动状态与服务：用户模式总是可以，系统模式需要 root。
func canManage(p paths.Paths) bool { return p.UserMode || execx.IsRoot() }

// RequireRoot 系统模式下以普通用户运行时，经 sudo 以 root 重新启动 sboxkit 并直接进入 e，
// 成功时不返回；未获授权时返回 errs.ErrCancelled，会话以普通用户继续。已能管理时返回 nil。
func RequireRoot(p paths.Paths, e Entry) error {
	if canManage(p) {
		return nil
	}
	return elevateTo(e)
}

// elevateTo 可在测试中替换。先经 sudo -v 验证身份：密码输错时留在当前会话，
// 而不是让 exec 后的 sudo 失败退出整个界面。
var elevateTo = func(e Entry) error {
	if err := execx.EnsureSudo(i18n.T("此操作会改动系统服务或配置，")); err != nil {
		return err
	}
	exe, err := sysd.StableExecutablePath()
	if err != nil {
		return err
	}
	return execx.ReexecAsRoot(exe, []string{ResumeFlag + string(e)})
}

// launchProbe LaunchNeedsRoot 依赖的只读检查，可在测试中替换。
type launchProbe struct {
	stateOwner func(path string) (uid uint32, isDir bool, err error)
	readable   func(paths.Paths) bool
	installed  func() bool
	reconcile  func(paths.Paths) bool
	repair     func(paths.Paths) (bool, error)
}

var defaultLaunchProbe = launchProbe{
	stateOwner: stateOwner,
	readable:   stateReadable,
	installed:  func() bool { return sysd.IsInstalled(sysd.DefaultName) },
	reconcile:  func(p paths.Paths) bool { return sysd.RuntimeNeedsReconcile(p, sysd.DefaultName) },
	repair:     func(p paths.Paths) (bool, error) { return sysd.ResilienceNeedsRepair(p, sysd.DefaultName) },
}

// LaunchNeedsRoot 系统模式管理界面以普通用户启动时，是否必须一开始就提权：首次使用、
// 旧版本的数据目录待收归、当前用户读不到数据，或启动时就要同步 / 修复服务。返回的
// 说明供提示用户。
func LaunchNeedsRoot(p paths.Paths) (string, bool) {
	return defaultLaunchProbe.check(p)
}

func (probe launchProbe) check(p paths.Paths) (string, bool) {
	uid, isDir, err := probe.stateOwner(p.State)
	if errors.Is(err, os.ErrNotExist) {
		return i18n.T("首次使用需要初始化"), true
	}
	if err != nil {
		return err.Error(), true
	}
	if uid != 0 || !isDir {
		return i18n.T("需要把旧版本的数据目录收归 root"), true
	}
	if !probe.readable(p) {
		_, group := adminGroup()
		return fmt.Sprintf(i18n.T("当前用户不在管理员组 %s 中，读不到 root 管理的数据"), group), true
	}
	if !probe.installed() {
		return i18n.T("服务尚未安装，需要初始化"), true
	}
	if probe.reconcile(p) {
		return i18n.T("sboxkit 版本已变化，需要同步运行时资源"), true
	}
	if needed, err := probe.repair(p); err == nil && needed {
		return i18n.T("网络自愈组件需要修复"), true
	}
	return "", false
}

func stateOwner(path string) (uint32, bool, error) {
	st, err := os.Lstat(path)
	if err != nil {
		return 0, false, err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false, fmt.Errorf("%s: no ownership information", path)
	}
	return sys.Uid, st.IsDir(), nil
}

// stateReadable 普通用户能否读取管理界面展示所需的状态（系统模式下靠管理员组读权限）。
func stateReadable(p paths.Paths) bool {
	for _, path := range []string{p.CustomizeFile, p.ConfigFile, p.ActiveFile} {
		f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			f.Close()
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return false
		}
	}
	_, err := os.ReadDir(p.Subscriptions)
	return err == nil || errors.Is(err, os.ErrNotExist)
}
