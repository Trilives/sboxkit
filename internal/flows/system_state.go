package flows

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

// systemStateMode 系统模式状态目录：root 可写，其他人只能进入。旧版自更新把
// /usr/bin/sboxkit 链接到其中的版本库，普通用户也要能执行只读子命令。
const systemStateMode = 0o755

// 可在测试中替换。
var (
	secureTree = paths.SecureTree
	adminGroup = paths.AdminGroup
)

// ensureSystemState 系统模式下创建或收归状态目录；非 root 直接拒绝，绝不把目录交给普通用户。
func ensureSystemState(p paths.Paths, euid int) error {
	if euid != 0 {
		return fmt.Errorf(i18n.T("系统模式的数据目录 %s 只能由 root 管理，请用 sudo 运行"), p.State)
	}
	if err := os.MkdirAll(p.State, systemStateMode); err != nil {
		return err
	}
	return secureSystemState(p)
}

// SecureSystemState 供 root 运行的定时器（watchdog 等）在读写状态前调用：
// 目录不存在时什么都不做，存在则确保整棵树只归 root。返回 nil 表示 root 可以
// 放心读写其中的文件。
func SecureSystemState(p paths.Paths) error {
	if p.UserMode || !execx.IsRoot() {
		return nil
	}
	if _, err := os.Lstat(p.State); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err := rejectForeignSystemRoot(p, os.Geteuid()); err != nil {
		return err
	}
	return secureSystemState(p)
}

// rejectForeignSystemRoot SBOXKIT_HOME 把系统模式状态目录指向别处时，只接管已归 root 的
// 目录：收归会改写整棵树的属主与权限，误设成家目录、用户模式安装包这类普通用户目录时
// 必须拒绝。默认目录不受此限（旧版本把它交给了调用用户，正需要收归）。
func rejectForeignSystemRoot(p paths.Paths, euid int) error {
	if p.UserMode || euid != 0 || filepath.Clean(p.State) == paths.DefaultStateRoot {
		return nil
	}
	owner, ok := ownerUID(p.State)
	if !ok || owner == 0 {
		return nil
	}
	return fmt.Errorf(i18n.T("SBOXKIT_HOME 指向的 %s 属于 UID %d：系统模式只接管 root 属主的目录，请改用默认的 %s 或 root 属主的目录"), p.State, owner, paths.DefaultStateRoot)
}

// secureSystemState 旧版本把系统模式状态目录交给了调用用户，root 定时器与服务却会
// 读取、执行其中的文件；整棵树收归 root、属组设为管理员组（幂等，已合规时只做一遍
// lstat），再按模式设定凭据权限。
func secureSystemState(p paths.Paths) error {
	gid, _ := adminGroup()
	if err := secureTree(p.State, gid); err != nil {
		return fmt.Errorf(i18n.T("收归数据目录 %s 为 root 属主失败：%w"), p.State, err)
	}
	return tightenCredentials(p)
}

// tightenCredentials 订阅原文、生成配置、customize.json 与日志含节点凭据或 Token，
// 旧版本以 0644 写入；仅对已存在的条目设定权限，目录保留 setgid。
func tightenCredentials(p paths.Paths) error {
	targets := []struct {
		path string
		mode os.FileMode
	}{
		{p.Subscriptions, p.SecretDirMode()},
		{p.ConfigFile, p.SecretFileMode()},
		{p.CustomizeFile, p.SecretFileMode()},
		{execx.LogPath(p.State), p.SecretFileMode()},
	}
	for _, t := range targets {
		st, err := os.Lstat(t.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			continue
		}
		mode := st.Mode() & (os.ModePerm | os.ModeSetgid)
		if want := mode&os.ModeSetgid | t.mode; want != mode {
			if err := os.Chmod(t.path, want); err != nil {
				return err
			}
		}
	}
	return nil
}
