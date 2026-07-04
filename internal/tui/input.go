package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

// AskOpts Ask 的选项。DisplayDefault 仅用于替换提示行里展示的默认值（如脱敏），
// 留空回车仍取 Default 本身。
type AskOpts struct {
	Default        string
	DisplayDefault string
	AllowEmpty     bool
}

// Ask 读取一行输入；esc / ^C → ErrCancelled。
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
		raw, err := readInput(fmt.Sprintf("%s%s: ", prompt, suffix))
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
			execx.Warn(i18n.T("不能为空。"))
			continue
		}
		return raw, nil
	}
}

// Confirm y/N 确认；esc / ^C → ErrCancelled。
func Confirm(prompt string, def bool) (bool, error) {
	suffix := " [y/N]"
	if def {
		suffix = " [Y/n]"
	}
	raw, err := readInput(prompt + suffix + ": ")
	if err != nil {
		return false, err
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return def, nil
	}
	return raw == "y" || raw == "yes" || raw == "是", nil
}

// Pause 等待回车（对应 Python keys.read_line 的「回车返回」用法）；取消亦返回。
func Pause(prompt string) {
	readInput(prompt) //nolint:errcheck // 取消与回车等价
}

func readInput(prompt string) (string, error) {
	if !UseTUI() {
		return readPlainLine(prompt)
	}
	// 长提示语先按终端宽度自动换行打印，只把最后一行接在输入光标前——
	// bubbles/textinput 的 Prompt 是单行组件，直接塞入整段长文本会把行撑
	// 破或被硬截断，观感很差。
	last := prompt
	if lines := wrapText(prompt, termWidth()-2); len(lines) > 1 {
		fmt.Println(strings.Join(lines[:len(lines)-1], "\n"))
		last = lines[len(lines)-1]
	}
	ti := textinput.New()
	ti.Prompt = ansiCyan + "❯ " + ansiReset + last
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
			m.err = errs.ErrCancelled
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	return m, cmd
}

func (m *inputModel) View() string { return m.ti.View() + "\n" }
