package paths

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UserModeMarker 随用户模式安装包发布、与 sboxkit 可执行文件同目录的标记文件。
// 存在即表示以用户模式运行：状态默认落在包目录内，服务以 systemd 用户
// 单元运行，全程不请求 sudo。
const UserModeMarker = "sboxkit.user-mode"

// UserModeStateDir 用户模式下（未设置 SBOXKIT_HOME 时）状态目录相对包目录的名字。
const UserModeStateDir = ".sboxkit"

// maxSymlinkHops 追踪可执行文件符号链接链的上限，防御循环链接。
const maxSymlinkHops = 16

// userModeEnv 显式指定模式：1=用户模式（用户级服务），0=系统级。
// 用户级 systemd 单元会写入该变量，保证 healthcheck/update 与交互会话一致。
const userModeEnv = "SBOXKIT_USER_MODE"

type userModeProbe struct {
	env      func(string) string
	arg0     string
	lookPath func(string) (string, error)
	exists   func(string) bool
	readlink func(string) (string, error)
}

func defaultUserModeProbe() userModeProbe {
	return userModeProbe{
		env:      os.Getenv,
		arg0:     os.Args[0],
		lookPath: exec.LookPath,
		exists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		readlink: os.Readlink,
	}
}

// detectUserModeRoot 返回用户模式安装包根目录（含 UserModeMarker 的目录）；空串表示系统模式。
//
// 依次检查用户调用路径（os.Args[0]）及其符号链接链上每一跳所在目录：自更新后
// 包内的 sboxkit 会变成指向托管版本的符号链接，~/.local/bin 之类的入口链接也
// 同理，逐跳检查才能始终找回包目录。
func detectUserModeRoot(probe userModeProbe) string {
	mode := strings.TrimSpace(probe.env(userModeEnv))
	if mode == "0" {
		return ""
	}
	exe := invokedPath(probe.arg0, probe.lookPath)
	for hop := 0; exe != "" && hop < maxSymlinkHops; hop++ {
		dir := filepath.Dir(exe)
		if probe.exists(filepath.Join(dir, UserModeMarker)) {
			return dir
		}
		target, err := probe.readlink(exe)
		if err != nil {
			break
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		exe = filepath.Clean(target)
	}
	if mode == "1" {
		if dir := invokedPath(probe.arg0, probe.lookPath); dir != "" {
			return filepath.Dir(dir)
		}
	}
	return ""
}

func invokedPath(arg0 string, lookPath func(string) (string, error)) string {
	path := arg0
	if path == "" {
		return ""
	}
	if !strings.ContainsRune(path, os.PathSeparator) {
		resolved, err := lookPath(path)
		if err != nil {
			return ""
		}
		path = resolved
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return abs
}

// FromUserModeRoot 以用户模式解析路径：运行时暂存在状态目录内（用户可写），
// 种子资源从包目录的 core/ 与 ruleset/ 接管。
func FromUserModeRoot(state, bundleRoot string) Paths {
	p := FromRoot(state)
	p.UserMode = true
	p.BundleRoot = bundleRoot
	p.Runtime = filepath.Join(state, "runtime")
	return p
}

// SeedBinDir / SeedRulesetDir 用户模式安装包内附带的内核与规则集种子目录。
func (p Paths) SeedBinDir() string {
	if !p.UserMode || p.BundleRoot == "" {
		return ""
	}
	return filepath.Join(p.BundleRoot, "core")
}

func (p Paths) SeedRulesetDir() string {
	if !p.UserMode || p.BundleRoot == "" {
		return ""
	}
	return filepath.Join(p.BundleRoot, "ruleset")
}
