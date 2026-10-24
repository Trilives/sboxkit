// Package compat 校验「配置 × sing-box 内核 × geo 规则集」组合能否启动：在私有临时
// 目录里用即将部署的内核执行 `sing-box check`，配置中的状态目录规则集路径重定向到
// 即将部署的规则集文件，面板目录与缓存库改指校验目录。订阅导入 / 刷新 / 重建 / 切换、服务注册与同步、内核 /
// 规则集 / sboxkit 更新都先经此校验，通过后才改动状态目录与运行时。
package compat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	checkTimeout   = 30 * time.Second
	versionTimeout = 10 * time.Second
)

// Bundle 一次部署实际使用的内核与规则集文件。
type Bundle struct {
	Kernel  string
	Geosite string
	GeoIP   string
}

// StateBundle 状态目录中现用的内核与规则集。
func StateBundle(p paths.Paths) Bundle {
	return Bundle{Kernel: p.SingBoxBin, Geosite: p.GeositeCN, GeoIP: p.GeoIPCN}
}

// ErrKernelMissing 尚无可用内核（例如首次初始化还未下载）；调用方决定推迟校验还是失败。
var ErrKernelMissing = errors.New("sing-box kernel not found")

// Error 内核拒绝了配置。
type Error struct {
	Kernel string // 内核版本；读取失败时为空
	Output string // sing-box check 输出（已去除颜色控制符）
}

func (e *Error) Error() string {
	version := e.Kernel
	if version == "" {
		version = "?"
	}
	msg := fmt.Sprintf(i18n.T("sing-box %s 拒绝该配置：%s"), version, e.Output)
	if hint := e.Hint(); hint != "" {
		msg += "\n" + hint
	}
	return msg
}

// 内核输出（已转小写）的归类依据，措辞取自 sing-box 源码：新内核弃用 / 要求新写法，
// 或旧内核不认识新写法（unknown DNS rule action、json unknown field 等）。
var (
	newerKernelSyntax = regexp.MustCompile(`is deprecated in sing-box|removed in sing-box|migration|require match_response`)
	olderKernelSyntax = regexp.MustCompile(`unknown (dns )?rule action|unknown field| type: `)
)

// Hint 按内核输出归类常见原因并给出处理方向；无法归类时返回空串。
func (e *Error) Hint() string {
	out := strings.ToLower(e.Output)
	switch {
	case newerKernelSyntax.MatchString(out):
		return i18n.T("该内核已弃用或移除配置中的写法：请更新 sboxkit（新版生成器）后重新生成配置，或继续使用原内核。")
	case olderKernelSyntax.MatchString(out):
		return i18n.T("配置使用了该内核尚不支持的写法：请更新 sing-box 内核，或继续使用原 sboxkit 版本。")
	default:
		return ""
	}
}

// Check 用 b.Kernel 校验 config。校验目录私有（0700）并在返回前删除：配置含节点凭据。
func Check(p paths.Paths, config []byte, b Bundle) error {
	if !isFile(b.Kernel) {
		return ErrKernelMissing
	}
	b, err := b.absolute()
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "sboxkit-check-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := mirrorRuntimeRuleSets(dir, b); err != nil {
		return err
	}
	file := filepath.Join(dir, "config.json")
	if err := os.WriteFile(file, redirectPaths(p, config, b, dir), 0o600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, b.Kernel, "check", "-c", file)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	output := cleanOutput(out)
	if ctx.Err() != nil {
		output = fmt.Sprintf(i18n.T("校验超时（%s）"), checkTimeout)
	} else if output == "" {
		output = err.Error()
	}
	return &Error{Kernel: KernelVersion(b.Kernel), Output: output}
}

// absolute 返回路径全部转为绝对路径的副本：内核在校验目录里运行，相对路径会解析错位。
func (b Bundle) absolute() (Bundle, error) {
	out := b
	for _, field := range []*string{&out.Kernel, &out.Geosite, &out.GeoIP} {
		if *field == "" {
			continue
		}
		abs, err := filepath.Abs(*field)
		if err != nil {
			return Bundle{}, err
		}
		*field = abs
	}
	return out, nil
}

// mirrorRuntimeRuleSets 运行时以运行时目录为工作目录，其中部署了两份规则集；
// 校验目录放置同名链接，使导入配置里的相对路径与运行时解析一致。b 的路径须为绝对路径。
func mirrorRuntimeRuleSets(dir string, b Bundle) error {
	for _, src := range []string{b.Geosite, b.GeoIP} {
		if !isFile(src) {
			continue
		}
		if err := os.Symlink(src, filepath.Join(dir, filepath.Base(src))); err != nil {
			return err
		}
	}
	return nil
}

// redirectPaths 改写配置中由部署方管理的路径：指向状态目录规则集的本地 rule_set 改指
// b 中的文件；面板目录与缓存库改指校验目录（运行时目录在系统模式仅 root 可读，
// 内核 check 会读取面板目录）。无法解析的配置原样交给内核，由内核给出解析错误。
func redirectPaths(p paths.Paths, config []byte, b Bundle, dir string) []byte {
	var doc map[string]any
	if err := json.Unmarshal(config, &doc); err != nil {
		return config
	}
	redirectRuleSets(p, doc, b)
	if exp, ok := doc["experimental"].(map[string]any); ok {
		if clash, ok := exp["clash_api"].(map[string]any); ok && clash["external_ui"] != nil {
			clash["external_ui"] = filepath.Join(dir, "ui")
		}
		if cache, ok := exp["cache_file"].(map[string]any); ok {
			cache["path"] = filepath.Join(dir, "cache.db")
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return config
	}
	return out
}

func redirectRuleSets(p paths.Paths, doc map[string]any, b Bundle) {
	targets := map[string]string{}
	if b.Geosite != "" && b.Geosite != p.GeositeCN {
		targets[p.GeositeCN] = b.Geosite
	}
	if b.GeoIP != "" && b.GeoIP != p.GeoIPCN {
		targets[p.GeoIPCN] = b.GeoIP
	}
	route, _ := doc["route"].(map[string]any)
	ruleSets, _ := route["rule_set"].([]any)
	for _, raw := range ruleSets {
		ruleSet, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if path, _ := ruleSet["path"].(string); targets[path] != "" {
			ruleSet["path"] = targets[path]
		}
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func cleanOutput(out []byte) string {
	return strings.TrimSpace(ansiPattern.ReplaceAllString(string(out), ""))
}

// KernelVersion 读取内核版本（`sing-box version` 首行末尾）；失败返回空串。
func KernelVersion(bin string) string {
	if !isFile(bin) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version").Output()
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(out), "\n")
	fields := strings.Fields(first)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[len(fields)-1], "v")
}

// AtLeast 比较 major.minor.patch（忽略 v 前缀与预发布后缀）；ok=false 表示版本号无法解析。
func AtLeast(version, min string) (bool, bool) {
	have, ok := parseVersion(version)
	if !ok {
		return false, false
	}
	want, ok := parseVersion(min)
	if !ok {
		return false, false
	}
	for i := range have {
		if have[i] != want[i] {
			return have[i] > want[i], true
		}
	}
	return true, true
}

func parseVersion(version string) ([3]int, bool) {
	var out [3]int
	core, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(version), "v"), "-")
	parts := strings.Split(core, ".")
	if len(parts) != len(out) {
		return out, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func isFile(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
