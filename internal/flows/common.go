// Package flows 交互流程编排（对应 Python 版 flows/ + 各模块 menu_flow）：
// 初始化 / 更改配置 / 网络测试 / 卸载 / 切换节点 / 定制层编辑 / 主菜单。
//
// 结构与 Python 版一比一对应：tui 的四类提示为阻塞调用，esc/^R 以
// ErrSaveExit/ErrCancelled 错误返回，事务语义由 txn.Run 承载。
package flows

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

// 菜单顺序即推荐优先级：Clash 订阅最常见；sing-box 原生订阅次之；本地文件排最后。
var sourceOptions = []string{
	"Clash 订阅（★推荐：机场通用格式，经 converter 转换为 sing-box 配置）",
	"sing-box 原生订阅（机场直接提供 sing-box JSON）",
	"通用 base64 订阅（经 subconverter 云端解析为 Clash）",
	"本地文件（直接导入为订阅，不联网拉取）",
}

var sourceTypes = []string{"clash", "sing-box", "base64", "local"}

// stripScheme 去掉 http:// / https:// 前缀，便于以 IP:端口 形式回显默认值。
func stripScheme(proxy string) string {
	p := strings.TrimSpace(proxy)
	if i := strings.Index(p, "://"); i >= 0 {
		return p[i+3:]
	}
	return p
}

// normalizeProxy 把用户输入的代理归一化为可用 URL：空→空；含 scheme 原样；否则补 http://。
func normalizeProxy(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return ""
	}
	if strings.Contains(p, "://") {
		return p
	}
	return "http://" + p
}

type newSub struct {
	Name          string
	URL           string
	SourceType    string
	ApplyOverlay  bool
	FetchViaProxy bool
}

// askNewSubscription 交互收集新订阅信息；订阅链接留空 → (nil, nil) 表示「暂不配置」。
func askNewSubscription() (*newSub, error) {
	defaultName := time.Now().Format("sub-20060102-150405")
	name, err := tui.Ask(i18n.T("订阅名称，留空=时间戳"), tui.AskOpts{Default: defaultName})
	if err != nil {
		return nil, err
	}
	translatedSources := make([]string, len(sourceOptions))
	for i, o := range sourceOptions {
		translatedSources[i] = i18n.T(o)
	}
	idx, err := tui.Select(i18n.T("选择订阅来源类型"), translatedSources, tui.SelectOpts{})
	if err != nil {
		return nil, err
	}
	sourceType := sourceTypes[idx]
	prompt := i18n.T("订阅链接，留空=暂不配置")
	if sourceType == "local" {
		prompt = i18n.T("本地文件路径（Clash YAML 或 sing-box JSON），留空=暂不配置")
	}
	subURL, err := tui.Ask(prompt, tui.AskOpts{AllowEmpty: true})
	if err != nil {
		return nil, err
	}
	if subURL == "" {
		return nil, nil
	}
	if sourceType == "local" {
		subURL, err = resolveLocalPath(subURL)
		if err != nil {
			return nil, err
		}
	}
	// ApplyOverlay 对 clash/base64 来源始终生效（converter 现场生成整份配置，
	// 含 AI / 流媒体 / 地区自动测速组，无需额外确认）；只有 sing-box 原生订阅
	// 需要用户选择「信任原配置只补面板」还是「按 sboxkit 统一规则重建」。
	rebuild := true
	if sourceType == "sing-box" {
		rebuild, err = tui.Confirm(i18n.T("按 sboxkit 统一规则重建该订阅（TUN / DNS / AI / 流媒体 / 地区自动测速组）？否则仅信任你的原生配置，只补齐面板/控制器设置。"), true)
		if err != nil {
			return nil, err
		}
	}
	fetchViaProxy := false
	if sourceType != "local" {
		fetchViaProxy, err = tui.Confirm(i18n.T("使用下载代理拉取此订阅？默认否＝直连"), false)
		if err != nil {
			return nil, err
		}
	}
	return &newSub{Name: name, URL: subURL, SourceType: sourceType, ApplyOverlay: rebuild, FetchViaProxy: fetchViaProxy}, nil
}

// EnsureStateRoot 确保固定数据目录存在且当前用户可写：能直接建则直接建，
// 无权限（如首次创建 /var/lib/sboxkit）则经 sudo 创建并把属主交回当前用户。
func EnsureStateRoot(p paths.Paths) error {
	if err := os.MkdirAll(p.State, 0o755); err == nil {
		probe := filepath.Join(p.State, ".probe")
		if werr := os.WriteFile(probe, nil, 0o644); werr == nil {
			os.Remove(probe)
			return nil
		}
	}
	uid, gid := fmt.Sprint(os.Getuid()), fmt.Sprint(os.Getgid())
	_, err := execx.RunRoot([]string{"install", "-d", "-m", "0755", "-o", uid, "-g", gid, p.State},
		i18n.T("创建数据目录 ")+p.State, nil)
	return err
}

// ensureGithubToken 未配置 GitHub Token 时交互式询问是否补充，输入后写回 customize.json
// （对应 core._prompt_and_save_token）。非 TTY 静默跳过。
func ensureGithubToken(p paths.Paths) {
	if kernel.LoadSettings(p).GithubToken != "" || !tui.UseTUI() {
		return
	}
	execx.Warn(i18n.T("未配置 GitHub Token，匿名 API 限额较低（60 次/小时），高频操作易被限流。"))
	ok, err := tui.Confirm(i18n.T("现在添加 GitHub Token？"), false)
	if err != nil || !ok {
		return
	}
	token, err := tui.Ask("GitHub Token", tui.AskOpts{AllowEmpty: true})
	if err != nil || token == "" {
		return
	}
	cfg := config.Load(p)
	cfg.GitHubToken = token
	if err := config.Save(p, cfg); err != nil {
		execx.Warn(i18n.T("Token 保存失败：") + err.Error())
		return
	}
	execx.Ok(i18n.T("GitHub Token 已保存到 customize.json。"))
}

// PickLanguage 语言选择器（主菜单「语言 / Language」与初始化流程第一步共用）：
// 标题与选项本身直接写死双语字面量（不经过 i18n.T），因为这是语言选择器自身——
// 用户在任何当前语言状态下都要能看懂两个选项各自对应哪种语言。esc/^R 取消，
// 语言保持不变（不算错误）。
func PickLanguage(p paths.Paths) error {
	current := 0
	if i18n.Current() == i18n.ZH {
		current = 1
	}
	i, err := tui.Select("Language / 语言", []string{"English", "中文"}, tui.SelectOpts{Initial: current})
	if err != nil {
		return nil
	}
	lang := i18n.EN
	if i == 1 {
		lang = i18n.ZH
	}
	cfg := config.Load(p)
	cfg.Language = string(lang)
	if err := config.Save(p, cfg); err != nil {
		return err
	}
	i18n.SetLang(lang)
	return nil
}
