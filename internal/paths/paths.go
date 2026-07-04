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
	ActivationsDir   string
	RuntimeLink      string
	SingBoxDir       string
	SingBoxCacheDB   string
	OperationLock    string
	ActiveFile       string
	ConfigFile       string
	CustomizeFile    string
	EtcDir           string
	AdminConfigFile  string
	SystemSingBoxBin string
	SystemUIDir      string
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
	cacheRoot := filepath.Join(root, "cache")
	runRoot := filepath.Join(root, "run")
	if root == "/var/lib/sboxkit" {
		cacheRoot = "/var/cache/sboxkit"
		runRoot = "/run/sboxkit"
	}
	singBox := filepath.Join(root, "sing-box")

	return Paths{
		Root:             root,
		TemplatesDir:     filepath.Join(root, "templates"),
		StateDir:         state,
		BinDir:           bin,
		SingBoxBin:       filepath.Join(bin, "sing-box"),
		SingBoxVersion:   filepath.Join(bin, "sing-box.version"),
		UIDir:            filepath.Join(state, "ui"),
		RulesetDir:       ruleset,
		DownloadsDir:     filepath.Join(cacheRoot, "downloads"),
		LogDir:           filepath.Join(state, "logs"),
		SubscriptionsDir: filepath.Join(state, "subscriptions"),
		ActivationsDir:   filepath.Join(root, "activations"),
		RuntimeLink:      filepath.Join(root, "runtime"),
		SingBoxDir:       singBox,
		SingBoxCacheDB:   filepath.Join(singBox, "cache.db"),
		OperationLock:    filepath.Join(runRoot, "operation.lock"),
		ActiveFile:       filepath.Join(state, "active"),
		ConfigFile:       filepath.Join(state, "config.json"),
		CustomizeFile:    filepath.Join(state, "customize.json"),
		EtcDir:           "/etc/sboxkit",
		AdminConfigFile:  "/etc/sboxkit/config.json",
		SystemSingBoxBin: "/usr/lib/sboxkit/sing-box",
		SystemUIDir:      "/usr/share/sboxkit/ui",
		GeositeCN:        filepath.Join(ruleset, "geosite-cn.srs"),
		GeoIPCN:          filepath.Join(ruleset, "geoip-cn.srs"),
	}
}

func DefaultRoot() string {
	if value := os.Getenv("SBOXKIT_ROOT"); value != "" {
		return value
	}
	return "/var/lib/sboxkit"
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
		p.ActivationsDir,
		p.SingBoxDir,
		filepath.Dir(p.OperationLock),
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
