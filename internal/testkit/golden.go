package testkit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func ReadFixture(t *testing.T, rel string) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	return string(data)
}

func AssertJSONEqual(t *testing.T, want string, got string) {
	t.Helper()

	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("parse expected JSON: %v", err)
	}

	var gotValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatalf("parse actual JSON: %v", err)
	}

	if !reflect.DeepEqual(wantValue, gotValue) {
		wantPretty, _ := json.MarshalIndent(wantValue, "", "  ")
		gotPretty, _ := json.MarshalIndent(gotValue, "", "  ")
		t.Fatalf("JSON mismatch\nwant:\n%s\n\ngot:\n%s", wantPretty, gotPretty)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repository root containing go.mod")
		}
		dir = parent
	}
}
