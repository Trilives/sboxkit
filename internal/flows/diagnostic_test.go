package flows

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
)

func TestRenderDiagnosticProducesStablePlainText(t *testing.T) {
	originalLanguage := i18n.Current()
	t.Cleanup(func() { i18n.SetLang(originalLanguage) })
	i18n.SetLang(i18n.EN)

	generatedAt := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	nextRestartAt := generatedAt.Add(4 * time.Minute)
	state := &sysd.WatchdogState{
		InFailure:        true,
		SnapshotCaptured: true,
		RestartedAt:      []time.Time{generatedAt, generatedAt.Add(2 * time.Minute)},
		NextRestartAt:    nextRestartAt,
	}
	snapshot := &sysd.DiagnosticSnapshot{
		GeneratedAt:      generatedAt,
		Failure:          sysd.FailureBootstrapDNS,
		DefaultInterface: "wlan0",
		BootstrapTargets: []string{"223.5.5.5:53", "1.1.1.1:853"},
		ProbeResults: []sysd.ProbeResult{
			{Layer: sysd.ProbePhysicalUplink, OK: true},
			{Layer: sysd.ProbeBootstrapDNS, OK: false, Detail: "dial tcp: connection refused"},
		},
	}

	var output bytes.Buffer
	if err := renderDiagnostic(&output, state, snapshot, false); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"Health diagnostic",
		"status: failing (bootstrap_dns)",
		"restart attempts: 2",
		"next restart allowed: 2026-09-11T16:39:00Z",
		"snapshot captured: 2026-09-11T16:35:00Z",
		"default interface: wlan0",
		"bootstrap targets: 223.5.5.5:53, 1.1.1.1:853",
		"probes:",
		"  physical_uplink: ok",
		"  bootstrap_dns: failed (dial tcp: connection refused)",
		"",
	}, "\n")
	if got := output.String(); got != want {
		t.Fatalf("plain diagnostic output is unstable:\nwant:\n%s\ngot:\n%s", want, got)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("non-TTY output contains terminal escape sequences: %q", output.String())
	}
}

func TestRenderDiagnosticProducesStableJSON(t *testing.T) {
	generatedAt := time.Date(2026, time.September, 11, 16, 35, 0, 0, time.UTC)
	state := &sysd.WatchdogState{
		InFailure:        true,
		SnapshotCaptured: true,
		RestartedAt:      []time.Time{generatedAt},
		NextRestartAt:    generatedAt.Add(2 * time.Minute),
	}
	snapshot := &sysd.DiagnosticSnapshot{
		GeneratedAt:      generatedAt,
		Failure:          sysd.FailureMixedPort,
		DefaultInterface: "eth0",
		BootstrapTargets: []string{"223.5.5.5:53"},
		ProbeResults: []sysd.ProbeResult{
			{Layer: sysd.ProbePhysicalUplink, OK: true},
			{Layer: sysd.ProbeBootstrapDNS, OK: true},
			{Layer: sysd.ProbeNodeResolution, OK: true},
			{Layer: sysd.ProbeMixedPort, OK: false, Detail: "connection refused"},
		},
	}

	var output bytes.Buffer
	if err := renderDiagnostic(&output, state, snapshot, true); err != nil {
		t.Fatal(err)
	}

	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("diagnostic JSON is invalid: %v\n%s", err, output.String())
	}
	want := map[string]any{
		"status": map[string]any{
			"available":       true,
			"in_failure":      true,
			"restart_count":   float64(1),
			"next_restart_at": "2026-09-11T16:37:00Z",
		},
		"snapshot": map[string]any{
			"available":         true,
			"generated_at":      "2026-09-11T16:35:00Z",
			"failure":           "mixed_port",
			"default_interface": "eth0",
			"bootstrap_targets": []any{"223.5.5.5:53"},
			"probes": []any{
				map[string]any{"layer": "physical_uplink", "ok": true},
				map[string]any{"layer": "bootstrap_dns", "ok": true},
				map[string]any{"layer": "node_resolution", "ok": true},
				map[string]any{"layer": "mixed_port", "ok": false, "detail": "connection refused"},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diagnostic JSON contract changed:\nwant %#v\n got %#v", want, got)
	}
}

func TestRenderDiagnosticExplainsMissingStateAndSnapshot(t *testing.T) {
	originalLanguage := i18n.Current()
	t.Cleanup(func() { i18n.SetLang(originalLanguage) })
	i18n.SetLang(i18n.EN)

	var output bytes.Buffer
	if err := renderDiagnostic(&output, nil, nil, false); err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"Health diagnostic",
		"watchdog state: unavailable (no state has been recorded)",
		"diagnostic snapshot: unavailable (no failure snapshot has been captured)",
		"",
	}, "\n")
	if got := output.String(); got != want {
		t.Fatalf("missing diagnostic evidence is not explained clearly:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestDiagnoseIsReadOnlyAndDoesNotRunExternalCommands(t *testing.T) {
	root := t.TempDir()
	p := paths.FromRoot(filepath.Join(root, "state"))
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	configBefore := []byte("{\"mixed_port\":2080}\n")
	if err := os.WriteFile(p.CustomizeFile, configBefore, 0o644); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(root, "watchdog-state.json")
	snapshotPath := filepath.Join(root, "diagnostic-snapshot.json")
	if err := os.WriteFile(statePath, []byte(`{"in_failure":false}`), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, []byte(`{"generated_at":"2026-09-11T16:35:00Z","failure":"end_to_end"}`), 0o444); err != nil {
		t.Fatal(err)
	}

	commandDir := filepath.Join(root, "commands")
	if err := os.Mkdir(commandDir, 0o755); err != nil {
		t.Fatal(err)
	}
	commandMarker := filepath.Join(root, "external-command-ran")
	commandStub := []byte("#!/bin/sh\nprintf called > \"" + commandMarker + "\"\n")
	for _, name := range []string{"systemctl", "sudo"} {
		if err := os.WriteFile(filepath.Join(commandDir, name), commandStub, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", commandDir)

	entriesBefore, err := os.ReadDir(p.State)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Diagnose(p, DiagnoseOptions{
		Writer:       &output,
		StatePath:    statePath,
		SnapshotPath: snapshotPath,
	}); err != nil {
		t.Fatal(err)
	}

	configAfter, err := os.ReadFile(p.CustomizeFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(configAfter, configBefore) {
		t.Fatalf("diagnose modified customize.json:\nwant %q\n got %q", configBefore, configAfter)
	}
	entriesAfter, err := os.ReadDir(p.State)
	if err != nil {
		t.Fatal(err)
	}
	if len(entriesAfter) != len(entriesBefore) {
		t.Fatalf("diagnose wrote state files: before=%v after=%v", entryNames(entriesBefore), entryNames(entriesAfter))
	}
	if _, err := os.Stat(commandMarker); !os.IsNotExist(err) {
		t.Fatalf("read-only diagnose invoked an external command; stat error = %v", err)
	}
}

func TestToolMenuOptionsIncludeDiagnostics(t *testing.T) {
	originalLanguage := i18n.Current()
	t.Cleanup(func() { i18n.SetLang(originalLanguage) })
	i18n.SetLang(i18n.EN)

	options := toolMenuOptions()
	want := i18n.T("故障诊断")
	for _, option := range options {
		if option == want {
			return
		}
	}
	t.Fatalf("tool menu options %q do not include %q", options, want)
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}
