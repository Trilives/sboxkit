package sysd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

const systemUnitDir = "/etc/systemd/system"

// unitScope 描述 systemd 单元装在哪一层：
//
//   - 系统级（零值）：/etc/systemd/system，运行时在 /var/lib/sboxkit-runtime，
//     全部写操作经 sudo（execx.RunRoot）。
//   - 用户级（用户模式）：~/.config/systemd/user + `systemctl --user`，运行时在
//     状态目录内，以当前用户身份执行；只有启用 TUN 时经 sudo 给运行时内核授予网络能力
//     （netcaps.go），并安装代为登记系统 DNS 的系统单元（tundns_user.go）。
//
// 用户级服务没有防火墙与 NetworkManager 钩子。
type unitScope struct {
	User       bool
	UnitDir    string
	RuntimeDir string
	State      string
}

func scopeFor(p paths.Paths) unitScope {
	if !p.UserMode {
		return unitScope{UnitDir: systemUnitDir, RuntimeDir: paths.RuntimeDir, State: p.State}
	}
	return unitScope{User: true, UnitDir: userUnitDir(), RuntimeDir: p.Runtime, State: p.State}
}

// currentScope 由当前进程的路径解析得出；单元函数多数没有 Paths 参数，
// 这里统一按调用进程所处模式（用户模式标记 / SBOXKIT_USER_MODE）判定。
func currentScope() unitScope { return scopeFor(paths.Detect()) }

// UserScope 报告当前进程是否以用户模式（systemd 用户单元）管理服务。
func UserScope() bool { return currentScope().User }

func userUnitDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "systemd", "user")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "sboxkit-systemd-user")
	}
	return filepath.Join(home, ".config", "systemd", "user")
}

func (s unitScope) unitDir() string {
	if s.UnitDir != "" {
		return s.UnitDir
	}
	return systemUnitDir
}

func (s unitScope) unitFile(unit string) string {
	return filepath.Join(s.unitDir(), unit)
}

func (s unitScope) systemctl(args ...string) []string {
	if s.User {
		return append([]string{"systemctl", "--user"}, args...)
	}
	return append([]string{"systemctl"}, args...)
}

// JournalCommand 查看主服务日志的命令（用户单元需 --user）。
func JournalCommand(name string, extra ...string) []string {
	cmd := []string{"journalctl"}
	if currentScope().User {
		cmd = append(cmd, "--user")
	}
	cmd = append(cmd, extra...)
	return append(cmd, "-u", name+".service")
}

// UnitPath 主服务单元文件路径（信息/文件位置页展示用）。
func UnitPath(name string) string {
	return currentScope().unitFile(name + ".service")
}

// ensurePrivilege 确认可以管理本层单元：系统级预热 sudo；用户级不提权，但先确认
// systemd 用户实例可达，避免 su/sudo -u、缺 XDG_RUNTIME_DIR 的会话里得到裸露的 bus 错误。
func (s unitScope) ensurePrivilege(reason string) error {
	if s.User {
		return userManagerProbe(s)
	}
	return execx.EnsureSudo(reason)
}

func (s unitScope) run(cmd []string, reason string, opt *execx.Opt) (execx.Result, error) {
	if s.User {
		return execx.Run(cmd, opt)
	}
	return execx.RunRoot(cmd, reason, opt)
}

func (s unitScope) write(path, content, mode, reason string) error {
	if s.User {
		return writeFileAtomic(path, content, mode)
	}
	return execx.WriteRoot(path, content, mode, reason)
}

func (s unitScope) wantedBy() string {
	if s.User {
		return "default.target"
	}
	return "multi-user.target"
}

// environmentLines 用户单元显式写入模式与状态目录，使 watchdog / 更新定时器 /
// relay 无论从哪条路径被 systemd 拉起，都与交互会话看到同一份数据。
func (s unitScope) environmentLines() string {
	if !s.User {
		return ""
	}
	return "Environment=SBOXKIT_USER_MODE=1\n" +
		"Environment=" + systemdQuote("SBOXKIT_HOME="+s.State) + "\n"
}

// systemdQuote 引用 Environment= 取值（空格安全，% 按说明符转义）。
func systemdQuote(value string) string {
	return strconv.Quote(systemdEscape(value))
}

// systemdExecArg 引用 ExecStart 参数：ExecStart 还会在引号内展开 $VAR，需写成 $$。
func systemdExecArg(value string) string {
	return systemdQuote(strings.ReplaceAll(value, "$", "$$"))
}

// systemdEscape 仅转义说明符，用于不解析引号的路径型设置（WorkingDirectory）。
func systemdEscape(value string) string {
	return strings.ReplaceAll(value, "%", "%%")
}

// writeFileAtomic 以当前用户写文件：同目录临时文件 + chmod + rename。
func writeFileAtomic(path, content, mode string) error {
	if mode == "" {
		mode = "0644"
	}
	perm, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return fmt.Errorf("invalid file mode %q: %w", mode, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), os.FileMode(perm)); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// userManagerReady 检查 systemd 用户实例是否可用（SSH 会话缺少
// XDG_RUNTIME_DIR / 用户 bus 时 systemctl --user 会失败）。
func (s unitScope) userManagerReady() error {
	if !s.User {
		return nil
	}
	return userManagerProbe(s)
}

// userManagerProbe 可在测试中替换。
var userManagerProbe = func(s unitScope) error {
	if _, err := execx.Run(s.systemctl("show-environment"), &execx.Opt{Capture: true}); err != nil {
		return fmt.Errorf("%s", i18n.T("无法连接 systemd 用户实例（systemctl --user），请在已登录的用户会话中运行，或确认 XDG_RUNTIME_DIR 已设置"))
	}
	return nil
}
