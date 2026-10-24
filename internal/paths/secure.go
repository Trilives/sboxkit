package paths

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// SecretFileMode 含节点凭据 / Token 的状态文件权限。系统模式下只有 root 能写，管理员组
// （见 AdminGroup）可读，管理界面因此不提权也能展示状态；用户模式只有属主可读写。
func (p Paths) SecretFileMode() os.FileMode {
	if p.UserMode {
		return 0o600
	}
	return 0o640
}

// SecretDirMode 存放凭据的目录权限，规则同 SecretFileMode。
func (p Paths) SecretDirMode() os.FileMode {
	if p.UserMode {
		return 0o700
	}
	return 0o750
}

// adminGroupNames 能经 sudo 取得 root 的常见管理员组：Debian / Ubuntu、Fedora / Arch / RHEL、
// 旧版 Ubuntu。组员本来就能读 root 的文件，给他们读权限不扩大任何人的能力。
var adminGroupNames = []string{"sudo", "wheel", "admin"}

// AdminGroup 系统模式状态目录的属组：第一个存在的管理员组，都不存在时为 root 组（gid 0），
// 此时只有 root 能读凭据，管理界面启动时就提权。
func AdminGroup() (gid int, name string) {
	return adminGroup(user.LookupGroup)
}

func adminGroup(lookup func(string) (*user.Group, error)) (int, string) {
	for _, name := range adminGroupNames {
		g, err := lookup(name)
		if err != nil {
			continue
		}
		if gid, err := strconv.Atoi(g.Gid); err == nil {
			return gid, name
		}
	}
	return 0, "root"
}

// writableByOthers 组 / 其他用户写权限位。
const writableByOthers os.FileMode = 0o022

// SecureTree 把 root 下整棵目录树收归 uid 0、属组 gid，幂等，已合规的条目不做改动。
//
// 旧版本把系统模式状态目录交给调用用户，而 root 运行的定时器与服务会读取、执行其中
// 的文件；该用户身份下的任意进程都可能在交还前埋下陷阱，所以这里不只是改属主：
//   - 目录：先改属主、去掉组 / 其他写权限并设置 setgid（新建条目继承属组），再列出内容，
//     此后无法再往里放东西；
//   - 普通文件：属主不是 root、组 / 其他可写或有多个硬链接时，在同目录以新 inode 重建，
//     旧文件上残留的写句柄与硬链接随之失效；只是属组不对时直接改属组；
//   - 符号链接：目标落在 root 之外（如指向 /etc/shadow 的日志）时删除，否则保留；
//   - 其他类型（FIFO、套接字等）一律删除，避免 root 读取时阻塞。
func SecureTree(root string, gid int) error {
	return secureTree(root, 0, gid, os.Lchown)
}

type treeOwner struct {
	uid, gid int
	chown    func(name string, uid, gid int) error
}

func secureTree(root string, uid, gid int, chown func(string, int, int) error) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	st, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s: not a directory", root)
	}
	o := treeOwner{uid: uid, gid: gid, chown: chown}
	// WalkDir 先回调目录本身再读取其内容，目录收归后才列出子项。
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode()
		switch {
		case mode.IsDir():
			return o.secureDir(path, info)
		case mode.IsRegular():
			return o.secureFile(path, info)
		case mode&fs.ModeSymlink != 0:
			return o.secureLink(root, path, info)
		default:
			return os.Remove(path)
		}
	})
}

func (o treeOwner) owns(info fs.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == o.uid && int(st.Gid) == o.gid
}

func (o treeOwner) secureDir(path string, info fs.FileInfo) error {
	if !o.owns(info) {
		if err := o.chown(path, o.uid, o.gid); err != nil {
			return err
		}
	}
	mode := info.Mode() & (fs.ModePerm | fs.ModeSetgid | fs.ModeSticky)
	if want := mode&^writableByOthers | fs.ModeSetgid; want != mode {
		return os.Chmod(path, want)
	}
	return nil
}

func (o treeOwner) secureFile(path string, info fs.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Uid) != o.uid || st.Nlink != 1 || info.Mode().Perm()&writableByOthers != 0 {
		return o.recreateFile(path, info.Mode().Perm()&^writableByOthers)
	}
	if int(st.Gid) != o.gid {
		// 属主已是 root 且没有他人可写的历史，旧 inode 上不会有别人的写句柄。
		return o.chown(path, o.uid, o.gid)
	}
	return nil
}

// recreateFile 把 path 的内容复制到同目录的新文件再改名覆盖；
// 父目录此时已收归，改名后 path 指向的只可能是新 inode。
func (o treeOwner) recreateFile(path string, perm os.FileMode) error {
	src, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".secure-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := o.chown(tmpName, o.uid, o.gid); err != nil {
		return err
	}
	// chown 会清掉 setuid / setgid 位，权限在改属主之后设置。
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (o treeOwner) secureLink(root, path string, info fs.FileInfo) error {
	target, err := os.Readlink(path)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	if !within(root, filepath.Clean(target)) {
		return os.Remove(path)
	}
	if o.owns(info) {
		return nil
	}
	return o.chown(path, o.uid, o.gid)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
