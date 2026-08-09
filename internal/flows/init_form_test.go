package flows

import (
	"testing"

	"github.com/Trilives/sboxkit/internal/tui"
)

func TestInitSubscriptionFromForm(t *testing.T) {
	sources := []string{"Clash", "sing-box", "base64", "local"}
	values := tui.FormResult{
		"sub_name": "demo", "source": "sing-box", "sub_url": "https://example.com/sub",
		"fetch_proxy": "true", "customize": "false",
	}
	got, err := initSubscriptionFromForm(values, sources)
	if err != nil {
		t.Fatalf("build subscription: %v", err)
	}
	if got.Name != "demo" || got.SourceType != "sing-box" || got.ApplyOverlay || !got.FetchViaProxy {
		t.Fatalf("unexpected subscription: %#v", got)
	}
}

func TestInitSubscriptionEmptyURLSkipsSubscription(t *testing.T) {
	got, err := initSubscriptionFromForm(tui.FormResult{"sub_url": ""}, []string{"Clash"})
	if err != nil || got != nil {
		t.Fatalf("empty URL should skip subscription, got %#v, %v", got, err)
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
