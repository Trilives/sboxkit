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
package sysd

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
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

const (
	DefaultName     = "sing-box"
	conflictingName = "mihomo"
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
}

func rtPaths(name string) runtimePaths {
	d := paths.RuntimeDir
	return runtimePaths{
		Dir:     d,
		Bin:     filepath.Join(d, "sing-box"),
		Config:  filepath.Join(d, name+".json"),
		UI:      filepath.Join(d, "ui"),
		Geosite: filepath.Join(d, "geosite-cn.srs"),
		GeoIP:   filepath.Join(d, "geoip-cn.srs"),
		CacheDB: filepath.Join(d, "cache.db"),
		Unit:    "/etc/systemd/system/" + name + ".service",
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
	rewriteRuntimePaths(data, rt, hasUIAssets(p))
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

func hasUIAssets(p paths.Paths) bool {
	_, err := os.Stat(filepath.Join(p.UI, "index.html"))
	return err == nil
}

// syncUIRuntime 把 state/ui 的最新面板资源同步到运行时目录（sing-box 只会读运行时
// 副本），Install 和 SyncAndRestart 都要调用，否则自更新/配置变更后面板会停留在
// 服务首次注册时的旧版本。先用当前二进制内置的面板重新物化 state/ui，确保运行时
// 拿到的面板与本二进制一致——否则 sboxkit 自更新换了新面板后，state/ui 仍是旧订阅
// 构建时落地的旧副本，光同步只会把旧面板拷过去。
func syncUIRuntime(p paths.Paths, rt runtimePaths) {
	if err := uiassets.Write(p.UI); err != nil {
		execx.Warn(i18n.T("刷新内置 Web 面板资源失败：") + err.Error())
	}
	if !hasUIAssets(p) {
		execx.Warn(i18n.T("未找到内置 Web 面板资源，面板将不可用。"))
		return
	}
	execx.RunRoot([]string{"rm", "-rf", rt.UI}, "", nil)
	execx.RunRoot([]string{"cp", "-a", p.UI, rt.UI}, "", nil)
}

// panelUpToDate 比对运行时目录里的面板与当前二进制内置的面板是否一致。运行时目录
// 内容世界可读（cp -a 保留 0644/0755），因此这一步无需 root 即可判定。
func panelUpToDate(rt runtimePaths) bool {
	src := uiassets.FS()
	for _, name := range []string{"index.html", "app.js", "styles.css"} {
		want, err := fs.ReadFile(src, name)
		if err != nil {
			return true // 内置资源异常，无从比对，别误判为过期
		}
		got, err := os.ReadFile(filepath.Join(rt.UI, name))
		if err != nil || !bytes.Equal(want, got) {
			return false
		}
	}
	return true
}

// RefreshPanelIfStale 若服务已安装、且运行时面板与当前二进制内置面板不一致（典型
// 场景：sboxkit 自更新后二进制带了新面板，但运行时目录仍是旧副本），就把内置面板
// 重新物化并同步到运行时目录。面板由 sing-box 内置文件服务实时读取，刷新文件即时
// 生效，无需重启服务。仅在检测到过期时才请求 sudo，日常启动零打扰。
func RefreshPanelIfStale(p paths.Paths, name string) {
	if name == "" {
		name = DefaultName
	}
	if !IsInstalled(name) {
		return
	}
	rt := rtPaths(name)
	if panelUpToDate(rt) {
		return
	}
	execx.Info(i18n.T("检测到内置 Web 面板有更新，正在同步到运行时…"))
	if err := execx.EnsureSudo(i18n.T("更新内置 Web 面板")); err != nil {
		return
	}
	syncUIRuntime(p, rt)
	execx.Ok(i18n.T("内置 Web 面板已更新（浏览器刷新 http://host:9090/ui/ 即可看到新版）。"))
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
	return nil
}

// Install 注册并（可选）启动主服务。会先移除同名及冲突的 mihomo 服务。
func Install(p paths.Paths, name string, start bool) error {
	if name == "" {
		name = DefaultName
	}
	if err := preflight(p); err != nil {
		return err
	}
	rt := rtPaths(name)
	if err := execx.EnsureSudo(i18n.T("注册系统服务")); err != nil {
		return err
	}

	staged, err := stageRuntimeConfig(p, rt)
	if err != nil {
		return err
	}
	defer os.Remove(staged)

	steps := [][]string{
		{"mkdir", "-p", rt.Dir},
		{"chmod", "0755", rt.Dir},
		// 二进制：临时名 + 原子改名，避免运行中替换报 ETXTBSY
		{"install", "-m", "0755", p.SingBoxBin, rt.Bin + ".new"},
		{"mv", "-f", rt.Bin + ".new", rt.Bin},
		{"install", "-m", "0644", p.GeositeCN, rt.Geosite},
		{"install", "-m", "0644", p.GeoIPCN, rt.GeoIP},
	}
	for _, cmd := range steps {
		if _, err := execx.RunRoot(cmd, i18n.T("部署运行时"), nil); err != nil {
			return err
		}
	}
	// UI（内置静态面板，见 internal/uiassets；缺失也不影响内核运行）
	syncUIRuntime(p, rt)
	// 配置 + 运行时校验
	if _, err := execx.RunRoot([]string{"install", "-m", "0644", staged, rt.Config}, "", nil); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{rt.Bin, "check", "-c", rt.Config}, i18n.T("校验配置"), nil); err != nil {
		return err
	}

	// 移除旧的同名 / 冲突服务
	removeUnit(name, true)
	if name != conflictingName {
		removeUnit(conflictingName, true)
	}

	// 写 unit
	unitText, err := renderUnit(name, rt)
	if err != nil {
		return err
	}
	if err := execx.WriteRoot(rt.Unit, unitText, "0644", i18n.T("写服务单元")); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{"systemctl", "daemon-reload"}, "", nil); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{"systemctl", "enable", name + ".service"}, "", nil); err != nil {
		return err
	}
	if start {
		if _, err := execx.RunRoot([]string{"systemctl", "restart", name + ".service"}, "", nil); err != nil {
			return err
		}
		execx.Ok(i18n.T("服务已启动: ") + name + ".service")
	} else {
		execx.Ok(i18n.T("服务已设为开机自启（未启动）: ") + name + ".service")
	}
	return nil
}

func renderUnit(name string, rt runtimePaths) (string, error) {
	t, err := template.New("unit").Parse(singBoxUnitTmpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	err = t.Execute(&sb, struct{ Name, RuntimeDir, Bin, Config string }{name, rt.Dir, rt.Bin, rt.Config})
	return sb.String(), err
}

// SyncAndRestart 把当前 state/config.json 同步到运行时并重启服务。
func SyncAndRestart(p paths.Paths, name string) error {
	if name == "" {
		name = DefaultName
	}
	if !IsInstalled(name) {
		execx.Warn(i18n.T("服务 ") + name + i18n.T(" 未安装，跳过同步。"))
		return nil
	}
	if err := preflight(p); err != nil {
		return err
	}
	rt := rtPaths(name)
	if err := execx.EnsureSudo(i18n.T("更新服务配置")); err != nil {
		return err
	}
	staged, err := stageRuntimeConfig(p, rt)
	if err != nil {
		return err
	}
	defer os.Remove(staged)
	syncUIRuntime(p, rt)
	if _, err := execx.RunRoot([]string{"install", "-m", "0644", staged, rt.Config}, "", nil); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{rt.Bin, "check", "-c", rt.Config}, i18n.T("校验配置"), nil); err != nil {
		return err
	}
	if _, err := execx.RunRoot([]string{"systemctl", "restart", name + ".service"}, "", nil); err != nil {
		return err
	}
	execx.Ok(i18n.T("已同步配置并重启: ") + name + ".service")
	return nil
}

// Remove 停止/禁用/删除服务，并清理运行时文件。
func Remove(p paths.Paths, name string, purgeRuntime bool) error {
	if name == "" {
		name = DefaultName
	}
	if err := execx.EnsureSudo(i18n.T("删除系统服务")); err != nil {
		return err
	}
	removeUnit(name, false)
	if purgeRuntime {
		rt := rtPaths(name)
		execx.RunRoot([]string{"rm", "-f", rt.Config}, "", nil)
		remaining, _ := execx.RunRoot(
			[]string{"bash", "-c", fmt.Sprintf("ls %s/*.json 2>/dev/null | wc -l", paths.RuntimeDir)},
			"", &execx.Opt{Capture: true})
		if strings.TrimSpace(remaining.Stdout) == "0" {
			execx.RunRoot([]string{"rm", "-rf", paths.RuntimeDir}, "", nil)
		}
	}
	execx.Ok(i18n.T("服务已删除: ") + name + ".service")
	return nil
}

func removeUnit(name string, quiet bool) {
	opt := &execx.Opt{Capture: quiet}
	execx.RunRoot([]string{"systemctl", "stop", name + ".service"}, "", opt)
	execx.RunRoot([]string{"systemctl", "disable", name + ".service"}, "", opt)
	execx.RunRoot([]string{"rm", "-f", rtPaths(name).Unit}, "", nil)
	execx.RunRoot([]string{"systemctl", "daemon-reload"}, "", opt)
	execx.RunRoot([]string{"systemctl", "reset-failed", name + ".service"}, "", opt)
}

func IsInstalled(name string) bool {
	if name == "" {
		name = DefaultName
	}
	_, err := os.Stat(rtPaths(name).Unit)
	return err == nil
}

func Status(name string) {
	if name == "" {
		name = DefaultName
	}
	execx.Run([]string{"systemctl", "status", "--no-pager", name + ".service"}, nil)
}

func unitActive(unit string) bool {
	r, _ := execx.Run([]string{"systemctl", "is-active", unit}, &execx.Opt{Capture: true})
	return strings.TrimSpace(r.Stdout) == "active"
}

func IsActive(name string) bool {
	if name == "" {
		name = DefaultName
	}
	return unitActive(name + ".service")
}

// CompanionUnits 已安装的伴生 systemd 单元（暂停/启动须一并带上，
// 否则 watchdog 会把刚停掉的主服务又拉起来）。
func CompanionUnits() []string {
	var units []string
	if ResilienceInstalled() {
		units = append(units, WatchdogName+".timer")
	}
	if TimerInstalled() {
		units = append(units, TimerName+".timer")
	}
	return units
}

// Pause 暂停主服务及全部伴生单元（运行时停止；单元保持开机自启）。
func Pause(name string) error {
	if name == "" {
		name = DefaultName
	}
	if !IsInstalled(name) {
		execx.Warn(i18n.T("服务 ") + name + i18n.T(" 未安装，无需暂停。"))
		return nil
	}
	companions := CompanionUnits()
	if err := execx.EnsureSudo(i18n.T("暂停服务")); err != nil {
		return err
	}
	// 先停伴生 watchdog，否则刚停掉主服务它又会拉起来
	for _, unit := range companions {
		execx.RunRoot([]string{"systemctl", "stop", unit}, "", &execx.Opt{Capture: true})
	}
	execx.RunRoot([]string{"systemctl", "stop", name + ".service"}, "", nil)
	suffix := ""
	if len(companions) > 0 {
		suffix = fmt.Sprintf(i18n.T(" + %d 个伴生单元"), len(companions))
	}
	execx.Ok(i18n.T("已暂停：") + name + ".service" + suffix)
	execx.Info(i18n.T("提示：暂停为运行时停止，重启系统后会自动恢复运行。"))
	return nil
}

// Resume 启动主服务及全部已安装的伴生单元。
func Resume(name string) error {
	if name == "" {
		name = DefaultName
	}
	if !IsInstalled(name) {
		execx.Warn(i18n.T("服务 ") + name + i18n.T(" 未安装，请先执行『初始化』。"))
		return nil
	}
	companions := CompanionUnits()
	if err := execx.EnsureSudo(i18n.T("启动服务")); err != nil {
		return err
	}
	execx.RunRoot([]string{"systemctl", "start", name + ".service"}, "", nil)
	for _, unit := range companions {
		execx.RunRoot([]string{"systemctl", "start", unit}, "", &execx.Opt{Capture: true})
	}
	suffix := ""
	if len(companions) > 0 {
		suffix = fmt.Sprintf(i18n.T(" + %d 个伴生单元"), len(companions))
	}
	execx.Ok(i18n.T("已启动：") + name + ".service" + suffix)
	return nil
}
