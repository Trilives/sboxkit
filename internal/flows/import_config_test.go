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

	if err := importConfigFromFile(p, sourceFile); err != nil {
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
	err := importConfigFromFile(paths.Paths{State: filepath.Join(root, "state")}, root)
	if err == nil || !strings.Contains(err.Error(), "文件路径") {
		t.Fatalf("expected file path error, got %v", err)
	}
}
