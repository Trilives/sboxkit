package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/i18n"
)

// SelectOpts Select 的选项。SaveLabel 缺省与 BackLabel 相同——多数菜单里两个键
// 效果一致；仅在需要区分「保存」与「放弃」时才各传各的。Initial 为初始光标位置，
// 同一菜单重入时传上次选中的下标使光标停留原位。
type SelectOpts struct {
	BackLabel string
	SaveLabel string
	Initial   int
}

// Select 返回选中项下标；esc → ErrSaveExit，^R → ErrCancelled。
func Select(title string, options []string, opts SelectOpts) (int, error) {
	if opts.BackLabel == "" {
		opts.BackLabel = i18n.T("返回")
	}
	if opts.SaveLabel == "" {
		opts.SaveLabel = opts.BackLabel
	}
	if len(options) == 0 {
		return 0, errs.ErrCancelled
	}
	if !UseTUI() {
		return selectPlain(title, options, opts)
	}
	idx := opts.Initial
	if idx < 0 || idx >= len(options) {
		idx = 0
	}
	m := &selectModel{
		title:   title,
		options: options,
		idx:     idx,
		footer:  fmt.Sprintf(i18n.T("↑/↓ 选择   ⏎ 确认   esc %s   ^R %s"), opts.SaveLabel, opts.BackLabel),
		width:   80,
		height:  24,
	}
	out, err := tea.NewProgram(m).Run()
	if err != nil {
		return 0, err
	}
	fm := out.(*selectModel)
	if fm.err != nil {
		return 0, fm.err
	}
	return fm.idx, nil
}

type selectModel struct {
	title   string
	options []string
	footer  string
	idx     int
	width   int
	height  int
	err     error
}

func (m *selectModel) Init() tea.Cmd { return nil }

func (m *selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		n := len(m.options)
		switch msg.String() {
		case "up":
			m.idx = (m.idx - 1 + n) % n
		case "down":
			m.idx = (m.idx + 1) % n
		case "enter":
			return m, tea.Quit
		case "esc", "ctrl+c": // Python 版 Ctrl-C 亦按 ESC 处理
			m.err = errs.ErrSaveExit
			return m, tea.Quit
		case "ctrl+r":
			m.err = errs.ErrCancelled
			return m, tea.Quit
		default:
			s := msg.String()
			if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
				if j := int(s[0] - '1'); j < n {
					m.idx = j
				}
			}
		}
	}
	return m, nil
}

func (m *selectModel) View() string {
	return strings.Join(buildSelect(m.title, m.options, m.idx, m.footer, m.width, m.height), "\n") + "\n"
}

// buildSelect 渲染选择框（对应 menu._build_select）：选项多于一屏时滑动显示；
// 单行内容按终端宽度截断（省略号收尾），避免超长选项把边框撑破。
func buildSelect(title string, options []string, idx int, footer string, termCols, termLines int) []string {
	n := len(options)
	visible := min(maxVisibleRows(termLines), n)
	top := scrollTop(n, idx, visible)
	end := top + visible
	maxW := maxBoxWidth(termCols)

	upHint, downHint := "", ""
	if top > 0 {
		upHint = truncate(fmt.Sprintf(i18n.T("  ▲ 上方还有 %d 项"), top), maxW)
	}
	if end < n {
		downHint = truncate(fmt.Sprintf(i18n.T("  ▼ 下方还有 %d 项"), n-end), maxW)
	}
	label := truncate(fmt.Sprintf("─ %s ", title), maxW)
	footerText := truncate("  "+footer, maxW)

	rowsText := make(map[int]string, visible)
	widths := []int{dispWidth(label), dispWidth(footerText), dispWidth(upHint), dispWidth(downHint)}
	for i := top; i < end; i++ {
		mark := " "
		if i == idx {
			mark = "❯"
		}
		t := truncate(fmt.Sprintf("  %s %s %s ", mark, numFor(n, i), options[i]), maxW)
		rowsText[i] = t
		widths = append(widths, dispWidth(t))
	}
	w := min(maxOf(widths)+2, maxW)

	rows := []string{"┌" + label + strings.Repeat("─", max(0, w-dispWidth(label))) + "┐"}
	rows = append(rows, "│"+dim(rowPad(upHint, w))+"│")
	for i := top; i < end; i++ {
		text := rowsText[i]
		if i == idx && useColor {
			text = ansiCyan + ansiBold + rowPad(text, w) + ansiReset
		} else {
			text = rowPad(text, w)
		}
		rows = append(rows, "│"+text+"│")
	}
	rows = append(rows, "│"+dim(rowPad(downHint, w))+"│")
	rows = append(rows, "│"+dim(rowPad(footerText, w))+"│")
	rows = append(rows, "└"+strings.Repeat("─", w)+"┘")
	return rows
}
