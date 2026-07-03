package updater

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func parseSHA256(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 || len(fields[0]) != 64 {
		return ""
	}
	return fields[0]
}

func extractPortableArchive(archive string, outDir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := map[string]bool{}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.FileInfo().IsDir() {
			continue
		}
		name := filepath.Base(header.Name)
		if name != "sboxkit" && name != "sing-box" {
			continue
		}
		if err := writeTarFile(filepath.Join(outDir, name), header.FileInfo().Mode()|0o755, tr); err != nil {
			return err
		}
		found[name] = true
	}
	for _, name := range []string{"sboxkit", "sing-box"} {
		if !found[name] {
			return fmt.Errorf("%s not found in portable archive", name)
		}
	}
	return nil
}

func writeTarFile(out string, mode os.FileMode, src io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, src); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
