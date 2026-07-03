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
			fmt.Println("Value cannot be empty.")
			continue
		}
		return raw, nil
	}
}

func Confirm(prompt string, def bool) (bool, error) {
	raw, err := readInput(prompt + confirmSuffix(def) + ": ")
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

func confirmSuffix(def bool) string {
	if def {
		return " [Y/n]"
	}
	return " [y/N]"
}

func Pause(prompt string) {
	_, _ = readInput(prompt)
}

func readInput(prompt string) (string, error) {
	if !UseTUI() {
		return readPlainLine(prompt)
	}
	ti := textinput.New()
	ti.Prompt = ansiCyan + "❯ " + ansiReset
	ti.Focus()
	m := &inputModel{ti: ti, prompt: prompt, width: 80}
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
	ti     textinput.Model
	prompt string
	width  int
	err    error
}

func (m *inputModel) Init() tea.Cmd { return textinput.Blink }

func (m *inputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
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
	width := maxBoxWidth(m.width)
	promptRows := wrapText(m.prompt, width)
	return strings.Join(promptRows, "\n") + "\n" + m.ti.View() + "\n"
}

func wrapText(text string, width int) []string {
	if width <= 0 {
		width = 80
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{""}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}
	rows := []string{}
	current := ""
	for _, word := range words {
		if dispWidth(word) > width {
			if current != "" {
				rows = append(rows, current)
				current = ""
			}
			rows = append(rows, splitWideWord(word, width)...)
			continue
		}
		if current == "" {
			current = word
			continue
		}
		next := current + " " + word
		if dispWidth(next) <= width {
			current = next
			continue
		}
		rows = append(rows, current)
		current = word
	}
	if current != "" {
		rows = append(rows, current)
	}
	return rows
}

func splitWideWord(word string, width int) []string {
	rows := []string{}
	var current strings.Builder
	currentWidth := 0
	for _, ch := range word {
		chWidth := dispWidth(string(ch))
		if currentWidth > 0 && currentWidth+chWidth > width {
			rows = append(rows, current.String())
			current.Reset()
			currentWidth = 0
		}
		current.WriteRune(ch)
		currentWidth += chWidth
	}
	if current.Len() > 0 {
		rows = append(rows, current.String())
	}
	return rows
}
