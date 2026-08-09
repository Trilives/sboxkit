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
	Key         string
	Label       string
	Kind        FormKind
	Value       string
	Placeholder string
	Options     []string
	Enabled     func(FormResult) bool
	Validate    func(string) error
}

type FormOpts struct {
	SubmitLabel string
	Hint        string
}

// Form renders a compact single-page editor. Text fields are edited directly;
// booleans use Space and choices use Left/Right. Enter validates and submits.
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
		if fieldEnabled(field, values) && field.Validate != nil {
			if err := field.Validate(field.Value); err != nil {
				return fmt.Errorf("%s: %w", field.Label, err)
			}
		}
	}
	return nil
}

type formModel struct {
	title  string
	fields []FormField
	opts   FormOpts
	idx    int
	width  int
	err    error
	note   string
}

func (m *formModel) Init() tea.Cmd { return nil }

func (m *formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if len(m.fields) == 0 {
			m.err = errs.ErrCancelled
			return m, tea.Quit
		}
		field := &m.fields[m.idx]
		enabled := fieldEnabled(*field, formValues(m.fields))
		switch msg.String() {
		case "up":
			m.idx = (m.idx - 1 + len(m.fields)) % len(m.fields)
		case "down", "tab":
			m.idx = (m.idx + 1) % len(m.fields)
		case "shift+tab":
			m.idx = (m.idx - 1 + len(m.fields)) % len(m.fields)
		case "esc", "ctrl+c":
			m.err = errs.ErrCancelled
			return m, tea.Quit
		case "enter":
			if err := validateForm(m.fields); err != nil {
				m.note = err.Error()
				return m, nil
			}
			return m, tea.Quit
		case " ":
			if enabled && field.Kind == FormBool {
				field.Value = fmt.Sprint(field.Value != "true")
			} else if enabled && field.Kind == FormText {
				field.Value += " "
			}
		case "left":
			if enabled && field.Kind == FormChoice {
				rotateChoice(field, -1)
			}
		case "right":
			if enabled && field.Kind == FormChoice {
				rotateChoice(field, 1)
			}
		case "backspace", "ctrl+h":
			if enabled && field.Kind == FormText && field.Value != "" {
				_, size := utf8.DecodeLastRuneInString(field.Value)
				field.Value = field.Value[:len(field.Value)-size]
			}
		case "ctrl+u":
			if enabled && field.Kind == FormText {
				field.Value = ""
			}
		default:
			if enabled && field.Kind == FormText && msg.Type == tea.KeyRunes {
				field.Value += string(msg.Runes)
			}
		}
		m.note = ""
	}
	return m, nil
}

func (m *formModel) View() string {
	return strings.Join(buildForm(m.title, m.fields, m.idx, m.opts, m.note, m.width), "\n") + "\n"
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

func buildForm(title string, fields []FormField, idx int, opts FormOpts, note string, termCols int) []string {
	maxW := maxBoxWidth(termCols)
	label := truncate(fmt.Sprintf("─ %s ", title), maxW)
	values := formValues(fields)
	texts := make([]string, len(fields))
	widths := []int{dispWidth(label)}
	for i, field := range fields {
		value := formDisplayValue(field)
		prefix := "  "
		if i == idx {
			prefix = "❯ "
		}
		text := fmt.Sprintf("  %s%-18s %s ", prefix, field.Label, value)
		if !fieldEnabled(field, values) {
			text += " (—)"
		}
		texts[i] = truncate(text, maxW)
		widths = append(widths, dispWidth(texts[i]))
	}
	button := "  [" + opts.SubmitLabel + "]"
	footer := "  " + i18n.T("↑/↓ 切换   Space 勾选   ←/→ 选择   Enter 提交   Esc 取消")
	widths = append(widths, dispWidth(button), dispWidth(footer), dispWidth(opts.Hint), dispWidth(note))
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
	rows = append(rows, "│"+rowPad("", w)+"│", "│"+rowPad(button, w)+"│")
	if opts.Hint != "" {
		rows = append(rows, "│"+dim(rowPad(truncate("  "+opts.Hint, w), w))+"│")
	}
	if note != "" {
		rows = append(rows, "│"+rowPad(truncate("  "+note, w), w)+"│")
	}
	rows = append(rows, "│"+dim(rowPad(truncate(footer, w), w))+"│", "└"+strings.Repeat("─", w)+"┘")
	return rows
}

func formDisplayValue(field FormField) string {
	switch field.Kind {
	case FormBool:
		if field.Value == "true" {
			return "[✓]"
		}
		return "[ ]"
	case FormChoice:
		return "< " + field.Value + " >"
	default:
		if field.Value == "" && field.Placeholder != "" {
			return "[ <" + field.Placeholder + "> ]"
		}
		return "[ " + field.Value + " ]"
	}
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}
