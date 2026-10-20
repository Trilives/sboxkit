package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/i18n"
)

type FormKind uint8

const (
	FormText FormKind = iota
	FormBool
	FormChoice
)

type FormResult map[string]string

func (r FormResult) Bool(key string) bool { return r[key] == "true" }

type FormField struct {
	Key          string
	Label        string
	Kind         FormKind
	Value        string
	Placeholder  string
	Options      []string
	OptionLabels map[string]string
	Enabled      func(FormResult) bool
	Validate     func(string) error
	// Required rejects an empty value on submit while the field is enabled.
	// Leaving the editor stays possible so the user can fill it in later.
	Required bool
}

type FormOpts struct {
	SubmitLabel string
	Hint        string
}

// Form renders a compact single-page editor. Typing on a text field starts
// editing right away (replacing the value; Enter or Backspace edit it in
// place); booleans use Space and choices use Left/Right. The final row is the
// only row that submits the form.
func Form(title string, fields []FormField, opts FormOpts) (FormResult, error) {
	if opts.SubmitLabel == "" {
		opts.SubmitLabel = i18n.T("提交")
	}
	if !UseTUI() {
		return formPlain(fields)
	}
	m := &formModel{title: title, fields: cloneFormFields(fields), opts: opts, width: 80}
	out, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, err
	}
	fm := out.(*formModel)
	if fm.err != nil {
		return nil, fm.err
	}
	return formValues(fm.fields), nil
}

func cloneFormFields(fields []FormField) []FormField {
	out := make([]FormField, len(fields))
	copy(out, fields)
	for i := range out {
		out[i].Options = append([]string(nil), fields[i].Options...)
		if fields[i].OptionLabels != nil {
			out[i].OptionLabels = make(map[string]string, len(fields[i].OptionLabels))
			for value, label := range fields[i].OptionLabels {
				out[i].OptionLabels[value] = label
			}
		}
		if out[i].Kind == FormBool && out[i].Value == "" {
			out[i].Value = "false"
		}
		if out[i].Kind == FormChoice && !containsString(out[i].Options, out[i].Value) && len(out[i].Options) > 0 {
			out[i].Value = out[i].Options[0]
		}
	}
	return out
}

func formValues(fields []FormField) FormResult {
	values := make(FormResult, len(fields))
	for _, field := range fields {
		values[field.Key] = field.Value
	}
	return values
}

func fieldEnabled(field FormField, values FormResult) bool {
	return field.Enabled == nil || field.Enabled(values)
}

func validateForm(fields []FormField) error {
	values := formValues(fields)
	for _, field := range fields {
		if !fieldEnabled(field, values) {
			continue
		}
		if field.Required && strings.TrimSpace(field.Value) == "" {
			return fmt.Errorf("%s: %s", field.Label, i18n.T("不能为空。"))
		}
		if field.Validate != nil {
			if err := field.Validate(field.Value); err != nil {
				return fmt.Errorf("%s: %w", field.Label, err)
			}
		}
	}
	return nil
}

type formModel struct {
	title        string
	fields       []FormField
	opts         FormOpts
	idx          int
	width        int
	err          error
	note         string
	editing      bool
	editOriginal string
}

func (m *formModel) Init() tea.Cmd { return nil }

func (m *formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if m.editing || m.beginTypedEdit(msg) {
			return m.updateText(msg)
		}
		itemCount := len(m.fields) + 1
		switch msg.String() {
		case "up":
			m.idx = (m.idx - 1 + itemCount) % itemCount
		case "down", "tab":
			m.idx = (m.idx + 1) % itemCount
		case "shift+tab":
			m.idx = (m.idx - 1 + itemCount) % itemCount
		case "esc", "ctrl+c":
			m.err = errs.ErrCancelled
			return m, tea.Quit
		case "enter":
			if m.idx < len(m.fields) {
				if m.focusedTextField() != nil {
					m.openTextEdit()
				}
				break
			}
			if err := validateForm(m.fields); err != nil {
				m.note = err.Error()
				return m, nil
			}
			return m, tea.Quit
		case " ":
			if m.idx >= len(m.fields) {
				break
			}
			field := &m.fields[m.idx]
			enabled := fieldEnabled(*field, formValues(m.fields))
			if enabled && field.Kind == FormBool {
				field.Value = fmt.Sprint(field.Value != "true")
			}
		case "left":
			if m.idx >= len(m.fields) {
				break
			}
			field := &m.fields[m.idx]
			enabled := fieldEnabled(*field, formValues(m.fields))
			if enabled && field.Kind == FormChoice {
				rotateChoice(field, -1)
			}
		case "right":
			if m.idx >= len(m.fields) {
				break
			}
			field := &m.fields[m.idx]
			enabled := fieldEnabled(*field, formValues(m.fields))
			if enabled && field.Kind == FormChoice {
				rotateChoice(field, 1)
			}
		}
		m.note = ""
	}
	return m, nil
}

func (m *formModel) updateText(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	field := &m.fields[m.idx]
	switch msg.String() {
	case "enter":
		if !m.finishTextEdit() {
			return m, nil
		}
	case "up":
		if !m.finishTextEdit() {
			return m, nil
		}
		m.idx = (m.idx - 1 + len(m.fields) + 1) % (len(m.fields) + 1)
	case "down":
		if !m.finishTextEdit() {
			return m, nil
		}
		m.idx = (m.idx + 1) % (len(m.fields) + 1)
	case "esc":
		field.Value = m.editOriginal
		m.editing = false
	case "ctrl+c":
		m.err = errs.ErrCancelled
		return m, tea.Quit
	case "backspace", "ctrl+h":
		if field.Value != "" {
			_, size := utf8.DecodeLastRuneInString(field.Value)
			field.Value = field.Value[:len(field.Value)-size]
		}
	case "ctrl+u":
		field.Value = ""
	case " ":
		field.Value += " "
	default:
		if msg.Type == tea.KeyRunes {
			field.Value += string(msg.Runes)
		}
	}
	if !m.editing || msg.String() != "enter" {
		m.note = ""
	}
	return m, nil
}

// focusedTextField returns the selected field when it is an enabled text field.
func (m *formModel) focusedTextField() *FormField {
	if m.idx >= len(m.fields) {
		return nil
	}
	field := &m.fields[m.idx]
	if field.Kind != FormText || !fieldEnabled(*field, formValues(m.fields)) {
		return nil
	}
	return field
}

func (m *formModel) openTextEdit() {
	m.editing = true
	m.editOriginal = m.fields[m.idx].Value
}

// beginTypedEdit opens the focused text field without a separate Enter. A
// printable keystroke or paste replaces the value, as when tabbing into a
// field selects its text; deletion keys edit the existing value. The caller
// then feeds the same key to updateText, and Esc restores the original.
func (m *formModel) beginTypedEdit(msg tea.KeyMsg) bool {
	field := m.focusedTextField()
	if field == nil {
		return false
	}
	switch msg.String() {
	case "backspace", "ctrl+h", "ctrl+u":
		m.openTextEdit()
		return true
	}
	if msg.Type != tea.KeyRunes || msg.Alt { // Alt+key is a terminal shortcut, not text
		return false
	}
	m.openTextEdit()
	field.Value = ""
	return true
}

func (m *formModel) finishTextEdit() bool {
	field := &m.fields[m.idx]
	if field.Validate != nil {
		if err := field.Validate(field.Value); err != nil {
			m.note = fmt.Sprintf("%s: %v", field.Label, err)
			return false
		}
	}
	m.editing = false
	return true
}

func (m *formModel) View() string {
	return strings.Join(buildForm(m.title, m.fields, m.idx, m.editing, m.opts, m.note, m.width), "\n") + "\n"
}

func rotateChoice(field *FormField, delta int) {
	if len(field.Options) == 0 {
		return
	}
	idx := 0
	for i, option := range field.Options {
		if option == field.Value {
			idx = i
			break
		}
	}
	field.Value = field.Options[(idx+delta+len(field.Options))%len(field.Options)]
}

func buildForm(title string, fields []FormField, idx int, editing bool, opts FormOpts, note string, termCols int) []string {
	maxW := maxBoxWidth(termCols)
	label := truncate(fmt.Sprintf("─ %s ", title), maxW)
	values := formValues(fields)
	labelW := formLabelWidth(fields)
	texts := make([]string, len(fields))
	widths := []int{dispWidth(label)}
	for i, field := range fields {
		value := formDisplayValueMode(field, editing && i == idx)
		prefix := "  "
		if i == idx {
			prefix = "❯ "
		}
		text := fmt.Sprintf("  %s%s %s ", prefix, rowPad(field.Label, labelW), value)
		if !fieldEnabled(field, values) {
			text += " (—)"
		}
		texts[i] = truncate(text, maxW)
		widths = append(widths, dispWidth(texts[i]))
	}
	button := "    [" + opts.SubmitLabel + "]"
	if idx == len(fields) {
		button = "  ❯ [" + opts.SubmitLabel + "]"
	}
	footer := "  " + i18n.T("↑/↓ 切换  输入 改写  Space 勾选  ←/→ 选择  Enter 编辑/执行  Esc 取消")
	if editing {
		footer = "  " + i18n.T("输入文字   Backspace 删除   Enter 完成   Esc 放弃修改")
	}
	// The note (usually a validation error) wraps inside the box instead of
	// widening it, so long messages stay readable on narrow terminals.
	widths = append(widths, dispWidth(button), dispWidth(footer), dispWidth(opts.Hint))
	w := min(maxOf(widths)+2, maxW)
	rows := []string{"┌" + label + strings.Repeat("─", max(0, w-dispWidth(label))) + "┐"}
	for i, row := range texts {
		if i == idx && useColor {
			row = ansiCyan + ansiBold + rowPad(row, w) + ansiReset
		} else if !fieldEnabled(fields[i], values) {
			row = dim(rowPad(row, w))
		} else {
			row = rowPad(row, w)
		}
		rows = append(rows, "│"+row+"│")
	}
	rows = append(rows, "│"+rowPad("", w)+"│")
	if idx == len(fields) && useColor {
		rows = append(rows, "│"+ansiCyan+ansiBold+rowPad(button, w)+ansiReset+"│")
	} else {
		rows = append(rows, "│"+rowPad(button, w)+"│")
	}
	if opts.Hint != "" {
		rows = append(rows, "│"+dim(rowPad(truncate("  "+opts.Hint, w), w))+"│")
	}
	if note != "" {
		for _, line := range wrapText(note, w-2) {
			rows = append(rows, "│"+rowPad(truncate("  "+line, w), w)+"│")
		}
	}
	rows = append(rows, "│"+dim(rowPad(truncate(footer, w), w))+"│", "└"+strings.Repeat("─", w)+"┘")
	return rows
}

// formLabelMaxWidth keeps one long (usually translated) label from pushing
// every value off narrow terminals; longer labels simply overflow their column.
const formLabelMaxWidth = 30

// formLabelWidth aligns values by display width: CJK labels take two columns
// per rune, so byte-based %-Ns padding would leave them misaligned.
func formLabelWidth(fields []FormField) int {
	width := 0
	for _, field := range fields {
		width = max(width, dispWidth(field.Label))
	}
	return min(width, formLabelMaxWidth)
}

func formDisplayValue(field FormField) string {
	return formDisplayValueMode(field, false)
}

func formDisplayValueMode(field FormField, editing bool) string {
	switch field.Kind {
	case FormBool:
		if field.Value == "true" {
			return "[✓]"
		}
		return "[ ]"
	case FormChoice:
		return "< " + choiceLabel(field, field.Value) + " >"
	default:
		if field.Value == "" && field.Placeholder != "" && !editing {
			return "[ <" + field.Placeholder + "> ]"
		}
		value := field.Value
		if !editing {
			value = maskLongFormValue(value)
		}
		return "[ " + value + " ]"
	}
}

const (
	formValueMaskThreshold = 32
	formValuePrefixRunes   = 12
	formValueSuffixRunes   = 8
)

// maskLongFormValue keeps a long value recognizable without leaving tokens,
// subscription URLs, or paths fully visible after the user leaves the field.
func maskLongFormValue(value string) string {
	runes := []rune(value)
	if len(runes) <= formValueMaskThreshold {
		return value
	}
	return string(runes[:formValuePrefixRunes]) + "******" + string(runes[len(runes)-formValueSuffixRunes:])
}

func choiceLabel(field FormField, value string) string {
	if label := field.OptionLabels[value]; label != "" {
		return label
	}
	return value
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
