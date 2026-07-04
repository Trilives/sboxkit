// Package i18n 极简界面文案双语支持：默认英文，可切换中文。
//
// 设计：源码里的中文字面量本身即翻译表的 key（T 的参数），zh 模式原样返回，
// en 模式查表翻译；查不到则兜底返回中文原文（暴露遗漏而不是崩溃）。
// 翻译表按来源包拆分登记在 strings_*.go 里的 register() 调用中。
package i18n

type Lang string

const (
	EN Lang = "en"
	ZH Lang = "zh"
)

var current = EN

var registry = map[string]string{}

// register 由各 strings_*.go 的 init() 调用，合并各包的翻译条目。
func register(entries map[string]string) {
	for zh, en := range entries {
		registry[zh] = en
	}
}

// SetLang 设置当前界面语言；非 ZH 一律按 EN 处理。
func SetLang(l Lang) {
	if l == ZH {
		current = ZH
		return
	}
	current = EN
}

// Current 返回当前界面语言。
func Current() Lang { return current }

// T 按当前语言翻译一段中文界面文案（原文即 key）。
func T(zh string) string {
	if current == ZH {
		return zh
	}
	if en, ok := registry[zh]; ok {
		return en
	}
	return zh
}
