package download

import "testing"

func TestMapArch(t *testing.T) {
	cases := map[string]string{
		"x86_64":  "amd64",
		"aarch64": "arm64",
		"armv7l":  "armv7",
		"i686":    "386",
	}
	for input, want := range cases {
		if got := mapArch(input); got != want {
			t.Fatalf("%s: expected %s, got %s", input, want, got)
		}
	}
}

func TestPickAsset(t *testing.T) {
	urls := []string{
		"https://example.com/sing-box-1.0-linux-arm64.tar.gz",
		"https://example.com/sing-box-1.0-linux-amd64.tar.gz",
	}
	got := pickAsset(urls, `linux-amd64.*\.tar\.gz`)
	if got != urls[1] {
		t.Fatalf("unexpected asset %q", got)
	}
}
