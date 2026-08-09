package proxyenv

import (
	"strings"
	"testing"
)

func TestBlockUsesConfiguredPort(t *testing.T) {
	got := block(17890)
	for _, want := range []string{
		`http_proxy="http://127.0.0.1:17890"`,
		`all_proxy="socks5://127.0.0.1:17890"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("proxy block missing %q:\n%s", want, got)
		}
	}
}
