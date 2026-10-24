package proxyenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBlockUsesConfiguredPort(t *testing.T) {
	got := block(17890)
	for _, want := range []string{
		`http_proxy="http://127.0.0.1:17890"`,
		`all_proxy="socks5://127.0.0.1:17890"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("proxy block missing %q:\n%s", want, got)
		}
	}
}

func TestWriteOwnedFileRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "shadow")
	if err := os.WriteFile(victim, []byte("root:x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bashrc := filepath.Join(dir, ".bashrc")
	if err := os.Symlink(victim, bashrc); err != nil {
		t.Fatal(err)
	}

	err := writeOwnedFile(bashrc, []byte("export x=1\n"), os.Getuid(), os.Getgid())

	if err == nil {
		t.Fatal("a symlinked ~/.bashrc must be refused: root would overwrite its target")
	}
	if got, _ := os.ReadFile(victim); string(got) != "root:x" {
		t.Fatalf("symlink target modified: %q", got)
	}
}

func TestWriteOwnedFileRefusesFileOfAnotherUser(t *testing.T) {
	bashrc := filepath.Join(t.TempDir(), ".bashrc")
	if err := os.WriteFile(bashrc, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeOwnedFile(bashrc, []byte("x"), os.Getuid()+1, os.Getgid()); err == nil {
		t.Fatal("a file not owned by the target user must be refused")
	}
}

func TestWriteOwnedFileRefusesHardLinkedFile(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	bashrc := filepath.Join(dir, ".bashrc")
	if err := os.Link(other, bashrc); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}

	if err := writeOwnedFile(bashrc, []byte("x"), os.Getuid(), os.Getgid()); err == nil {
		t.Fatal("a hard-linked ~/.bashrc must be refused")
	}
	if got, _ := os.ReadFile(other); string(got) != "keep" {
		t.Fatalf("hard link target modified: %q", got)
	}
}

func TestWriteOwnedFileCreatesAndRewritesOwnFile(t *testing.T) {
	bashrc := filepath.Join(t.TempDir(), ".bashrc")

	if err := writeOwnedFile(bashrc, []byte("first line that is longer\n"), os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}
	if err := writeOwnedFile(bashrc, []byte("second\n"), os.Getuid(), os.Getgid()); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(bashrc); string(got) != "second\n" {
		t.Fatalf("content = %q, rewrite must truncate the old content", got)
	}
}
