package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/paths"
)

func writeTarGz(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	archive := filepath.Join(dir, "test.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		hdr := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(content))}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	f.Close()
	return archive
}

func TestExtractTarGz(t *testing.T) {
	dir := t.TempDir()
	archive := writeTarGz(t, dir, map[string]string{"sboxkit": "binary-content", "README.md": "hi"})
	outDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(archive, outDir); err != nil {
		t.Fatalf("extractTarGz: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(outDir, "sboxkit"))
	if err != nil {
		t.Fatalf("sboxkit not extracted: %v", err)
	}
	if string(got) != "binary-content" {
		t.Errorf("content mismatch: %q", got)
	}
}

func TestExtractTarGzRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	archive := writeTarGz(t, dir, map[string]string{"../evil": "x"})
	outDir := filepath.Join(dir, "out")
	os.MkdirAll(outDir, 0o755)
	if err := extractTarGz(archive, outDir); err == nil {
		t.Error("expected path-traversal entry to be rejected")
	}
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "sboxkit_1.0.0_linux_amd64.tar.gz")
	if err := os.WriteFile(archivePath, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("payload"))
	sumsPath := filepath.Join(dir, "checksums.txt")
	content := hex.EncodeToString(sum[:]) + "  sboxkit_1.0.0_linux_amd64.tar.gz\n"
	if err := os.WriteFile(sumsPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifySHA256(archivePath, sumsPath, "sboxkit_1.0.0_linux_amd64.tar.gz"); err != nil {
		t.Errorf("expected valid checksum to pass: %v", err)
	}
}

func TestVerifySHA256Mismatch(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "a.tar.gz")
	os.WriteFile(archivePath, []byte("payload"), 0o644)
	sumsPath := filepath.Join(dir, "checksums.txt")
	os.WriteFile(sumsPath, []byte("0000000000000000000000000000000000000000000000000000000000000000  a.tar.gz\n"), 0o644)
	if err := verifySHA256(archivePath, sumsPath, "a.tar.gz"); err == nil {
		t.Error("expected checksum mismatch to fail")
	}
}

func TestAtomicSymlinkSwap(t *testing.T) {
	dir := t.TempDir()
	targetA := filepath.Join(dir, "a")
	targetB := filepath.Join(dir, "b")
	os.WriteFile(targetA, []byte("a"), 0o644)
	os.WriteFile(targetB, []byte("b"), 0o644)
	link := filepath.Join(dir, "current")

	if err := atomicSymlink(targetA, link); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != targetA {
		t.Fatalf("expected link -> %s, got %s", targetA, got)
	}
	if err := atomicSymlink(targetB, link); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(link); got != targetB {
		t.Fatalf("expected link -> %s after swap, got %s", targetB, got)
	}
}

func TestPruneOldVersionsKeepsCurrentAndPrevious(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{State: dir}
	for _, v := range []string{"0.1.0", "0.1.1", "0.1.2", "0.1.3"} {
		os.MkdirAll(filepath.Join(versionsDir(p), v), 0o755)
	}
	pruneOldVersions(p, "0.1.3")

	entries, _ := os.ReadDir(versionsDir(p))
	var remaining []string
	for _, e := range entries {
		remaining = append(remaining, e.Name())
	}
	want := map[string]bool{"0.1.2": true, "0.1.3": true}
	if len(remaining) != len(want) {
		t.Fatalf("expected 2 remaining versions, got %v", remaining)
	}
	for _, r := range remaining {
		if !want[r] {
			t.Errorf("unexpected leftover version dir: %s", r)
		}
	}
}

func TestAssetName(t *testing.T) {
	name := assetName("0.1.2")
	if !bytes.Contains([]byte(name), []byte("sboxkit_0.1.2_linux_")) {
		t.Errorf("unexpected asset name: %s", name)
	}
}

func fakeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestPruneOldVersionsPreservesLastStable(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{State: dir}
	for _, v := range []string{"0.1.0", "0.1.1", "0.1.2", "0.1.3"} {
		os.MkdirAll(filepath.Join(versionsDir(p), v), 0o755)
	}
	// last-stable 记录的是 0.1.0——本应被"只留当前+上一版本"规则清理掉的版本，
	// 但因为有 last-stable 指着它，必须保留。
	if err := atomicSymlink(versionBin(p, "0.1.0"), lastStableLink(p)); err != nil {
		t.Fatal(err)
	}
	pruneOldVersions(p, "0.1.3")

	entries, _ := os.ReadDir(versionsDir(p))
	remaining := map[string]bool{}
	for _, e := range entries {
		remaining[e.Name()] = true
	}
	for _, want := range []string{"0.1.0", "0.1.2", "0.1.3"} {
		if !remaining[want] {
			t.Errorf("expected %s to survive pruning, remaining=%v", want, remaining)
		}
	}
	if remaining["0.1.1"] {
		t.Errorf("0.1.1 should have been pruned, remaining=%v", remaining)
	}
}

func TestLastStableVersionAndRollback(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{State: dir}

	if _, ok := LastStableVersion(p); ok {
		t.Fatal("expected no last-stable recorded initially")
	}

	stableBin := versionBin(p, "0.1.0")
	fakeExecutable(t, stableBin)
	if err := atomicSymlink(stableBin, lastStableLink(p)); err != nil {
		t.Fatal(err)
	}

	v, ok := LastStableVersion(p)
	if !ok || v != "0.1.0" {
		t.Fatalf("LastStableVersion() = (%q, %v), want (0.1.0, true)", v, ok)
	}

	previewBin := versionBin(p, "0.1.1-beta.1")
	fakeExecutable(t, previewBin)
	if err := atomicSymlink(previewBin, currentLink(p)); err != nil {
		t.Fatal(err)
	}

	stable, err := PrepareStable(p)
	if err != nil {
		t.Fatalf("PrepareStable: %v", err)
	}
	if stable.Version != "0.1.0" {
		t.Errorf("PrepareStable().Version = %q, want 0.1.0", stable.Version)
	}
	if target, _ := os.Readlink(currentLink(p)); target != previewBin {
		t.Fatalf("prepare must not switch current, got -> %s", target)
	}
	undo, err := stable.Activate(p)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if target, _ := os.Readlink(currentLink(p)); target != stableBin {
		t.Errorf("current -> %s, want %s", target, stableBin)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}
	if target, _ := os.Readlink(currentLink(p)); target != previewBin {
		t.Errorf("undo: current -> %s, want %s", target, previewBin)
	}
	stable.Discard()
	if _, err := os.Stat(stableBin); err != nil {
		t.Fatalf("discarding a recorded stable version deleted it: %v", err)
	}
}

func TestPrepareStableWithoutRecordFails(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{State: dir}
	if _, err := PrepareStable(p); err == nil {
		t.Error("expected error when no stable version has been recorded")
	}
}

// fakeSboxkit 写一个 sboxkit 替身：compat-check 按 handler 处理（空表示旧版本，未知子命令）。
func fakeSboxkit(t *testing.T, handler string) *Prepared {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sboxkit")
	script := "#!/bin/sh\ncase \"$1\" in\nversion) echo sboxkit 9.9.9 ;;\n"
	if handler != "" {
		script += "compat-check)\n" + handler + "\n;;\n"
	}
	script += "*) echo \"unknown subcommand: $1\" >&2; exit 2 ;;\nesac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Prepared{Version: "9.9.9", Bin: bin}
}

func TestCompatCheckReportsOldVersionsAsUnverifiable(t *testing.T) {
	tool := fakeSboxkit(t, "")

	_, _, err := tool.CompatCheck(paths.Paths{State: t.TempDir()}, compat.Bundle{Kernel: "/k"})

	if !errors.Is(err, ErrNoCompatCheck) || !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("expected ErrNoCompatCheck with the child's output, got %v", err)
	}
}

func TestCompatCheckReadsReportAndGeneratedConfigFromSameState(t *testing.T) {
	// 参数布局：compat-check --kernel K --geosite G --geoip I --report R --out O（$9 = R，${11} = O）
	tool := fakeSboxkit(t, `[ "$SBOXKIT_HOME" = "$EXPECT_HOME" ] || exit 3
echo '{"generator":"9.9.9","min_kernel":"1.14.0","kernel":"1.14.3","subscription":"Hua","compatible":true}' > "$9"
echo '{"generated":true}' > "${11}"`)
	home := t.TempDir()
	t.Setenv("EXPECT_HOME", home)

	report, cfg, err := tool.CompatCheck(paths.Paths{State: home}, compat.Bundle{Kernel: "/k", Geosite: "/g", GeoIP: "/i"})

	if err != nil || !report.Compatible || report.Subscription != "Hua" {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if strings.TrimSpace(string(cfg)) != `{"generated":true}` {
		t.Fatalf("generated config = %q", cfg)
	}
}

func TestReplaceExecutableLinkWithoutRoot(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sboxkit")
	target := filepath.Join(dir, "managed", "sboxkit")
	fakeExecutable(t, exe)
	fakeExecutable(t, target)
	if err := replaceExecutableLink(exe, target); err != nil {
		t.Fatalf("replace executable link: %v", err)
	}
	got, err := os.Readlink(exe)
	if err != nil || got != target {
		t.Fatalf("executable link = %q, %v; want %q", got, err, target)
	}
}

// 回退切换失败时 current 仍指向新版本：Discard 不得删除该版本目录。
func TestDiscardKeepsVersionWhenUndoCannotSwitchBack(t *testing.T) {
	p := paths.Paths{State: t.TempDir()}
	running := versionBin(p, "0.3.1")
	fakeExecutable(t, running)
	if err := atomicSymlink(running, currentLink(p)); err != nil {
		t.Fatal(err)
	}
	tool := &Prepared{Version: "0.3.2", Bin: versionBin(p, "0.3.2"), created: true}
	fakeExecutable(t, tool.Bin)
	undo, err := tool.Activate(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(versionsDir(p), 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(versionsDir(p), 0o755)

	if err := undo(); err == nil {
		t.Skip("switching back unexpectedly succeeded in a read-only directory (running as root?)")
	}
	tool.Discard()

	if _, err := os.Stat(tool.Bin); err != nil {
		t.Fatalf("binary that current still points at was deleted: %v", err)
	}
}

// 解压或试跑失败时，已存在（可能被 last-stable 引用）的版本目录保持原样。
func TestExtractFailureKeepsExistingVersionDirectory(t *testing.T) {
	p := paths.Paths{State: t.TempDir()}
	tool := &Prepared{Version: "0.3.2", Bin: versionBin(p, "0.3.2")}
	fakeExecutable(t, tool.Bin)
	archive := writeTarGz(t, t.TempDir(), map[string]string{"README": "no binary"})

	if err := tool.extract(p, archive); err == nil {
		t.Fatal("archive without a binary accepted")
	}
	if _, err := os.Stat(tool.Bin); err != nil {
		t.Fatalf("existing version binary removed by a failed extract: %v", err)
	}
	entries, _ := os.ReadDir(versionsDir(p))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".incoming-") {
			t.Fatalf("temporary extract directory left behind: %s", e.Name())
		}
	}
}
