package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errFakeStart = errors.New("fake start failure")

func TestApplyInstallsVersionSwitchesCurrentAndKeepsOneOldVersion(t *testing.T) {
	root := t.TempDir()
	paths := PathsForRoot(root)
	oldVersion := filepath.Join(paths.VersionsDir, "0.1.0")
	olderVersion := filepath.Join(paths.VersionsDir, "0.0.9")
	writeExecutable(t, filepath.Join(oldVersion, "sboxkit"), "old")
	writeExecutable(t, filepath.Join(oldVersion, "sing-box"), "old-core")
	writeExecutable(t, filepath.Join(olderVersion, "sboxkit"), "older")
	writeExecutable(t, filepath.Join(olderVersion, "sing-box"), "older-core")
	linkCurrent(t, paths.CurrentLink, oldVersion)

	archive, sum := createPortableArchive(t, root, "new", "new-core")
	remote := &fakeRemote{release: Release{
		Version:    "0.2.0",
		ArchiveURL: archive,
		SHA256:     sum,
	}}
	services := &fakeServiceControl{}
	verifier := &fakeVerifier{}

	result, err := New(paths, remote, services, verifier).Apply(context.Background(), "0.1.0")
	if err != nil {
		t.Fatalf("apply update: %v", err)
	}

	if result.PreviousVersion != "0.1.0" || result.Version != "0.2.0" {
		t.Fatalf("unexpected result %#v", result)
	}
	current, err := os.Readlink(paths.CurrentLink)
	if err != nil {
		t.Fatalf("read current link: %v", err)
	}
	if filepath.Base(current) != "0.2.0" {
		t.Fatalf("current link = %s, want 0.2.0", current)
	}
	if got := strings.Join(services.calls, ","); got != "stop,start" {
		t.Fatalf("service calls = %s, want stop,start", got)
	}
	for _, binary := range []string{"sboxkit", "sing-box"} {
		if !verifier.saw(filepath.Join(paths.VersionsDir, "0.2.0", binary)) {
			t.Fatalf("expected verifier to run %s", binary)
		}
	}
	if _, err := os.Stat(olderVersion); !os.IsNotExist(err) {
		t.Fatalf("older version should be pruned, stat err=%v", err)
	}
	if _, err := os.Stat(oldVersion); err != nil {
		t.Fatalf("previous version should remain: %v", err)
	}
}

func TestApplyRejectsSHA256MismatchBeforeStoppingService(t *testing.T) {
	root := t.TempDir()
	paths := PathsForRoot(root)
	oldVersion := filepath.Join(paths.VersionsDir, "0.1.0")
	writeExecutable(t, filepath.Join(oldVersion, "sboxkit"), "old")
	writeExecutable(t, filepath.Join(oldVersion, "sing-box"), "old-core")
	linkCurrent(t, paths.CurrentLink, oldVersion)

	archive, _ := createPortableArchive(t, root, "new", "new-core")
	remote := &fakeRemote{release: Release{
		Version:    "0.2.0",
		ArchiveURL: archive,
		SHA256:     strings.Repeat("0", 64),
	}}
	services := &fakeServiceControl{}

	if _, err := New(paths, remote, services, &fakeVerifier{}).Apply(context.Background(), "0.1.0"); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if len(services.calls) != 0 {
		t.Fatalf("service should not be touched on checksum failure: %#v", services.calls)
	}
	current, _ := os.Readlink(paths.CurrentLink)
	if current != oldVersion {
		t.Fatalf("current link changed on checksum failure: %s", current)
	}
}

func TestApplyRollsBackWhenNewServiceFailsToStart(t *testing.T) {
	root := t.TempDir()
	paths := PathsForRoot(root)
	oldVersion := filepath.Join(paths.VersionsDir, "0.1.0")
	writeExecutable(t, filepath.Join(oldVersion, "sboxkit"), "old")
	writeExecutable(t, filepath.Join(oldVersion, "sing-box"), "old-core")
	linkCurrent(t, paths.CurrentLink, oldVersion)

	archive, sum := createPortableArchive(t, root, "new", "new-core")
	services := &fakeServiceControl{failFirstStart: true}
	_, err := New(paths, &fakeRemote{release: Release{
		Version:    "0.2.0",
		ArchiveURL: archive,
		SHA256:     sum,
	}}, services, &fakeVerifier{}).Apply(context.Background(), "0.1.0")
	if err == nil {
		t.Fatal("expected start failure")
	}

	current, _ := os.Readlink(paths.CurrentLink)
	if current != oldVersion {
		t.Fatalf("current link = %s, want rollback to %s", current, oldVersion)
	}
	if got := strings.Join(services.calls, ","); got != "stop,start,start" {
		t.Fatalf("service calls = %s, want stop,start,start", got)
	}
}

func TestCheckReportsNoUpdateWhenLatestIsNotNewer(t *testing.T) {
	paths := PathsForRoot(t.TempDir())
	manager := New(paths, &fakeRemote{release: Release{Version: "0.1.0"}}, &fakeServiceControl{}, &fakeVerifier{})

	result, err := manager.Check(context.Background(), "0.1.0")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if result.Available {
		t.Fatalf("expected no update, got %#v", result)
	}
}

func TestCheckPreviewUsesPreviewChannel(t *testing.T) {
	paths := PathsForRoot(t.TempDir())
	remote := &fakeRemote{releases: map[Channel]Release{
		ChannelStable:  {Version: "0.2.0"},
		ChannelPreview: {Version: "0.2.1-beta.1", ArchiveURL: "preview.tar.gz"},
	}}
	manager := New(paths, remote, &fakeServiceControl{}, &fakeVerifier{})

	result, err := manager.CheckChannel(context.Background(), "0.2.0", ChannelPreview)
	if err != nil {
		t.Fatalf("check preview: %v", err)
	}
	if !result.Available || result.LatestVersion != "0.2.1-beta.1" || result.ArchiveURL != "preview.tar.gz" {
		t.Fatalf("unexpected preview result %#v", result)
	}
	if remote.lastChannel != ChannelPreview {
		t.Fatalf("remote channel = %s, want %s", remote.lastChannel, ChannelPreview)
	}
}

type fakeRemote struct {
	release     Release
	releases    map[Channel]Release
	lastChannel Channel
}

func (r *fakeRemote) Latest(ctx context.Context, arch string) (Release, error) {
	return r.LatestChannel(ctx, arch, ChannelStable)
}

func (r *fakeRemote) LatestChannel(ctx context.Context, arch string, channel Channel) (Release, error) {
	r.lastChannel = channel
	if r.releases != nil {
		return r.releases[channel], nil
	}
	return r.release, nil
}

func (r *fakeRemote) Download(ctx context.Context, rawURL string, out string) error {
	data, err := os.ReadFile(rawURL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, data, 0o644)
}

type fakeServiceControl struct {
	calls          []string
	failFirstStart bool
}

func (s *fakeServiceControl) Stop(ctx context.Context) error {
	s.calls = append(s.calls, "stop")
	return nil
}

func (s *fakeServiceControl) Start(ctx context.Context) error {
	s.calls = append(s.calls, "start")
	if s.failFirstStart {
		s.failFirstStart = false
		return errFakeStart
	}
	return nil
}

type fakeVerifier struct {
	paths []string
}

func (v *fakeVerifier) Verify(ctx context.Context, path string, args ...string) error {
	v.paths = append(v.paths, path)
	return nil
}

func (v *fakeVerifier) saw(path string) bool {
	for _, candidate := range v.paths {
		if candidate == path {
			return true
		}
	}
	return false
}

func createPortableArchive(t *testing.T, root string, sboxkit string, singBox string) (string, string) {
	t.Helper()
	archive := filepath.Join(root, "sboxkit_0.2.0_amd64_portable.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"sboxkit_0.2.0_amd64/sboxkit":  sboxkit,
		"sboxkit_0.2.0_amd64/sing-box": singBox,
	} {
		header := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	sum := sha256.Sum256(data)
	return archive, hex.EncodeToString(sum[:])
}

func writeExecutable(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}

func linkCurrent(t *testing.T, link string, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("mkdir link parent: %v", err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("link current: %v", err)
	}
}
