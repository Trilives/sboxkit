package subscription

import "testing"

func TestFetchUsesSourceUserAgent(t *testing.T) {
	if got := userAgent(SourceClash); got != "clash-verge/v2.0.0" {
		t.Fatalf("unexpected clash user-agent %q", got)
	}
	if got := userAgent(SourceSingBox); got != "sing-box/1.13.0" {
		t.Fatalf("unexpected sing-box user-agent %q", got)
	}
	if got := userAgent(SourceBase64); got != "v2rayN/6.0" {
		t.Fatalf("unexpected base64 user-agent %q", got)
	}
}
