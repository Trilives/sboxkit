package app

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
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
	t.Setenv("SBOXKIT_ROOT", t.TempDir())
	var out bytes.Buffer
	session := newTUISession(&out, &out)
	session.serviceExistsF = func() bool { return true }
	session.pauseF = func(string) {}

	var calls []int
	submenuCalls := 0
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		switch title {
		case "sboxkit":
			calls = append(calls, opts.Initial)
			switch len(calls) {
			case 1:
				return indexOfLabel(options, "Restart Required"), nil
			case 2:
				return 0, ui.ErrCancelled
			default:
				t.Fatalf("unexpected main select call %d", len(calls))
				return 0, ui.ErrCancelled
			}
		case "Restart Required":
			submenuCalls++
			if submenuCalls > 1 {
				return 0, ui.ErrCancelled
			}
			return indexOfLabel(options, "Help"), nil
		default:
			t.Fatalf("unexpected title %q", title)
			return 0, ui.ErrCancelled
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
	configIndex := indexOfLabel(mainLabels, "Restart Required")
	if configIndex < 0 {
		t.Fatal("missing Restart Required item in main menu")
	}
	want := []int{0, configIndex}
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
	if _, ok := session.selectMenu("Nodes", items); !ok {
		t.Fatal("first selectMenu call failed")
	}
	if _, ok := session.selectMenu("Nodes", items); !ok {
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

func TestMainMenuDefaultsToEnglishAndAggregatesSystemSettings(t *testing.T) {
	items := mainTUIItems()
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}

	want := []string{"No-Restart Changes", "Restart Required", "Diagnostics", "Service Control", "Language / 语言", "Uninstall"}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("main menu labels = %#v, want %#v", labels, want)
	}
	for _, removed := range []string{"Quick Setup", "Subscriptions", "Nodes", "Settings", "System", "Help", "Quit"} {
		if indexOfLabel(labels, removed) >= 0 {
			t.Fatalf("main menu should not expose %q at top level: %#v", removed, labels)
		}
	}
}

func TestConfigAndSetupMenuPutsSetupAndSubscriptionsFirst(t *testing.T) {
	items := restartRequiredTUIItemsFor(languageEnglish)
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	wantPrefix := []string{"First Setup", "Subscriptions", "Custom Config"}
	if strings.Join(labels[:len(wantPrefix)], "\x00") != strings.Join(wantPrefix, "\x00") {
		t.Fatalf("config/setup menu labels = %#v, want prefix %#v", labels, wantPrefix)
	}
}

func TestChineseLanguageMainMenu(t *testing.T) {
	items := mainTUIItemsFor(languageChinese)
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	want := []string{"无需重启配置", "需重启配置", "诊断工具", "服务控制", "Language / 语言", "卸载"}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Chinese main menu labels = %#v, want %#v", labels, want)
	}
}

func TestDiagnosticsMenuGroupsNetworkTestAndFileLocations(t *testing.T) {
	items := diagnosticsTUIItemsFor(languageEnglish)
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	want := []string{"Network Test", "File Locations"}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("diagnostics labels = %#v, want %#v", labels, want)
	}
}

func TestPrintMainFileLocations(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SBOXKIT_ROOT", root)
	var out bytes.Buffer

	printMainFileLocations(&out)

	text := out.String()
	for _, want := range []string{
		"State root",
		filepath.Join(root, "state", "config.json"),
		filepath.Join(root, "state", "customize.json"),
		filepath.Join(root, "state", "subscriptions"),
		filepath.Join(root, "state", "logs"),
		"/etc/sboxkit",
		"/usr/lib/sboxkit/sing-box",
		"/etc/systemd/system/sboxkit.service",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("file locations missing %q:\n%s", want, text)
		}
	}
}

func TestRunAutoStartsFirstSetupWhenServiceMissing(t *testing.T) {
	t.Setenv("SBOXKIT_ROOT", t.TempDir())
	var out bytes.Buffer
	session := newTUISession(&out, &out)
	session.serviceExistsF = func() bool { return false }
	session.pauseF = func(string) {}
	session.confirmF = func(prompt string, fallback bool) (bool, error) {
		lower := strings.ToLower(prompt)
		switch {
		case strings.Contains(prompt, "TUN"):
			return true, nil
		case strings.Contains(lower, "import"), strings.Contains(prompt, "导入"):
			return false, nil
		case strings.Contains(lower, "service"):
			return false, nil
		default:
			return fallback, nil
		}
	}

	var titles []string
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		titles = append(titles, title)
		switch len(titles) {
		case 1:
			if title != "Language / 语言" {
				t.Fatalf("first setup should ask language first, got %q", title)
			}
			return 0, nil
		case 2:
			if title != "sboxkit" {
				t.Fatalf("expected main menu after setup, got %q", title)
			}
			return indexOfLabel(options, "Service Control"), ui.ErrCancelled
		default:
			t.Fatalf("unexpected select call %d title=%q", len(titles), title)
			return 0, ui.ErrCancelled
		}
	}

	if code := session.run(); code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if len(titles) != 2 {
		t.Fatalf("select titles = %#v, want language then main menu", titles)
	}
}

func TestRunCanSkipAutoFirstSetupAfterLanguageSelection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SBOXKIT_ROOT", root)
	var out bytes.Buffer
	session := newTUISession(&out, &out)
	session.serviceExistsF = func() bool { return false }
	session.pauseF = func(string) {}
	session.confirmF = func(prompt string, fallback bool) (bool, error) {
		if strings.Contains(prompt, "first setup") || strings.Contains(prompt, "初始化") {
			return false, nil
		}
		return fallback, nil
	}

	var titles []string
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		titles = append(titles, title)
		switch len(titles) {
		case 1:
			if title != "Language / 语言" {
				t.Fatalf("first prompt should select language, got %q", title)
			}
			return 1, nil
		case 2:
			if title != "sboxkit" {
				t.Fatalf("expected main menu after skipping setup, got %q", title)
			}
			return 0, ui.ErrCancelled
		default:
			t.Fatalf("unexpected select call %d title=%q", len(titles), title)
			return 0, ui.ErrCancelled
		}
	}

	if code := session.run(); code != 0 {
		t.Fatalf("run() = %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(root, "state", "customize.json")); !os.IsNotExist(err) {
		t.Fatalf("customize should not be written when setup is skipped, stat err=%v", err)
	}
	if reloaded := newTUISession(io.Discard, io.Discard); reloaded.language != languageChinese {
		t.Fatalf("language should persist even when setup is skipped, got %q", reloaded.language)
	}
}

func TestServiceIntegrationExistsForStoppedUnit(t *testing.T) {
	root := t.TempDir()
	unit := filepath.Join(root, "sboxkit.service")
	if err := os.WriteFile(unit, []byte("[Unit]\n"), 0o644); err != nil {
		t.Fatalf("write unit: %v", err)
	}

	if !serviceIntegrationExists([]string{unit}) {
		t.Fatal("stopped unit file should count as existing service integration")
	}
}

func TestLanguagePreferencePersists(t *testing.T) {
	t.Setenv("SBOXKIT_ROOT", t.TempDir())
	session := newTUISession(io.Discard, io.Discard)
	if session.language != languageEnglish {
		t.Fatalf("default language = %q, want English", session.language)
	}
	if err := session.setLanguage(languageChinese); err != nil {
		t.Fatalf("set language: %v", err)
	}
	reloaded := newTUISession(io.Discard, io.Discard)
	if reloaded.language != languageChinese {
		t.Fatalf("reloaded language = %q, want Chinese", reloaded.language)
	}
}

func TestFirstSetupUpdatesRulesThroughRunningProxy(t *testing.T) {
	want := []string{"--proxy", "http://127.0.0.1:7890", "--sync-service"}
	got := firstSetupPostStartUpdateArgs()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("first setup update args = %#v, want %#v", got, want)
	}
}

func TestFirstSetupKeepsStartedServiceWhenOptionalRuleUpdateFails(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SBOXKIT_ROOT", root)
	var out bytes.Buffer
	session := newTUISession(&out, &out)
	session.pauseF = func(string) {}
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		if title != "Language / 语言" {
			t.Fatalf("unexpected select title %q", title)
		}
		return 0, nil
	}
	confirms := []bool{
		true,  // run first setup
		true,  // enable TUN
		false, // skip import
		true,  // install and start service
		true,  // accept service traffic risk
	}
	session.confirmF = func(prompt string, fallback bool) (bool, error) {
		if len(confirms) == 0 {
			t.Fatalf("unexpected confirm prompt %q", prompt)
		}
		next := confirms[0]
		confirms = confirms[1:]
		return next, nil
	}
	var serviceCalls, updateCalls int
	session.runServiceF = func(args []string, stdout io.Writer, stderr io.Writer) int {
		serviceCalls++
		if strings.Join(args, "\x00") != "install" {
			t.Fatalf("service args = %#v, want install", args)
		}
		return 0
	}
	session.runUpdateF = func(args []string, stdout io.Writer, stderr io.Writer) int {
		updateCalls++
		return 23
	}

	if quit := runTUIFirstSetup(session); quit {
		t.Fatal("first setup should return to the menu")
	}
	if serviceCalls != 1 || updateCalls != 1 {
		t.Fatalf("serviceCalls=%d updateCalls=%d, want 1 each", serviceCalls, updateCalls)
	}
	if strings.Contains(out.String(), "命令以状态") {
		t.Fatalf("optional update failure should not fail first setup output:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "可选规则集下载失败") {
		t.Fatalf("expected optional update warning, got:\n%s", out.String())
	}
}

func TestConfigMenuGroupsToggleItems(t *testing.T) {
	items := configTUIItems()
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	want := []string{
		"Show Config",
		"Edit Custom Layer",
		"Shell Proxy Environment",
		"Advanced Key/Value",
	}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("config menu labels = %#v, want %#v", labels, want)
	}

	for _, label := range labels {
		for _, flatPrefix := range []string{"Enable", "Disable", "Write", "Remove", "Set"} {
			if strings.HasPrefix(label, flatPrefix) {
				t.Fatalf("config top-level item %q should be under a grouped submenu", label)
			}
		}
	}
}

func TestPromptRequiredCancelsOnInputCancel(t *testing.T) {
	var out bytes.Buffer
	session := newTUISession(&out, &out)
	session.askF = func(string, ui.AskOpts) (string, error) {
		return "", ui.ErrCancelled
	}
	session.confirmF = func(string, bool) (bool, error) {
		t.Fatal("promptRequired should not ask a second confirmation after input cancellation")
		return false, nil
	}

	if value, ok := session.promptRequired("Name"); ok || value != "" {
		t.Fatalf("promptRequired = %q, %v; want cancelled empty value", value, ok)
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
			if title != "Edit Custom Layer" {
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
			if title != "Edit Custom Layer" {
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
