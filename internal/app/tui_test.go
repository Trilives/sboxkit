package app

import (
	"bytes"
	"strings"
	"testing"

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
	sourceIndex := strings.Index(text, "Source")
	nameIndex := strings.Index(text, "Name")
	urlIndex := strings.Index(text, "URL")
	proxyIndex := strings.Index(text, "Download proxy (empty by default)")
	if sourceIndex < 0 || nameIndex < 0 || urlIndex < 0 || proxyIndex < 0 {
		t.Fatalf("missing expected prompts in %q", text)
	}
	if !(sourceIndex < nameIndex && nameIndex < urlIndex && urlIndex < proxyIndex) {
		t.Fatalf("prompt order = %q, want source before name before URL before proxy", text)
	}
}

func TestMainMenuPutsNodesNearTop(t *testing.T) {
	items := mainTUIItems()
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}

	nodesIndex := indexOfLabel(labels, "Nodes")
	subscriptionsIndex := indexOfLabel(labels, "Subscriptions")
	serviceIndex := indexOfLabel(labels, "Service")
	if nodesIndex < 0 || subscriptionsIndex < 0 || serviceIndex < 0 {
		t.Fatalf("missing expected main menu items: %#v", labels)
	}
	if !(nodesIndex < subscriptionsIndex && nodesIndex < serviceIndex) {
		t.Fatalf("Nodes should be before Subscriptions and Service, got order %#v", labels)
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
		"Show config",
		"TUN and routing",
		"WebUI and LAN",
		"Shell proxy environment",
		"Advanced key/value",
	}
	if strings.Join(labels, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("config menu labels = %#v, want %#v", labels, want)
	}

	for _, label := range labels {
		for _, flatPrefix := range []string{"Enable ", "Disable ", "Write ", "Remove ", "Set config key"} {
			if strings.HasPrefix(label, flatPrefix) {
				t.Fatalf("config top-level item %q should be under a grouped submenu", label)
			}
		}
	}
}

func TestWebUILANMenuIncludesLANProxyToggle(t *testing.T) {
	items := webUIConfigTUIItems()
	var labels []string
	for _, item := range items {
		labels = append(labels, item.Label)
	}
	for _, want := range []string{"Enable LAN proxy", "Disable LAN proxy"} {
		if indexOfLabel(labels, want) < 0 {
			t.Fatalf("WebUI and LAN menu missing %q: %#v", want, labels)
		}
	}
}

func TestNetworkTestActionPrintsProgressPrompt(t *testing.T) {
	var out bytes.Buffer

	printNetworkTestProgress(&out)

	if !strings.Contains(out.String(), "Testing network through 127.0.0.1:7890") {
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
