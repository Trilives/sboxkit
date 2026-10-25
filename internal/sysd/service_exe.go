// 系统单元以 root 执行写入其中的 sboxkit（bootstrap relay、watchdog、每周更新、TUN DNS 钩子）。
// 这条路径上任一环节可被非 root 改写时，改写者之后就能借这些单元以 root 运行任意程序，例如
// `sudo ./sboxkit init` 从家目录启动；因此系统单元只接受 root 独占的路径。
package sysd

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/Trilives/sboxkit/internal/i18n"
)

// 可在测试中替换。
var (
	executablePath  = StableExecutablePath
	checkExecutable = trustedExecutable
)

// serviceExecutable 写进 s 范围单元的 sboxkit 路径；系统单元要求路径 root 独占。
func serviceExecutable(s unitScope) (string, error) {
	exe, err := executablePath()
	if err != nil {
		return "", fmt.Errorf(i18n.T("定位 sboxkit 可执行文件: %w"), err)
	}
	if s.User {
		return exe, nil
	}
	if err := checkExecutable(exe); err != nil {
		return "", err
	}
	return exe, nil
}

// trustedExecutable 路径本身、解析符号链接后的文件，以及两者的每一级上级目录都须归 root，
// 且组和其他人不可写。
func trustedExecutable(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf(i18n.T("定位 sboxkit 可执行文件: %w"), err)
	}
	checks := append(append(parentDirs(path), parentDirs(resolved)...), resolved)
	for _, p := range checks {
		info, err := os.Stat(p)
		if err != nil {
			return fmt.Errorf(i18n.T("定位 sboxkit 可执行文件: %w"), err)
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf(i18n.T("%s 不是 root 独占（须归 root 且组和其他人不可写），root 运行的服务不能执行其中的 sboxkit：请用 deb 安装，或把 sboxkit 放到 /usr/local/bin 等 root 独占的目录后再运行"), p)
		}
	}
	return nil
}

// parentDirs path 的各级上级目录，从近到远直到根目录。
func parentDirs(path string) []string {
	var dirs []string
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		dirs = append(dirs, dir)
		if dir == filepath.Dir(dir) {
			return dirs
		}
	}
}
