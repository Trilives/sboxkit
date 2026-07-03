package proxyenv

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	ProxyHost = "127.0.0.1"
	ProxyPort = 7890

	beginMarker = "# >>> sboxkit proxy env >>>"
	endMarker   = "# <<< sboxkit proxy env <<<"
)

func TargetBashrc() string {
	if override := os.Getenv("SBOXKIT_PROXY_ENV_FILE"); override != "" {
		return override
	}
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" && sudoUser != "root" {
		if u, err := user.Lookup(sudoUser); err == nil && u.HomeDir != "" {
			return filepath.Join(u.HomeDir, ".bashrc")
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".bashrc")
	}
	return ".bashrc"
}

func Write(path string) error {
	if path == "" {
		path = TargetBashrc()
	}
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read proxy env target: %w", err)
	}
	body := strings.TrimRight(stripBlock(string(old)), "\n")
	text := block()
	if body != "" {
		text = body + "\n\n" + text
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create proxy env target dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(text+"\n"), 0o644); err != nil {
		return fmt.Errorf("write proxy env target: %w", err)
	}
	return nil
}

func Remove(path string) error {
	if path == "" {
		path = TargetBashrc()
	}
	old, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read proxy env target: %w", err)
	}
	next := strings.TrimRight(stripBlock(string(old)), "\n")
	if next != "" {
		next += "\n"
	}
	if err := os.WriteFile(path, []byte(next), 0o644); err != nil {
		return fmt.Errorf("write proxy env target: %w", err)
	}
	return nil
}

func block() string {
	http := fmt.Sprintf("http://%s:%d", ProxyHost, ProxyPort)
	socks := fmt.Sprintf("socks5://%s:%d", ProxyHost, ProxyPort)
	lines := []string{
		beginMarker,
		fmt.Sprintf("export http_proxy=%q", http),
		fmt.Sprintf("export https_proxy=%q", http),
		fmt.Sprintf("export all_proxy=%q", socks),
		`export HTTP_PROXY="$http_proxy"`,
		`export HTTPS_PROXY="$https_proxy"`,
		`export ALL_PROXY="$all_proxy"`,
		`export no_proxy="localhost,127.0.0.1,::1"`,
		`export NO_PROXY="$no_proxy"`,
		endMarker,
	}
	return strings.Join(lines, "\n")
}

func stripBlock(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case beginMarker:
			skip = true
			continue
		case endMarker:
			skip = false
			continue
		}
		if !skip {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}
