package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/flows"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestRunDiagnosePassesStableOutputModeToReadOnlyFlow(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	tests := []struct {
		name     string
		args     []string
		wantJSON bool
	}{
		{name: "plain text", args: nil, wantJSON: false},
		{name: "json", args: []string{"--json"}, wantJSON: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			calls := 0
			runner := func(gotPaths paths.Paths, options flows.DiagnoseOptions) error {
				calls++
				if gotPaths != p {
					t.Fatalf("paths = %#v, want %#v", gotPaths, p)
				}
				if options.JSON != test.wantJSON {
					t.Fatalf("JSON = %v, want %v", options.JSON, test.wantJSON)
				}
				if options.Writer != io.Writer(&stdout) {
					t.Fatal("diagnostic output was not routed to command stdout")
				}
				return nil
			}

			if code := runDiagnose(p, test.args, &stdout, &stderr, runner); code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr=%q", code, stderr.String())
			}
			if calls != 1 {
				t.Fatalf("diagnostic flow calls = %d, want 1", calls)
			}
			if stderr.Len() != 0 {
				t.Fatalf("successful diagnose wrote stderr: %q", stderr.String())
			}
		})
	}
}

func TestRunDiagnoseRejectsUnknownOptionsWithoutCallingFlow(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	called := false
	runner := func(paths.Paths, flows.DiagnoseOptions) error {
		called = true
		return nil
	}

	code := runDiagnose(paths.Paths{}, []string{"--repair"}, &stdout, &stderr, runner)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	if called {
		t.Fatal("unknown diagnose option invoked the diagnostic flow")
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid invocation wrote stdout: %q", stdout.String())
	}
	if got := stderr.String(); got != "usage: sboxkit diagnose [--json]\n" {
		t.Fatalf("usage output = %q", got)
	}
}

func TestRunDiagnoseReportsReadFailureWithoutPanicking(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	runner := func(paths.Paths, flows.DiagnoseOptions) error {
		return errors.New("cannot read diagnostic state")
	}

	code := runDiagnose(paths.Paths{}, nil, &stdout, &stderr, runner)

	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if got := stderr.String(); !strings.Contains(got, "cannot read diagnostic state") {
		t.Fatalf("stderr does not explain diagnostic read failure: %q", got)
	}
}

func TestUsageTextIncludesDiagnoseCommand(t *testing.T) {
	if got := usageText(); !strings.Contains(got, "diagnose") {
		t.Fatalf("usage omits the read-only diagnose command: %q", got)
	}
}
