package uiassets

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/*
var embedded embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(embedded, "assets")
	if err != nil {
		panic(err)
	}
	return sub
}

func Write(outDir string) error {
	if err := os.RemoveAll(outDir); err != nil {
		return fmt.Errorf("clear embedded UI dir: %w", err)
	}
	source := FS()
	return fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == "." {
			return os.MkdirAll(outDir, 0o755)
		}
		clean := filepath.Clean(path)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("invalid embedded UI path %q", path)
		}
		target := filepath.Join(outDir, clean)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
