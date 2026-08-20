package flows

import (
	"errors"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestResiliencePreferenceRollsBackWhenEnableFails(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.EnableResilience = false
	if err := config.Save(p, cfg); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("install failed")
	if err := applyResiliencePreference(p, true, func() error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if config.Load(p).EnableResilience {
		t.Fatal("failed install must restore the disabled preference")
	}
}

func TestResiliencePreferenceStaysEnabledWhenDisableFails(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(p, config.Defaults()); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("remove failed")
	if err := applyResiliencePreference(p, false, func() error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if !config.Load(p).EnableResilience {
		t.Fatal("failed removal must preserve the enabled preference")
	}
}
