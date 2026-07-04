// Package execx 公共工具：彩色输出、日志、子进程、root/sudo（对应 Python 版 shell.py）。
package execx

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/i18n"
)

// 无 TTY 或 NO_COLOR 时自动降级为无色。
var useColor = term.IsTerminal(int(os.Stdout.Fd())) && os.Getenv("NO_COLOR") == ""

func tint(code, text string) string {
	if !useColor {
		return text
	}
	return "\033[" + code + "m" + text + "\033[0m"
}

func Info(msg string) {
	prefix := i18n.T("[信息] ")
	fmt.Println(tint("36", prefix) + msg)
	writeLog(prefix + msg)
}
func Ok(msg string) {
	prefix := i18n.T("[完成] ")
	fmt.Println(tint("32", prefix) + msg)
	writeLog(prefix + msg)
}
func Warn(msg string) {
	prefix := i18n.T("[注意] ")
	fmt.Println(tint("33", prefix) + msg)
	writeLog(prefix + msg)
}
func Error(msg string) {
	prefix := i18n.T("[错误] ")
	fmt.Fprintln(os.Stderr, tint("31", prefix)+msg)
	writeLog(prefix + msg)
}

func Header(title string) {
	n := len([]rune(title))
	if n < 16 {
		n = 16
	}
	fmt.Println()
	fmt.Println(tint("1", title))
	fmt.Println(strings.Repeat("─", n))
	writeLog("== " + title + " ==")
}

// CommandError 子进程非零退出。
type CommandError struct {
	Cmd    []string
	Code   int
	Output string
}

func (e *CommandError) Error() string {
	return fmt.Sprintf(i18n.T("命令失败(%d): %s"), e.Code, strings.Join(e.Cmd, " "))
}

// Opt 子进程选项；nil / 零值即默认（继承 stdio、当前目录、当前环境）。
type Opt struct {
	Capture bool              // 捕获 stdout/stderr（否则直通终端）
	Env     map[string]string // 追加/覆盖环境变量
	Dir     string
}

// Result 子进程结果；Capture 时含输出。
type Result struct {
	Code   int
	Stdout string
	Stderr string
}

// Run 运行子进程；非零退出返回 *CommandError（对应 Python 版 check=True）。
// 想忽略退出码的调用方自行忽略错误并读取 Result。
func Run(cmd []string, opt *Opt) (Result, error) {
	if opt == nil {
		opt = &Opt{}
	}
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Dir = opt.Dir
	if len(opt.Env) > 0 {
		c.Env = os.Environ()
		for k, v := range opt.Env {
			c.Env = append(c.Env, k+"="+v)
		}
	}
	var stdout, stderr strings.Builder
	if opt.Capture {
		c.Stdout, c.Stderr = &stdout, &stderr
	} else {
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	err := c.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.Code = ee.ExitCode()
			return res, &CommandError{Cmd: cmd, Code: res.Code, Output: res.Stdout + res.Stderr}
		}
		return res, fmt.Errorf(i18n.T("启动命令 %s: %w"), cmd[0], err)
	}
	return res, nil
}

func IsRoot() bool { return os.Geteuid() == 0 }

// Have 判断可执行文件是否在 PATH 中。
func Have(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

var sudoOK bool

// EnsureSudo 确保后续 root 操作可执行：已是 root 直接返回；
// 否则用 `sudo -v` 交互式预热（会话内缓存），失败返回 ErrCancelled。
func EnsureSudo(reason string) error {
	if IsRoot() || sudoOK {
		return nil
	}
	if !Have("sudo") {
		return fmt.Errorf("%s", i18n.T("需要管理员权限，但未找到 sudo，请改用 root 运行"))
	}
	Info(reason + i18n.T("需要管理员权限。"))
	Info(i18n.T("提示：也可以直接用 sudo 启动，避免中途输入密码。"))
	c := exec.Command("sudo", "-v")
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return errs.ErrCancelled
	}
	sudoOK = true
	return nil
}

// RunRoot 以 root 运行命令：已是 root 直接执行，否则自动加 sudo（先 EnsureSudo）。
func RunRoot(cmd []string, reason string, opt *Opt) (Result, error) {
	if IsRoot() {
		return Run(cmd, opt)
	}
	if reason == "" {
		reason = i18n.T("该操作")
	}
	if err := EnsureSudo(reason); err != nil {
		return Result{}, err
	}
	return Run(append([]string{"sudo"}, cmd...), opt)
}

// WriteRoot 以 root 把 content 写到 path（经临时文件 + install，保证权限/原子）。
func WriteRoot(path, content, mode, reason string) error {
	tf, err := os.CreateTemp("", "sboxkit-*")
	if err != nil {
		return err
	}
	tmp := tf.Name()
	defer os.Remove(tmp)
	if _, err := tf.WriteString(content); err != nil {
		tf.Close()
		return err
	}
	if err := tf.Close(); err != nil {
		return err
	}
	if mode == "" {
		mode = "0644"
	}
	_, err = RunRoot([]string{"install", "-m", mode, tmp, path}, reason, nil)
	return err
}
