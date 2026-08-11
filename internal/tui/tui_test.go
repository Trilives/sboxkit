package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Trilives/sboxkit/internal/i18n"
)

// TestMain 强制中文模式：本文件断言的是源码里的中文原文，与界面默认语言无关。
func TestMain(m *testing.M) {
	i18n.SetLang(i18n.ZH)
	os.Exit(m.Run())
}

func TestTruncateCJK(t *testing.T) {
	s := "生成新加坡自动测速聚合组"
	got := truncate(s, 10)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("超宽应以省略号收尾: %q", got)
	}
	if dispWidth(got) > 10 {
		t.Fatalf("截断后宽度 %d 超过上限 10", dispWidth(got))
	}
	if truncate("short", 10) != "short" {
		t.Fatal("未超宽不应截断")
	}
}

func TestScrollTop(t *testing.T) {
	if scrollTop(5, 2, 10) != 0 {
		t.Fatal("项数少于窗口时不滚动")
	}
	if got := scrollTop(100, 0, 10); got != 0 {
		t.Fatalf("顶部不越界: %d", got)
	}
	if got := scrollTop(100, 99, 10); got != 90 {
		t.Fatalf("底部不越界: %d", got)
	}
	if got := scrollTop(100, 50, 10); got != 45 {
		t.Fatalf("选中项应尽量居中: %d", got)
	}
}

func TestBuildSelectBoxAligned(t *testing.T) {
	rows := buildSelect("测试菜单", []string{"选项一", "第二个更长的选项 option", "三"}, 1,
		"↑/↓ 选择   ⏎ 确认   esc 返回   ^R 返回", 80, 24)
	if len(rows) < 6 {
		t.Fatalf("盒子行数异常: %d", len(rows))
	}
	w := dispWidth(stripAnsi(rows[0]))
	for i, r := range rows {
		if got := dispWidth(stripAnsi(r)); got != w {
			t.Errorf("第 %d 行宽度 %d ≠ 首行 %d: %q", i, got, w, stripAnsi(r))
		}
	}
	joined := stripAnsi(strings.Join(rows, "\n"))
	if !strings.Contains(joined, "❯ ② 第二个更长的选项 option") {
		t.Error("选中行应带 ❯ 与圈号")
	}
	if !strings.HasPrefix(rows[0], "┌─ 测试菜单 ") || !strings.HasPrefix(rows[len(rows)-1], "└") {
		t.Error("边框结构不符")
	}
}

func TestBuildSelectCapsWidth(t *testing.T) {
	long := strings.Repeat("ghp_verylongtoken", 20)
	rows := buildSelect("编辑定制层", []string{"GitHub Token：" + long}, 0, "footer", 60, 24)
	maxW := maxBoxWidth(60) + 2 // 边框两列
	for i, r := range rows {
		if got := dispWidth(stripAnsi(r)); got > maxW {
			t.Errorf("第 %d 行宽度 %d 超过终端上限 %d", i, got, maxW)
		}
	}
	if !strings.Contains(stripAnsi(strings.Join(rows, "")), "…") {
		t.Error("超长行应被省略号截断")
	}
}

func TestBuildSelectScrollHints(t *testing.T) {
	opts := make([]string, 40)
	for i := range opts {
		opts[i] = "节点 " + strings.Repeat("x", i%5)
	}
	rows := buildSelect("选择节点", opts, 20, "footer", 80, 24)
	joined := stripAnsi(strings.Join(rows, "\n"))
	if !strings.Contains(joined, "▲ 上方还有") || !strings.Contains(joined, "▼ 下方还有") {
		t.Error("滚动窗口应显示上下提示")
	}
}

func TestNumFor(t *testing.T) {
	// 总数在圈号范围内：整份统一用带圈数字。
	if numFor(20, 0) != "①" || numFor(20, 19) != "⑳" {
		t.Error("总数≤20 时应整份使用带圈数字")
	}
	// 总数超出圈号范围：整份统一退化为普通数字（含前 20 项也不例外），
	// 避免同一菜单内前面带圈、后面变数字的不统一观感。
	if numFor(25, 0) != "1" || numFor(25, 19) != "20" || numFor(25, 24) != "25" {
		t.Error("总数超过 20 时应整份统一使用普通数字，不应部分带圈")
	}
}

func TestWrapText(t *testing.T) {
	short := wrapText("hello world", 20)
	if len(short) != 1 || short[0] != "hello world" {
		t.Errorf("短文本不应换行: %#v", short)
	}
	words := wrapText("aaaa bbbb cccc dddd", 9)
	for _, l := range words {
		if dispWidth(l) > 9 {
			t.Errorf("按词换行超出宽度: %q", l)
		}
	}
	if len(words) < 2 {
		t.Error("超宽文本应换成多行")
	}
	cjk := wrapText("这是一段很长的中文提示语用来测试自动换行是否正常工作", 10)
	if len(cjk) < 2 {
		t.Error("长中文提示语应按字符宽度换成多行")
	}
	for _, l := range cjk {
		if dispWidth(l) > 10 {
			t.Errorf("中文换行超出宽度: %q", l)
		}
	}
}

func TestFormModelRequiresTextEditModeAndSubmitRow(t *testing.T) {
	fields := []FormField{
		{Key: "text", Label: "文本", Kind: FormText, Placeholder: "示例"},
		{Key: "flag", Label: "开关", Kind: FormBool},
		{Key: "choice", Label: "类型", Kind: FormChoice, Value: "A", Options: []string{"A", "B"}},
	}
	m := &formModel{fields: cloneFormFields(fields), width: 80}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	if m.fields[0].Value != "" {
		t.Fatal("text changed before Enter opened edit mode")
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || !m.editing {
		t.Fatal("Enter on text field should open editing without submitting")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.editing {
		t.Fatal("Enter should finish text editing")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	got := formValues(m.fields)
	if got["text"] != "x" || !got.Bool("flag") || got["choice"] != "B" {
		t.Fatalf("unexpected form values: %#v", got)
	}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("Enter on choice field should not submit")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd == nil {
		t.Fatal("Enter on submit row should quit")
	}
}

func TestFormTextEscapeDiscardsCurrentEdit(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{Key: "text", Kind: FormText, Value: "old"}}), width: 80}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("new")})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.editing || m.fields[0].Value != "old" || m.err != nil {
		t.Fatalf("Esc should discard only current text edit: %#v", m)
	}
}

func TestFormSubmitValidatesOnlyOnSubmitRow(t *testing.T) {
	m := &formModel{fields: cloneFormFields([]FormField{{
		Key: "text", Label: "文本", Kind: FormText,
		Validate: func(string) error { return fmt.Errorf("invalid") },
	}}), idx: 1, width: 80}
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("invalid form should remain open")
	}
	if !strings.Contains(m.note, "invalid") {
		t.Fatalf("validation note = %q", m.note)
	}
}

func TestFormPlaceholderIsDisplayOnly(t *testing.T) {
	field := FormField{Key: "proxy", Label: "下载代理", Kind: FormText, Placeholder: "127.0.0.1:7890"}
	if got := formValues([]FormField{field})["proxy"]; got != "" {
		t.Fatalf("placeholder leaked into value: %q", got)
	}
	if !strings.Contains(formDisplayValue(field), "127.0.0.1:7890") {
		t.Fatal("placeholder should remain visible")
	}
}

func TestBuildFormRowsAligned(t *testing.T) {
	fields := []FormField{
		{Key: "proxy", Label: "下载代理", Kind: FormText, Placeholder: "127.0.0.1:7890"},
		{Key: "tun", Label: "启用 TUN", Kind: FormBool, Value: "true"},
		{Key: "type", Label: "订阅类型", Kind: FormChoice, Value: "Clash", Options: []string{"Clash", "sing-box"}},
	}
	rows := buildForm("初始化", fields, len(fields), false, FormOpts{SubmitLabel: "开始初始化", Hint: "详细配置请启动后在配置里面设置"}, "", 80)
	w := dispWidth(stripAnsi(rows[0]))
	for i, row := range rows {
		if got := dispWidth(stripAnsi(row)); got != w {
			t.Fatalf("row %d width %d, want %d: %q", i, got, w, row)
		}
	}
	joined := stripAnsi(strings.Join(rows, "\n"))
	if !strings.Contains(joined, "< Clash >") || strings.Contains(joined, "sing-box") {
		t.Fatalf("choice should display only its selected value:\n%s", joined)
	}
	if !strings.Contains(joined, "❯ [开始初始化]") {
		t.Fatalf("submit row should be selectable:\n%s", joined)
	}
}

func TestFormChoiceUsesStableValueAndDisplayLabel(t *testing.T) {
	field := FormField{
		Key: "source", Kind: FormChoice, Value: "local", Options: []string{"clash", "local"},
		OptionLabels: map[string]string{"clash": "Clash", "local": "本地文件"},
	}
	if got := formDisplayValue(field); got != "< 本地文件 >" {
		t.Fatalf("display value = %q", got)
	}
	if got := formValues([]FormField{field})["source"]; got != "local" {
		t.Fatalf("stored value = %q", got)
	}
}
