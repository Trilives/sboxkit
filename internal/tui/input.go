package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type AskOpts struct {
	Default        string
	DisplayDefault string
	AllowEmpty     bool
}

func Ask(prompt string, opts AskOpts) (string, error) {
	shown := opts.Default
	if opts.DisplayDefault != "" {
		shown = opts.DisplayDefault
	}
	suffix := ""
	if opts.Default != "" {
		suffix = fmt.Sprintf(" [%s]", shown)
	}
	for {
		raw, err := readInput(fmt.Sprintf("%s%s：", prompt, suffix))
		if err != nil {
			return "", err
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			if opts.Default != "" {
				return opts.Default, nil
			}
			if opts.AllowEmpty {
				return "", nil
			}
			fmt.Println("内容不能为空。")
			continue
		}
		return raw, nil
	}
}

func Confirm(prompt string, def bool) (bool, error) {
	suffix := " [是/否]"
	if def {
		suffix = " [是/否，默认是]"
	} else {
		suffix = " [是/否，默认否]"
	}
	raw, err := readInput(prompt + suffix + "：")
	if err != nil {
		return false, err
	}
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return def, nil
	}
	switch raw {
	case "y", "yes", "true", "是", "对", "好", "1", "t", "ok":
		return true, nil
	case "n", "no", "false", "否", "不", "错", "0", "f", "off":
		return false, nil
	default:
		return def, nil
	}
}

func Pause(prompt string) {
	_, _ = readInput(prompt)
}

func readInput(prompt string) (string, error) {
	if !UseTUI() {
		return readPlainLine(prompt)
	}
	ti := textinput.New()
	ti.Prompt = ansiCyan + "❯ " + ansiReset + prompt
	ti.Focus()
	m := &inputModel{ti: ti}
	out, err := tea.NewProgram(m).Run()
	if err != nil {
		return "", err
	}
	fm := out.(*inputModel)
	if fm.err != nil {
		return "", fm.err
	}
	return fm.ti.Value(), nil
}

type inputModel struct {
	ti  textinput.Model
	err error
}

func (m *inputModel) Init() tea.Cmd { return textinput.Blink }

func (m *inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "enter":
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.err = ErrCancelled
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m *inputModel) View() string {
	return m.ti.View() + "\n"
}
