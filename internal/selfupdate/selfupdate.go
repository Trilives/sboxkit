// Package selfupdate 更新 sboxkit 自身到最新发行版（区别于 kernel 包更新的
// sing-box 内核 / geo 数据——那些是被管理的依赖资源，这里更新的是
// sboxkit 这个程序本身）。
//
// 版本化目录 + current 符号链接方案：
//
//	<state>/sboxkit-versions/<version>/sboxkit   —— 各版本二进制，各自独立
//	<state>/sboxkit-versions/current                —— 指向某个 <version>/sboxkit 的符号链接
//
// 首次自更新时，若当前运行的可执行文件（os.Executable，一般是 apt 安装的
// /usr/bin/sboxkit）还是普通文件而非上述符号链接，会先把它原样迁移进版本
// 目录（作为已知可回退的基线版本），再把该路径本身替换为指向 current 的符号
// 链接——此后再更新只需要原子重写 current 指向，不用碰 /usr/bin 下的文件。
//
// 更新流程：下载发行包 → 校验 SHA-256 → 解压到独立版本目录 → 试跑新二进制确认
// 能正常执行 → 原子切换 current 符号链接 → 再次试跑确认成功；若启动校验失败，
// 回退 current 指向。sing-box 内核是完全独立的另一个二进制，不受影响，本流程
// 不涉及任何 systemd 单元的停止/启动。仅保留 current 指向的版本、紧邻的上一
// 个版本、以及 last-stable 记录的稳定版（如果三者不同），其余版本目录清理掉。
//
// 更新渠道（Channel）：稳定版走 GitHub /releases/latest（排除 prerelease），
// 预览版走 /releases 列表最新一项（不论是否标 prerelease）。每次稳定渠道更新
// 成功都会把 current 同时记到 <state>/sboxkit-versions/last-stable，供切到
// 预览版之后想回退的用户一键切回。
package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/fetchx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
)

// Repo sboxkit 自身的发行仓库（.goreleaser.yaml 里的项目/归属一致）。
const Repo = "Trilives/sboxkit"

// Channel 自更新渠道：稳定版只看 GitHub 的 /releases/latest（该接口本身就
// 排除 prerelease/draft，语义上等价于"最新正式版"）；预览版看 /releases 列表
// 第一项，即仓库里创建时间最新的发行版，不论是否标了 prerelease，供想尝鲜的
// 用户提前用上还没转正的版本。
type Channel string

const (
	Stable  Channel = "stable"
	Preview Channel = "preview"
)

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchRelease(f *fetchx.Fetcher, channel Channel) (ghRelease, error) {
	if channel == Preview {
		var rels []ghRelease
		if err := f.ReadJSON("https://api.github.com/repos/"+Repo+"/releases?per_page=1", &rels); err != nil {
			return ghRelease{}, err
		}
		if len(rels) == 0 {
			return ghRelease{}, fmt.Errorf("%s", i18n.T("仓库还没有任何发行版"))
		}
		return rels[0], nil
	}
	var rel ghRelease
	err := f.ReadJSON("https://api.github.com/repos/"+Repo+"/releases/latest", &rel)
	return rel, err
}

func assetName(version string) string {
	return fmt.Sprintf("sboxkit_%s_linux_%s.tar.gz", version, runtime.GOARCH)
}

func findAssetURL(rel ghRelease, name string) string {
	for _, a := range rel.Assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func versionsDir(p paths.Paths) string    { return filepath.Join(p.State, "sboxkit-versions") }
func currentLink(p paths.Paths) string    { return filepath.Join(versionsDir(p), "current") }
func lastStableLink(p paths.Paths) string { return filepath.Join(versionsDir(p), "last-stable") }
func versionBin(p paths.Paths, version string) string {
	return filepath.Join(versionsDir(p), version, "sboxkit")
}

// LatestVersion 只查询最新版本号（不下载），供菜单展示"当前 vX，最新 vY"。
func LatestVersion(p paths.Paths, channel Channel) (string, error) {
	f, _ := kernel.NewFetcher(p)
	rel, err := fetchRelease(f, channel)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

// LastStableVersion 返回记录的"上一个稳定版"版本号（每次稳定渠道更新成功后都会
// 刷新）；尚未记录过（从未在这台机器上完成过一次稳定渠道更新）则 ok=false。
func LastStableVersion(p paths.Paths) (string, bool) {
	target, err := os.Readlink(lastStableLink(p))
	if err != nil {
		return "", false
	}
	return filepath.Base(filepath.Dir(target)), true
}

// RollbackToStable 把 current 切回 last-stable 记录的稳定版（试跑校验通过才
// 切换），供切到预览版后想回退的用户使用。返回切回的版本号。
func RollbackToStable(p paths.Paths) (string, error) {
	target, err := os.Readlink(lastStableLink(p))
	if err != nil {
		return "", fmt.Errorf("%s", i18n.T("尚未记录过稳定版，无法回退"))
	}
	if err := probeBinary(target); err != nil {
		return "", fmt.Errorf(i18n.T("记录的稳定版二进制无法正常运行：%w"), err)
	}
	if err := swapCurrentLink(p, target); err != nil {
		return "", err
	}
	if err := probeBinary(currentLink(p)); err != nil {
		return "", fmt.Errorf(i18n.T("回退后校验失败：%w"), err)
	}
	return filepath.Base(filepath.Dir(target)), nil
}

// Update 把 sboxkit 自身更新到最新版本；currentVersion 是当前运行版本号
// （main.version，"dev" 视为无基线）。返回 (最新版本号, 是否已是最新, error)。
func Update(p paths.Paths, currentVersion string, channel Channel) (string, bool, error) {
	f, s := kernel.NewFetcher(p)
	rel, err := fetchRelease(f, channel)
	if err != nil {
		return "", false, err
	}
	version := strings.TrimPrefix(rel.TagName, "v")
	if version == "" {
		return "", false, fmt.Errorf("%s", i18n.T("发行版没有有效的版本号"))
	}
	if version == strings.TrimPrefix(currentVersion, "v") {
		return version, true, nil
	}

	name := assetName(version)
	assetURL := findAssetURL(rel, name)
	if assetURL == "" {
		return "", false, fmt.Errorf(i18n.T("未找到本机架构的发行包: %s"), name)
	}
	sumsURL := findAssetURL(rel, "checksums.txt")
	if sumsURL == "" {
		return "", false, fmt.Errorf("%s", i18n.T("发行版缺少 checksums.txt，无法校验完整性"))
	}

	if err := os.MkdirAll(p.Downloads, 0o755); err != nil {
		return "", false, err
	}
	archivePath := filepath.Join(p.Downloads, name)
	sumsPath := filepath.Join(p.Downloads, name+".sha256")
	defer os.Remove(archivePath)
	defer os.Remove(sumsPath)

	execx.Info(i18n.T("下载 sboxkit: ") + assetURL)
	if err := f.FetchFile(kernel.Mirror(assetURL, s.GithubMirror), archivePath); err != nil {
		return "", false, err
	}
	if err := f.FetchFile(kernel.Mirror(sumsURL, s.GithubMirror), sumsPath); err != nil {
		return "", false, err
	}
	if err := verifySHA256(archivePath, sumsPath, name); err != nil {
		return "", false, err
	}
	execx.Ok(i18n.T("SHA-256 校验通过。"))

	verDir := filepath.Join(versionsDir(p), version)
	if err := os.RemoveAll(verDir); err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		return "", false, err
	}
	if err := extractTarGz(archivePath, verDir); err != nil {
		return "", false, err
	}
	newBin := versionBin(p, version)
	if _, err := os.Stat(newBin); err != nil {
		return "", false, fmt.Errorf("%s", i18n.T("解压后未找到 sboxkit 可执行文件"))
	}
	if err := os.Chmod(newBin, 0o755); err != nil {
		return "", false, err
	}
	if err := probeBinary(newBin); err != nil {
		os.RemoveAll(verDir)
		return "", false, fmt.Errorf(i18n.T("新版本二进制无法正常运行，已放弃更新：%w"), err)
	}

	if err := ensureManagedByCurrentLink(p, currentVersion); err != nil {
		return "", false, err
	}

	prevTarget, _ := os.Readlink(currentLink(p))

	if err := swapCurrentLink(p, newBin); err != nil {
		return "", false, err
	}

	// 注意：不能用 os.Executable() 来试跑"切换后的版本"——它在 Linux 上读
	// /proc/self/exe，返回的是当前进程加载时就已解析到底的真实文件（也就是
	// 切换前那个版本），并不会因为重写了 current 符号链接而改变。要验证刚
	// 切换到的新版本，必须显式执行 currentLink（符号链接，每次 exec 都会
	// 重新解析到它当前指向的文件）。
	if err := probeBinary(currentLink(p)); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("新版本启动校验失败，回退到旧版本：%v"), err))
		if prevTarget != "" {
			swapCurrentLink(p, prevTarget) //nolint:errcheck // 回退已在出错路径，尽力而为
		}
		return "", false, fmt.Errorf(i18n.T("已回退到原版本：%w"), err)
	}

	if channel == Stable {
		// 记录本次稳定版，供之后切到预览版的用户一键回退；失败不影响本次更新
		// 结果本身，下次稳定渠道更新时会自然覆盖重试。
		atomicSymlink(newBin, lastStableLink(p)) //nolint:errcheck
	}

	pruneOldVersions(p, version)
	execx.Ok(fmt.Sprintf(i18n.T("sboxkit 已更新到 %s。"), version))
	return version, false, nil
}

func exeSelf() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// ensureManagedByCurrentLink 首次自更新迁移：若正在运行的可执行文件还不是
// current 符号链接（apt 刚装好的普通文件），把它搬进版本目录当基线版本，
// 再把该路径替换成指向 current 的符号链接。之后的更新只需重写 current。
//
// 判断"是否已迁移"不能用 os.Readlink(exe)：os.Executable() 在 Linux 上读
// /proc/self/exe，内核已经把符号链接完全解析成真实文件路径，exe 本身永远
// 不是符号链接（迁移与否都一样），Readlink 对它总会失败。真正可靠的判断是
// 看这个已解析的真实路径是否已经落在 versionsDir 下——已经在，说明早迁移
// 过了；如果这里误判成"未迁移"又重新执行一遍复制，会把正在运行的二进制
// 自身当成迁移源尝试原地覆写自己，触发 "text file busy"。
func ensureManagedByCurrentLink(p paths.Paths, currentVersion string) error {
	if err := os.MkdirAll(versionsDir(p), 0o755); err != nil {
		return err
	}
	exe := exeSelf()
	if exe == "" {
		return fmt.Errorf("%s", i18n.T("无法定位当前运行的可执行文件"))
	}
	if strings.HasPrefix(exe, versionsDir(p)+string(os.PathSeparator)) {
		return nil // 已经落在版本目录下，说明早迁移过了
	}
	baseline := strings.TrimPrefix(currentVersion, "v")
	if baseline == "" || baseline == "dev" {
		baseline = "installed"
	}
	baselineBin := versionBin(p, baseline)
	if err := os.MkdirAll(filepath.Dir(baselineBin), 0o755); err != nil {
		return err
	}
	if err := copyFileMode(exe, baselineBin, 0o755); err != nil {
		return err
	}
	if err := atomicSymlink(baselineBin, currentLink(p)); err != nil {
		return err
	}
	execx.Info(fmt.Sprintf(i18n.T("已把当前运行的可执行文件迁移为托管版本 %s。"), baseline))
	reason := i18n.T("首次自更新需要把 ") + exe + i18n.T(" 接管为指向托管版本的符号链接")
	if _, err := execx.RunRoot([]string{"ln", "-sfn", currentLink(p), exe}, reason, nil); err != nil {
		return err
	}
	return nil
}

// swapCurrentLink 原子重写 current 符号链接指向 target（versionsDir 属当前
// 用户所有，不需要 root）。
func swapCurrentLink(p paths.Paths, target string) error {
	return atomicSymlink(target, currentLink(p))
}

func atomicSymlink(target, linkPath string) error {
	tmp := linkPath + ".new"
	os.Remove(tmp)
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, linkPath)
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
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

// probeBinary 试跑新二进制，确认它能正常执行（"version" 子命令，无需 root/网络）。
func probeBinary(path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// pruneOldVersions 只保留 current 指向的版本、紧邻上一个版本、以及 last-stable
// 记录的稳定版（如果三者不同的话），其余版本目录删除。
func pruneOldVersions(p paths.Paths, keepVersion string) {
	entries, err := os.ReadDir(versionsDir(p))
	if err != nil {
		return
	}
	var versions []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != "current" {
			versions = append(versions, e.Name())
		}
	}
	sort.Strings(versions)
	keep := map[string]bool{keepVersion: true}
	if idx := sort.SearchStrings(versions, keepVersion); idx > 0 {
		keep[versions[idx-1]] = true
	}
	if target, err := os.Readlink(lastStableLink(p)); err == nil {
		keep[filepath.Base(filepath.Dir(target))] = true
	}
	for _, v := range versions {
		if !keep[v] {
			os.RemoveAll(filepath.Join(versionsDir(p), v))
		}
	}
}

func verifySHA256(archivePath, sumsPath, name string) error {
	sums, err := os.ReadFile(sumsPath)
	if err != nil {
		return err
	}
	var want string
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			want = strings.ToLower(fields[0])
			break
		}
	}
	if want == "" {
		return fmt.Errorf(i18n.T("checksums.txt 里没有 %s 的记录"), name)
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf(i18n.T("SHA-256 校验失败：期望 %s，实际 %s"), want, got)
	}
	return nil
}

func extractTarGz(archive, outDir string) error {
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
	cleanOut := filepath.Clean(outDir)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(outDir, hdr.Name)
		if target != cleanOut && !strings.HasPrefix(target, cleanOut+string(os.PathSeparator)) {
			return fmt.Errorf(i18n.T("非法压缩条目路径: %s"), hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode&0o777))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
}
