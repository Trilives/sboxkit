package main

import (
	"fmt"
	"io"

	"github.com/Trilives/sboxkit/internal/flows"
	"github.com/Trilives/sboxkit/internal/paths"
)

const diagnoseUsage = "usage: sboxkit diagnose [--json]"

type diagnoseRunner func(paths.Paths, flows.DiagnoseOptions) error

func runDiagnose(p paths.Paths, args []string, stdout, stderr io.Writer, runner diagnoseRunner) int {
	jsonOutput := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--json":
		jsonOutput = true
	default:
		fmt.Fprintln(stderr, diagnoseUsage)
		return 2
	}

	if err := runner(p, flows.DiagnoseOptions{Writer: stdout, JSON: jsonOutput}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
