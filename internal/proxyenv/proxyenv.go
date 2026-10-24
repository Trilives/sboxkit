// Package proxyenv 把代理环境变量写入用户 ~/.bashrc（对应 proxyenv.py）。
//
// TUN 关闭后是纯代理模式：内核只在本机提供 mixed 入站，
// 各程序需自行走代理。以带标记的代码块写入 bashrc，幂等更新、卸载时整块移除。
package proxyenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

const (
	ProxyHost = "127.0.0.1"

	beginMark = "# >>> sboxkit proxy env >>>"
	endMark   = "# <<< sboxkit proxy env <<<"

	bashrcMode = 0o644
)

func block(port int) string {
	httpURL := fmt.Sprintf("http://%s:%d", ProxyHost, port)
	socksURL := fmt.Sprintf("socks5://%s:%d", ProxyHost, port)
	return strings.Join([]string{
		beginMark,
		fmt.Sprintf(`export http_proxy="%s"`, httpURL),
		fmt.Sprintf(`export https_proxy="%s"`, httpURL),
		fmt.Sprintf(`export all_proxy="%s"`, socksURL),
		`export HTTP_PROXY="$http_proxy"`,
		`export HTTPS_PROXY="$https_proxy"`,
		`export ALL_PROXY="$all_proxy"`,
		`export no_proxy="localhost,127.0.0.1,::1"`,
		`export NO_PROXY="$no_proxy"`,
		endMark,
	}, "\n")
}

// TargetBashrc 目标 bashrc：sudo 运行时落到真实调用用户，否则当前用户。
func TargetBashrc() string {
	return filepath.Join(execx.InvokingUserHome(), ".bashrc")
}

// IsConfigured reports whether sboxkit's managed proxy block already exists.
func IsConfigured() bool {
	raw, err := os.ReadFile(TargetBashrc())
	return err == nil && strings.Contains(string(raw), beginMark)
}

func stripBlock(text string) string {
	var out []string
	skip := false
	for _, ln := range strings.Split(text, "\n") {
		switch strings.TrimSpace(ln) {
		case beginMark:
			skip = true
			continue
		case endMark:
			skip = false
			continue
		}
		if !skip {
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
}

// Write 幂等写入代理块到 bashrc；返回写入的文件路径。
func Write(port int) (string, error) {
	path := TargetBashrc()
	old := ""
	if raw, err := os.ReadFile(path); err == nil {
		old = string(raw)
	}
	body := strings.TrimRight(stripBlock(old), "\n")
	content := block(port) + "\n"
	if body != "" {
		content = body + "\n\n" + content
	}
	if err := writeBashrc(path, []byte(content)); err != nil {
		return "", err
	}
	execx.Ok(fmt.Sprintf(i18n.T("已写入代理环境变量到 %s（新开终端生效；当前终端可 `source %s`）。"), path, path))
	return path, nil
}

// Remove 从 bashrc 移除代理块（无则跳过）。
func Remove() error {
	path := TargetBashrc()
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	old := string(raw)
	if !strings.Contains(old, beginMark) {
		return nil
	}
	content := strings.TrimRight(stripBlock(old), "\n") + "\n"
	if err := writeBashrc(path, []byte(content)); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("已从 %s 移除代理环境变量。"), path))
	return nil
}

// writeBashrc 写入 bashrc。root 代 SUDO_USER 写入时，路径完全由该用户控制，
// 交给 writeOwnedFile 防止被引向系统文件。
func writeBashrc(path string, data []byte) error {
	owner := execx.InvokingSudoUser()
	if owner == nil {
		return os.WriteFile(path, data, bashrcMode)
	}
	uid, err1 := strconv.Atoi(owner.Uid)
	gid, err2 := strconv.Atoi(owner.Gid)
	if err1 != nil || err2 != nil {
		return fmt.Errorf(i18n.T("无法解析用户 %s 的 uid/gid"), owner.Username)
	}
	return writeOwnedFile(path, data, uid, gid)
}

// writeOwnedFile 以 root 改写 uid 家目录中的文件：不跟随符号链接（~/.bashrc →
// /etc/shadow 会让 root 覆写系统文件），只改写属于 uid 的普通文件；新建的文件交还 uid。
// 属主校验基于已打开的句柄，校验后无法再被替换。
func writeOwnedFile(path string, data []byte, uid, gid int) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, bashrcMode)
	if err == nil {
		defer f.Close()
		if err := f.Chown(uid, gid); err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	}
	if !errors.Is(err, os.ErrExist) {
		return err
	}
	f, err = os.OpenFile(path, os.O_WRONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf(i18n.T("拒绝写入 %s（不是普通文件或是符号链接）：%w"), path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || !ok || int(sys.Uid) != uid || sys.Nlink != 1 {
		return fmt.Errorf(i18n.T("拒绝写入 %s：它不是该用户自己的普通文件"), path)
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err = f.WriteAt(data, 0)
	return err
}
