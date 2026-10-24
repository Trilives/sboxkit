// 下载完整性：期望摘要一律取自 api.github.com（不经 github_mirror）。发行资产用 GitHub
// 为每个资产计算的 SHA-256（release API 的 digest 字段），raw.githubusercontent.com 上的
// 规则集用该文件在分支上的 git blob SHA-1（contents API 的 sha 字段）。镜像只负责传输，
// 内容不符即拒绝，root 定时器因此不会部署被镜像或中间人篡改的内核与规则集。
package kernel

import (
	"crypto/sha1" //nolint:gosec // git blob 对象名就是 SHA-1；这里比对的是 GitHub 给出的对象名
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Trilives/sboxkit/internal/i18n"
)

// Digest 下载文件的期望摘要；零值表示没有可用的摘要。
type Digest struct {
	Algo string // "sha256" 或 "gitblob"
	Hex  string
}

// IsZero 没有可用的摘要。
func (d Digest) IsZero() bool { return d.Hex == "" }

// ParseAssetDigest 解析 release API 资产的 digest 字段（"sha256:<hex>"）。
func ParseAssetDigest(value string) (Digest, bool) {
	algo, sum, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok || algo != "sha256" || len(sum) != sha256.Size*2 {
		return Digest{}, false
	}
	if _, err := hex.DecodeString(sum); err != nil {
		return Digest{}, false
	}
	return Digest{Algo: "sha256", Hex: strings.ToLower(sum)}, true
}

// Verify 校验 path 的内容；零值摘要直接通过。
func (d Digest) Verify(path string) error {
	if d.IsZero() {
		return nil
	}
	got, err := d.sum(path)
	if err != nil {
		return err
	}
	if got != d.Hex {
		return fmt.Errorf(i18n.T("%s 完整性校验失败：期望 %s，实际 %s"), filepath.Base(path), d.Hex, got)
	}
	return nil
}

func (d Digest) sum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var h hash.Hash
	switch d.Algo {
	case "sha256":
		h = sha256.New()
	case "gitblob":
		st, err := f.Stat()
		if err != nil {
			return "", err
		}
		h = sha1.New() //nolint:gosec // 见包注释
		fmt.Fprintf(h, "blob %d\x00", st.Size())
	default:
		return "", fmt.Errorf("unsupported digest algorithm %q", d.Algo)
	}
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// RequireDigest 拿不到摘要时的策略：经镜像下载必须有摘要，否则拒绝；直连 GitHub 时
// 内容由 TLS 保证来源，允许没有摘要（零值）继续。
func RequireDigest(d Digest, ok bool, mirror, name string) (Digest, error) {
	if ok && !d.IsZero() {
		return d, nil
	}
	if strings.TrimSpace(mirror) == "" {
		return Digest{}, nil
	}
	return Digest{}, fmt.Errorf(i18n.T("无法从 GitHub API 取得 %s 的摘要，拒绝经镜像下载未经校验的文件"), name)
}

// jsonReader fetchx.Fetcher 的 ReadJSON，抽成接口便于测试。
type jsonReader interface {
	ReadJSON(rawURL string, v any) error
}

// rawDigest 查询 raw.githubusercontent.com/<owner>/<repo>/<ref>/<path> 在该分支上的
// git blob SHA-1。
func rawDigest(f jsonReader, rawURL string) (Digest, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host != "raw.githubusercontent.com" {
		return Digest{}, fmt.Errorf("not a raw.githubusercontent.com URL: %s", rawURL)
	}
	parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 4)
	if len(parts) != 4 || parts[3] == "" {
		return Digest{}, fmt.Errorf("unexpected raw URL layout: %s", rawURL)
	}
	owner, repo, ref, file := parts[0], parts[1], parts[2], parts[3]
	var meta struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
	}
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		owner, repo, file, url.QueryEscape(ref))
	if err := f.ReadJSON(api, &meta); err != nil {
		return Digest{}, err
	}
	if meta.Type != "file" || len(meta.SHA) != sha1.Size*2 {
		return Digest{}, errors.New("contents API returned no file object name for " + rawURL)
	}
	return Digest{Algo: "gitblob", Hex: strings.ToLower(meta.SHA)}, nil
}
