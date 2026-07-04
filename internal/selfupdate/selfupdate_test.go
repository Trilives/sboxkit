package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

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

	got, err := RollbackToStable(p)
	if err != nil {
		t.Fatalf("RollbackToStable: %v", err)
	}
	if got != "0.1.0" {
		t.Errorf("RollbackToStable() = %q, want 0.1.0", got)
	}
	if target, _ := os.Readlink(currentLink(p)); target != stableBin {
		t.Errorf("current -> %s, want %s", target, stableBin)
	}
}

func TestRollbackToStableWithoutRecordFails(t *testing.T) {
	dir := t.TempDir()
	p := paths.Paths{State: dir}
	if _, err := RollbackToStable(p); err == nil {
		t.Error("expected error when no stable version has been recorded")
	}
}
