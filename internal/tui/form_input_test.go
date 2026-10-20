package tui

import (
	"bufio"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeRunes(m *formModel, text string) {
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
}

func TestFormTypingOnTextFieldStartsEditingAndReplacesValue(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{Key: "port", Label: "端口", Kind: FormText, Value: "7890"}}), width: 80}

	typeRunes(m, "8")
	if !m.editing || m.fields[0].Value != "8" {
		t.Fatalf("first keystroke should open the editor and replace the value: editing=%v value=%q", m.editing, m.fields[0].Value)
	}
	typeRunes(m, "0")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.editing || m.fields[0].Value != "80" {
		t.Fatalf("subsequent keystrokes should append and Enter should finish: editing=%v value=%q", m.editing, m.fields[0].Value)
	}
}

func TestFormTypedReplacementIsDiscardedByEsc(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{Key: "name", Kind: FormText, Value: "sub-20261020-170000"}}), width: 80}

	typeRunes(m, "my")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.editing || m.err != nil || m.fields[0].Value != "sub-20261020-170000" {
		t.Fatalf("Esc should restore the value from before typing started: %#v", m)
	}
}

func TestFormPastedTextReplacesValueInOneStep(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{Key: "url", Kind: FormText, Value: "old"}}), width: 80}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("https://example.com/sub"), Paste: true})

	if !m.editing || m.fields[0].Value != "https://example.com/sub" {
		t.Fatalf("paste should replace the value: editing=%v value=%q", m.editing, m.fields[0].Value)
	}
}

func TestFormBackspaceOnTextFieldEditsInPlace(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{Key: "port", Kind: FormText, Value: "7890"}}), width: 80}

	m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	typeRunes(m, "1")

	if !m.editing || m.fields[0].Value != "7891" {
		t.Fatalf("Backspace should open the editor without discarding the value: editing=%v value=%q", m.editing, m.fields[0].Value)
	}
}

func TestFormEnterStillEditsTextInPlace(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{Key: "port", Kind: FormText, Value: "7890"}}), width: 80}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	typeRunes(m, "1")

	if !m.editing || m.fields[0].Value != "78901" {
		t.Fatalf("Enter should keep the value and append typed text: editing=%v value=%q", m.editing, m.fields[0].Value)
	}
}

func TestFormTypingIgnoresNonTextAndDisabledFields(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{
		{Key: "flag", Kind: FormBool},
		{Key: "choice", Kind: FormChoice, Value: "A", Options: []string{"A", "B"}},
		{Key: "off", Kind: FormText, Value: "keep", Enabled: func(FormResult) bool { return false }},
	}), width: 80}

	for idx := range m.fields {
		m.idx = idx
		typeRunes(m, "x")
		m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}

	got := formValues(m.fields)
	if m.editing || got["flag"] != "false" || got["choice"] != "A" || got["off"] != "keep" {
		t.Fatalf("typing changed a non-text or disabled field: editing=%v values=%#v", m.editing, got)
	}
}

func TestFormSpaceOnTextFieldDoesNotStartEditing(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{Key: "port", Kind: FormText, Value: "7890"}}), width: 80}

	m.Update(tea.KeyMsg{Type: tea.KeySpace})

	if m.editing || m.fields[0].Value != "7890" {
		t.Fatalf("Space is the toggle key and must not blank a text field: editing=%v value=%q", m.editing, m.fields[0].Value)
	}
}

func TestFormSubmitRejectsEmptyRequiredEnabledField(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{
		{Key: "use", Label: "使用", Kind: FormBool, Value: "true"},
		{Key: "proxy", Label: "下载代理", Kind: FormText, Required: true, Enabled: func(v FormResult) bool { return v.Bool("use") }},
	}), idx: 2, width: 80}

	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("form with an empty required field should stay open")
	}
	if !strings.Contains(m.note, "下载代理") || !strings.Contains(m.note, "不能为空") {
		t.Fatalf("validation note = %q", m.note)
	}

	m.fields[0].Value = "false"
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("disabled required field should not block submit")
	}
}

func TestFormRequiredFieldMayBeLeftEmptyWhileEditing(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{
		{Key: "proxy", Kind: FormText, Required: true},
		{Key: "next", Kind: FormText},
	}), width: 80}

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	if m.editing || m.idx != 1 {
		t.Fatalf("required check belongs to submit, not to leaving the editor: %#v", m)
	}
}

func TestBuildFormAlignsValuesForCJKAndASCIILabels(t *testing.T) {
	fields := []FormField{
		{Key: "proxy", Label: "下载代理", Kind: FormText, Value: "x"},
		{Key: "tun", Label: "启用 TUN", Kind: FormBool},
		{Key: "shell", Label: "shell 代理", Kind: FormBool},
		{Key: "uid", Label: "TUN 直连 UID", Kind: FormText, Value: "y"},
	}
	rows := buildForm("初始化", fields, len(fields), false, FormOpts{SubmitLabel: "提交"}, "", 100)

	column := -1
	for i := range fields {
		row := stripAnsi(rows[i+1])
		start := strings.Index(row, "[")
		if start < 0 {
			t.Fatalf("row %d has no value: %q", i, row)
		}
		got := dispWidth(row[:start])
		if column >= 0 && got != column {
			t.Fatalf("row %d value starts at column %d, want %d:\n%s", i, got, column, strings.Join(rows, "\n"))
		}
		column = got
	}
}

func TestBuildFormWrapsLongNoteInsideBox(t *testing.T) {
	fields := []FormField{{Key: "proxy", Label: "下载代理地址", Kind: FormText, Value: "x"}}
	note := "下载代理地址: 下载代理需为 http(s):// 或 socks5(h):// 地址，例如 http://127.0.0.1:7890"
	rows := buildForm("初始化", fields, 0, true, FormOpts{SubmitLabel: "提交"}, note, 60)

	w := dispWidth(stripAnsi(rows[0]))
	for i, row := range rows {
		if got := dispWidth(stripAnsi(row)); got != w {
			t.Fatalf("row %d width %d, want %d: %q", i, got, w, row)
		}
	}
	joined := strings.ReplaceAll(stripAnsi(strings.Join(rows, "")), " ", "")
	if !strings.Contains(joined, "http://127.0.0.1:7890") || strings.Contains(joined, "…") {
		t.Fatalf("long note was truncated instead of wrapped:\n%s", strings.Join(rows, "\n"))
	}
}

func TestFormPlainReasksInvalidValueWithPreviousDefault(t *testing.T) {
	original := stdinReader
	stdinReader = bufio.NewReader(strings.NewReader("bad\n\n"))
	t.Cleanup(func() { stdinReader = original })
	fields := []FormField{{
		Key: "port", Label: "端口", Kind: FormText, Value: "7890",
		Validate: func(value string) error {
			if value == "bad" {
				return errors.New("invalid")
			}
			return nil
		},
	}}

	got, err := formPlain(fields)

	if err != nil || got["port"] != "7890" {
		t.Fatalf("rejected input must not become the next default: %#v, %v", got, err)
	}
}

func TestFormAltKeyDoesNotOverwriteTextField(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{Key: "proxy", Kind: FormText, Value: "http://127.0.0.1:7890"}}), width: 80}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b"), Alt: true})

	if m.editing || m.fields[0].Value != "http://127.0.0.1:7890" {
		t.Fatalf("Alt+key is a shortcut, not typing: editing=%v value=%q", m.editing, m.fields[0].Value)
	}
}
