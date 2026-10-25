package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
)

// writeDeb 生成一个最小的 deb：ar 归档内含 debian-binary、control.tar.gz 与 data.tar.gz。
func writeDeb(t *testing.T, path string, files map[string]string) {
	t.Helper()
	var data bytes.Buffer
	gz := gzip.NewWriter(&data)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(content)) //nolint:errcheck
	}
	tw.Close()
	gz.Close()
	var ar bytes.Buffer
	ar.WriteString("!<arch>\n")
	for _, m := range []struct {
		name string
		body []byte
	}{{"debian-binary", []byte("2.0\n")}, {"control.tar.gz", []byte("odd")}, {"data.tar.gz", data.Bytes()}} {
		fmt.Fprintf(&ar, "%-16s%-12s%-6s%-6s%-8s%-10d`\n", m.name, "0", "0", "0", "100644", len(m.body))
		ar.Write(m.body)
		if len(m.body)%2 == 1 {
			ar.WriteByte('\n')
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, ar.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

const fakeBinary = "#!/bin/sh\necho sboxkit 9.9.9\n"

func TestExtractDebMemberFindsPackagedBinary(t *testing.T) {
	dir := t.TempDir()
	deb := filepath.Join(dir, "sboxkit.deb")
	writeDeb(t, deb, map[string]string{"./usr/share/doc/x": "doc", "./usr/bin/sboxkit": fakeBinary})
	out := filepath.Join(dir, "sboxkit")

	if err := extractDebMember(deb, debBinMember, out); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(out); string(got) != fakeBinary {
		t.Fatalf("extracted %q", got)
	}
	if st, _ := os.Stat(out); st.Mode().Perm() != 0o755 {
		t.Fatalf("extracted mode = %v, want executable", st.Mode().Perm())
	}
}

func TestExtractDebMemberRejectsPackageWithoutBinary(t *testing.T) {
	dir := t.TempDir()
	deb := filepath.Join(dir, "sboxkit.deb")
	writeDeb(t, deb, map[string]string{"./usr/share/doc/x": "doc"})

	if err := extractDebMember(deb, debBinMember, filepath.Join(dir, "out")); err == nil {
		t.Fatal("package without usr/bin/sboxkit accepted")
	}
	if err := os.WriteFile(deb, []byte("not an ar archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractDebMember(deb, debBinMember, filepath.Join(dir, "out2")); err == nil {
		t.Fatal("garbage accepted as a deb")
	}
}

// fakeDpkg 记录 dpkg 调用；installed 为 dpkg-query 报告的版本，lockedTries 次 dpkg -i 报锁占用。
type fakeDpkg struct {
	installed   string
	lockedTries int
	failInstall bool
	calls       []string
}

func (f *fakeDpkg) run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	switch name {
	case "dpkg-query":
		if f.installed == "" {
			return []byte("not-installed "), errors.New("exit 1")
		}
		return []byte("installed " + f.installed), nil
	case "dpkg":
		if f.lockedTries > 0 {
			f.lockedTries--
			return []byte("dpkg: error: dpkg frontend lock was locked by another process"), errors.New("exit 2")
		}
		if f.failInstall {
			return []byte("dpkg: error processing archive"), errors.New("exit 1")
		}
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected command %s", name)
}

func stubDpkg(t *testing.T, f *fakeDpkg) {
	t.Helper()
	origRun, origWait, origExe := runCommand, dpkgLockWait, packagedExe
	runCommand, dpkgLockWait = f.run, 0
	packagedExe = filepath.Join(t.TempDir(), "usr-bin-sboxkit")
	fakeExecutable(t, packagedExe)
	t.Cleanup(func() { runCommand, dpkgLockWait, packagedExe = origRun, origWait, origExe })
}

func TestInstalledPackageVersionReadsDpkgStatus(t *testing.T) {
	f := &fakeDpkg{installed: "0.3.3"}
	stubDpkg(t, f)

	if v, err := installedPackageVersion(); err != nil || v != "0.3.3" {
		t.Fatalf("installedPackageVersion = %q, %v", v, err)
	}
	// nfpm 把预发布后缀写成 Debian 的 ~；撤销更新要按发行版标签找回原版本。
	f.installed = "0.4.1~beta.1"
	if v, err := installedPackageVersion(); err != nil || v != "0.4.1-beta.1" {
		t.Fatalf("installedPackageVersion for a preview = %q, %v, want the release version", v, err)
	}
	f.installed = ""
	if _, err := installedPackageVersion(); err == nil {
		t.Fatal("missing package reported as installed")
	}
}

func TestDpkgInstallWaitsForTheLock(t *testing.T) {
	f := &fakeDpkg{lockedTries: 2}
	stubDpkg(t, f)

	if err := dpkgInstall("/tmp/x.deb"); err != nil {
		t.Fatalf("dpkgInstall = %v, want success once the lock is released", err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("dpkg calls = %v, want two locked attempts and one success", f.calls)
	}
}

func TestDpkgLockedMatchesOnlyLockContention(t *testing.T) {
	for out, want := range map[string]bool{
		"dpkg: error: dpkg frontend lock was locked by another process with pid 42": true,
		"dpkg: error: dpkg status database is locked by another process":            true,
		"dpkg: error processing archive x.deb: cannot copy extracted data: block":   false,
		"dpkg: warning: files list file for package 'x' missing; unlocked":          false,
	} {
		if got := dpkgLocked([]byte(out)); got != want {
			t.Errorf("dpkgLocked(%q) = %v, want %v", out, got, want)
		}
	}
}

func TestDpkgInstallReportsRealFailuresWithoutRetrying(t *testing.T) {
	f := &fakeDpkg{failInstall: true}
	stubDpkg(t, f)

	err := dpkgInstall("/tmp/x.deb")

	if err == nil || !strings.Contains(err.Error(), "error processing archive") {
		t.Fatalf("dpkgInstall = %v, want dpkg's output", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("dpkg calls = %v, a non-lock failure must not be retried", f.calls)
	}
}

// stubReleases 让 lookupRelease 返回 rel、downloadDeb 写出 body（并记录下载次数），不联网。
func stubReleases(t *testing.T, rel ghRelease, body []byte) *int {
	t.Helper()
	origLookup, origDownload := lookupRelease, downloadDeb
	t.Cleanup(func() { lookupRelease, downloadDeb = origLookup, origDownload })
	downloads := 0
	lookupRelease = func(paths.Paths, string) (ghRelease, error) { return rel, nil }
	downloadDeb = func(p paths.Paths, _ ghRelease, name string) (string, error) {
		downloads++
		out := filepath.Join(p.Downloads, name)
		if err := os.MkdirAll(p.Downloads, 0o755); err != nil {
			return "", err
		}
		return out, os.WriteFile(out, body, 0o644)
	}
	return &downloads
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256Hex(t, string(raw))
}

func TestActivateDebInstallsAndUndoReinstallsCachedPrevious(t *testing.T) {
	f := &fakeDpkg{installed: "0.3.3"}
	stubDpkg(t, f)
	p := paths.FromRoot(t.TempDir())
	previous := cachedDeb(p, "0.3.3")
	writeDeb(t, previous, map[string]string{"./usr/bin/sboxkit": fakeBinary})
	downloads := stubReleases(t, releaseWith(debAssetName("0.3.3"), "sha256:"+fileSHA256(t, previous)), nil)
	tool := &Prepared{Version: "0.4.0", deb: filepath.Join(p.Downloads, debAssetName("0.4.0")), debFresh: true}

	undo, err := tool.Activate(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := undo(); err != nil {
		t.Fatal(err)
	}

	want := []string{"dpkg -i " + tool.deb, "dpkg -i " + previous}
	var installs []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, "dpkg ") {
			installs = append(installs, c)
		}
	}
	if strings.Join(installs, "|") != strings.Join(want, "|") {
		t.Fatalf("dpkg installs = %v, want %v", installs, want)
	}
	if tool.activated {
		t.Fatal("after undo the prepared version is no longer active")
	}
	if *downloads != 0 {
		t.Fatalf("downloads = %d, a cache matching the API digest must be reused", *downloads)
	}
}

func TestCachedDebThatDoesNotMatchTheDigestIsReplaced(t *testing.T) {
	stubDpkg(t, &fakeDpkg{installed: "0.4.0"})
	p := paths.FromRoot(t.TempDir())
	genuine := filepath.Join(t.TempDir(), "genuine.deb")
	writeDeb(t, genuine, map[string]string{"./usr/bin/sboxkit": fakeBinary})
	body, err := os.ReadFile(genuine)
	if err != nil {
		t.Fatal(err)
	}
	// 收归 root 前预埋在旧版（用户属主）状态目录里的 deb 与稳定版记录。
	writeDeb(t, cachedDeb(p, "0.3.3"), map[string]string{"./usr/bin/sboxkit": "#!/bin/sh\necho planted\n"})
	if err := os.WriteFile(lastStableRecord(p), []byte("0.3.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloads := stubReleases(t, releaseWith(debAssetName("0.3.3"), "sha256:"+sha256Hex(t, string(body))), body)

	tool, err := prepareStableDeb(p)
	if err != nil {
		t.Fatal(err)
	}
	defer tool.Discard()

	if *downloads != 1 {
		t.Fatalf("downloads = %d, a cache that fails the digest must be fetched again", *downloads)
	}
	if got := fileSHA256(t, tool.deb); got != sha256Hex(t, string(body)) {
		t.Fatal("stable rollback would install the planted package")
	}
}

func TestReinstallRefusesUnknownPreviousVersion(t *testing.T) {
	stubDpkg(t, &fakeDpkg{})
	p := paths.FromRoot(t.TempDir())
	for _, v := range []string{"", "dev", "0.3.3~SNAPSHOT-2724b50"} {
		if err := reinstallPackage(p, v); err == nil {
			t.Errorf("reinstallPackage(%q) succeeded without a release to install", v)
		}
	}
}

func TestFinishDebCachesPackageRecordsStableAndDropsLegacyVersions(t *testing.T) {
	stubDpkg(t, &fakeDpkg{installed: "0.4.0"})
	p := paths.FromRoot(t.TempDir())
	fakeExecutable(t, versionBin(p, "0.3.3"))
	for _, v := range []string{"0.3.1", "0.3.3", "0.3.10", "0.3.9"} {
		writeDeb(t, cachedDeb(p, v), nil)
	}
	fresh := filepath.Join(p.Downloads, debAssetName("0.4.0"))
	writeDeb(t, fresh, nil)
	tool := &Prepared{Version: "0.4.0", deb: fresh, channel: Stable, activated: true}

	tool.Finish(p)
	// 0.3.10 是 0.4.0 的上一个版本：按数值而非字典序比较。

	if !fileExists(cachedDeb(p, "0.4.0")) {
		t.Fatal("installed package not cached for later rollback")
	}
	if v, ok := lastStableDebVersion(p); !ok || v != "0.4.0" {
		t.Fatalf("last stable = %q, %v", v, ok)
	}
	for v, kept := range map[string]bool{"0.3.1": false, "0.3.3": false, "0.3.9": false, "0.3.10": true} {
		if fileExists(cachedDeb(p, v)) != kept {
			t.Errorf("cached %s kept = %v, want %v", v, !kept, kept)
		}
	}
	if _, err := os.Stat(versionsDir(p)); !os.IsNotExist(err) {
		t.Fatalf("legacy versions dir survived although dpkg owns %s again: %v", packagedExe, err)
	}
}

func TestLegacyVersionsStayWhileTheExecutableStillLinksIntoThem(t *testing.T) {
	stubDpkg(t, &fakeDpkg{})
	p := paths.FromRoot(t.TempDir())
	fakeExecutable(t, versionBin(p, "0.3.3"))
	if err := atomicSymlink(versionBin(p, "0.3.3"), currentLink(p)); err != nil {
		t.Fatal(err)
	}
	os.Remove(packagedExe)
	if err := os.Symlink(currentLink(p), packagedExe); err != nil {
		t.Fatal(err)
	}

	removeLegacyVersions(p)

	if _, err := os.Stat(versionBin(p, "0.3.3")); err != nil {
		t.Fatalf("versions removed while %s still resolves into them: %v", packagedExe, err)
	}
}

func TestPackageManagedNeverInUserMode(t *testing.T) {
	stubDpkg(t, &fakeDpkg{installed: "0.4.0"})

	if defaultPackageManaged(paths.FromUserModeRoot("/k/.sboxkit", "/k")) {
		t.Fatal("user mode bundles update through their own versions directory")
	}
}

// fileFetcher 把固定内容写到目标路径，模拟（可能被篡改的）镜像。
type fileFetcher struct {
	content map[string]string
	urls    []string
}

func (f *fileFetcher) FetchFile(rawURL, out string) error {
	f.urls = append(f.urls, rawURL)
	for suffix, body := range f.content {
		if strings.HasSuffix(rawURL, suffix) {
			return os.WriteFile(out, []byte(body), 0o644)
		}
	}
	return fmt.Errorf("404 %s", rawURL)
}

func releaseWith(name, digest string) ghRelease {
	return ghRelease{TagName: "v0.4.0", Assets: []ghAsset{
		{Name: name, BrowserDownloadURL: "https://github.com/Trilives/sboxkit/releases/download/v0.4.0/" + name, Digest: digest},
		{Name: "checksums.txt", BrowserDownloadURL: "https://github.com/Trilives/sboxkit/releases/download/v0.4.0/checksums.txt"},
	}}
}

func TestDownloadVerifiedTrustsTheAPIDigestNotTheMirror(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	name := debAssetName("0.4.0")
	digest := "sha256:" + sha256Hex(t, "genuine")
	mirror := kernel.Settings{GithubMirror: "https://mirror.example"}

	good := &fileFetcher{content: map[string]string{name: "genuine"}}
	path, err := downloadVerified(p, good, mirror, releaseWith(name, digest), name)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(good.urls[0], "https://mirror.example/") {
		t.Fatalf("download went to %s, want the configured mirror", good.urls[0])
	}
	os.Remove(path)

	// 镜像连同 checksums.txt 一起篡改也无济于事：期望值来自 API。
	bad := &fileFetcher{content: map[string]string{name: "tampered", "checksums.txt": sha256Hex(t, "tampered") + "  " + name}}
	if _, err := downloadVerified(p, bad, mirror, releaseWith(name, digest), name); err == nil {
		t.Fatal("tampered mirror content accepted")
	}
	if fileExists(filepath.Join(p.Downloads, name)) {
		t.Fatal("rejected download left on disk")
	}
}

func TestDownloadVerifiedWithoutDigest(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	name := assetName("0.4.0")
	content := map[string]string{name: "genuine", "checksums.txt": sha256Hex(t, "genuine") + "  " + name + "\n"}

	if _, err := downloadVerified(p, &fileFetcher{content: content}, kernel.Settings{GithubMirror: "https://mirror.example"}, releaseWith(name, ""), name); err == nil {
		t.Fatal("mirror download without an API digest accepted")
	}
	if _, err := downloadVerified(p, &fileFetcher{content: content}, kernel.Settings{}, releaseWith(name, ""), name); err != nil {
		t.Fatalf("direct GitHub download should fall back to checksums.txt: %v", err)
	}
}

func TestAssetNamesMatchGoreleaserArchNames(t *testing.T) {
	for _, name := range []string{assetName("1.0.0"), debAssetName("1.0.0")} {
		if strings.HasSuffix(strings.TrimSuffix(strings.TrimSuffix(name, ".tar.gz"), ".deb"), "_arm") {
			t.Errorf("%s: goreleaser names 32-bit ARM assets armv7", name)
		}
		if !strings.Contains(name, "_linux_"+releaseArch()) {
			t.Errorf("%s does not carry the release arch %s", name, releaseArch())
		}
	}
}

func sha256Hex(t *testing.T, s string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestOlderRanksReleasesAboveTheirPreviews(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.4.0", "0.4.1-beta.1", true},  // 预览版切回更旧的稳定版
		{"0.4.1", "0.4.1-beta.1", false}, // 预览版转正后的稳定版更新
		{"0.4.1-beta.1", "0.4.1", true},
		{"0.3.9", "0.3.10", true},
		{"0.4.1", "0.4.0", false},
		{"0.4.0", "0.4.0", false},
	}
	for _, c := range cases {
		if got := Older(c.a, c.b); got != c.want {
			t.Errorf("Older(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
