package flows

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestReadFileTail(t *testing.T) {
	file := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(file, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := readFileTail(file, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0] != "two" || lines[1] != "three" {
		t.Fatalf("tail = %#v", lines)
	}
}

func TestFileLocationsIncludeBothLogs(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	locations := fileLocations(p)
	wantTUI := execx.LogPath(p.State)
	foundTUI, foundCore := false, false
	for _, location := range locations {
		foundTUI = foundTUI || location.path == wantTUI
		foundCore = foundCore || location.path == "journalctl -u sing-box.service"
	}
	if !foundTUI || !foundCore {
		t.Fatalf("log locations missing: %#v", locations)
	}
}
