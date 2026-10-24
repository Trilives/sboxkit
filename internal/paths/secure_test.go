package paths

import (
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"syscall"
	"testing"
)

// foreignUID 测试里充当「目标属主」的 uid：与当前用户不同，所有条目都视为待收归。
const foreignUID = 54321

type chownLog struct{ paths []string }

func (c *chownLog) chown(name string, uid, gid int) error {
	if uid != foreignUID || gid != foreignUID {
		return os.ErrInvalid
	}
	c.paths = append(c.paths, filepath.Base(name))
	return nil
}

func inode(t *testing.T, path string) uint64 {
	t.Helper()
	st, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Sys().(*syscall.Stat_t).Ino
}

func mustWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestSecureTreeHandsEveryEntryToTargetOwner(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "customize.json"), "{}", 0o600)
	mustWrite(t, filepath.Join(root, "bin", "sing-box"), "ELF", 0o755)
	log := &chownLog{}

	if err := secureTree(root, foreignUID, foreignUID, log.chown); err != nil {
		t.Fatal(err)
	}

	sort.Strings(log.paths)
	if len(log.paths) != 4 { // root、bin/ 与两个重建文件的临时名
		t.Fatalf("chowned %v, want root, bin and both files", log.paths)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "bin", "sing-box")); string(got) != "ELF" {
		t.Fatalf("kernel content = %q after securing", got)
	}
	if st, _ := os.Stat(filepath.Join(root, "bin", "sing-box")); st.Mode().Perm() != 0o755 {
		t.Fatalf("kernel mode = %v, executable bit must survive", st.Mode().Perm())
	}
}

func TestSecureTreeRecreatesFilesSoOldWriteHandlesGoStale(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	mustWrite(t, path, "trusted", 0o644)
	before := inode(t, path)
	stale, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Close()

	if err := secureTree(root, foreignUID, foreignUID, (&chownLog{}).chown); err != nil {
		t.Fatal(err)
	}
	if _, err := stale.WriteAt([]byte("hijack!"), 0); err != nil {
		t.Fatal(err)
	}

	if inode(t, path) == before {
		t.Fatal("foreign-owned file must be replaced by a new inode")
	}
	if got, _ := os.ReadFile(path); string(got) != "trusted" {
		t.Fatalf("content = %q, a handle opened before securing must not reach the new file", got)
	}
}

func TestSecureTreeStripsGroupAndOtherWrite(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ruleset")
	file := filepath.Join(dir, "geoip-cn.srs")
	mustWrite(t, file, "srs", 0o666)
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}

	if err := secureTree(root, os.Getuid(), os.Getgid(), os.Lchown); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]os.FileMode{dir: 0o755, file: 0o644} {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", filepath.Base(path), st.Mode().Perm(), want)
		}
	}
}

func TestSecureTreeBreaksHardLinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim")
	mustWrite(t, outside, "victim", 0o644)
	linked := filepath.Join(root, "active")
	if err := os.Link(outside, linked); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}

	if err := secureTree(root, os.Getuid(), os.Getgid(), os.Lchown); err != nil {
		t.Fatal(err)
	}

	if inode(t, linked) == inode(t, outside) {
		t.Fatal("hard link into the tree must be replaced by an independent copy")
	}
}

func TestSecureTreeRemovesLinksEscapingRootAndKeepsInternalOnes(t *testing.T) {
	root := t.TempDir()
	versions := filepath.Join(root, "sboxkit-versions")
	mustWrite(t, filepath.Join(versions, "0.3.4", "sboxkit"), "bin", 0o755)
	links := map[string]string{
		filepath.Join(versions, "current"):     filepath.Join(versions, "0.3.4", "sboxkit"),
		filepath.Join(versions, "last-stable"): "0.3.4/sboxkit",
		filepath.Join(root, "sboxkit.log"):     "/etc/shadow",
		filepath.Join(root, "config.json"):     "../../etc/passwd",
	}
	for link, target := range links {
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
	}

	if err := secureTree(root, os.Getuid(), os.Getgid(), os.Lchown); err != nil {
		t.Fatal(err)
	}

	for _, kept := range []string{"current", "last-stable"} {
		if _, err := os.Lstat(filepath.Join(versions, kept)); err != nil {
			t.Errorf("internal link %s removed: %v", kept, err)
		}
	}
	for _, gone := range []string{"sboxkit.log", "config.json"} {
		if _, err := os.Lstat(filepath.Join(root, gone)); !os.IsNotExist(err) {
			t.Errorf("escaping link %s must be removed, lstat err = %v", gone, err)
		}
	}
}

func TestSecureTreeRemovesSpecialFiles(t *testing.T) {
	root := t.TempDir()
	fifo := filepath.Join(root, "customize.json")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}

	if err := secureTree(root, os.Getuid(), os.Getgid(), os.Lchown); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(fifo); !os.IsNotExist(err) {
		t.Fatalf("FIFO must be removed so root never blocks reading it, lstat err = %v", err)
	}
}

func TestSecureTreeLeavesCompliantTreeUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "subscriptions", "a", "raw.yaml")
	mustWrite(t, path, "proxies: []", 0o600)
	before := inode(t, path)

	if err := secureTree(root, os.Getuid(), os.Getgid(), os.Lchown); err != nil {
		t.Fatal(err)
	}

	if inode(t, path) != before {
		t.Fatal("compliant file must not be rewritten on every run")
	}
}

func TestSecureTreeRefusesSymlinkedRoot(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	if err := secureTree(link, os.Getuid(), os.Getgid(), os.Lchown); err == nil {
		t.Fatal("a symlinked state root must be refused")
	}
}

func TestEnsureStateDirsLocksSubscriptions(t *testing.T) {
	p := FromRoot(t.TempDir())
	if err := os.MkdirAll(p.Subscriptions, 0o755); err != nil { // 旧版本留下的 0755
		t.Fatal(err)
	}

	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(p.Subscriptions)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != p.SecretDirMode() {
		t.Fatalf("subscriptions mode = %v, want %v: raw subscriptions hold node credentials", st.Mode().Perm(), p.SecretDirMode())
	}
}

func TestEnsureStateDirsKeepsSetgidOnSubscriptions(t *testing.T) {
	p := FromRoot(t.TempDir())
	if err := os.MkdirAll(p.Subscriptions, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p.Subscriptions, 0o750|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}

	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}

	if st, _ := os.Stat(p.Subscriptions); st.Mode()&os.ModeSetgid == 0 {
		t.Fatalf("subscriptions mode = %v, setgid must survive", st.Mode())
	}
}

func TestSecretModesDependOnMode(t *testing.T) {
	system := FromRoot("/var/lib/sboxkit")
	user := FromUserModeRoot("/k/.sboxkit", "/k")

	if system.SecretFileMode() != 0o640 || system.SecretDirMode() != 0o750 {
		t.Errorf("system secrets = %v/%v, want root-writable, admin-group-readable", system.SecretFileMode(), system.SecretDirMode())
	}
	if user.SecretFileMode() != 0o600 || user.SecretDirMode() != 0o700 {
		t.Errorf("user secrets = %v/%v, want owner-only", user.SecretFileMode(), user.SecretDirMode())
	}
}

func TestAdminGroupPrefersSudoThenWheel(t *testing.T) {
	groups := map[string]string{"wheel": "10", "admin": "4"}
	lookup := func(name string) (*user.Group, error) {
		if gid, ok := groups[name]; ok {
			return &user.Group{Gid: gid, Name: name}, nil
		}
		return nil, user.UnknownGroupError(name)
	}

	if gid, name := adminGroup(lookup); gid != 10 || name != "wheel" {
		t.Fatalf("adminGroup = %d %s, want wheel when sudo is absent", gid, name)
	}
	groups["sudo"] = "27"
	if gid, name := adminGroup(lookup); gid != 27 || name != "sudo" {
		t.Fatalf("adminGroup = %d %s, want sudo first", gid, name)
	}
	if gid, _ := adminGroup(func(n string) (*user.Group, error) { return nil, user.UnknownGroupError(n) }); gid != 0 {
		t.Fatalf("adminGroup without admin groups = %d, want root group so only root reads secrets", gid)
	}
}

func TestSecureTreeMarksDirectoriesSetgid(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "subscriptions")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := secureTree(root, os.Getuid(), os.Getgid(), os.Lchown); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{root, dir} {
		if st, _ := os.Stat(d); st.Mode()&os.ModeSetgid == 0 {
			t.Errorf("%s mode = %v, want setgid so files root writes later inherit the admin group", d, st.Mode())
		}
	}
}

func TestSecureTreeRegroupsRootOwnedFileInPlace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config.json")
	mustWrite(t, path, "{}", 0o640)
	before := inode(t, path)
	var chowned []string
	chown := func(name string, uid, gid int) error {
		chowned = append(chowned, filepath.Base(name))
		return nil
	}

	// 属主已对（当前用户充当 root），只有属组不同：改属组即可，不必重建。
	if err := secureTree(root, os.Getuid(), foreignUID, chown); err != nil {
		t.Fatal(err)
	}

	if inode(t, path) != before {
		t.Fatal("a file already owned by the target user must keep its inode")
	}
	if !containsString(chowned, "config.json") {
		t.Fatalf("chowned %v, want config.json regrouped", chowned)
	}
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
