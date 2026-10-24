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
	if err := EnableLog(path, 200, 0o600); err != nil {
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

func TestEnableLogRefusesSymlinkedLog(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "sboxkit.log")
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}

	if err := EnableLog(path, 0, 0o600); err == nil {
		DisableLog()
		t.Fatal("EnableLog must not follow a symlinked log: root would append to its target")
	}
	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Fatalf("symlink target modified: %q", got)
	}
}

func TestLogTrimReplacesFileInsteadOfWritingThroughPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sboxkit.log")
	if err := EnableLog(path, 200, 0o600); err != nil {
		t.Fatal(err)
	}
	defer DisableLog()
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 日志已打开后被换成符号链接：裁剪时不能把尾部内容写进链接目标。
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 50; i++ {
		Info("line filler to grow the log file past the cap so trimming kicks in")
	}

	if got, _ := os.ReadFile(victim); string(got) != "keep" {
		t.Fatalf("trim wrote through the symlink into its target: %q", got)
	}
}

func TestEnableLogCreatesFileWithRequestedMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sboxkit.log")
	if err := EnableLog(path, 0, 0o600); err != nil {
		t.Fatal(err)
	}
	defer DisableLog()

	if got := modeOf(t, path); got != 0o600 {
		t.Fatalf("log mode = %v, want 0600: logs may contain subscription URLs", got)
	}
}

func TestLogKeepsGroupReadableModeAcrossTrims(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sboxkit.log")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 系统模式：管理员组不经 sudo 查看日志。
	if err := EnableLog(path, 200, 0o640); err != nil {
		t.Fatal(err)
	}
	defer DisableLog()
	if got := modeOf(t, path); got != 0o640 {
		t.Fatalf("existing log mode = %v, want 0640", got)
	}

	for i := 0; i < 50; i++ {
		Info("line filler to grow the log file past the cap so trimming kicks in")
	}

	if got := modeOf(t, path); got != 0o640 {
		t.Fatalf("trimmed log mode = %v, want 0640", got)
	}
}

func modeOf(t *testing.T, path string) os.FileMode {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

func TestDisableLogStopsWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sboxkit.log")
	if err := EnableLog(path, 0, 0o600); err != nil {
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
