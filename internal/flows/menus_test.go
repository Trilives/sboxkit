package flows

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestPrintAccessHintUsesEmbeddedPanelWithoutStateUICopy(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(p, config.Defaults()); err != nil {
		t.Fatal(err)
	}

	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStdout := os.Stdout
	os.Stdout = writeEnd
	defer func() { os.Stdout = originalStdout }()

	printAccessHint(p)
	if err := writeEnd.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(readEnd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "http://127.0.0.1:9090/ui/") {
		t.Fatalf("access hint omitted embedded panel URL when state UI was empty:\n%s", out)
	}
}
