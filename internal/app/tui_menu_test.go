package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func TestBuildAddSubscriptionArgsSupportsLocalFileSource(t *testing.T) {
	var out bytes.Buffer
	session := scriptedTUISession(&out, []string{
		"local-file",
		"local",
		"/tmp/config.yaml",
		"",
	}, []bool{true})

	args, ok := session.buildAddSubscriptionArgs()
	if !ok {
		t.Fatal("expected add subscription args")
	}
	wantArgs := []string{"add", "--name", "local", "--file", "/tmp/config.yaml"}
	if strings.Join(args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}
	if strings.Contains(out.String(), "是否将 sing-box 配置原样透传") {
		t.Fatalf("local file source should not ask for passthrough:\n%s", out.String())
	}
}

func TestSubscriptionMenuIntegratesLocalFileIntoAddSubscription(t *testing.T) {
	items := subscriptionTUIItemsFor(languageEnglish)
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	if labels[0] != "Switch Active Subscription" || labels[2] != "Add Subscription" {
		t.Fatalf("unexpected subscription menu labels: %#v", labels)
	}
	if indexOfLabel(labels, "Add Local Config") >= 0 || indexOfLabel(labels, "Add Local Subscription") >= 0 {
		t.Fatalf("local file source should be integrated into Add Subscription, got %#v", labels)
	}
	if indexOfLabel(labels, "Overwrite Current From Local File") < 0 {
		t.Fatalf("subscription menu should expose local file overwrite, got %#v", labels)
	}
}

func TestSwitchSubscriptionUsesListSelection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SBOXKIT_ROOT", root)
	p := paths.FromRoot(root)
	for _, sub := range []struct {
		name          string
		sourceType    string
		lastNodeCount int
	}{
		{name: "alpha", sourceType: "clash", lastNodeCount: 12},
		{name: "beta", sourceType: "sing-box", lastNodeCount: 8},
	} {
		dir := filepath.Join(p.SubscriptionsDir, sub.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir subscription dir: %v", err)
		}
		meta := map[string]any{
			"name":            sub.name,
			"source_type":     sub.sourceType,
			"last_node_count": sub.lastNodeCount,
		}
		data, err := json.Marshal(meta)
		if err != nil {
			t.Fatalf("marshal meta: %v", err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o600); err != nil {
			t.Fatalf("write meta: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{}\n"), 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
	if err := os.WriteFile(p.ActiveFile, []byte("beta\n"), 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}

	var out bytes.Buffer
	session := newTUISession(&out, &out)
	session.askF = func(string, ui.AskOpts) (string, error) {
		t.Fatal("switch subscription should not prompt for manual input")
		return "", ui.ErrCancelled
	}
	session.selectF = func(title string, options []string, opts ui.SelectOpts) (int, error) {
		if title != "Switch Active Subscription" {
			t.Fatalf("select title = %q, want switch title", title)
		}
		if opts.Initial < 0 || opts.Initial >= len(options) {
			t.Fatalf("initial selection out of range: %d", opts.Initial)
		}
		if !strings.Contains(options[opts.Initial], "当前") {
			t.Fatalf("initial option should point at active subscription, got %#v", options)
		}
		idx := indexOfLabelContaining(options, "alpha")
		if idx < 0 {
			t.Fatalf("missing alpha option in %#v", options)
		}
		return idx, nil
	}

	args, ok := session.buildSwitchSubscriptionArgs()
	if !ok {
		t.Fatal("expected switch subscription args")
	}
	want := []string{"switch", "--name", "alpha"}
	if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, want)
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
				return indexOfLabel(options, "Custom Layer Config"), nil
			case 2:
				return 0, ui.ErrCancelled
			default:
				t.Fatalf("unexpected main select call %d", len(calls))
				return 0, ui.ErrCancelled
			}
		case "Custom Layer Config":
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
	configIndex := indexOfLabel(mainLabels, "Custom Layer Config")
	if configIndex < 0 {
		t.Fatal("missing Custom Layer Config item in main menu")
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

	want := []string{"Runtime Settings", "Custom Layer Config", "Diagnostics", "Service Control", "Language / 语言", "Uninstall"}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("main menu labels = %#v, want %#v", labels, want)
	}
	for _, removed := range []string{"Quick Setup", "Subscriptions", "Nodes", "Settings", "System", "Help", "Quit"} {
		if indexOfLabel(labels, removed) >= 0 {
			t.Fatalf("main menu should not expose %q at top level: %#v", removed, labels)
		}
	}
}

func TestUpdateMenuIncludesSelfUpdateChannels(t *testing.T) {
	items := updateTUIItemsFor(languageEnglish)
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	for _, want := range []string{"Update sboxkit Stable", "Update sboxkit Preview"} {
		if indexOfLabel(labels, want) < 0 {
			t.Fatalf("update menu missing %q: %#v", want, labels)
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
	want := []string{"运行时配置", "定制层配置", "诊断工具", "服务控制", "Language / 语言", "卸载"}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("Chinese main menu labels = %#v, want %#v", labels, want)
	}
}

func TestServiceControlMenuIsFocusedOnStartStop(t *testing.T) {
	items := serviceTUIItems()
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	want := []string{"Start Service", "Stop Service", "Service Status"}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("service menu labels = %#v, want %#v", labels, want)
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
		filepath.Join(root, "cache", "downloads"),
		filepath.Join(root, "revisions"),
		filepath.Join(root, "current"),
		filepath.Join(root, "sing-box", "cache.db"),
		"/etc/sboxkit/config.json",
		"/usr/share/sboxkit/ui",
		"/usr/lib/sboxkit/sing-box",
		"/etc/systemd/system/sboxkit.service",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("file locations missing %q:\n%s", want, text)
		}
	}
}
