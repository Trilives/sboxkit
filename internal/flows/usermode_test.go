package flows

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

func TestSystemModeOnlyDisablesFirewallFieldInUserMode(t *testing.T) {
	p := paths.FromUserModeRoot("/k/.sboxkit", "/k")
	field := tui.FormField{Key: "allow_firewall", Label: "Firewall", Kind: tui.FormBool, Value: "true"}

	got := systemModeOnly(p, field)

	if got.Value != "false" || got.Enabled == nil || got.Enabled(tui.FormResult{}) {
		t.Fatalf("user mode firewall field should be forced off and disabled: %+v", got)
	}
	if got.Label == field.Label {
		t.Fatal("disabled field label should explain why it is unavailable")
	}
}

func TestSystemModeOnlyLeavesSystemFieldsUntouched(t *testing.T) {
	field := tui.FormField{Key: "allow_firewall", Label: "Firewall", Kind: tui.FormBool, Value: "true"}

	got := systemModeOnly(paths.FromRoot("/var/lib/sboxkit"), field)

	if got.Value != "true" || got.Enabled != nil || got.Label != "Firewall" {
		t.Fatalf("system field changed: %+v", got)
	}
}

func TestTunStaysSelectableInUserModeWithSudoHint(t *testing.T) {
	system := tunFieldLabel(paths.FromRoot("/var/lib/sboxkit"))
	user := tunFieldLabel(paths.FromUserModeRoot("/k/.sboxkit", "/k"))

	if user == system || !strings.HasPrefix(user, system) {
		t.Fatalf("user mode TUN label = %q, want %q plus a sudo hint", user, system)
	}
}

func TestEnsureStateRootNeverEscalatesInUserMode(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere; the unwritable branch is unreachable")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o755) })
	p := paths.FromUserModeRoot(filepath.Join(root, ".sboxkit"), root)

	err := EnsureStateRoot(p)

	if err == nil || !strings.Contains(err.Error(), p.State) {
		t.Fatalf("EnsureStateRoot error = %v, want a clear non-sudo failure", err)
	}
}

func TestFileLocationsShowUserModeRuntime(t *testing.T) {
	p := paths.FromUserModeRoot("/k/.sboxkit", "/k")

	for _, row := range fileLocations(p) {
		if row.path == p.Runtime {
			return
		}
	}
	t.Fatalf("file locations do not list runtime dir %s", p.Runtime)
}
