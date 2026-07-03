package applog

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenDisabledReturnsOriginalWriter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	writer, closeFn, err := Open(dir, Config{Enabled: false}, io.Discard)
	if err != nil {
		t.Fatalf("open disabled log: %v", err)
	}
	defer closeFn()

	if writer != io.Discard {
		t.Fatal("disabled logging should return original writer")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("disabled logging should not create log dir, stat err=%v", err)
	}
}

func TestOpenWritesCommandLogAndMirrorsStderr(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	var stderr strings.Builder

	writer, closeFn, err := Open(dir, Config{Enabled: true, MaxBytes: 1024}, &stderr)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	if _, err := writer.Write([]byte("network failed\n")); err != nil {
		t.Fatalf("write log: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close log: %v", err)
	}

	if stderr.String() != "network failed\n" {
		t.Fatalf("stderr mirror = %q", stderr.String())
	}
	logs, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read log dir: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("log files = %d, want 1", len(logs))
	}
	data, err := os.ReadFile(filepath.Join(dir, logs[0].Name()))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "network failed") {
		t.Fatalf("log missing mirrored stderr:\n%s", data)
	}
}

func TestPruneDeletesOldLogsToStayUnderLimit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	old := filepath.Join(dir, "sboxkit-20000101-000000.log")
	newer := filepath.Join(dir, "sboxkit-20000102-000000.log")
	if err := os.WriteFile(old, []byte(strings.Repeat("o", 800)), 0o600); err != nil {
		t.Fatalf("write old: %v", err)
	}
	if err := os.WriteFile(newer, []byte(strings.Repeat("n", 400)), 0o600); err != nil {
		t.Fatalf("write newer: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	newTime := time.Now().Add(-1 * time.Hour)
	_ = os.Chtimes(old, oldTime, oldTime)
	_ = os.Chtimes(newer, newTime, newTime)

	if err := Prune(dir, 700); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old log should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(newer); err != nil {
		t.Fatalf("newer log should remain: %v", err)
	}
}

func TestMaxBytesIsClamped(t *testing.T) {
	if got := MaxBytes(0); got != DefaultMaxBytes {
		t.Fatalf("MaxBytes(0) = %d, want default %d", got, DefaultMaxBytes)
	}
	if got := MaxBytes(9999); got != HardMaxBytes {
		t.Fatalf("MaxBytes(9999) = %d, want hard max %d", got, HardMaxBytes)
	}
}
