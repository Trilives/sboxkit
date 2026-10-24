// Package sysd systemd 单元管理（对应 service.py + resilience.py + timer.py）：
// 在 /var/lib/sboxkit-runtime 暂存自包含运行时并注册主服务，以及两类伴生单元
// （网络自愈 watchdog / 每周更新定时器）。sing-box 自带的面板走内置控制器路径
// （http://host:9090/ui/，由 experimental.clash_api.external_ui 指向运行时 UI
// 目录），与 mihomo 版一致，不再单占一个端口。
//
// 把内核、配置、geo 规则集、UI 暂存到 /var/lib/sboxkit-runtime（sing-box 的
// 工作目录），并把配置内的 external_ui / cache_file.path 改写为该目录下的绝对
// 路径，使服务与状态目录（可能在 /home）解耦。所有 root 操作经
// execx.RunRoot（非 root 自动 sudo，凭证会话内缓存）。
//
// 用户模式改用 systemd 用户单元（见 scope.go）：运行时暂存在状态目录内，
// 全部操作以当前用户执行，不请求 sudo。
package sysd

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/uiassets"
)

//go:embed assets/sing-box.service.tmpl
var singBoxUnitTmpl string

//go:embed assets/sing-box.user.service.tmpl
var singBoxUserUnitTmpl string

const (
	DefaultName          = "sing-box"
	conflictingName      = "mihomo"
	runtimeDirectoryMode = "0700"
	runtimeConfigMode    = "0600"
)

type runtimePaths struct {
	Dir     string
	Bin     string
	Config  string
	UI      string
	Geosite string
	GeoIP   string
	CacheDB string
	Unit    string
	Scope   unitScope
}

func rtPaths(name string) runtimePaths {
	return rtPathsFor(currentScope(), name)
}

func rtPathsFor(s unitScope, name string) runtimePaths {
	d := s.RuntimeDir
	if d == "" {
		d = paths.RuntimeDir
	}
	return runtimePaths{
		Scope:   s,
		Dir:     d,
		Bin:     filepath.Join(d, "sing-box"),
		Config:  filepath.Join(d, name+".json"),
		UI:      filepath.Join(d, "ui"),
		Geosite: filepath.Join(d, "geosite-cn.srs"),
		GeoIP:   filepath.Join(d, "geoip-cn.srs"),
		CacheDB: filepath.Join(d, "cache.db"),
		Unit:    s.unitFile(name + ".service"),
	}
}

// stageRuntimeConfig 读 state/config.json，把 external_ui / cache_file.path 改写为
// 运行时绝对路径，写临时文件返回。
func stageRuntimeConfig(p paths.Paths, rt runtimePaths) (string, error) {
	raw, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		return "", err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", fmt.Errorf(i18n.T("解析 state/config.json: %w"), err)
	}
	// The panel is embedded in every sboxkit binary, so runtime staging does
	// not depend on a mutable state/ui copy being present or user-writable.
	rewriteRuntimePaths(data, rt, true)
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	out = append(out, '\n')
	tmp, err := os.CreateTemp(p.State, ".runtime-config.*.json")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// rewriteRuntimePaths 把 external_ui 指向运行时 UI 目录、cache_file.path 指向运行时
// cache.db，使生效配置里保存的仍是 state 侧路径（跨主机/备份友好），只有运行时暂存
// 副本落地时才改写成绝对路径。
func rewriteRuntimePaths(doc map[string]any, rt runtimePaths, hasUI bool) {
	exp, ok := doc["experimental"].(map[string]any)
	if !ok {
		exp = map[string]any{}
		doc["experimental"] = exp
	}
	cache, ok := exp["cache_file"].(map[string]any)
	if !ok {
		cache = map[string]any{}
		exp["cache_file"] = cache
	}
	cache["enabled"] = true
	cache["path"] = rt.CacheDB
	if clash, ok := exp["clash_api"].(map[string]any); ok && hasUI {
		clash["external_ui"] = rt.UI
	}
}

type rootCommandRunner func([]string, string, *execx.Opt) (execx.Result, error)

// syncUIRuntimeWith materializes the embedded panel in a caller-owned temporary
// directory and only uses privileged operations for the runtime copy. It must
// not rewrite state/ui: the weekly root timer would otherwise make that shared
// state root-owned and break the next interactive refresh by a regular user.
func syncUIRuntimeWith(rt runtimePaths, runRoot rootCommandRunner) error {
	staged, err := os.MkdirTemp("", "sboxkit-ui-*")
	if err != nil {
		return fmt.Errorf("stage embedded UI: %w", err)
	}
	defer os.RemoveAll(staged)
	if err := uiassets.Write(staged); err != nil {
		return err
	}
	if _, err := runRoot([]string{"rm", "-rf", rt.UI}, "", nil); err != nil {
		return err
	}
	if _, err := runRoot([]string{"cp", "-a", staged, rt.UI}, "", nil); err != nil {
		return err
	}
	return nil
}

func syncUIRuntime(rt runtimePaths) error {
	return syncUIRuntimeWith(rt, rt.Scope.run)
}

func preflight(p paths.Paths) error {
	if _, err := os.Stat(p.SingBoxBin); err != nil {
		return fmt.Errorf("%s", i18n.T("未找到 sing-box 内核，请先执行『下载内核/geo 数据』"))
	}
	if _, err := os.Stat(p.ConfigFile); err != nil {
		return fmt.Errorf("%s", i18n.T("未找到生效配置 config.json，请先添加订阅"))
	}
	if _, err := os.Stat(p.GeositeCN); err != nil {
		return fmt.Errorf("%s", i18n.T("未找到 geo 规则集 geosite-cn.srs，请先执行『下载内核/geo 数据』"))
	}
	if _, err := os.Stat(p.GeoIPCN); err != nil {
		return fmt.Errorf("%s", i18n.T("未找到 geo 规则集 geoip-cn.srs，请先执行『下载内核/geo 数据』"))
	}
	if !execx.Have("systemctl") {
		return fmt.Errorf("%s", i18n.T("未找到 systemctl，注册服务需要 systemd"))
	}
	return scopeFor(p).userManagerReady()
}

// Install 注册并（可选）启动主服务。会先移除同名及冲突的 mihomo 服务；
// 校验暂存配置失败时（PrecheckError）旧服务与运行时文件保持原样。
func Install(p paths.Paths, name string, start bool) error {
	if name == "" {
		name = DefaultName
	}
	if err := validateManagedServiceName(name); err != nil {
		return precheckFailed(err)
	}
	rt, staged, err := prepareRuntime(p, name, i18n.T("注册系统服务"), true)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	s := rt.Scope

	if err := deployRuntime(p, rt, staged); err != nil {
		return err
	}

	// 移除旧的同名 / 冲突服务
	removeUnit(s, name, true)
	if name != conflictingName {
		removeUnit(s, conflictingName, true)
	}

	// 写 bootstrap DNS relay 和主 unit；主服务 Requires relay，relay 通过
	// PartOf 跟随主服务的 stop/restart 生命周期。
	if err := installBootstrapDNS(p, name, rt); err != nil {
		return err
	}
	if err := writeMainUnit(name, rt); err != nil {
		return err
	}
	if _, err := s.run(s.systemctl("enable", name+".service"), "", nil); err != nil {
		return err
	}
	s.ensureLinger()
	if start {
		if _, err := s.run(s.systemctl("restart", name+".service"), "", nil); err != nil {
			return err
		}
		execx.Ok(i18n.T("服务已启动: ") + name + ".service")
	} else {
		execx.Ok(i18n.T("服务已设为开机自启（未启动）: ") + name + ".service")
	}
	recordRuntimeVersionBestEffort(p)
	reconcileResilienceBestEffort(p, name)
	return nil
}

func renderUnit(name string, rt runtimePaths) (string, error) {
	source := singBoxUnitTmpl
	if rt.Scope.User {
		source = singBoxUserUnitTmpl
	}
	t, err := template.New("unit").Parse(source)
	if err != nil {
		return "", err
	}
	dir, bin, cfg := rt.Dir, rt.Bin, rt.Config
	if rt.Scope.User {
		// 用户模式目录可能含空格或 %，用户单元统一引用/转义；系统单元路径固定，保持原样。
		dir, bin, cfg = systemdEscape(dir), systemdExecArg(bin), systemdExecArg(cfg)
	}
	var sb strings.Builder
	err = t.Execute(&sb, struct{ Name, RuntimeDir, Bin, Config, BootstrapUnit string }{
		name, dir, bin, cfg, bootstrapServiceName(name) + ".service",
	})
	return sb.String(), err
}

// SyncAndRestart 把当前 state 中的配置、内核与 geo 规则集同步到运行时并重启服务；
// 校验暂存配置失败时（PrecheckError）运行时与运行中的服务保持原样。
func SyncAndRestart(p paths.Paths, name string) error {
	if name == "" {
		name = DefaultName
	}
	if err := validateManagedServiceName(name); err != nil {
		return precheckFailed(err)
	}
	if !IsInstalled(name) {
		execx.Warn(i18n.T("服务 ") + name + i18n.T(" 未安装，跳过同步。"))
		return nil
	}
	rt, staged, err := prepareRuntime(p, name, i18n.T("更新服务配置"), false)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	s := rt.Scope
	// 更新（含每周定时器）下载的新内核/规则在这里进入运行时
	if err := deployRuntime(p, rt, staged); err != nil {
		return err
	}
	if err := installBootstrapDNS(p, name, rt); err != nil {
		return err
	}
	if err := writeMainUnit(name, rt); err != nil {
		return err
	}
	if _, err := s.run(s.systemctl("restart", name+".service"), "", nil); err != nil {
		return err
	}
	execx.Ok(i18n.T("已同步配置并重启: ") + name + ".service")
	recordRuntimeVersionBestEffort(p)
	reconcileResilienceBestEffort(p, name)
	return nil
}

func reconcileResilienceBestEffort(p paths.Paths, name string) {
	if err := ReconcileResilience(p, name); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("修复网络自愈失败（主服务不受影响）：%v"), err))
	}
}

// Remove 停止/禁用/删除服务，并清理运行时文件。
func Remove(p paths.Paths, name string, purgeRuntime bool) error {
	if name == "" {
		name = DefaultName
	}
	if err := validateManagedServiceName(name); err != nil {
		return err
	}
	rt := rtPathsFor(scopeFor(p), name)
	s := rt.Scope
	if err := s.ensurePrivilege(i18n.T("删除系统服务")); err != nil {
		return err
	}
	wasInstalled := fileExists(rt.Unit)
	removeUnit(s, name, false)
	if purgeRuntime {
		s.run([]string{"rm", "-f", rt.Config}, "", nil)
		remaining, findErr := s.run(
			[]string{"find", rt.Dir, "-maxdepth", "1", "-name", "*.json"},
			"", &execx.Opt{Capture: true})
		if findErr == nil && strings.TrimSpace(remaining.Stdout) == "" {
			s.run([]string{"rm", "-rf", rt.Dir}, "", nil)
		}
	}
	if wasInstalled { // 安装失败的回退也会走到这里，未装过就不报「已删除」
		execx.Ok(i18n.T("服务已删除: ") + name + ".service")
	}
	return nil
}

func removeUnit(s unitScope, name string, quiet bool) {
	opt := &execx.Opt{Capture: quiet}
	rt := rtPathsFor(s, name)
	s.run(s.systemctl("stop", name+".service"), "", opt)
	s.run(s.systemctl("disable", name+".service"), "", opt)
	removeBootstrapDNS(name, rt, quiet)
	s.run([]string{"rm", "-f", rt.Unit}, "", nil)
	s.run(s.systemctl("daemon-reload"), "", opt)
	s.run(s.systemctl("reset-failed", name+".service"), "", opt)
}

func IsInstalled(name string) bool {
	if name == "" {
		name = DefaultName
	}
	_, err := os.Stat(rtPaths(name).Unit)
	return err == nil
}
