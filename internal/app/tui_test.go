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

func TestFirstSetupCanUseExistingLocalSubscription(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SBOXKIT_ROOT", root)
	p := paths.FromRoot(root)
	subDir := filepath.Join(p.SubscriptionsDir, "local")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir subscription: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "meta.json"), []byte(`{
  "name": "local",
  "source_type": "clash",
  "converter": "local",
  "last_node_count": 2
}
`), 0o600); err != nil {
		t.Fatalf("write meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "config.json"), []byte(`{"inbounds":[],"outbounds":[]}`+"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out bytes.Buffer
	session := newTUISession(&out, &out)
	session.pauseF = func(string) {}
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		switch title {
		case "Language / 语言":
			return 0, nil
		case "Existing Subscriptions":
			if len(options) != 1 || !strings.Contains(options[0], "local") {
				t.Fatalf("existing subscription options = %#v", options)
			}
			return 0, nil
		default:
			t.Fatalf("unexpected select title %q", title)
			return 0, ui.ErrCancelled
		}
	}
	confirms := []bool{
		true,  // run first setup
		true,  // enable TUN
		true,  // use existing subscription
		false, // do not install service
	}
	session.confirmF = func(prompt string, fallback bool) (bool, error) {
		if strings.Contains(prompt, "Import a subscription") || strings.Contains(prompt, "导入订阅") {
			t.Fatalf("first setup should not ask for new subscription after using existing one: %q", prompt)
		}
		if len(confirms) == 0 {
			t.Fatalf("unexpected confirm prompt %q", prompt)
		}
		next := confirms[0]
		confirms = confirms[1:]
		return next, nil
	}

	if quit := runTUIFirstSetup(session); quit {
		t.Fatal("first setup should return to the menu")
	}
	active, err := os.ReadFile(p.ActiveFile)
	if err != nil {
		t.Fatalf("read active: %v", err)
	}
	if string(active) != "local\n" {
		t.Fatalf("active = %q, want local", active)
	}
	if _, err := os.Stat(p.ConfigFile); err != nil {
		t.Fatalf("generated config was not written: %v", err)
	}
	if !strings.Contains(out.String(), "Active subscription: local") {
		t.Fatalf("expected active subscription message, got:\n%s", out.String())
	}
}

func TestRestartConfirmationIncludesSSHRiskInSinglePrompt(t *testing.T) {
	var out bytes.Buffer
	session := newTUISession(&out, &out)
	calls := 0
	session.confirmF = func(prompt string, fallback bool) (bool, error) {
		calls++
		if !strings.Contains(prompt, "SSH") {
			t.Fatalf("restart prompt should include SSH warning, got %q", prompt)
		}
		return true, nil
	}

	if !session.confirmServiceRestart("Restart sboxkit.service?", false) {
		t.Fatal("expected restart confirmation to pass")
	}
	if calls != 1 {
		t.Fatalf("confirm calls = %d, want 1", calls)
	}
}

func TestCommandActionAutoReturnsUnlessExplicitlyPaused(t *testing.T) {
	var out bytes.Buffer
	session := newTUISession(&out, &out)
	pauses := 0
	session.pauseF = func(string) { pauses++ }

	commandAction("Quick", func(*tuiSession) int { return 0 })(session)
	if pauses != 0 {
		t.Fatalf("commandAction pauses = %d, want 0", pauses)
	}

	commandActionPaused("Diagnostics", func(*tuiSession) int { return 0 })(session)
	if pauses != 1 {
		t.Fatalf("commandActionPaused pauses = %d, want 1", pauses)
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

func TestConfigEditorCanToggleFileLogging(t *testing.T) {
	cfg := config.Defaults()
	if cfg.EnableFileLog {
		t.Fatal("file log should default to disabled")
	}
	toggleBoolField(&cfg, "enable_file_log")
	if !cfg.EnableFileLog {
		t.Fatal("enable_file_log should toggle on")
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
