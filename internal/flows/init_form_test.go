package flows

import (
	"testing"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/tui"
)

func TestInitSubscriptionFromForm(t *testing.T) {
	values := tui.FormResult{
		"sub_name": "demo", "source": "sing-box", "sub_url": "https://example.com/sub",
		"fetch_proxy": "true", "customize": "false",
	}
	got, err := initSubscriptionFromForm(values)
	if err != nil {
		t.Fatalf("build subscription: %v", err)
	}
	if got.Name != "demo" || got.SourceType != "sing-box" || got.ApplyOverlay || !got.FetchViaProxy {
		t.Fatalf("unexpected subscription: %#v", got)
	}
}

func TestInitSubscriptionEmptyURLSkipsSubscription(t *testing.T) {
	got, err := initSubscriptionFromForm(tui.FormResult{"sub_url": ""})
	if err != nil || got != nil {
		t.Fatalf("empty URL should skip subscription, got %#v, %v", got, err)
	}
}

func TestInitSourceOptionsUseStableValuesAndShortLabels(t *testing.T) {
	values, labels := initSourceOptions()
	want := []string{"clash", "sing-box", "base64", "local"}
	if len(values) != len(want) {
		t.Fatalf("source values = %#v", values)
	}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("source values = %#v", values)
		}
	}
	for value, wantLabel := range map[string]string{
		"clash": "Clash", "sing-box": "sing-box", "base64": "base64", "local": "本地文件",
	} {
		if labels[value] != wantLabel {
			t.Fatalf("label[%s] = %q, want %q", value, labels[value], wantLabel)
		}
	}

	i18n.SetLang(i18n.EN)
	t.Cleanup(func() { i18n.SetLang(i18n.ZH) })
	englishValues, englishLabels := initSourceOptions()
	for i := range want {
		if englishValues[i] != want[i] {
			t.Fatalf("English source values = %#v", englishValues)
		}
	}
	if englishLabels["local"] != "Local file" {
		t.Fatalf("English local label = %q", englishLabels["local"])
	}
}

func TestParseUIDs(t *testing.T) {
	got, err := parseUIDs("1000, 1001")
	if err != nil || len(got) != 2 || got[0] != 1000 || got[1] != 1001 {
		t.Fatalf("parse UIDs = %#v, %v", got, err)
	}
	if _, err := parseUIDs("root"); err == nil {
		t.Fatal("non-numeric UID should fail")
	}
}
