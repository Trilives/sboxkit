package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/converter"
	"github.com/Trilives/sboxkit/internal/paths"
)

func ToSingBox(raw []byte, source SourceKind, customize bool, cfg config.Config, p paths.Paths) (converter.Config, converter.Info, error) {
	text := string(raw)
	switch source {
	case SourceClash:
		return converter.ClashToSingBox(text, cfg, p)
	case SourceSingBox:
		return converter.SingBoxDirect(text, cfg, p, customize)
	case SourceBase64:
		return base64ToSingBox(text, cfg, p)
	default:
		return converter.Config{}, nil, fmt.Errorf("unknown source type %q", source)
	}
}

func base64ToSingBox(text string, cfg config.Config, p paths.Paths) (converter.Config, converter.Info, error) {
	if cfg.SubconverterBackend != "" {
		clash, err := toClashViaSubconverter(text, cfg.SubconverterBackend, cfg.DownloadProxy)
		if err == nil {
			return converter.ClashToSingBox(clash, cfg, p)
		}
		if !cfg.Base64LocalFallback {
			return converter.Config{}, nil, fmt.Errorf("subconverter failed: %w", err)
		}
	}
	proxies, err := localBase64ToClash(text)
	if err != nil {
		return converter.Config{}, nil, err
	}
	return converter.ClashToSingBox(proxies, cfg, p)
}

func toClashViaSubconverter(rawText string, backend string, proxy string) (string, error) {
	backend = strings.TrimRight(backend, "/")
	escaped := url.QueryEscape(strings.TrimSpace(rawText))
	data, err := Fetch(backend+"/sub?target=clash&list=false&url="+escaped, SourceClash, proxy)
	if err != nil {
		return "", err
	}
	text := string(data)
	if !strings.Contains(text, "proxies:") {
		return "", fmt.Errorf("subconverter response does not contain proxies")
	}
	return text, nil
}

func decodeBase64Text(text string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(text, "-", "+"), "_", "/"))
	if rem := len(clean) % 4; rem != 0 {
		clean += strings.Repeat("=", 4-rem)
	}
	data, err := base64.StdEncoding.DecodeString(clean)
	if err != nil {
		return text
	}
	return string(data)
}

func localBase64ToClash(text string) (string, error) {
	decoded := decodeBase64Text(text)
	lines := []string{}
	for _, line := range strings.Split(decoded, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "://") {
			continue
		}
		proxy := parseShareLink(line)
		if proxy != "" {
			lines = append(lines, proxy)
		}
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("local base64 parser found no supported nodes")
	}
	return "proxies:\n" + strings.Join(lines, "\n"), nil
}

func parseShareLink(line string) string {
	if strings.HasPrefix(line, "ss://") {
		return parseSSLink(line)
	}
	return ""
}

func parseSSLink(line string) string {
	body := strings.TrimPrefix(line, "ss://")
	name := "ss"
	if idx := strings.Index(body, "#"); idx >= 0 {
		if decoded, err := url.QueryUnescape(body[idx+1:]); err == nil && decoded != "" {
			name = decoded
		}
		body = body[:idx]
	}
	if strings.Contains(body, "?") {
		body = strings.SplitN(body, "?", 2)[0]
	}
	var creds, server string
	if strings.Contains(body, "@") {
		parts := strings.SplitN(body, "@", 2)
		creds = decodeBase64Text(parts[0])
		server = parts[1]
	} else {
		decoded := decodeBase64Text(body)
		parts := strings.SplitN(decoded, "@", 2)
		if len(parts) != 2 {
			return ""
		}
		creds, server = parts[0], parts[1]
	}
	credParts := strings.SplitN(creds, ":", 2)
	serverParts := strings.Split(server, ":")
	if len(credParts) != 2 || len(serverParts) < 2 {
		return ""
	}
	return fmt.Sprintf("  - {name: %q, type: ss, server: %q, port: %s, cipher: %q, password: %q}", name, serverParts[0], serverParts[1], credParts[0], credParts[1])
}
