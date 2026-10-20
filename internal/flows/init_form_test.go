package flows

import (
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

func TestInitSubscriptionFromForm(t *testing.T) {
	values := tui.FormResult{
		"sub_name": "demo", "source": "sing-box", "sub_url": "https://example.com/sub",
		"use_download_proxy": "true", "fetch_proxy": "true", "customize": "false",
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

func initFormField(t *testing.T, fields []tui.FormField, key string) tui.FormField {
	t.Helper()
	for _, field := range fields {
		if field.Key == key {
			return field
		}
	}
	t.Fatalf("init form has no %q field", key)
	return tui.FormField{}
}

func TestInitFormAsksForDownloadProxyURLOnlyAfterOptIn(t *testing.T) {
	fields := initFormFields(paths.Paths{}, config.Defaults())

	use := initFormField(t, fields, "use_download_proxy")
	if use.Kind != tui.FormBool || use.Value != "false" {
		t.Fatalf("download proxy toggle = %#v, want an unchecked boolean", use)
	}
	proxy := initFormField(t, fields, "download_proxy")
	if proxy.Enabled == nil || proxy.Enabled(tui.FormResult{"use_download_proxy": "false"}) {
		t.Fatal("download proxy URL must stay disabled until the toggle is on")
	}
	if !proxy.Enabled(tui.FormResult{"use_download_proxy": "true"}) || !proxy.Required {
		t.Fatalf("enabled download proxy URL must be required: %#v", proxy)
	}
	if proxy.Validate == nil || proxy.Validate("127.0.0.1:7890") == nil {
		t.Fatal("download proxy URL must be validated")
	}
}

func TestInitFormKeepsConfiguredDownloadProxyEnabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.DownloadProxy = "http://10.0.0.2:7890"
	fields := initFormFields(paths.Paths{}, cfg)

	if got := initFormField(t, fields, "use_download_proxy").Value; got != "true" {
		t.Fatalf("toggle = %q, want true for an existing proxy", got)
	}
	if got := initFormField(t, fields, "download_proxy").Value; got != cfg.DownloadProxy {
		t.Fatalf("download proxy = %q, want %q", got, cfg.DownloadProxy)
	}
}

func TestInitFormFetchThroughProxyNeedsDownloadProxy(t *testing.T) {
	fetch := initFormField(t, initFormFields(paths.Paths{}, config.Defaults()), "fetch_proxy")

	for _, tc := range []struct {
		use, source string
		want        bool
	}{
		{use: "false", source: "clash", want: false},
		{use: "true", source: "clash", want: true},
		{use: "true", source: "local", want: false},
	} {
		got := fetch.Enabled(tui.FormResult{"use_download_proxy": tc.use, "source": tc.source})
		if got != tc.want {
			t.Fatalf("fetch_proxy enabled(use=%s, source=%s) = %v, want %v", tc.use, tc.source, got, tc.want)
		}
	}
}

func TestDownloadProxyFromFormClearsProxyWhenDeclined(t *testing.T) {
	declined := tui.FormResult{"use_download_proxy": "false", "download_proxy": "http://127.0.0.1:7890"}
	if got := downloadProxyFromForm(declined); got != "" {
		t.Fatalf("declined proxy = %q, want empty", got)
	}
	accepted := tui.FormResult{"use_download_proxy": "true", "download_proxy": " http://127.0.0.1:7890 "}
	if got := downloadProxyFromForm(accepted); got != "http://127.0.0.1:7890" {
		t.Fatalf("accepted proxy = %q", got)
	}
}

func TestInitSubscriptionDoesNotFetchThroughUnconfiguredProxy(t *testing.T) {
	values := tui.FormResult{
		"sub_name": "demo", "source": "clash", "sub_url": "https://example.com/sub",
		"use_download_proxy": "false", "fetch_proxy": "true",
	}
	got, err := initSubscriptionFromForm(values)
	if err != nil {
		t.Fatal(err)
	}
	if got.FetchViaProxy {
		t.Fatal("subscription must not request a download proxy the user declined")
	}
}
