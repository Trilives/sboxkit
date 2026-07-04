// Package kernel 核心资源下载/更新：sing-box 内核 + geo 规则集。
//
// 下载相关设置（download_proxy / github_mirror / github_token）读 customize.json，
// 未配置回退环境变量（DOWNLOAD_PROXY、http_proxy 系、GITHUB_TOKEN/GH_TOKEN）。
// 另提供 deb 种子接管：/usr/libexec/sboxkit 与 /usr/share/sboxkit/ruleset 里
// 打包附带的 sing-box 与基础规则文件，在 state 缺失时复制为初始资源，安装后离线即可启动。
//
// 与 mihomo 版的关键差异：sing-box 官方发行版是 tar.gz 压缩包（内含二进制，
// 而非单文件 gzip），geo 数据是 sing-geosite/sing-geoip 项目发布的 .srs 规则集
// （一国家一份文件，而非 mihomo 的 geoip.metadb/geosite.dat/country.mmdb 三件套）；
// sing-box 也没有官方 Web UI 可下载——面板改由 internal/uiassets 内置在二进制里，
// 因此本包不再有"下载 Web UI"这一步。
package kernel

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/fetchx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	SingBoxRepo = "SagerNet/sing-box"

	GeositeCNURL = "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-cn.srs"
	GeoIPCNURL   = "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs"
)

// deb 包内种子位置（见 .goreleaser nfpm 配置；许可见 /usr/share/doc/sboxkit/copyright）。
const (
	SeedBinDir     = "/usr/libexec/sboxkit"
	SeedRulesetDir = "/usr/share/sboxkit/ruleset"
)

// Settings 下载相关设置。
type Settings struct {
	DownloadProxy string
	GithubMirror  string
	GithubToken   string
}

// LoadSettings 读 customize.json 的下载字段，缺失回退环境变量。
func LoadSettings(p paths.Paths) Settings {
	cfg := config.Load(p)
	proxy := cfg.DownloadProxy
	if proxy == "" {
		proxy = os.Getenv("DOWNLOAD_PROXY")
	}
	if proxy == "" {
		proxy = ambientProxy()
	}
	token := cfg.GitHubToken
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return Settings{
		DownloadProxy: strings.TrimSpace(proxy),
		GithubMirror:  strings.TrimSpace(cfg.GitHubMirror),
		GithubToken:   strings.TrimSpace(token),
	}
}

// ambientProxy 当前 shell 的代理环境变量（proxyenv 写入 bashrc 的那套），
// 仅作 download_proxy 的隐式回退；fetchx 直连可达时会彻底绕过它。
func ambientProxy() string {
	for _, v := range []string{"https_proxy", "HTTPS_PROXY", "all_proxy", "ALL_PROXY", "http_proxy", "HTTP_PROXY"} {
		if s := strings.TrimSpace(os.Getenv(v)); s != "" {
			return s
		}
	}
	return ""
}

// Mirror 对 GitHub 下载/raw 链接套加速前缀；api.github.com 不套（多数镜像不代理 API）。
func Mirror(rawURL, mirror string) string {
	if mirror == "" || strings.Contains(rawURL, "api.github.com") {
		return rawURL
	}
	if strings.HasPrefix(rawURL, "https://github.com/") || strings.HasPrefix(rawURL, "https://raw.githubusercontent.com/") {
		return strings.TrimRight(mirror, "/") + "/" + rawURL
	}
	return rawURL
}

// archName Go arch → sing-box 资产命名（本二进制按目标架构编译，GOARCH 即目标机架构）。
func archName() string {
	switch runtime.GOARCH {
	case "amd64", "arm64", "386":
		return runtime.GOARCH
	case "arm":
		return "armv7"
	default:
		return runtime.GOARCH
	}
}

type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func latestRelease(f *fetchx.Fetcher, repo string) (release, error) {
	var rel release
	err := f.ReadJSON("https://api.github.com/repos/"+repo+"/releases/latest", &rel)
	return rel, err
}

func assetURLs(rel release) []string {
	urls := make([]string, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		urls = append(urls, a.BrowserDownloadURL)
	}
	return urls
}

func pickAsset(urls []string, pattern string) string {
	rx := regexp.MustCompile("(?i)" + pattern)
	for _, u := range urls {
		if rx.MatchString(u) {
			return u
		}
	}
	return ""
}

// pickSingBoxAsset 选 sing-box Linux 内核 tar.gz 资产。
func pickSingBoxAsset(urls []string, arch, version string) string {
	v := regexp.QuoteMeta(strings.TrimPrefix(version, "v"))
	order := []string{
		fmt.Sprintf(`sing-box-%s-linux-%s\.tar\.gz$`, v, arch),
		fmt.Sprintf(`linux-%s.*\.tar\.gz$`, arch),
	}
	for _, pat := range order {
		if u := pickAsset(urls, pat); u != "" {
			return u
		}
	}
	return ""
}

// --------------------------------------------------------------------------
// 下载 + 缓存校验
// --------------------------------------------------------------------------

func cacheValid(path string) bool {
	st, err := os.Stat(path)
	if err != nil || st.Size() == 0 {
		return false
	}
	name := filepath.Base(path)
	if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
		f, err := os.Open(path)
		if err != nil {
			return false
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return false
		}
		tr := tar.NewReader(gz)
		for {
			if _, err := tr.Next(); err == io.EOF {
				return true
			} else if err != nil {
				return false
			}
		}
	}
	return true
}

func downloadTo(f *fetchx.Fetcher, rawURL, out string, force bool) error {
	part := out + ".part"
	if !force && cacheValid(out) {
		execx.Info(i18n.T("使用缓存: ") + filepath.Base(out))
		return nil
	}
	if _, err := os.Stat(out); err == nil {
		execx.Info(i18n.T("丢弃无效缓存: ") + filepath.Base(out))
		os.Remove(out)
		os.Remove(part)
	}
	execx.Info(i18n.T("下载: ") + rawURL)
	if err := f.FetchFile(rawURL, part); err != nil {
		return err
	}
	isArchive := strings.HasSuffix(part, ".tar.gz.part") || strings.HasSuffix(part, ".tgz.part")
	if isArchive {
		// 校验以 part 实际内容为准（按最终名判断格式）
		tmp := strings.TrimSuffix(part, ".part") + ".check"
		if err := os.Rename(part, tmp); err != nil {
			return err
		}
		if !cacheValid(tmp) {
			os.Remove(tmp)
			return fmt.Errorf(i18n.T("下载文件完整性校验失败: %s"), filepath.Base(out))
		}
		return os.Rename(tmp, out)
	}
	if st, err := os.Stat(part); err != nil || st.Size() == 0 {
		os.Remove(part)
		return fmt.Errorf(i18n.T("下载文件为空: %s"), filepath.Base(out))
	}
	return os.Rename(part, out)
}

// --------------------------------------------------------------------------
// 部署各组件
// --------------------------------------------------------------------------

// UpdateCore 下载并部署 sing-box 内核（tar.gz 内含二进制），返回版本号。
func UpdateCore(p paths.Paths, f *fetchx.Fetcher, s Settings, force bool) (string, error) {
	if err := p.EnsureStateDirs(); err != nil {
		return "", err
	}
	execx.Info(i18n.T("查找最新 sing-box 版本…"))
	rel, err := latestRelease(f, SingBoxRepo)
	if err != nil {
		return "", err
	}
	version := strings.TrimSpace(rel.TagName)
	u := pickSingBoxAsset(assetURLs(rel), archName(), version)
	if u == "" {
		return "", fmt.Errorf(i18n.T("未找到架构 %s 的 Linux sing-box 资源"), archName())
	}

	archive := filepath.Join(p.Downloads, filepath.Base(u))
	if err := downloadTo(f, Mirror(u, s.GithubMirror), archive, force); err != nil {
		return "", err
	}

	if err := extractSingBoxBinary(archive, p.SingBoxBin); err != nil {
		return "", err
	}
	if err := os.WriteFile(p.SingBoxVersion, []byte(version+"\n"), 0o644); err != nil {
		return "", err
	}
	execx.Ok(i18n.T("内核已部署: ") + version)
	return version, nil
}

// extractSingBoxBinary 从 tar.gz 里找到 sing-box 二进制，原子写入 dst。
func extractSingBoxBinary(archive, dst string) error {
	src, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer src.Close()
	gz, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf(i18n.T("压缩包内未找到 sing-box 二进制: %s"), filepath.Base(archive))
		}
		if err != nil {
			return err
		}
		if hdr.FileInfo().IsDir() || filepath.Base(hdr.Name) != "sing-box" {
			continue
		}
		tmp := dst + ".new"
		out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Rename(tmp, dst)
	}
}

// UpdateGeodata 下载 geo 规则集（geosite-cn.srs + geoip-cn.srs）。
// 机场订阅 rules 普遍内联 GEOIP/GEOSITE，缺 geo 数据时相关规则不会生效
// （sing-box 对未知 rule-set 引用是拒绝启动，而非静默忽略）。
func UpdateGeodata(p paths.Paths, f *fetchx.Fetcher, s Settings, force bool) error {
	if err := p.EnsureStateDirs(); err != nil {
		return err
	}
	for _, item := range []struct{ url, dest string }{
		{GeositeCNURL, p.GeositeCN},
		{GeoIPCNURL, p.GeoIPCN},
	} {
		cache := filepath.Join(p.Downloads, filepath.Base(item.dest))
		if err := downloadTo(f, Mirror(item.url, s.GithubMirror), cache, force); err != nil {
			return err
		}
		if err := copyFile(cache, item.dest, 0o644); err != nil {
			return err
		}
	}
	execx.Ok(i18n.T("geo 数据已更新"))
	return nil
}

// --------------------------------------------------------------------------
// deb 种子接管
// --------------------------------------------------------------------------

// SeedFromSystem 把 deb 附带的种子资源（sing-box 二进制 + 基础规则文件）复制到
// state（仅当 state 对应文件缺失时），使 apt 安装后无需联网即可初始化。
// 返回实际接管的文件列表。
func SeedFromSystem(p paths.Paths) ([]string, error) {
	var seeded []string
	if err := p.EnsureStateDirs(); err != nil {
		return nil, err
	}
	seedBin := filepath.Join(SeedBinDir, "sing-box")
	if _, err := os.Stat(p.SingBoxBin); os.IsNotExist(err) {
		if _, err := os.Stat(seedBin); err == nil {
			if err := copyFile(seedBin, p.SingBoxBin, 0o755); err != nil {
				return seeded, err
			}
			os.WriteFile(p.SingBoxVersion, []byte("bundled\n"), 0o644)
			seeded = append(seeded, p.SingBoxBin)
		}
	}
	for _, item := range []struct{ name, dest string }{
		{"geosite-cn.srs", p.GeositeCN},
		{"geoip-cn.srs", p.GeoIPCN},
	} {
		src := filepath.Join(SeedRulesetDir, item.name)
		if _, err := os.Stat(item.dest); !os.IsNotExist(err) {
			continue
		}
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := copyFile(src, item.dest, 0o644); err != nil {
			return seeded, err
		}
		seeded = append(seeded, item.dest)
	}
	if len(seeded) > 0 {
		execx.Info(fmt.Sprintf(i18n.T("已从系统包接管 %d 个种子文件（离线可用；后续可在线更新）。"), len(seeded)))
	}
	return seeded, nil
}

// NewFetcher 按设置构造下载器（Token 缺失时的交互式补充由上层流程负责）。
func NewFetcher(p paths.Paths) (*fetchx.Fetcher, Settings) {
	s := LoadSettings(p)
	if s.DownloadProxy != "" {
		execx.Info(i18n.T("下载代理（直连不可用时回退）: ") + s.DownloadProxy)
	}
	return fetchx.New(s.DownloadProxy, s.GithubToken), s
}

// Options DownloadAll 的选项。
type Options struct {
	Force bool
}

// DownloadAll 下载内核 + geo 数据，返回内核版本。Web UI 内置在二进制里，
// 不需要单独下载部署（见 internal/uiassets）。
func DownloadAll(p paths.Paths, opts Options) (string, error) {
	f, s := NewFetcher(p)
	version, err := UpdateCore(p, f, s, opts.Force)
	if err != nil {
		return "", err
	}
	if err := UpdateGeodata(p, f, s, opts.Force); err != nil {
		return version, err
	}
	return version, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
