// Package flows 交互流程编排（对应 Python 版 flows/ + 各模块 menu_flow）：
// 初始化 / 更改配置 / 网络测试 / 卸载 / 切换节点 / 定制层编辑 / 主菜单。
//
// 结构与 Python 版一比一对应：tui 的四类提示为阻塞调用，esc/^R 以
// ErrSaveExit/ErrCancelled 错误返回，事务语义由 txn.Run 承载。
package flows

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

type subscriptionSource struct {
	Type       string
	LongLabel  string
	ShortLabel string
}

// 顺序即推荐优先级：Clash 最常见；sing-box 原生次之；本地文件排最后。
// Type 是持久化值，界面文案变化或切换语言不得影响其含义。
var subscriptionSources = []subscriptionSource{
	{Type: "clash", LongLabel: "Clash 订阅（★推荐：机场通用格式，经 converter 转换为 sing-box 配置）", ShortLabel: "Clash"},
	{Type: "sing-box", LongLabel: "sing-box 原生订阅（机场直接提供 sing-box JSON）", ShortLabel: "sing-box"},
	{Type: "base64", LongLabel: "通用 base64 订阅（经 subconverter 云端解析为 Clash）", ShortLabel: "base64"},
	{Type: "local", LongLabel: "本地文件（直接导入为订阅，不联网拉取）", ShortLabel: "本地文件"},
}

type newSub struct {
	Name          string
	URL           string
	SourceType    string
	ApplyOverlay  bool
	FetchViaProxy bool
}

// isRemoteSource 带 scheme 的输入按远程订阅拉取，其余按本地文件路径。
func isRemoteSource(value string) bool { return strings.Contains(value, "://") }

// subscriptionSourceType 按输入决定实际来源：不带 scheme 的输入不论所选类型都按本地文件导入
// （本地导入按内容识别 Clash / sing-box / base64，与选对类型的结果相同）；选了「本地文件」
// 却填写远程地址时无从判断格式，报错让用户改选。
func subscriptionSourceType(chosen, input string) (string, error) {
	switch {
	case !isRemoteSource(input):
		return "local", nil
	case chosen == "local":
		return "", fmt.Errorf("%s", i18n.T("订阅类型为「本地文件」时请填写文件路径；远程订阅请选择 Clash、sing-box 或 base64"))
	}
	return chosen, nil
}

// askNewSubscription 交互收集新订阅信息；订阅链接留空 → (nil, nil) 表示「暂不配置」。
func askNewSubscription() (*newSub, error) {
	defaultName := time.Now().Format("sub-20060102-150405")
	name, err := tui.Ask(i18n.T("订阅名称，留空=时间戳"), tui.AskOpts{Default: defaultName})
	if err != nil {
		return nil, err
	}
	translatedSources := make([]string, len(subscriptionSources))
	for i, source := range subscriptionSources {
		translatedSources[i] = i18n.T(source.LongLabel)
	}
	idx, err := tui.Select(i18n.T("选择订阅来源类型"), translatedSources, tui.SelectOpts{})
	if err != nil {
		return nil, err
	}
	chosen := subscriptionSources[idx].Type
	prompt := i18n.T("订阅链接（也可填写本地文件路径），留空=暂不配置")
	if chosen == "local" {
		prompt = i18n.T("本地文件路径（Clash YAML 或 sing-box JSON），留空=暂不配置")
	}
	subURL, err := tui.Ask(prompt, tui.AskOpts{AllowEmpty: true})
	if err != nil {
		return nil, err
	}
	if subURL == "" {
		return nil, nil
	}
	sourceType, err := subscriptionSourceType(chosen, subURL)
	if err != nil {
		return nil, err
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
	if chosen == "sing-box" {
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

// EnsureStateRoot 确保数据目录存在且可写：用户模式收紧为属主独占；系统模式要求
// root（main 在启动时经 sudo 提权），目录只归 root，旧版本交给用户的目录在此收归。
func EnsureStateRoot(p paths.Paths) error {
	if err := rejectForeignRoot(p, os.Geteuid()); err != nil {
		return err
	}
	if p.UserMode {
		return ensureUserModeState(p)
	}
	if err := rejectForeignSystemRoot(p, os.Geteuid()); err != nil {
		return err
	}
	return ensureSystemState(p, os.Geteuid())
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
