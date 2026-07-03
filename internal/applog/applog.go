package applog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	DefaultMaxMB    = 10
	HardMaxMB       = 100
	DefaultMaxBytes = int64(DefaultMaxMB * 1024 * 1024)
	HardMaxBytes    = int64(HardMaxMB * 1024 * 1024)
)

type Config struct {
	Enabled  bool
	MaxBytes int64
}

func Open(dir string, cfg Config, stderr io.Writer) (io.Writer, func() error, error) {
	if !cfg.Enabled {
		return stderr, func() error { return nil }, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return stderr, nil, err
	}
	if err := Prune(dir, MaxBytesFromBytes(cfg.MaxBytes)); err != nil {
		return stderr, nil, err
	}
	path := filepath.Join(dir, "sboxkit-"+time.Now().UTC().Format("20060102-150405")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return stderr, nil, err
	}
	writer := io.MultiWriter(stderr, file)
	return writer, func() error {
		err := file.Close()
		if pruneErr := Prune(dir, MaxBytesFromBytes(cfg.MaxBytes)); err == nil {
			err = pruneErr
		}
		return err
	}, nil
}

func WriteHeader(writer io.Writer, args []string) {
	fmt.Fprintf(writer, "\n[%s] sboxkit %v\n", time.Now().UTC().Format(time.RFC3339), args)
}

func WriteFooter(writer io.Writer, code int) {
	fmt.Fprintf(writer, "[%s] exit code: %d\n", time.Now().UTC().Format(time.RFC3339), code)
}

func MaxBytes(maxMB int) int64 {
	if maxMB <= 0 {
		return DefaultMaxBytes
	}
	if maxMB > HardMaxMB {
		return HardMaxBytes
	}
	return int64(maxMB) * 1024 * 1024
}

func MaxBytesFromBytes(maxBytes int64) int64 {
	if maxBytes <= 0 {
		return DefaultMaxBytes
	}
	if maxBytes > HardMaxBytes {
		return HardMaxBytes
	}
	return maxBytes
}

func Prune(dir string, maxBytes int64) error {
	maxBytes = MaxBytesFromBytes(maxBytes)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	logs := make([]logFile, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		path := filepath.Join(dir, entry.Name())
		logs = append(logs, logFile{path: path, size: info.Size(), modified: info.ModTime()})
		total += info.Size()
	}
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].modified.Before(logs[j].modified)
	})
	for _, log := range logs {
		if total <= maxBytes {
			break
		}
		if err := os.Remove(log.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= log.size
	}
	return nil
}

type logFile struct {
	path     string
	size     int64
	modified time.Time
}
