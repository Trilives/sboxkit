package sysd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

func TestStageRuntimeConfigRewritesExternalUIAndCachePath(t *testing.T) {
	state := t.TempDir()
	p := paths.Paths{
		State:      state,
		ConfigFile: filepath.Join(state, "config.json"),
		UI:         state, // hasUIAssets 检查 p.UI/index.html
	}
	if err := os.WriteFile(filepath.Join(state, "index.html"), []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := `{"inbounds":[],"outbounds":[],"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","external_ui":"ui"}}}`
	if err := os.WriteFile(p.ConfigFile, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := stageRuntimeConfig(p, runtimePaths{
		UI:      "/var/lib/sboxkit-runtime/ui",
		CacheDB: "/var/lib/sboxkit-runtime/cache.db",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(staged)

	out, err := os.ReadFile(staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"/var/lib/sboxkit-runtime/ui"`) {
		t.Fatalf("external_ui was not rewritten in staged config:\n%s", out)
	}
	if !strings.Contains(string(out), `"/var/lib/sboxkit-runtime/cache.db"`) {
		t.Fatalf("cache_file.path was not rewritten in staged config:\n%s", out)
	}
}
