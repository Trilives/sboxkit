package updater

import (
	"context"
	"path/filepath"
)

const defaultRepo = "Trilives/sboxkit"

type Paths struct {
	InstallDir   string
	VersionsDir  string
	CurrentLink  string
	DownloadsDir string
}

func PathsForRoot(root string) Paths {
	installDir := filepath.Join(root, "sboxkit")
	return Paths{
		InstallDir:   installDir,
		VersionsDir:  filepath.Join(installDir, "versions"),
		CurrentLink:  filepath.Join(installDir, "current"),
		DownloadsDir: filepath.Join(installDir, "downloads"),
	}
}

func DefaultPaths() Paths {
	return Paths{
		InstallDir:   "/usr/lib/sboxkit",
		VersionsDir:  "/usr/lib/sboxkit/versions",
		CurrentLink:  "/usr/lib/sboxkit/current",
		DownloadsDir: "/var/cache/sboxkit/self-update",
	}
}

type Release struct {
	Version    string
	ArchiveURL string
	SHA256URL  string
	SHA256     string
}

type CheckResult struct {
	CurrentVersion string
	LatestVersion  string
	Available      bool
	ArchiveURL     string
}

type ApplyResult struct {
	Version         string
	PreviousVersion string
	InstalledDir    string
	RolledBack      bool
}

type Remote interface {
	Latest(ctx context.Context, arch string) (Release, error)
	Download(ctx context.Context, rawURL string, out string) error
}

type ServiceControl interface {
	Stop(ctx context.Context) error
	Start(ctx context.Context) error
}

type Verifier interface {
	Verify(ctx context.Context, path string, args ...string) error
}
