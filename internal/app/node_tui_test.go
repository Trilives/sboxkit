package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestBuildSwitchNodeArgsDoesNotReorderByDefault(t *testing.T) {
	var out bytes.Buffer
	session := scriptedTUISession(&out, []string{"Proxy", "Node A"}, []bool{false})

	args, syncService, ok := session.buildSwitchNodeArgs()
	if !ok {
		t.Fatal("expected switch node args")
	}
	if syncService {
		t.Fatal("syncService = true, want false when reorder is not selected")
	}
	if got := strings.Join(args, "\x00"); strings.Contains(got, "--reorder") {
		t.Fatalf("args should not reorder by default: %#v", args)
	}
}

func TestBuildSwitchNodeArgsCanRequestReorderAndServiceSync(t *testing.T) {
	var out bytes.Buffer
	session := scriptedTUISession(&out, []string{"Proxy", "Node A"}, []bool{true, true})

	args, syncService, ok := session.buildSwitchNodeArgs()
	if !ok {
		t.Fatal("expected switch node args")
	}
	if !syncService {
		t.Fatal("syncService = false, want true when reorder and sync are selected")
	}
	if got := strings.Join(args, "\x00"); !strings.Contains(got, "--reorder") {
		t.Fatalf("args should include --reorder: %#v", args)
	}
}
