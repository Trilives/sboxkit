package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	ui "github.com/Trilives/sboxkit/internal/tui"
)

func TestRemoteSubscriptionPromptOrderStartsWithSource(t *testing.T) {
	var out bytes.Buffer
	session := scriptedTUISession(&out, []string{
		"sing-box",
		"work",
		"https://example.test/config.json",
		"",
	}, []bool{true})

	args, ok := session.buildRemoteSubscriptionArgs()
	if !ok {
		t.Fatal("expected remote subscription args")
	}
	wantArgs := []string{"add", "--name", "work", "--source", "sing-box", "--url", "https://example.test/config.json"}
	if strings.Join(args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}

	text := out.String()
	sourceIndex := strings.Index(text, "来源类型")
	nameIndex := strings.Index(text, "名称")
	urlIndex := strings.Index(text, "订阅链接")
	proxyIndex := strings.Index(text, "下载代理（默认留空）")
	if sourceIndex < 0 || nameIndex < 0 || urlIndex < 0 || proxyIndex < 0 {
		t.Fatalf("missing expected prompts in %q", text)
	}
	if !(sourceIndex < nameIndex && nameIndex < urlIndex && urlIndex < proxyIndex) {
		t.Fatalf("prompt order = %q, want source before name before URL before proxy", text)
	}
}

func TestRunRemembersMainMenuSelectionAfterReturning(t *testing.T) {
	var out bytes.Buffer
	session := newTUISession(&out, &out)
	session.pauseF = func(string) {}

	var calls []int
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		if title != "sboxkit" {
			t.Fatalf("unexpected title %q", title)
		}
		calls = append(calls, opts.Initial)
		switch len(calls) {
		case 1:
			return indexOfLabel(options, "帮助"), nil
		case 2:
			return indexOfLabel(options, "退出"), nil
		default:
			t.Fatalf("unexpected select call %d", len(calls))
			return 0, nil
		}
	}

	if code := session.run(); code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}

	if len(calls) != 2 {
		t.Fatalf("select calls = %d, want 2", len(calls))
	}
	var mainLabels []string
	for _, item := range mainTUIItems() {
		mainLabels = append(mainLabels, item.Label)
	}
	helpIndex := indexOfLabel(mainLabels, "帮助")
	if helpIndex < 0 {
		t.Fatal("missing 帮助 item in main menu")
	}
	want := []int{0, helpIndex}
	for i, got := range calls {
		if got != want[i] {
			t.Fatalf("select initial[%d] = %d, want %d", i, got, want[i])
		}
	}
}

func TestSelectMenuRemembersSelectionAcrossEntries(t *testing.T) {
	var out bytes.Buffer
	session := newTUISession(&out, &out)

	var initials []int
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		initials = append(initials, opts.Initial)
		switch len(initials) {
		case 1:
			return 1, nil
		case 2:
			return 0, nil
		default:
			t.Fatalf("unexpected select call %d", len(initials))
			return 0, nil
		}
	}

	items := []tuiItem{{Label: "A"}, {Label: "B"}}
	if _, ok := session.selectMenu("节点", items); !ok {
		t.Fatal("first selectMenu call failed")
	}
	if _, ok := session.selectMenu("节点", items); !ok {
		t.Fatal("second selectMenu call failed")
	}

	want := []int{0, 1}
	if len(initials) != len(want) {
		t.Fatalf("initial calls = %v, want %v", initials, want)
	}
	for i, got := range initials {
		if got != want[i] {
			t.Fatalf("initial[%d] = %d, want %d", i, got, want[i])
		}
	}
}

func TestMainMenuPutsNodesNearTop(t *testing.T) {
	items := mainTUIItems()
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}

	nodesIndex := indexOfLabel(labels, "节点")
	subscriptionsIndex := indexOfLabel(labels, "订阅")
	serviceIndex := indexOfLabel(labels, "服务")
	if nodesIndex < 0 || subscriptionsIndex < 0 || serviceIndex < 0 {
		t.Fatalf("missing expected main menu items: %#v", labels)
	}
	if !(nodesIndex < subscriptionsIndex && nodesIndex < serviceIndex) {
		t.Fatalf("节点 should be before 订阅 and 服务, got order %#v", labels)
	}
}

func TestFirstSetupUpdatesRulesThroughRunningProxy(t *testing.T) {
	want := []string{"--proxy", "http://127.0.0.1:7890", "--sync-service"}
	got := firstSetupPostStartUpdateArgs()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("first setup update args = %#v, want %#v", got, want)
	}
}

func TestConfigMenuGroupsToggleItems(t *testing.T) {
	items := configTUIItems()
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	want := []string{
		"显示配置",
		"编辑定制层",
		"Shell 代理环境",
		"高级键值",
	}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("config menu labels = %#v, want %#v", labels, want)
	}

	for _, label := range labels {
		for _, flatPrefix := range []string{"启用", "关闭", "写入", "移除", "设置"} {
			if strings.HasPrefix(label, flatPrefix) {
				t.Fatalf("config top-level item %q should be under a grouped submenu", label)
			}
		}
	}
}

func TestConfigEditorTogglesBoolAndSavesOnSaveExit(t *testing.T) {
	t.Setenv("SBOXKIT_ROOT", t.TempDir())
	var out bytes.Buffer
	session := newTUISession(&out, &out)

	calls := 0
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		calls++
		switch calls {
		case 1:
			if title != "编辑定制层" {
				t.Fatalf("unexpected title %q", title)
			}
			idx := indexOfLabelContaining(options, "常用部署")
			if idx < 0 {
				t.Fatalf("missing common section in options: %#v", options)
			}
			return idx, nil
		case 2:
			if title != "配置区块 · 常用部署" {
				t.Fatalf("unexpected title %q", title)
			}
			idx := indexOfLabelContaining(options, "TUN 模式")
			if idx < 0 {
				t.Fatalf("missing TUN field in options: %#v", options)
			}
			return idx, nil
		case 3:
			if title != "配置区块 · 常用部署" {
				t.Fatalf("unexpected title %q", title)
			}
			return 0, ui.ErrSaveExit
		case 4:
			if title != "编辑定制层" {
				t.Fatalf("unexpected title %q", title)
			}
			return 0, ui.ErrSaveExit
		default:
			t.Fatalf("unexpected select call %d", calls)
			return 0, ui.ErrCancelled
		}
	}

	changed, err := session.editCustomize()
	if err != nil {
		t.Fatalf("editCustomize error: %v", err)
	}
	if !changed {
		t.Fatal("expected editor to save a change")
	}

	cfg := mustLoadConfigForTest(t)
	if cfg.EnableTun {
		t.Fatal("expected enable_tun to be toggled off and saved")
	}
}

func TestConfigEditorIncludesLANProxyField(t *testing.T) {
	cfg := mustLoadConfigForTest(t)
	sections := configSectionLabels(cfg)
	common := indexOfLabelContaining(sections, "常用部署")
	if common < 0 {
		t.Fatalf("config editor missing common section: %#v", sections)
	}
	labels := sectionFieldLabels(cfg, configSections[common])
	if indexOfLabelContaining(labels, "局域网代理") < 0 {
		t.Fatalf("common config section missing LAN proxy field: %#v", labels)
	}
}

func TestNetworkTestActionPrintsProgressPrompt(t *testing.T) {
	var out bytes.Buffer

	printNetworkTestProgress(&out)

	if !strings.Contains(out.String(), "正在通过 127.0.0.1:7890 测试网络") {
		t.Fatalf("missing network test progress prompt: %q", out.String())
	}
}

func scriptedTUISession(out *bytes.Buffer, answers []string, confirms []bool) *tuiSession {
	answerIndex := 0
	confirmIndex := 0
	session := newTUISession(out, out)
	session.askF = func(prompt string, opts ui.AskOpts) (string, error) {
		out.WriteString(prompt)
		out.WriteByte('\n')
		if answerIndex >= len(answers) {
			return opts.Default, nil
		}
		answer := answers[answerIndex]
		answerIndex++
		if answer == "" && opts.Default != "" {
			return opts.Default, nil
		}
		return answer, nil
	}
	session.confirmF = func(prompt string, fallback bool) (bool, error) {
		out.WriteString(prompt)
		out.WriteByte('\n')
		if confirmIndex >= len(confirms) {
			return fallback, nil
		}
		answer := confirms[confirmIndex]
		confirmIndex++
		return answer, nil
	}
	session.pauseF = func(string) {}
	return session
}

func indexOfLabel(labels []string, label string) int {
	for i, value := range labels {
		if value == label {
			return i
		}
	}
	return -1
}

func indexOfLabelContaining(labels []string, fragment string) int {
	for i, value := range labels {
		if strings.Contains(value, fragment) {
			return i
		}
	}
	return -1
}

func mustLoadConfigForTest(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load(paths.FromRoot("").CustomizeFile)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}
