// 把 state 中的内核与 geo 规则集部署到运行时目录。安装与同步（含 `sboxkit update`
// 与每周定时器）走同一条路径，因此更新下载的新内核/规则会真正进入运行中的服务。
//
// 用户模式 + TUN 时，运行时内核靠文件能力（setcap）拿到 CAP_NET_ADMIN，替换文件
// 会丢失能力，所以新内核先以 .new 暂存、授予能力后再原子改名。无人值守（定时器）
// 拿不到 sudo 时保留旧内核继续运行，只更新规则与配置，并留下提示。
package sysd

import (
	"bytes"
	"crypto/sha256"
	"io"
	"os"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

type coreAction int

const (
	coreKeep    coreAction = iota // 运行时内核已是最新
	coreReplace                   // 部署新内核
	coreDefer                     // 新内核需要 sudo 授予能力但当前无法提权：保留旧内核
)

// canElevate 可在测试中替换。
var canElevate = execx.CanElevate

// planCoreDeploy 决定运行时内核如何处理。needCaps 表示新配置需要 CAP_NET_ADMIN。
// 系统模式由 unit 授予能力，总是覆盖；用户模式内容一致时不覆盖，以保住文件能力。
func planCoreDeploy(p paths.Paths, rt runtimePaths, needCaps bool) coreAction {
	if !rt.Scope.User {
		return coreReplace
	}
	if sameFileContent(p.SingBoxBin, rt.Bin) {
		return coreKeep
	}
	if needCaps && fileExists(rt.Bin) && !canElevate() {
		return coreDefer
	}
	return coreReplace
}

// deployRuntimeAssets 部署内核与 geo 规则集；staged 为即将安装的运行时配置。
func deployRuntimeAssets(p paths.Paths, rt runtimePaths, staged string) error {
	s := rt.Scope
	reason := i18n.T("部署运行时")
	for _, cmd := range [][]string{
		{"mkdir", "-p", rt.Dir},
		{"chmod", runtimeDirectoryMode, rt.Dir},
	} {
		if _, err := s.run(cmd, reason, nil); err != nil {
			return err
		}
	}
	if err := deployRuntimeCore(p, rt, staged); err != nil {
		return err
	}
	for _, cmd := range [][]string{
		{"install", "-m", "0644", p.GeositeCN, rt.Geosite},
		{"install", "-m", "0644", p.GeoIPCN, rt.GeoIP},
	} {
		if _, err := s.run(cmd, reason, nil); err != nil {
			return err
		}
	}
	return nil
}

func deployRuntimeCore(p paths.Paths, rt runtimePaths, staged string) error {
	needCaps := false
	if rt.Scope.User {
		need, err := runtimeConfigNeedsNetAdmin(staged)
		if err != nil {
			return err
		}
		needCaps = need
	}
	switch planCoreDeploy(p, rt, needCaps) {
	case coreKeep:
		return nil
	case coreDefer:
		execx.Warn(i18n.T("新内核已下载，但启用 TUN 需要 sudo 为其授予网络能力，当前无法交互授权：服务继续使用旧内核（规则集与配置已更新）。请稍后在 sboxkit 中执行『运行时管理 → 服务设置 → 重载服务』完成切换。"))
		return nil
	}
	s := rt.Scope
	staging := rt.Bin + ".new"
	// 临时名 + 原子改名，避免运行中替换报 ETXTBSY；能力授予在改名前完成，
	// 授权失败时运行时仍是可用的旧内核。
	if _, err := s.run([]string{"install", "-m", "0755", p.SingBoxBin, staging}, i18n.T("部署运行时"), nil); err != nil {
		return err
	}
	if needCaps {
		if err := grantNetCapabilities(staging); err != nil {
			s.run([]string{"rm", "-f", staging}, "", nil)
			return err
		}
	}
	_, err := s.run([]string{"mv", "-f", staging, rt.Bin}, i18n.T("部署运行时"), nil)
	return err
}

// WarnPendingCore 用户模式下，若 state 中的内核比运行时新（定时器推迟了切换，或
// 仅执行过 assets-update），进入菜单时提示如何让服务用上新内核。
func WarnPendingCore(p paths.Paths, name string) {
	if name == "" {
		name = DefaultName
	}
	if corePending(p, rtPathsFor(scopeFor(p), name)) {
		execx.Warn(i18n.T("已下载的新内核尚未被服务使用：请执行『运行时管理 → 服务设置 → 重载服务』（启用 TUN 时需一次 sudo 授权）。"))
	}
}

func corePending(p paths.Paths, rt runtimePaths) bool {
	if !rt.Scope.User || !fileExists(rt.Unit) || !fileExists(p.SingBoxBin) || !fileExists(rt.Bin) {
		return false
	}
	return !sameFileContent(p.SingBoxBin, rt.Bin)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func sameFileContent(a, b string) bool {
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	if errA != nil || errB != nil || sa.Size() != sb.Size() {
		return false
	}
	ha, errA := fileSHA256(a)
	hb, errB := fileSHA256(b)
	return errA == nil && errB == nil && bytes.Equal(ha, hb)
}

func fileSHA256(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}
