package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/i18n"
)

// MultiSelect 空格勾选、⏎ 确认、esc → ErrCancelled。返回选中下标（升序）。
func MultiSelect(title string, options []string, defaultOn []int) ([]int, error) {
	if !UseTUI() {
		return multiSelectPlain(title, options, defaultOn)
	}
	chosen := map[int]bool{}
	for _, i := range defaultOn {
		chosen[i] = true
	}
	m := &multiModel{
		title:   title,
		options: options,
		chosen:  chosen,
		footer:  i18n.T("↑/↓ 移动   空格 勾选   ⏎ 确认   esc 取消"),
		width:   80,
	}
	out, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, err
	}
	fm := out.(*multiModel)
	if fm.err != nil {
		return nil, fm.err
	}
	var picked []int
	for i, on := range fm.chosen {
		if on {
			picked = append(picked, i)
		}
	}
	sort.Ints(picked)
	return picked, nil
}

type multiModel struct {
	title   string
	options []string
	chosen  map[int]bool
	footer  string
	idx     int
	width   int
	err     error
}

func (m *multiModel) Init() tea.Cmd { return nil }

func (m *multiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		n := len(m.options)
		switch msg.String() {
		case "up":
			m.idx = (m.idx - 1 + n) % n
		case "down":
			m.idx = (m.idx + 1) % n
		case " ":
			m.chosen[m.idx] = !m.chosen[m.idx]
		case "enter":
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.err = errs.ErrCancelled
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *multiModel) View() string {
	maxW := maxBoxWidth(m.width)
	label := truncate(fmt.Sprintf("─ %s ", m.title), maxW)
	footerText := truncate("  "+m.footer, maxW)
	texts := make([]string, len(m.options))
	widths := []int{dispWidth(label), dispWidth(footerText)}
	for i, opt := range m.options {
		mark := "[ ]"
		if m.chosen[i] {
			mark = "[x]"
		}
		texts[i] = truncate(fmt.Sprintf("  %s %s ", mark, opt), maxW)
		widths = append(widths, dispWidth(texts[i]))
	}
	w := min(maxOf(widths)+2, maxW)

	rows := []string{"┌" + label + strings.Repeat("─", max(0, w-dispWidth(label))) + "┐"}
	rows = append(rows, "│"+strings.Repeat(" ", w)+"│")
	for i, text := range texts {
		if i == m.idx && useColor {
			text = ansiCyan + ansiBold + rowPad(text, w) + ansiReset
		} else {
			text = rowPad(text, w)
		}
		rows = append(rows, "│"+text+"│")
	}
	rows = append(rows, "│"+strings.Repeat(" ", w)+"│")
	rows = append(rows, "│"+dim(rowPad(footerText, w))+"│")
	rows = append(rows, "└"+strings.Repeat("─", w)+"┘")
	return strings.Join(rows, "\n") + "\n"
}
