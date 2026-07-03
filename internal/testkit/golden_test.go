package testkit

import (
	"path/filepath"
	"testing"
)

func TestReadFixture(t *testing.T) {
	got := ReadFixture(t, filepath.Join("testdata", "converter", "clash-basic.yaml"))

	if got == "" {
		t.Fatal("expected fixture content")
	}
}

func TestAssertJSONEqualAcceptsEquivalentFormatting(t *testing.T) {
	AssertJSONEqual(t, `{"b":2,"a":1}`, "{\n  \"a\": 1,\n  \"b\": 2\n}")
}
