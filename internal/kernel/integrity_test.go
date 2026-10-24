package kernel

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/fetchx"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestParseAssetDigestAcceptsOnlyWellFormedSHA256(t *testing.T) {
	sum := sha256Hex("payload")
	if d, ok := ParseAssetDigest("sha256:" + strings.ToUpper(sum)); !ok || d.Hex != sum || d.Algo != "sha256" {
		t.Fatalf("ParseAssetDigest(valid) = %+v, %v", d, ok)
	}
	for _, bad := range []string{"", "sha256:", "md5:" + sum, "sha256:" + sum[:10], "sha256:" + strings.Repeat("z", 64)} {
		if _, ok := ParseAssetDigest(bad); ok {
			t.Errorf("ParseAssetDigest(%q) accepted", bad)
		}
	}
}

func TestDigestVerifyChecksSHA256AndGitBlobNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := []Digest{
		{Algo: "sha256", Hex: sha256Hex("hello\n")},
		// git hash-object 对 "hello\n" 的对象名
		{Algo: "gitblob", Hex: "ce013625030ba8dba906f756967f9e9ca394464a"},
		{}, // 没有摘要：不校验
	}
	for _, d := range good {
		if err := d.Verify(path); err != nil {
			t.Errorf("Verify(%+v) = %v", d, err)
		}
	}
	for _, d := range []Digest{
		{Algo: "sha256", Hex: sha256Hex("tampered")},
		{Algo: "gitblob", Hex: strings.Repeat("0", 40)},
	} {
		if err := d.Verify(path); err == nil {
			t.Errorf("Verify(%+v) accepted mismatching content", d)
		}
	}
}

func TestRequireDigestRefusesUnverifiedMirrorDownloads(t *testing.T) {
	d := Digest{Algo: "sha256", Hex: sha256Hex("x")}
	if got, err := RequireDigest(d, true, "https://mirror.example", "a"); err != nil || got != d {
		t.Fatalf("with digest = %+v, %v", got, err)
	}
	if got, err := RequireDigest(Digest{}, false, "", "a"); err != nil || !got.IsZero() {
		t.Fatalf("direct GitHub without digest = %+v, %v, want TLS-only download allowed", got, err)
	}
	if _, err := RequireDigest(Digest{}, false, "https://mirror.example", "a"); err == nil {
		t.Fatal("a mirror without an API digest must be refused")
	}
}

type fakeAPI struct {
	urls []string
	sha  string
	typ  string
}

func (f *fakeAPI) ReadJSON(rawURL string, v any) error {
	f.urls = append(f.urls, rawURL)
	out := v.(*struct {
		Type string `json:"type"`
		SHA  string `json:"sha"`
	})
	out.Type, out.SHA = f.typ, f.sha
	return nil
}

func TestRawDigestAsksContentsAPIForTheBranchFile(t *testing.T) {
	api := &fakeAPI{typ: "file", sha: strings.Repeat("ab", 20)}

	d, err := rawDigest(api, GeositeCNURL)

	if err != nil || d.Algo != "gitblob" || d.Hex != api.sha {
		t.Fatalf("rawDigest = %+v, %v", d, err)
	}
	want := "https://api.github.com/repos/SagerNet/sing-geosite/contents/geosite-cn.srs?ref=rule-set"
	if len(api.urls) != 1 || api.urls[0] != want {
		t.Fatalf("queried %v, want %s", api.urls, want)
	}
	if _, err := rawDigest(api, "https://mirror.example/geosite-cn.srs"); err == nil {
		t.Fatal("non-GitHub URL accepted")
	}
	api.typ = "dir"
	if _, err := rawDigest(api, GeoIPCNURL); err == nil {
		t.Fatal("a directory listing must not yield a digest")
	}
}

func serveContent(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/geosite-cn.srs"
}

func TestDownloadToRejectsContentThatDoesNotMatchTheDigest(t *testing.T) {
	out := filepath.Join(t.TempDir(), "geosite-cn.srs")
	url := serveContent(t, "tampered by mirror")

	err := downloadTo(fetchx.New("", ""), url, out, true, Digest{Algo: "sha256", Hex: sha256Hex("genuine")})

	if err == nil {
		t.Fatal("tampered download accepted")
	}
	for _, leftover := range []string{out, out + ".part"} {
		if _, statErr := os.Stat(leftover); !os.IsNotExist(statErr) {
			t.Errorf("%s left behind after a failed verification", filepath.Base(leftover))
		}
	}
}

func TestDownloadToReusesCacheOnlyWhenItMatches(t *testing.T) {
	out := filepath.Join(t.TempDir(), "geosite-cn.srs")
	writeFile(t, out, "stale cache")
	url := serveContent(t, "genuine")
	want := Digest{Algo: "sha256", Hex: sha256Hex("genuine")}

	if err := downloadTo(fetchx.New("", ""), url, out, false, want); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, out); got != "genuine" {
		t.Fatalf("cache = %q, a mismatching cache must be replaced", got)
	}
	// 缓存相符时不再联网：服务器内容即使变了也不会被取用。
	url = serveContent(t, "changed upstream")
	if err := downloadTo(fetchx.New("", ""), url, out, false, want); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, out); got != "genuine" {
		t.Fatalf("cache = %q, a verified cache must be reused", got)
	}
}

func TestForcedDownloadReusesCacheOnlyWhenADigestVouchesForIt(t *testing.T) {
	out := filepath.Join(t.TempDir(), "geosite-cn.srs")
	writeFile(t, out, "genuine")
	url := serveContent(t, "changed upstream")

	// 有摘要时，相符的缓存就是这次要下载的内容：强制更新也不必重新下载。
	if err := downloadTo(fetchx.New("", ""), url, out, true, Digest{Algo: "sha256", Hex: sha256Hex("genuine")}); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, out); got != "genuine" {
		t.Fatalf("cache = %q, a cache matching the digest must be reused", got)
	}
	// 没有摘要可比对时，强制更新照旧重新下载。
	if err := downloadTo(fetchx.New("", ""), url, out, true, Digest{}); err != nil {
		t.Fatal(err)
	}
	if got := readText(t, out); got != "changed upstream" {
		t.Fatalf("cache = %q, a forced download without a digest must fetch again", got)
	}
}
