// 用户模式的 linger 管理：用户级服务要在注销后继续运行、开机自启，需要
// `loginctl enable-linger`。sboxkit 只在它亲自开启 linger 时于状态目录留下标记，
// 卸载服务时据此关闭；用户原本就开着的 linger 不会被动。
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

const lingerMarkerName = "linger-enabled-by-sboxkit"

// 可在测试中替换。
var (
	haveLoginctl = func() bool { return execx.Have("loginctl") }
	runLoginctl  = func(args ...string) (string, error) {
		res, err := execx.Run(append([]string{"loginctl"}, args...), &execx.Opt{Capture: true})
		return res.Stdout, err
	}
)

func (s unitScope) lingerMarker() string { return filepath.Join(s.State, lingerMarkerName) }

func currentUserName() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	return strconv.Itoa(os.Getuid())
}

// ensureLinger 让用户级服务在注销后继续运行并随开机启动。loginctl 对本人
// 账户通常无需 sudo（polkit set-self-linger）；失败只提示，不影响服务本身。
func (s unitScope) ensureLinger() {
	if !s.User || !haveLoginctl() {
		return
	}
	user := currentUserName()
	out, err := runLoginctl("show-user", user, "-p", "Linger", "--value")
	if err == nil && strings.TrimSpace(out) == "yes" {
		return
	}
	// 显式带上用户名：无参形式靠调用者的登录会话定位用户，不在会话内（如 su -c、
	// 容器 exec）时会以 ENXIO 失败；对本人账户仍按 set-self-linger 授权。
	if _, err := runLoginctl("enable-linger", user); err != nil {
		execx.Warn(i18n.T("未能开启 linger：注销后用户级服务会停止、开机也不会自动启动。可手动执行 sudo loginctl enable-linger ") + user)
		return
	}
	if err := os.WriteFile(s.lingerMarker(), []byte(user+"\n"), 0o600); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("记录 linger 状态失败（卸载时不会自动关闭 linger）：%v"), err))
	}
	execx.Ok(i18n.T("已开启 linger：用户级服务在注销后继续运行，并随开机启动。"))
}

// RevertLinger 卸载用户模式服务后调用：关闭由 sboxkit 开启的 linger。
// 系统模式、或 linger 并非 sboxkit 开启时什么也不做。
func RevertLinger(p paths.Paths) error {
	return scopeFor(p).revertLinger()
}

func (s unitScope) revertLinger() error {
	if !s.User || !fileExists(s.lingerMarker()) {
		return nil
	}
	if !haveLoginctl() {
		return fmt.Errorf("%s", i18n.T("未找到 loginctl，无法关闭 sboxkit 开启的 linger；可手动执行 loginctl disable-linger"))
	}
	if _, err := runLoginctl("disable-linger", currentUserName()); err != nil {
		return fmt.Errorf(i18n.T("关闭 linger 失败，可手动执行 loginctl disable-linger：%w"), err)
	}
	if err := os.Remove(s.lingerMarker()); err != nil && !os.IsNotExist(err) {
		execx.Warn(err.Error())
	}
	execx.Ok(i18n.T("已关闭 sboxkit 开启的 linger（恢复安装前的登录会话设置）。"))
	return nil
}
