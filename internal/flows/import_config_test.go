package flows

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

// TestMain 强制中文模式：本包测试断言的是源码里的中文原文，与界面默认语言无关。
func TestMain(m *testing.M) {
	i18n.SetLang(i18n.ZH)
	os.Exit(m.Run())
}

func TestImportConfigFromFileCopiesConfigAndClearsActiveSubscription(t *testing.T) {
	root := t.TempDir()
	sourceFile := filepath.Join(root, "source", "config.yaml")
	stateDir := filepath.Join(root, "state")
	p := paths.Paths{
		State:      stateDir,
		ConfigFile: filepath.Join(stateDir, "config.yaml"),
		ActiveFile: filepath.Join(stateDir, "active"),
	}
	if err := os.MkdirAll(filepath.Dir(sourceFile), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "mixed-port: 7890\nproxy-groups: []\n"
	if err := os.WriteFile(sourceFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ActiveFile, []byte("airport\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := loadImportConfig(p, sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyImportConfig(p, raw, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(p.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != source {
		t.Fatalf("imported config mismatch\nwant:\n%s\ngot:\n%s", source, got)
	}
	if _, err := os.Stat(p.ActiveFile); !os.IsNotExist(err) {
		t.Fatalf("active subscription pointer should be removed, got err=%v", err)
	}
}

func TestImportConfigFromFileRejectsDirectoryPath(t *testing.T) {
	root := t.TempDir()
	_, err := loadImportConfig(paths.Paths{State: filepath.Join(root, "state")}, root)
	if err == nil || !strings.Contains(err.Error(), "文件路径") {
		t.Fatalf("expected file path error, got %v", err)
	}
}

// fakeKernel 在 path 写一个 sing-box 替身：check 拒绝含 marker 的配置。
func fakeKernel(t *testing.T, path, marker string) {
	t.Helper()
	script := `#!/bin/sh
case "$1" in
version) echo "sing-box version 1.14.3" ;;
check) if grep -q '` + marker + `' "$3"; then echo "FATAL[0000] unknown field" >&2; exit 1; fi ;;
esac
`
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestImportRejectedByKernelKeepsActiveConfigAndSubscription(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	fakeKernel(t, p.SingBoxBin, "future_field")
	source := filepath.Join(t.TempDir(), "config.json")
	for path, content := range map[string]string{
		source:       `{"future_field":true}`,
		p.ConfigFile: "deployed",
		p.ActiveFile: "Hua\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := loadImportConfig(p, source)

	if err == nil || !strings.Contains(err.Error(), "1.14.3") || !strings.Contains(err.Error(), "保持不变") {
		t.Fatalf("expected a kernel rejection that names the version, got %v", err)
	}
	for path, want := range map[string]string{p.ConfigFile: "deployed", p.ActiveFile: "Hua\n"} {
		if got, _ := os.ReadFile(path); string(got) != want {
			t.Fatalf("%s changed to %q", path, got)
		}
	}
}
