package flows

import (
	"testing"

	"github.com/Trilives/sboxkit/internal/configfile"
)

func TestRuntimeConfigReadsSingBoxInboundsAndSecret(t *testing.T) {
	doc, err := configfile.Parse([]byte(`{
  "inbounds": [
    {"type": "tun", "tag": "tun-in"},
    {"type": "mixed", "listen_port": 17890}
  ],
  "experimental": {"clash_api": {"secret": "panel-secret"}}
}`))
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	port, tun := runtimeInboundState(doc)
	if port != 17890 || !tun {
		t.Fatalf("runtime state = port %d, tun %v", port, tun)
	}
	if got := runtimeClashAPISecret(doc); got != "panel-secret" {
		t.Fatalf("secret = %q", got)
	}
}
