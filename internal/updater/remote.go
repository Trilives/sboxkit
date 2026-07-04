package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type GitHubRemote struct {
	repo string
	http *http.Client
}

func NewGitHubRemote(repo string, client *http.Client) *GitHubRemote {
	if repo == "" {
		repo = defaultRepo
	}
	if client == nil {
		client = &http.Client{Timeout: 180 * time.Second}
	}
	return &GitHubRemote{repo: repo, http: client}
}

func (r *GitHubRemote) Latest(ctx context.Context, arch string) (Release, error) {
	return r.LatestChannel(ctx, arch, ChannelStable)
}

func (r *GitHubRemote) LatestChannel(ctx context.Context, arch string, channel Channel) (Release, error) {
	if channel == ChannelPreview {
		return r.latestPreview(ctx, arch)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+r.repo+"/releases/latest", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := r.http.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Release{}, fmt.Errorf("GitHub latest release %s: HTTP %d", r.repo, resp.StatusCode)
	}
	var doc struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return Release{}, err
	}
	return releaseFromAssets(doc.TagName, arch, doc.Assets), nil
}

func (r *GitHubRemote) latestPreview(ctx context.Context, arch string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+r.repo+"/releases?per_page=20", nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := r.http.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Release{}, fmt.Errorf("GitHub preview releases %s: HTTP %d", r.repo, resp.StatusCode)
	}
	var docs []struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&docs); err != nil {
		return Release{}, err
	}
	for _, doc := range docs {
		if doc.Draft || !doc.Prerelease {
			continue
		}
		return releaseFromAssets(doc.TagName, arch, doc.Assets), nil
	}
	return Release{}, fmt.Errorf("no preview release found for %s", r.repo)
}

func releaseFromAssets(tag string, arch string, assets []struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}) Release {
	archivePattern := regexp.MustCompile(`^sboxkit_.+_` + regexp.QuoteMeta(arch) + `_portable\.tar\.gz$`)
	release := Release{Version: strings.TrimPrefix(tag, "v")}
	for _, asset := range assets {
		if archivePattern.MatchString(asset.Name) {
			release.ArchiveURL = asset.URL
			continue
		}
		if strings.HasSuffix(asset.Name, "_"+arch+"_portable.tar.gz.sha256") {
			release.SHA256URL = asset.URL
		}
	}
	return release
}

func (r *GitHubRemote) Download(ctx context.Context, rawURL string, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := r.http.Do(req)
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
