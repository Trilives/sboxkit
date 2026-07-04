package updater

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type Manager struct {
	paths    Paths
	remote   Remote
	service  ServiceControl
	verifier Verifier
	arch     string
}

func New(paths Paths, remote Remote, service ServiceControl, verifier Verifier) *Manager {
	if remote == nil {
		remote = NewGitHubRemote(defaultRepo, nil)
	}
	if service == nil {
		service = SystemdService{}
	}
	if verifier == nil {
		verifier = ExecVerifier{}
	}
	return &Manager{
		paths:    paths,
		remote:   remote,
		service:  service,
		verifier: verifier,
		arch:     portableArch(runtime.GOARCH),
	}
}

func (m *Manager) Check(ctx context.Context, currentVersion string) (CheckResult, error) {
	return m.CheckChannel(ctx, currentVersion, ChannelStable)
}

func (m *Manager) CheckChannel(ctx context.Context, currentVersion string, channel Channel) (CheckResult, error) {
	release, err := m.latest(ctx, channel)
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{
		CurrentVersion: currentVersion,
		LatestVersion:  release.Version,
		Available:      compareVersion(release.Version, currentVersion) > 0,
		ArchiveURL:     release.ArchiveURL,
	}, nil
}

func (m *Manager) Apply(ctx context.Context, currentVersion string) (ApplyResult, error) {
	return m.ApplyChannel(ctx, currentVersion, ChannelStable)
}

func (m *Manager) ApplyChannel(ctx context.Context, currentVersion string, channel Channel) (ApplyResult, error) {
	check, err := m.CheckChannel(ctx, currentVersion, channel)
	if err != nil {
		return ApplyResult{}, err
	}
	if !check.Available {
		return ApplyResult{Version: check.LatestVersion, PreviousVersion: currentVersion}, nil
	}
	release, err := m.latest(ctx, channel)
	if err != nil {
		return ApplyResult{}, err
	}
	if release.ArchiveURL == "" {
		return ApplyResult{}, fmt.Errorf("release %s has no portable archive for %s", release.Version, m.arch)
	}
	archive := filepath.Join(m.paths.DownloadsDir, filepath.Base(release.ArchiveURL))
	if err := m.remote.Download(ctx, release.ArchiveURL, archive); err != nil {
		return ApplyResult{}, fmt.Errorf("download archive: %w", err)
	}
	if err := m.verifyArchiveChecksum(ctx, release, archive); err != nil {
		return ApplyResult{}, err
	}

	versionDir := filepath.Join(m.paths.VersionsDir, sanitizeVersion(release.Version))
	if err := replaceVersionDir(archive, versionDir); err != nil {
		return ApplyResult{}, err
	}
	if err := m.verifyVersion(ctx, versionDir); err != nil {
		_ = os.RemoveAll(versionDir)
		return ApplyResult{}, err
	}

	oldTarget, oldVersion := currentTarget(m.paths.CurrentLink)
	if oldVersion == "" {
		oldVersion = currentVersion
	}
	if err := m.service.Stop(ctx); err != nil {
		return ApplyResult{}, fmt.Errorf("stop service: %w", err)
	}
	if err := switchCurrent(m.paths.CurrentLink, versionDir); err != nil {
		return ApplyResult{}, fmt.Errorf("switch current: %w", err)
	}
	if err := m.service.Start(ctx); err != nil {
		rolledBack := rollbackCurrent(ctx, m.service, m.paths.CurrentLink, oldTarget)
		return ApplyResult{Version: release.Version, PreviousVersion: oldVersion, InstalledDir: versionDir, RolledBack: rolledBack}, fmt.Errorf("start service after update: %w", err)
	}
	if err := pruneVersions(m.paths.VersionsDir, versionDir, oldTarget); err != nil {
		return ApplyResult{}, err
	}
	return ApplyResult{Version: release.Version, PreviousVersion: oldVersion, InstalledDir: versionDir}, nil
}

func (m *Manager) latest(ctx context.Context, channel Channel) (Release, error) {
	if channel == "" {
		channel = ChannelStable
	}
	if remote, ok := m.remote.(ChannelRemote); ok {
		return remote.LatestChannel(ctx, m.arch, channel)
	}
	return m.remote.Latest(ctx, m.arch)
}

func (m *Manager) verifyArchiveChecksum(ctx context.Context, release Release, archive string) error {
	want := strings.TrimSpace(release.SHA256)
	if want == "" && release.SHA256URL != "" {
		sumFile := archive + ".sha256"
		if err := m.remote.Download(ctx, release.SHA256URL, sumFile); err != nil {
			return fmt.Errorf("download sha256: %w", err)
		}
		data, err := os.ReadFile(sumFile)
		if err != nil {
			return err
		}
		want = parseSHA256(string(data))
	}
	if want == "" {
		return fmt.Errorf("release %s has no SHA-256 checksum", release.Version)
	}
	got, err := fileSHA256(archive)
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("SHA-256 mismatch for %s: got %s want %s", archive, got, want)
	}
	return nil
}

func (m *Manager) verifyVersion(ctx context.Context, versionDir string) error {
	if err := m.verifier.Verify(ctx, filepath.Join(versionDir, "sboxkit"), "version"); err != nil {
		return fmt.Errorf("verify sboxkit: %w", err)
	}
	if err := m.verifier.Verify(ctx, filepath.Join(versionDir, "sing-box"), "version"); err != nil {
		return fmt.Errorf("verify sing-box: %w", err)
	}
	return nil
}

func replaceVersionDir(archive string, versionDir string) error {
	if err := os.RemoveAll(versionDir); err != nil {
		return err
	}
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return err
	}
	if err := extractPortableArchive(archive, versionDir); err != nil {
		_ = os.RemoveAll(versionDir)
		return err
	}
	return nil
}

func rollbackCurrent(ctx context.Context, service ServiceControl, link string, oldTarget string) bool {
	if oldTarget == "" {
		return false
	}
	if err := switchCurrent(link, oldTarget); err != nil {
		return false
	}
	_ = service.Start(ctx)
	return true
}

func switchCurrent(link string, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	tmp := link + ".next"
	_ = os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, link)
}

func currentTarget(link string) (string, string) {
	target, err := os.Readlink(link)
	if err != nil {
		return "", ""
	}
	return target, filepath.Base(target)
}

func pruneVersions(versionsDir string, current string, previous string) error {
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return err
	}
	keep := map[string]bool{
		filepath.Clean(current):  true,
		filepath.Clean(previous): true,
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(versionsDir, entry.Name()))
		}
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		if keep[filepath.Clean(dir)] {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}
