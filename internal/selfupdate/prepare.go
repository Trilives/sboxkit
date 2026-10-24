package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
)

// Prepared 已下载、校验并试跑通过，但尚未切换 current 的 sboxkit 版本。
type Prepared struct {
	Version   string
	Bin       string
	channel   Channel // 空表示回退到已记录的稳定版
	current   string  // 当前运行版本（首次自更新迁移的基线名）
	created   bool    // 版本目录由本次 Prepare 新建：放弃时删除
	activated bool
}

// Prepare 下载并校验 channel 上的最新版本，不切换 current；latest=true 表示当前已是
// 最新版本（返回的 Prepared 为 nil）。
func Prepare(p paths.Paths, currentVersion string, channel Channel) (*Prepared, bool, error) {
	f, s := kernel.NewFetcher(p)
	rel, err := fetchRelease(f, channel)
	if err != nil {
		return nil, false, err
	}
	version := strings.TrimPrefix(rel.TagName, "v")
	if version == "" {
		return nil, false, fmt.Errorf("%s", i18n.T("发行版没有有效的版本号"))
	}
	if version == strings.TrimPrefix(currentVersion, "v") {
		return nil, true, nil
	}
	name := assetName(version)
	assetURL := findAssetURL(rel, name)
	if assetURL == "" {
		return nil, false, fmt.Errorf(i18n.T("未找到本机架构的发行包: %s"), name)
	}
	sumsURL := findAssetURL(rel, "checksums.txt")
	if sumsURL == "" {
		return nil, false, fmt.Errorf("%s", i18n.T("发行版缺少 checksums.txt，无法校验完整性"))
	}
	archive, err := downloadVerified(p, f, s, name, assetURL, sumsURL)
	if err != nil {
		return nil, false, err
	}
	defer os.Remove(archive)
	bin := versionBin(p, version)
	_, statErr := os.Stat(filepath.Dir(bin))
	// 已存在的版本目录可能正被 last-stable 引用：只在新内容试跑通过后才替换，放弃时不删除。
	t := &Prepared{Version: version, Bin: bin, channel: channel, current: currentVersion, created: os.IsNotExist(statErr)}
	if err := t.extract(p, archive); err != nil {
		return nil, false, err
	}
	return t, false, nil
}

func downloadVerified(p paths.Paths, f fetcher, s kernel.Settings, name, assetURL, sumsURL string) (string, error) {
	if err := os.MkdirAll(p.Downloads, 0o755); err != nil {
		return "", err
	}
	archivePath := filepath.Join(p.Downloads, name)
	sumsPath := filepath.Join(p.Downloads, name+".sha256")
	defer os.Remove(sumsPath)
	execx.Info(i18n.T("下载 sboxkit: ") + assetURL)
	if err := f.FetchFile(kernel.Mirror(assetURL, s.GithubMirror), archivePath); err != nil {
		return "", err
	}
	if err := f.FetchFile(kernel.Mirror(sumsURL, s.GithubMirror), sumsPath); err != nil {
		os.Remove(archivePath)
		return "", err
	}
	if err := verifySHA256(archivePath, sumsPath, name); err != nil {
		os.Remove(archivePath)
		return "", err
	}
	execx.Ok(i18n.T("SHA-256 校验通过。"))
	return archivePath, nil
}

type fetcher interface {
	FetchFile(rawURL, out string) error
}

// extract 解压到版本目录旁的临时目录并试跑，通过后才换到版本目录：失败时已有的
// 版本目录（可能被 last-stable 引用）保持原样。
func (t *Prepared) extract(p paths.Paths, archive string) error {
	verDir := filepath.Dir(t.Bin)
	if err := os.MkdirAll(versionsDir(p), 0o755); err != nil {
		return err
	}
	incoming, err := os.MkdirTemp(versionsDir(p), ".incoming-"+t.Version+"-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(incoming)
	if err := os.Chmod(incoming, 0o755); err != nil {
		return err
	}
	if err := extractTarGz(archive, incoming); err != nil {
		return err
	}
	bin := filepath.Join(incoming, filepath.Base(t.Bin))
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("%s", i18n.T("解压后未找到 sboxkit 可执行文件"))
	}
	if err := os.Chmod(bin, 0o755); err != nil {
		return err
	}
	if err := probeBinary(bin); err != nil {
		return fmt.Errorf(i18n.T("新版本二进制无法正常运行，已放弃更新：%w"), err)
	}
	return replaceDir(incoming, verDir)
}

// replaceDir 用 src 替换 dst：旧目录先改名让位，换入成功后再删除。
func replaceDir(src, dst string) error {
	old := ""
	if _, err := os.Stat(dst); err == nil {
		old = dst + ".old"
		os.RemoveAll(old)
		if err := os.Rename(dst, old); err != nil {
			return err
		}
	}
	if err := os.Rename(src, dst); err != nil {
		if old != "" {
			os.Rename(old, dst)
		}
		return err
	}
	if old != "" {
		os.RemoveAll(old)
	}
	return nil
}

// PrepareStable 以 last-stable 记录的稳定版为目标（试跑通过，不切换）。
func PrepareStable(p paths.Paths) (*Prepared, error) {
	target, err := os.Readlink(lastStableLink(p))
	if err != nil {
		return nil, fmt.Errorf("%s", i18n.T("尚未记录过稳定版，无法回退"))
	}
	if err := probeBinary(target); err != nil {
		return nil, fmt.Errorf(i18n.T("记录的稳定版二进制无法正常运行：%w"), err)
	}
	return &Prepared{Version: filepath.Base(filepath.Dir(target)), Bin: target}, nil
}

// Activate 把 current 切换到该版本并试跑；返回切回原版本的回退函数。
func (t *Prepared) Activate(p paths.Paths) (func() error, error) {
	if t.channel != "" {
		if err := ensureManagedByCurrentLink(p, t.current); err != nil {
			return nil, err
		}
	}
	prevTarget, _ := os.Readlink(currentLink(p))
	if err := swapCurrentLink(p, t.Bin); err != nil {
		return nil, err
	}
	undo := func() error {
		var err error
		if prevTarget == "" {
			err = os.Remove(currentLink(p))
		} else {
			err = swapCurrentLink(p, prevTarget)
		}
		if err == nil {
			t.activated = false // 只有 current 已切回，Discard 才可删除该版本目录
		}
		return err
	}
	// 注意：不能用 os.Executable() 来试跑"切换后的版本"——它在 Linux 上读
	// /proc/self/exe，返回的是当前进程加载时就已解析到底的真实文件（也就是
	// 切换前那个版本），并不会因为重写了 current 符号链接而改变。要验证刚
	// 切换到的新版本，必须显式执行 currentLink（符号链接，每次 exec 都会
	// 重新解析到它当前指向的文件）。
	if err := probeBinary(currentLink(p)); err != nil {
		execx.Warn(fmt.Sprintf(i18n.T("新版本启动校验失败，回退到旧版本：%v"), err))
		undo() //nolint:errcheck // 回退已在出错路径，尽力而为
		return nil, fmt.Errorf(i18n.T("已回退到原版本：%w"), err)
	}
	t.activated = true
	return undo, nil
}

// Finish 激活并提交后：稳定渠道记录 last-stable，清理旧版本目录。
func (t *Prepared) Finish(p paths.Paths) {
	if t.channel == Stable {
		// 记录本次稳定版，供之后切到预览版的用户一键回退；失败不影响本次更新
		// 结果本身，下次稳定渠道更新时会自然覆盖重试。
		atomicSymlink(t.Bin, lastStableLink(p)) //nolint:errcheck
	}
	if t.channel != "" {
		pruneOldVersions(p, t.Version)
	}
}

// Discard 未激活时删除本次新建的版本目录。
func (t *Prepared) Discard() {
	if t != nil && t.created && !t.activated {
		os.RemoveAll(filepath.Dir(t.Bin))
	}
}
