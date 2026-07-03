package subscription

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/uiassets"
)

type Subscription struct {
	Name          string     `json:"name"`
	URL           string     `json:"url"`
	File          string     `json:"file,omitempty"`
	SourceType    SourceKind `json:"source_type"`
	Customize     bool       `json:"customize"`
	Converter     string     `json:"converter"`
	CreatedAt     string     `json:"created_at"`
	UpdatedAt     string     `json:"updated_at"`
	LastNodeCount int        `json:"last_node_count"`
}

type Manager struct {
	paths  paths.Paths
	config config.Config
	fetch  func(rawURL string, source SourceKind, proxy string) ([]byte, error)
}

func NewManager(p paths.Paths, cfg config.Config) *Manager {
	return &Manager{paths: p, config: cfg, fetch: Fetch}
}

func (m *Manager) Add(name string, rawURL string, source SourceKind, customize bool, setActive bool) (Subscription, error) {
	name = slug(name)
	if name == "" {
		name = "sub"
	}
	if _, err := os.Stat(m.metaFile(name)); err == nil {
		return Subscription{}, fmt.Errorf("subscription %q already exists", name)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sub := Subscription{
		Name: name, URL: rawURL, SourceType: source, Customize: customize,
		Converter: "local", CreatedAt: now, UpdatedAt: now,
	}
	if err := m.build(&sub, true); err != nil {
		return Subscription{}, err
	}
	if setActive {
		if err := m.Switch(name); err != nil {
			return Subscription{}, err
		}
	}
	return sub, nil
}

func (m *Manager) AddFile(name string, sourcePath string, source SourceKind, customize bool, setActive bool) (Subscription, error) {
	name = slug(name)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
		name = slug(name)
	}
	if name == "" {
		name = "local-config"
	}
	if _, err := os.Stat(m.metaFile(name)); err == nil {
		return Subscription{}, fmt.Errorf("subscription %q already exists", name)
	}
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return Subscription{}, fmt.Errorf("read config file: %w", err)
	}
	if source == "" {
		source, err = Detect(raw)
		if err != nil {
			return Subscription{}, fmt.Errorf("detect config file type: %w", err)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	sub := Subscription{
		Name: name, File: sourcePath, SourceType: source, Customize: customize,
		Converter: "local", CreatedAt: now, UpdatedAt: now,
	}
	if err := os.MkdirAll(m.subDir(sub.Name), 0o755); err != nil {
		return Subscription{}, err
	}
	if err := atomicWrite(m.rawFile(sub), raw, 0o600); err != nil {
		return Subscription{}, err
	}
	if err := copyFile(sourcePath, m.originalFile(sub)); err != nil {
		return Subscription{}, err
	}
	if err := m.convertAndWrite(&sub, raw); err != nil {
		return Subscription{}, err
	}
	if setActive {
		if err := m.Switch(name); err != nil {
			return Subscription{}, err
		}
	}
	return sub, nil
}

func (m *Manager) Refresh(name string) (Subscription, error) {
	sub, err := m.Get(name)
	if err != nil {
		return Subscription{}, err
	}
	sub.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := m.build(sub, true); err != nil {
		return Subscription{}, err
	}
	if active, _ := m.Active(); active != nil && active.Name == name {
		if err := m.Switch(name); err != nil {
			return Subscription{}, err
		}
	}
	return *sub, nil
}

func (m *Manager) Rebuild(name string) (Subscription, error) {
	sub, err := m.Get(name)
	if err != nil {
		return Subscription{}, err
	}
	raw, err := os.ReadFile(m.rawFile(*sub))
	if err != nil {
		return m.Refresh(name)
	}
	sub.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := m.convertAndWrite(sub, raw); err != nil {
		return Subscription{}, err
	}
	if active, _ := m.Active(); active != nil && active.Name == name {
		if err := m.Switch(name); err != nil {
			return Subscription{}, err
		}
	}
	return *sub, nil
}

func (m *Manager) List() ([]Subscription, error) {
	entries, err := os.ReadDir(m.paths.SubscriptionsDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	subs := []Subscription{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sub, err := m.Get(entry.Name())
		if err == nil {
			subs = append(subs, *sub)
		}
	}
	return subs, nil
}

func (m *Manager) Get(name string) (*Subscription, error) {
	data, err := os.ReadFile(m.metaFile(name))
	if err != nil {
		return nil, err
	}
	var sub Subscription
	if err := json.Unmarshal(data, &sub); err != nil {
		return nil, err
	}
	return &sub, nil
}

func (m *Manager) Active() (*Subscription, error) {
	data, err := os.ReadFile(m.paths.ActiveFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return m.Get(strings.TrimSpace(string(data)))
}

func (m *Manager) Switch(name string) error {
	if _, err := os.Stat(m.configFile(name)); err != nil {
		return fmt.Errorf("subscription %q is not ready: %w", name, err)
	}
	if err := m.paths.EnsureStateDirs(); err != nil {
		return err
	}
	configData, err := os.ReadFile(m.configFile(name))
	if err != nil {
		return err
	}
	if err := atomicWrite(m.paths.ConfigFile, configData, 0o644); err != nil {
		return err
	}
	return atomicWrite(m.paths.ActiveFile, []byte(name+"\n"), 0o644)
}

func (m *Manager) Remove(name string) error {
	wasActive := false
	if active, _ := m.Active(); active != nil && active.Name == name {
		wasActive = true
	}
	if err := os.RemoveAll(m.subDir(name)); err != nil {
		return err
	}
	if wasActive {
		_ = os.Remove(m.paths.ActiveFile)
	}
	return nil
}

func (m *Manager) Rename(oldName string, newName string) error {
	newName = slug(newName)
	if _, err := os.Stat(m.metaFile(newName)); err == nil {
		return fmt.Errorf("target subscription %q already exists", newName)
	}
	if err := os.Rename(m.subDir(oldName), m.subDir(newName)); err != nil {
		return err
	}
	sub, err := m.Get(newName)
	if err != nil {
		return err
	}
	sub.Name = newName
	sub.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := m.writeMeta(*sub); err != nil {
		return err
	}
	if active, _ := m.Active(); active != nil && active.Name == oldName {
		return atomicWrite(m.paths.ActiveFile, []byte(newName+"\n"), 0o644)
	}
	return nil
}

func (m *Manager) build(sub *Subscription, fetchRemote bool) error {
	var raw []byte
	var err error
	if fetchRemote {
		raw, err = m.fetch(sub.URL, sub.SourceType, m.config.DownloadProxy)
		if err != nil {
			return err
		}
		if found, err := Detect(raw); err == nil && found != sub.SourceType {
			// Keep the declared type; callers may intentionally choose a converter path.
		}
	} else {
		raw, err = os.ReadFile(m.rawFile(*sub))
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(m.subDir(sub.Name), 0o755); err != nil {
		return err
	}
	if err := atomicWrite(m.rawFile(*sub), raw, 0o600); err != nil {
		return err
	}
	return m.convertAndWrite(sub, raw)
}

func (m *Manager) convertAndWrite(sub *Subscription, raw []byte) error {
	if m.config.LanPanel {
		if err := uiassets.Write(m.paths.UIDir); err != nil {
			return err
		}
	}
	cfg, info, err := ToSingBox(raw, sub.SourceType, sub.Customize, m.config, m.paths)
	if err != nil {
		return err
	}
	if converted, ok := info["converted"].(int); ok {
		sub.LastNodeCount = converted
	} else if autoCount, ok := info["auto_count"].(int); ok {
		sub.LastNodeCount = autoCount
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := atomicWrite(m.configFile(sub.Name), data, 0o600); err != nil {
		return err
	}
	return m.writeMeta(*sub)
}

func (m *Manager) writeMeta(sub Subscription) error {
	data, err := json.MarshalIndent(sub, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWrite(m.metaFile(sub.Name), data, 0o600)
}

func (m *Manager) subDir(name string) string {
	return filepath.Join(m.paths.SubscriptionsDir, name)
}

func (m *Manager) metaFile(name string) string {
	return filepath.Join(m.subDir(name), "meta.json")
}

func (m *Manager) configFile(name string) string {
	return filepath.Join(m.subDir(name), "config.json")
}

func (m *Manager) rawFile(sub Subscription) string {
	ext := map[SourceKind]string{SourceClash: "yaml", SourceSingBox: "json", SourceBase64: "txt"}[sub.SourceType]
	if ext == "" {
		ext = "txt"
	}
	return filepath.Join(m.subDir(sub.Name), "raw."+ext)
}

func (m *Manager) originalFile(sub Subscription) string {
	ext := strings.TrimPrefix(filepath.Ext(sub.File), ".")
	if ext == "" {
		ext = strings.TrimPrefix(filepath.Ext(m.rawFile(sub)), ".")
	}
	if ext == "" {
		ext = "config"
	}
	return filepath.Join(m.subDir(sub.Name), "source."+ext)
}

func slug(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, "..", "-")
	name = strings.Join(strings.Fields(name), "-")
	return strings.Trim(name, ". ")
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func copyFile(src string, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return atomicWrite(dst, data, 0o600)
}
