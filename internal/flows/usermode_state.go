package flows

import (
	"fmt"
	"os"
	"syscall"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

// userModeStateMode 状态目录含订阅原文与节点凭据，用户模式下只允许本人访问
// （服务、watchdog 与定时器都以同一用户运行，无需他人可读）。
const userModeStateMode = 0o700

// rejectForeignRoot 用户模式以包目录属主身份运行。用 sudo/su 以 root 启动会把 root
// 属主的文件写进用户的包目录，并去连接 root 自己的 systemd 用户实例，因此直接拒绝；
// 包目录本身属于 root 时（root 自行解压使用）不受影响。
func rejectForeignRoot(p paths.Paths, euid int) error {
	if !p.UserMode || euid != 0 {
		return nil
	}
	owner, ok := ownerUID(p.BundleRoot)
	if !ok || owner == 0 {
		return nil
	}
	return fmt.Errorf(i18n.T("用户模式请以安装包目录的属主（UID %d）运行，不要使用 sudo 或 root；启用 TUN 时 sboxkit 会单独请求一次 sudo。需要系统级部署请安装 .deb（系统模式）。"), owner)
}

func ownerUID(path string) (uint32, bool) {
	if path == "" {
		return 0, false
	}
	st, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return sys.Uid, true
}

// ensureUserModeState 创建或收紧用户模式状态目录；不可写时直接说明，
// 用户模式承诺全程不用 sudo，不会悄悄提权。
func ensureUserModeState(p paths.Paths) error {
	notWritable := func() error {
		return fmt.Errorf(i18n.T("用户模式状态目录不可写：%s（请把用户模式安装包解压到当前用户可写的位置，或设置 SBOXKIT_HOME）"), p.State)
	}
	if err := os.MkdirAll(p.State, userModeStateMode); err != nil {
		return notWritable()
	}
	if err := os.Chmod(p.State, userModeStateMode); err != nil {
		return notWritable()
	}
	probe, err := os.CreateTemp(p.State, ".probe-*")
	if err != nil {
		return notWritable()
	}
	probe.Close()
	return os.Remove(probe.Name())
}
