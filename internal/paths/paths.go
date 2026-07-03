package paths

import (
	"os"
	"path/filepath"
)

type Paths struct {
	Root             string
	TemplatesDir     string
	StateDir         string
	BinDir           string
	SingBoxBin       string
	SingBoxVersion   string
	UIDir            string
	RulesetDir       string
	DownloadsDir     string
	LogDir           string
	SubscriptionsDir string
	ActiveFile       string
	ConfigFile       string
	CustomizeFile    string
	EtcDir           string
	SystemSingBoxBin string
	GeositeCN        string
	GeoIPCN          string
}

func FromRoot(root string) Paths {
	if root == "" {
		root = DefaultRoot()
	}
	root = filepath.Clean(root)
	state := filepath.Join(root, "state")
	bin := filepath.Join(state, "bin")
	ruleset := filepath.Join(state, "ruleset")

	return Paths{
		Root:             root,
		TemplatesDir:     filepath.Join(root, "templates"),
		StateDir:         state,
		BinDir:           bin,
		SingBoxBin:       filepath.Join(bin, "sing-box"),
		SingBoxVersion:   filepath.Join(bin, "sing-box.version"),
		UIDir:            filepath.Join(state, "ui"),
		RulesetDir:       ruleset,
		DownloadsDir:     filepath.Join(state, "downloads"),
		LogDir:           filepath.Join(state, "logs"),
		SubscriptionsDir: filepath.Join(state, "subscriptions"),
		ActiveFile:       filepath.Join(state, "active"),
		ConfigFile:       filepath.Join(state, "config.json"),
		CustomizeFile:    filepath.Join(state, "customize.json"),
		EtcDir:           "/etc/sboxkit",
		SystemSingBoxBin: "/usr/lib/sboxkit/sing-box",
		GeositeCN:        filepath.Join(ruleset, "geosite-cn.srs"),
		GeoIPCN:          filepath.Join(ruleset, "geoip-cn.srs"),
	}
}

func DefaultRoot() string {
	if value := os.Getenv("SBOXKIT_ROOT"); value != "" {
		return value
	}
	if value := os.Getenv("XDG_STATE_HOME"); value != "" {
		return filepath.Join(value, "sboxkit")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "state", "sboxkit")
	}
	return filepath.Join(os.TempDir(), "sboxkit")
}

func (p Paths) EnsureStateDirs() error {
	for _, dir := range []string{
		p.StateDir,
		p.BinDir,
		p.UIDir,
		p.RulesetDir,
		p.DownloadsDir,
		p.LogDir,
		p.SubscriptionsDir,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
