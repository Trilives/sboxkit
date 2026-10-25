package sysd

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func stubServiceExecutable(t *testing.T, path string) {
	t.Helper()
	origPath, origCheck := executablePath, checkExecutable
	executablePath = func() (string, error) { return path, nil }
	checkExecutable = func(string) error { return nil }
	t.Cleanup(func() { executablePath, checkExecutable = origPath, origCheck })
}

func rootOwnedExecutable(t *testing.T) string {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh in PATH")
	}
	resolved, err := filepath.EvalSymlinks(sh)
	if err != nil {
		t.Skip(err)
	}
	for _, p := range append(parentDirs(resolved), resolved) {
		info, err := os.Stat(p)
		if err != nil || info.Sys().(*syscall.Stat_t).Uid != 0 || info.Mode().Perm()&0o022 != 0 {
			t.Skipf("%s is not root-owned on this host", p)
		}
	}
	return resolved
}

func TestTrustedExecutableAcceptsRootOwnedPaths(t *testing.T) {
	if err := trustedExecutable(rootOwnedExecutable(t)); err != nil {
		t.Fatal(err)
	}
}

func TestTrustedExecutableRejectsPathsOthersCanReplace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("files created by root are root-owned")
	}
	dir := t.TempDir()
	own := filepath.Join(dir, "sboxkit")
	if err := os.WriteFile(own, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "via-link")
	if err := os.Symlink(rootOwnedExecutable(t), link); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{own, link} {
		err := trustedExecutable(path)
		// 用户自己的目录：即使链接指向 root 的文件，链接本身也能被改指向别处。
		if err == nil || !strings.Contains(err.Error(), dir) {
			t.Errorf("trustedExecutable(%s) = %v, want a rejection naming %s", path, err, dir)
		}
	}
}

func TestServiceExecutableChecksOnlySystemUnits(t *testing.T) {
	origPath, origCheck := executablePath, checkExecutable
	t.Cleanup(func() { executablePath, checkExecutable = origPath, origCheck })
	executablePath = func() (string, error) { return "/home/u/sboxkit", nil }
	checked := 0
	checkExecutable = func(string) error { checked++; return os.ErrPermission }

	if exe, err := serviceExecutable(unitScope{User: true}); err != nil || exe != "/home/u/sboxkit" || checked != 0 {
		t.Fatalf("user scope: %q, %v, checks=%d; user units run as the user", exe, err, checked)
	}
	if _, err := serviceExecutable(unitScope{}); err == nil || checked != 1 {
		t.Fatalf("system scope: %v, checks=%d; want the check's rejection", err, checked)
	}
}

func TestParentDirs(t *testing.T) {
	if got, want := parentDirs("/usr/bin/sboxkit"), []string{"/usr/bin", "/usr", "/"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("parentDirs = %q, want %q", got, want)
	}
}
