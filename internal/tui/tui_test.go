package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTruncateCJK(t *testing.T) {
	s := "生成新加坡自动测速聚合组"
	got := truncate(s, 10)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("wide text should end with ellipsis: %q", got)
	}
	if dispWidth(got) > 10 {
		t.Fatalf("truncated width %d exceeds limit 10", dispWidth(got))
	}
	if truncate("short", 10) != "short" {
		t.Fatal("short text should not be truncated")
	}
}

func TestScrollTop(t *testing.T) {
	if scrollTop(5, 2, 10) != 0 {
		t.Fatal("short list should not scroll")
	}
	if got := scrollTop(100, 0, 10); got != 0 {
		t.Fatalf("top should not underflow: %d", got)
	}
	if got := scrollTop(100, 99, 10); got != 90 {
		t.Fatalf("bottom should not overflow: %d", got)
	}
	if got := scrollTop(100, 50, 10); got != 45 {
		t.Fatalf("selection should be centered where possible: %d", got)
	}
}

func TestBuildSelectBoxAligned(t *testing.T) {
	rows := buildSelect("Test menu", []string{"Option one", "A longer option", "Third"}, 1, "footer", 80, 24)
	if len(rows) < 6 {
		t.Fatalf("unexpected row count: %d", len(rows))
	}
	w := dispWidth(stripAnsi(rows[0]))
	for i, r := range rows {
		if got := dispWidth(stripAnsi(r)); got != w {
			t.Errorf("row %d width %d != first row %d: %q", i, got, w, stripAnsi(r))
		}
	}
	joined := stripAnsi(strings.Join(rows, "\n"))
	if !strings.Contains(joined, "❯ ② A longer option") {
		t.Error("selected row should include pointer and circled number")
	}
	if !strings.HasPrefix(rows[0], "┌─ Test menu ") || !strings.HasPrefix(rows[len(rows)-1], "└") {
		t.Error("box border structure mismatch")
	}
}

func TestBuildSelectScrollHints(t *testing.T) {
	opts := make([]string, 40)
	for i := range opts {
		opts[i] = "Node " + strings.Repeat("x", i%5)
	}
	rows := buildSelect("Select node", opts, 20, "footer", 80, 24)
	joined := stripAnsi(strings.Join(rows, "\n"))
	if !strings.Contains(joined, "more above") || !strings.Contains(joined, "more below") {
		t.Error("scrolling window should show up and down hints")
	}
}

func TestConfirmPromptUsesYN(t *testing.T) {
	if got := confirmSuffix(true); got != " [Y/n]" {
		t.Fatalf("confirmSuffix(true) = %q, want [Y/n]", got)
	}
	if got := confirmSuffix(false); got != " [y/N]" {
		t.Fatalf("confirmSuffix(false) = %q, want [y/N]", got)
	}
}

func TestInputModelWrapsLongPrompt(t *testing.T) {
	model := &inputModel{
		prompt: "This is a very long prompt that should wrap instead of pushing the input field outside of the terminal width",
		width:  32,
	}
	rows := strings.Split(strings.TrimSuffix(stripAnsi(model.View()), "\n"), "\n")
	if len(rows) < 3 {
		t.Fatalf("expected wrapped prompt plus input row, got %#v", rows)
	}
	for _, row := range rows[:len(rows)-1] {
		if got := dispWidth(row); got > 32 {
			t.Fatalf("wrapped prompt row width %d exceeds terminal width: %q", got, row)
		}
	}
}

func TestSelectEscCancelsAndCtrlRSaves(t *testing.T) {
	model := &selectModel{options: []string{"A"}, idx: 0}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := updated.(*selectModel).err; got != ErrCancelled {
		t.Fatalf("Esc error = %v, want ErrCancelled", got)
	}

	model = &selectModel{options: []string{"A"}, idx: 0}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if got := updated.(*selectModel).err; got != ErrSaveExit {
		t.Fatalf("Ctrl+R error = %v, want ErrSaveExit", got)
	}
}

func TestNum(t *testing.T) {
	if num(0) != "①" || num(19) != "⑳" {
		t.Error("circled number mapping mismatch")
	}
	if num(20) != "21" {
		t.Error("numbers above circled range should fall back to digits")
	}
}
