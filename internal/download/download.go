package download

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	singBoxRepo  = "SagerNet/sing-box"
	geositeCNURL = "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs"
	geoipCNURL   = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs"
)

type Client struct {
	HTTP *http.Client
}

func NewClient(cfg config.Config) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.DownloadProxy != "" {
		proxyURL, err := url.Parse(cfg.DownloadProxy)
		if err != nil {
			return nil, fmt.Errorf("parse download proxy: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &Client{HTTP: &http.Client{Transport: transport, Timeout: 180 * time.Second}}, nil
}

func DownloadAll(ctx context.Context, p paths.Paths, cfg config.Config, force bool) error {
	if err := p.EnsureStateDirs(); err != nil {
		return err
	}
	client, err := NewClient(cfg)
	if err != nil {
		return err
	}
	if err := client.DownloadSingBox(ctx, p, cfg, force); err != nil {
		return err
	}
	if err := client.DownloadRules(ctx, p, cfg, force); err != nil {
		return err
	}
	return nil
}

func DownloadRuntimeAssets(ctx context.Context, p paths.Paths, cfg config.Config, force bool) error {
	if err := p.EnsureStateDirs(); err != nil {
		return err
	}
	client, err := NewClient(cfg)
	if err != nil {
		return err
	}
	if err := client.DownloadRules(ctx, p, cfg, force); err != nil {
		return err
	}
	return nil
}

func (c *Client) DownloadSingBox(ctx context.Context, p paths.Paths, cfg config.Config, force bool) error {
	release, err := c.latestRelease(ctx, singBoxRepo, cfg.GitHubToken)
	if err != nil {
		return err
	}
	arch := mapArch(runtime.GOARCH)
	if runtime.GOARCH == "amd64" {
		arch = "amd64"
	}
	asset := pickAsset(release.AssetURLs(), fmt.Sprintf(`linux-%s.*\.tar\.gz`, regexp.QuoteMeta(arch)))
	if asset == "" {
		return fmt.Errorf("no sing-box asset found for linux-%s", arch)
	}
	asset = mirrorURL(asset, cfg.GitHubMirror)
	archive := filepath.Join(p.DownloadsDir, filepath.Base(asset))
	if err := c.downloadFile(ctx, asset, archive, force); err != nil {
		return err
	}
	if err := extractSingBoxTarGZ(archive, p.BinDir); err != nil {
		return err
	}
	_ = os.WriteFile(p.SingBoxVersion, []byte(release.TagName+"\n"), 0o644)
	return nil
}

func (c *Client) DownloadRules(ctx context.Context, p paths.Paths, cfg config.Config, force bool) error {
	for _, item := range []struct {
		url string
		out string
	}{
		{mirrorURL(geositeCNURL, cfg.GitHubMirror), p.GeositeCN},
		{mirrorURL(geoipCNURL, cfg.GitHubMirror), p.GeoIPCN},
	} {
		if err := c.downloadFile(ctx, item.url, item.out, force); err != nil {
			return err
		}
	}
	return nil
}

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		URL string `json:"browser_download_url"`
	} `json:"assets"`
}

func (r release) AssetURLs() []string {
	urls := make([]string, 0, len(r.Assets))
	for _, asset := range r.Assets {
		if asset.URL != "" {
			urls = append(urls, asset.URL)
		}
	}
	return urls
}

func (c *Client) latestRelease(ctx context.Context, repo string, token string) (release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return release{}, fmt.Errorf("GitHub latest release %s: HTTP %d", repo, resp.StatusCode)
	}
	var out release
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return release{}, err
	}
	return out, nil
}

func (c *Client) downloadFile(ctx context.Context, rawURL string, out string, force bool) error {
	if !force {
		if info, err := os.Stat(out); err == nil && info.Size() > 0 {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: HTTP %d", rawURL, resp.StatusCode)
	}
	tmp := out + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, out)
}

func extractSingBoxTarGZ(archive string, outDir string) error {
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
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("sing-box binary not found in archive")
		}
		if err != nil {
			return err
		}
		if header.FileInfo().IsDir() || filepath.Base(header.Name) != "sing-box" {
			continue
		}
		out := filepath.Join(outDir, "sing-box")
		f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(f, tr); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
}

func mapArch(machine string) string {
	switch machine {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "armv7":
		return "armv7"
	case "armv6l", "armv6":
		return "armv6"
	case "i386", "i686", "386":
		return "386"
	default:
		return machine
	}
}

func pickAsset(urls []string, pattern string) string {
	rx := regexp.MustCompile(pattern)
	for _, candidate := range urls {
		if rx.MatchString(candidate) {
			return candidate
		}
	}
	return ""
}

func mirrorURL(rawURL string, mirror string) string {
	if mirror == "" || strings.Contains(rawURL, "api.github.com") {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "https://github.com/") || strings.HasPrefix(rawURL, "https://raw.githubusercontent.com/") {
		return strings.TrimRight(mirror, "/") + "/" + rawURL
	}
	return rawURL
}
