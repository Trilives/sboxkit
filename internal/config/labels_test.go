package config

import (
	"testing"

	"github.com/Trilives/sboxkit/internal/i18n"
)

// 字段标签经 i18n.T 动态查表，扫描 i18n.T("…") 字面量的检查发现不了遗漏的翻译。
func TestFieldLabelsHaveEnglishTranslations(t *testing.T) {
	orig := i18n.Current()
	i18n.SetLang(i18n.EN)
	t.Cleanup(func() { i18n.SetLang(orig) })

	for _, fields := range []map[string]string{ListFields, BoolFields, ScalarFields} {
		for key, label := range fields {
			if i18n.T(label) == label && !isASCII(label) {
				t.Errorf("%s: label %q has no English translation", key, label)
			}
		}
	}
}

func isASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}
