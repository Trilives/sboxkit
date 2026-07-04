package execx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableLogWritesAndTrims(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sboxkit.log")
	if err := EnableLog(path, 200); err != nil {
		t.Fatalf("EnableLog: %v", err)
	}
	defer DisableLog()

	for i := 0; i < 50; i++ {
		Info("line filler to grow the log file past the cap so trimming kicks in")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if int64(len(data)) > 200*2 {
		t.Errorf("log file not trimmed, size=%d", len(data))
	}
	if !strings.Contains(string(data), "line filler") {
		t.Error("expected trimmed log to still contain recent content")
	}
}

func TestDisableLogStopsWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sboxkit.log")
	if err := EnableLog(path, 0); err != nil {
		t.Fatal(err)
	}
	Info("before disable")
	DisableLog()
	before, _ := os.ReadFile(path)
	Info("after disable")
	after, _ := os.ReadFile(path)
	if len(before) != len(after) {
		t.Error("expected no further writes after DisableLog")
	}
}
