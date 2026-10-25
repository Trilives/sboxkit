package selfupdate

// 系统模式下 .deb 安装的 sboxkit 交给 dpkg 自更新：下载同版本 .deb（按 GitHub 资产摘要
// 校验），解出包内 sboxkit 试跑并做兼容性自检，通过后用 dpkg 安装。/usr/bin/sboxkit 始终
// 归 dpkg 管理，不再改写成指向版本库的符号链接，apt / dpkg 升级与自更新走同一条路径；
// 旧版自更新留下的符号链接与版本库在第一次经 dpkg 安装后清理。

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/fetchx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
)

const (
	packageName   = "sboxkit"
	debBinMember  = "usr/bin/sboxkit"
	dpkgLockTries = 12
	// maxPackagedBinary 包内 sboxkit 的体积上限（实际约 10 MB），防止解压时写满磁盘。
	maxPackagedBinary = 256 << 20
)

// 可在测试中替换。
var (
	packagedExe    = "/usr/bin/sboxkit"
	packageManaged = defaultPackageManaged
	runCommand     = func(name string, args ...string) ([]byte, error) {
		cmd := exec.Command(name, args...)
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
		return cmd.CombinedOutput()
	}
	dpkgLockWait = 5 * time.Second
	// lookupRelease 查询指定版本的发行版（含资产摘要）。
	lookupRelease = func(p paths.Paths, version string) (ghRelease, error) {
		f, _ := kernel.NewFetcher(p)
		return fetchReleaseByTag(f, version)
	}
	// downloadDeb 下载发行版中的 deb 并按摘要校验。
	downloadDeb = func(p paths.Paths, rel ghRelease, name string) (string, error) {
		f, s := kernel.NewFetcher(p)
		return downloadVerified(p, f, s, rel, name)
	}
)

func debAssetName(version string) string {
	return fmt.Sprintf("sboxkit_%s_linux_%s.deb", version, releaseArch())
}

func packagesDir(p paths.Paths) string { return filepath.Join(p.Downloads, "packages") }
func cachedDeb(p paths.Paths, version string) string {
	return filepath.Join(packagesDir(p), debAssetName(version))
}
func lastStableRecord(p paths.Paths) string { return filepath.Join(packagesDir(p), "last-stable") }

// defaultPackageManaged 系统模式、以 /usr/bin/sboxkit 调用且 sboxkit 包已由 dpkg 安装。
func defaultPackageManaged(p paths.Paths) bool {
	if p.UserMode || invokedPath() != packagedExe {
		return false
	}
	_, err := installedPackageVersion()
	return err == nil
}

// invokedPath 用户调用的路径（与 sysd.StableExecutablePath 相同的解析，保留符号链接）。
func invokedPath() string {
	path := os.Args[0]
	if !strings.ContainsRune(path, os.PathSeparator) {
		found, err := exec.LookPath(path)
		if err != nil {
			return ""
		}
		path = found
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return abs
}

// installedPackageVersion dpkg 记录的已安装 sboxkit 版本（未安装时报错），换回发行版标签的写法：
// nfpm 按 Debian 习惯把预发布后缀写成 ~（0.4.1-beta.1 记为 0.4.1~beta.1，才排在 0.4.1 之前），
// 撤销更新时要凭它找回原版本的发行版。
func installedPackageVersion() (string, error) {
	out, err := runCommand("dpkg-query", "-W", "-f=${db:Status-Status} ${Version}", packageName)
	if err != nil {
		return "", fmt.Errorf("dpkg-query: %w", err)
	}
	status, version, _ := strings.Cut(strings.TrimSpace(string(out)), " ")
	if status != "installed" || version == "" {
		return "", fmt.Errorf("package %s is not installed (%s)", packageName, strings.TrimSpace(string(out)))
	}
	return strings.Replace(version, "~", "-", 1), nil
}

// prepareDeb 下载并校验 version 的 .deb，解出其中的 sboxkit 试跑；不安装。
func prepareDeb(p paths.Paths, f *fetchx.Fetcher, s kernel.Settings, rel ghRelease, version string, channel Channel) (*Prepared, error) {
	deb, err := downloadVerified(p, f, s, rel, debAssetName(version))
	if err != nil {
		return nil, err
	}
	t, err := unpackDeb(p, deb, version, channel)
	if err != nil {
		os.Remove(deb)
		return nil, err
	}
	t.debFresh = true
	return t, nil
}

// unpackDeb 把 deb 里的 sboxkit 解到 root 独占的临时目录并试跑。
func unpackDeb(p paths.Paths, deb, version string, channel Channel) (*Prepared, error) {
	scratch, err := os.MkdirTemp(p.Downloads, ".deb-"+version+"-*")
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(scratch, "sboxkit")
	if err := extractDebMember(deb, debBinMember, bin); err != nil {
		os.RemoveAll(scratch)
		return nil, err
	}
	if err := probeBinary(bin); err != nil {
		os.RemoveAll(scratch)
		return nil, fmt.Errorf(i18n.T("新版本二进制无法正常运行，已放弃更新：%w"), err)
	}
	return &Prepared{Version: version, Bin: bin, channel: channel, deb: deb, scratch: scratch}, nil
}

// extractDebMember 从 deb（ar 归档，data.tar.gz）中取出一个文件，写为 0755 的 out。
func extractDebMember(deb, member, out string) error {
	f, err := os.Open(deb)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := arMember(f, "data.tar.gz")
	if err != nil {
		return err
	}
	gz, err := gzip.NewReader(data)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf(i18n.T("安装包中没有 %s"), member)
		}
		if err != nil {
			return err
		}
		if strings.TrimPrefix(hdr.Name, "./") != member || hdr.Typeflag != tar.TypeReg {
			continue
		}
		if hdr.Size > maxPackagedBinary {
			return fmt.Errorf("%s: %s is larger than %d bytes", filepath.Base(deb), member, maxPackagedBinary)
		}
		w, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, io.LimitReader(tr, maxPackagedBinary)); err != nil {
			w.Close()
			return err
		}
		return w.Close()
	}
}

// arMember 在 ar 归档中定位名为 name 的成员，返回限定在其内容范围内的 Reader。
func arMember(f *os.File, name string) (io.Reader, error) {
	r := bufio.NewReader(f)
	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "!<arch>\n" {
		return nil, fmt.Errorf("%s: not a deb archive", f.Name())
	}
	header := make([]byte, 60)
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			return nil, fmt.Errorf(i18n.T("安装包中没有 %s"), name)
		}
		member := strings.TrimSuffix(strings.TrimSpace(string(header[:16])), "/")
		size, err := strconv.ParseInt(strings.TrimSpace(string(header[48:58])), 10, 64)
		if err != nil || size < 0 {
			return nil, fmt.Errorf("%s: corrupt ar header", f.Name())
		}
		if member == name {
			return io.LimitReader(r, size), nil
		}
		if _, err := r.Discard(int(size + size%2)); err != nil {
			return nil, err
		}
	}
}

// dpkgLocked dpkg 因锁被占用而失败（"dpkg frontend lock was locked by another process"，
// 旧版为 "dpkg status database is locked by another process"）。
func dpkgLocked(out []byte) bool {
	return bytes.Contains(out, []byte("locked by another process"))
}

// dpkgInstall 用 dpkg 安装 deb；dpkg 锁被占用（如自动更新正在运行）时稍候重试。
func dpkgInstall(deb string) error {
	var out []byte
	var err error
	for try := 0; try < dpkgLockTries; try++ {
		out, err = runCommand("dpkg", "-i", deb)
		if err == nil {
			return nil
		}
		if !dpkgLocked(out) {
			break
		}
		if try == 0 {
			execx.Info(i18n.T("dpkg 正被其他程序占用，等待其完成…"))
		}
		time.Sleep(dpkgLockWait)
	}
	return fmt.Errorf(i18n.T("dpkg 安装 %s 失败：%v\n%s"), filepath.Base(deb), err, strings.TrimSpace(string(out)))
}

// activateDeb 安装该版本的 deb 并试跑 /usr/bin/sboxkit；返回重新安装原版本的回退函数。
func (t *Prepared) activateDeb(p paths.Paths) (func() error, error) {
	previous, _ := installedPackageVersion()
	if err := dpkgInstall(t.deb); err != nil {
		return nil, err
	}
	t.activated = true
	undo := func() error {
		if err := reinstallPackage(p, previous); err != nil {
			return err
		}
		t.activated = false
		return nil
	}
	if err := probeBinary(packagedExe); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("新版本启动校验失败，回退到旧版本：%v"), err))
		if uerr := undo(); uerr != nil {
			return nil, errors.Join(err, uerr)
		}
		return nil, fmt.Errorf(i18n.T("已回退到原版本：%w"), err)
	}
	return undo, nil
}

// reinstallPackage 重新安装 version 的 deb（缓存经摘要确认后直接用，否则重新下载）。
func reinstallPackage(p paths.Paths, version string) error {
	if version == "" || strings.Contains(version, "SNAPSHOT") || version == "dev" {
		return fmt.Errorf(i18n.T("无法自动重新安装原版本 %q 的 sboxkit，请手动安装对应的 .deb"), version)
	}
	deb, fresh, err := fetchDeb(p, version)
	if err != nil {
		return err
	}
	if fresh {
		if err := os.MkdirAll(packagesDir(p), 0o755); err == nil && os.Rename(deb, cachedDeb(p, version)) == nil {
			deb = cachedDeb(p, version)
		}
	}
	return dpkgInstall(deb)
}

// fetchDeb 取得 version 的 deb：缓存与 GitHub API 摘要相符才直接使用（fresh=false），否则
// 删除缓存，重新下载并校验。缓存同样要比对：旧版本下状态目录曾归普通用户，收归 root 时
// 不检查内容。
func fetchDeb(p paths.Paths, version string) (deb string, fresh bool, err error) {
	rel, err := lookupRelease(p, version)
	if err != nil {
		return "", false, err
	}
	name := debAssetName(version)
	if cached := cachedDeb(p, version); fileExists(cached) {
		if asset, ok := findAsset(rel, name); ok {
			if d, ok := kernel.ParseAssetDigest(asset.Digest); ok && d.Verify(cached) == nil {
				return cached, false, nil
			}
		}
		os.Remove(cached)
	}
	deb, err = downloadDeb(p, rel, name)
	return deb, err == nil, err
}

// finishDeb 安装并提交后：缓存这次的 deb（回退与稳定版回滚用），稳定渠道记录 last-stable，
// 清理多余的缓存与旧版自更新的版本库。
func (t *Prepared) finishDeb(p paths.Paths) {
	if err := os.MkdirAll(packagesDir(p), 0o755); err == nil && t.deb != cachedDeb(p, t.Version) {
		os.Rename(t.deb, cachedDeb(p, t.Version)) //nolint:errcheck // 缓存失败只影响日后的离线回退
	}
	if t.channel == Stable {
		os.WriteFile(lastStableRecord(p), []byte(t.Version+"\n"), 0o644) //nolint:errcheck
	}
	pruneCachedDebs(p, t.Version)
	removeLegacyVersions(p)
}

// pruneCachedDebs 只保留当前、紧邻上一版本与 last-stable 记录的 deb。
func pruneCachedDebs(p paths.Paths, keepVersion string) {
	entries, err := os.ReadDir(packagesDir(p))
	if err != nil {
		return
	}
	prefix, suffix := "sboxkit_", "_linux_"+releaseArch()+".deb"
	var versions []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			versions = append(versions, strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix))
		}
	}
	keep := map[string]bool{keepVersion: true}
	if previous, ok := latestBelow(versions, keepVersion); ok {
		keep[previous] = true
	}
	if stable, ok := lastStableDebVersion(p); ok {
		keep[stable] = true
	}
	for _, v := range versions {
		if !keep[v] {
			os.Remove(cachedDeb(p, v))
		}
	}
}

// removeLegacyVersions 旧版自更新把 /usr/bin/sboxkit 改成指向状态目录版本库的符号链接；
// dpkg 安装会把它换回普通文件，此后版本库不再被引用。
func removeLegacyVersions(p paths.Paths) {
	if target, err := os.Readlink(packagedExe); err == nil && strings.HasPrefix(target, versionsDir(p)) {
		return
	}
	if _, err := os.Lstat(versionsDir(p)); err == nil {
		os.RemoveAll(versionsDir(p))
	}
}

// latestBelow versions 中低于 version 的最高版本（按数值比较，0.3.10 高于 0.3.9）。
func latestBelow(versions []string, version string) (string, bool) {
	best, found := "", false
	for _, v := range versions {
		if compareVersions(v, version) < 0 && (!found || compareVersions(v, best) > 0) {
			best, found = v, true
		}
	}
	return best, found
}

// compareVersions 按 compat.AtLeast 的数值规则比较；数值相同时正式版高于其预发布版本
// （0.4.1 高于 0.4.1-beta.1），预发布版本之间及无法解析时退回字符串比较。
func compareVersions(a, b string) int {
	if a == b {
		return 0
	}
	aGE, ok1 := compat.AtLeast(a, b)
	bGE, ok2 := compat.AtLeast(b, a)
	switch {
	case !ok1 || !ok2:
		return strings.Compare(a, b)
	case aGE && !bGE:
		return 1
	case bGE && !aGE:
		return -1
	}
	aPre, bPre := strings.Contains(a, "-"), strings.Contains(b, "-")
	switch {
	case aPre == bPre:
		return strings.Compare(a, b)
	case bPre:
		return 1
	default:
		return -1
	}
}

// Older 版本 a 是否低于 b（规则同 compareVersions）。
func Older(a, b string) bool { return compareVersions(a, b) < 0 }

func lastStableDebVersion(p paths.Paths) (string, bool) {
	raw, err := os.ReadFile(lastStableRecord(p))
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(raw))
	return v, v != ""
}

// prepareStableDeb 以 last-stable 记录的稳定版为目标：缓存的 deb 优先，否则重新下载校验。
func prepareStableDeb(p paths.Paths) (*Prepared, error) {
	version, ok := lastStableDebVersion(p)
	if !ok {
		return nil, fmt.Errorf("%s", i18n.T("尚未记录过稳定版，无法回退"))
	}
	deb, fresh, err := fetchDeb(p, version)
	if err != nil {
		return nil, err
	}
	t, err := unpackDeb(p, deb, version, "")
	if err != nil {
		if fresh {
			os.Remove(deb)
		}
		return nil, err
	}
	t.debFresh = fresh
	return t, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
