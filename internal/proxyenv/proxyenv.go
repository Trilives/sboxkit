// Package proxyenv 把代理环境变量写入用户 ~/.bashrc（对应 proxyenv.py）。
//
// TUN 关闭后是纯代理模式：内核只在 127.0.0.1:7890 提供 mixed 入站，
// 各程序需自行走代理。以带标记的代码块写入 bashrc，幂等更新、卸载时整块移除。
package proxyenv

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

const (
	ProxyHost = "127.0.0.1"
	ProxyPort = 7890

	beginMark = "# >>> sboxkit proxy env >>>"
	endMark   = "# <<< sboxkit proxy env <<<"
)

func block() string {
	httpURL := fmt.Sprintf("http://%s:%d", ProxyHost, ProxyPort)
	socksURL := fmt.Sprintf("socks5://%s:%d", ProxyHost, ProxyPort)
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
	home := ""
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		if u, err := user.Lookup(sudoUser); err == nil {
			home = u.HomeDir
		}
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".bashrc")
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
func Write() (string, error) {
	path := TargetBashrc()
	old := ""
	existed := false
	if raw, err := os.ReadFile(path); err == nil {
		old = string(raw)
		existed = true
	}
	body := strings.TrimRight(stripBlock(old), "\n")
	content := block() + "\n"
	if body != "" {
		content = body + "\n\n" + content
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	if !existed {
		chownToSudoUser(path)
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
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("已从 %s 移除代理环境变量。"), path))
	return nil
}

// chownToSudoUser 新建文件时若在 sudo 下，把属主还给真实用户，避免 root 占用。
func chownToSudoUser(path string) {
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" || sudoUser == "root" || os.Geteuid() != 0 {
		return
	}
	u, err := user.Lookup(sudoUser)
	if err != nil {
		return
	}
	uid, err1 := strconv.Atoi(u.Uid)
	gid, err2 := strconv.Atoi(u.Gid)
	if err1 == nil && err2 == nil {
		os.Chown(path, uid, gid)
	}
}
