package app

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestClampOffsetKeepsSelectionVisible(t *testing.T) {
	tests := []struct {
		name     string
		selected int
		offset   int
		visible  int
		total    int
		want     int
	}{
		{name: "above window", selected: 1, offset: 3, visible: 5, total: 20, want: 1},
		{name: "below window", selected: 9, offset: 3, visible: 5, total: 20, want: 5},
		{name: "clamps to max", selected: 19, offset: 30, visible: 5, total: 20, want: 15},
		{name: "short list", selected: 2, offset: 2, visible: 10, total: 3, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampOffset(tt.selected, tt.offset, tt.visible, tt.total)
			if got != tt.want {
				t.Fatalf("clampOffset() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTruncateKeepsStableWidth(t *testing.T) {
	got := truncate("abcdefghijklmnopqrstuvwxyz", 10)
	if got != "abcdefg..." {
		t.Fatalf("truncate() = %q", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("short truncate() = %q", got)
	}
}

func TestRenderMenuUsesCarriageReturnNewlines(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "tui-*")
	if err != nil {
		t.Fatalf("create temp tty: %v", err)
	}
	defer file.Close()

	session := &tuiSession{tty: file, status: "ready"}
	session.renderMenu("sboxkit", "Terminal UI", []tuiItem{
		{Label: "First setup wizard", Detail: "Initialize state"},
		{Label: "Quit", Detail: "Exit"},
	}, 0, 0, 2, 80)

	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("seek rendered output: %v", err)
	}
	rendered, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("read rendered output: %v", err)
	}
	text := string(rendered)
	if strings.Contains(text, "\n") && !strings.Contains(text, "\r\n") {
		t.Fatalf("rendered menu uses bare LF newlines: %q", text)
	}
	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		t.Fatalf("rendered menu contains a bare LF after CRLF normalization: %q", text)
	}
}

func TestRawModeArgsBlockForInput(t *testing.T) {
	got := strings.Join(rawModeArgs(), " ")
	if strings.Contains(got, "min 0") || strings.Contains(got, "time 1") {
		t.Fatalf("raw mode must block for input, got args %q", got)
	}
	if !strings.Contains(got, "min 1") {
		t.Fatalf("raw mode should request one byte before read returns, got args %q", got)
	}
}

func TestRemoteSubscriptionPromptOrderStartsWithSource(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "tui-prompts-*")
	if err != nil {
		t.Fatalf("create temp tty: %v", err)
	}
	defer file.Close()

	session := &tuiSession{
		tty:    file,
		reader: bufio.NewReader(strings.NewReader("sing-box\nwork\nhttps://example.test/config.json\n\n\n")),
	}

	args, ok := session.buildRemoteSubscriptionArgs()
	if !ok {
		t.Fatal("expected remote subscription args")
	}
	wantArgs := []string{"add", "--name", "work", "--source", "sing-box", "--url", "https://example.test/config.json"}
	if strings.Join(args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("args = %#v, want %#v", args, wantArgs)
	}

	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("seek prompts: %v", err)
	}
	rendered, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("read prompts: %v", err)
	}
	text := string(rendered)
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

func indexOfLabel(labels []string, label string) int {
	for i, value := range labels {
		if value == label {
			return i
		}
	}
	return -1
}
