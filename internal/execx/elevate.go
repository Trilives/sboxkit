package execx

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"syscall"

	"github.com/Trilives/sboxkit/internal/i18n"
)

// elevationEnv 提权重执行时透传的环境变量；sudo 默认清空环境，仅在已设置时传递，
// 未设置时不需要 sudoers 的 SETENV 授权。
var elevationEnv = []string{"SBOXKIT_HOME", "SBOXKIT_LANG", "SBOXKIT_USER_MODE", "NO_COLOR"}

// ReexecAsRoot 经 sudo 以 root 重新执行 exe args…，成功时不返回。exe 应是用户调用的
// 路径（如 /usr/bin/sboxkit），而非解析后的版本目录文件，单元里记录的入口才保持稳定。
func ReexecAsRoot(exe string, args []string) error {
	if os.Getenv("SUDO_UID") != "" {
		// 已经过一次 sudo 仍不是 root（如 sudoers 的 runas_default 指向他人）：不再循环提权。
		return fmt.Errorf("%s", i18n.T("经 sudo 重新执行后仍不是 root，请检查 sudoers 配置或直接以 root 运行"))
	}
	sudo, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("%s", i18n.T("需要管理员权限，但未找到 sudo，请改用 root 运行"))
	}
	return syscall.Exec(sudo, sudoArgv(sudo, exe, args, os.Getenv), os.Environ())
}

// sudoArgv 组装 `sudo [VAR=值…] -- exe args…`：sudo 只在 -- 之前识别 VAR=值，
// -- 之后的一切都是命令本身（路径含 = 也不会被误当成环境变量）。
func sudoArgv(sudo, exe string, args []string, getenv func(string) string) []string {
	argv := []string{sudo}
	for _, key := range elevationEnv {
		if value := getenv(key); value != "" {
			argv = append(argv, key+"="+value)
		}
	}
	argv = append(argv, "--", exe)
	return append(argv, args...)
}

// InvokingUserHome 发起操作的真实用户的家目录：经 sudo 运行时是 SUDO_USER 的家目录
// （sudo 可能把 HOME 换成 /root），否则是当前用户的。
func InvokingUserHome() string {
	if u := InvokingSudoUser(); u != nil {
		return u.HomeDir
	}
	home, _ := os.UserHomeDir()
	return home
}

// InvokingSudoUser root 代真实用户（SUDO_USER）操作时返回该用户，否则 nil。
func InvokingSudoUser() *user.User {
	name := os.Getenv("SUDO_USER")
	if name == "" || name == "root" || !IsRoot() {
		return nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return nil
	}
	return u
}
