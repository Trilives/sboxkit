// 暂存更新：内核与规则集先下载到 state/downloads 下的私有暂存目录，调用方校验
// 「生成器 + 内核 + 规则集 + 生效订阅」组合通过后才 Commit 原子换入状态目录，
// 否则 Discard，状态目录里正在使用的组合保持不变。
package kernel

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

// StageOptions 选择要暂存的组件。
type StageOptions struct {
	Core  bool
	Geo   bool
	Force bool
}

// Staged 已下载、尚未换入状态目录的组件；未暂存的组件对应字段为空。
type Staged struct {
	dir     string
	Kernel  string
	Version string
	Geosite string
	GeoIP   string
}

// NewStaging 在 state/downloads 下创建空的暂存目录（与状态目录同一文件系统，换入为原子改名）。
func NewStaging(p paths.Paths) (*Staged, error) {
	if err := p.EnsureStateDirs(); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(p.Downloads, "staged-*")
	if err != nil {
		return nil, err
	}
	return &Staged{dir: dir}, nil
}

// Stage 下载所选组件的最新版到暂存目录。
func Stage(p paths.Paths, opts StageOptions) (*Staged, error) {
	s, err := NewStaging(p)
	if err != nil {
		return nil, err
	}
	if err := s.fetch(p, opts); err != nil {
		s.Discard()
		return nil, err
	}
	return s, nil
}

func (s *Staged) fetch(p paths.Paths, opts StageOptions) error {
	f, settings := NewFetcher(p)
	if opts.Core {
		bin := filepath.Join(s.dir, "sing-box")
		version, err := fetchCore(p, f, settings, opts.Force, bin)
		if err != nil {
			return err
		}
		s.Kernel, s.Version = bin, version
	}
	if opts.Geo {
		geosite, geoip := filepath.Join(s.dir, "geosite-cn.srs"), filepath.Join(s.dir, "geoip-cn.srs")
		if err := fetchGeo(p, f, settings, opts.Force, geosite, geoip); err != nil {
			return err
		}
		s.Geosite, s.GeoIP = geosite, geoip
	}
	return nil
}

// StageCore 向已有暂存追加最新内核（例如规则集更新时用户同意一并更新内核）。
func (s *Staged) StageCore(p paths.Paths, force bool) error {
	return s.fetch(p, StageOptions{Core: true, Force: force})
}

// Dir 暂存目录。
func (s *Staged) Dir() string { return s.dir }

// Bundle 换入后服务使用的组合：未暂存的组件沿用状态目录现用文件。
func (s *Staged) Bundle(p paths.Paths) compat.Bundle {
	b := compat.StateBundle(p)
	if s.Kernel != "" {
		b.Kernel = s.Kernel
	}
	if s.Geosite != "" {
		b.Geosite = s.Geosite
	}
	if s.GeoIP != "" {
		b.GeoIP = s.GeoIP
	}
	return b
}

// Commit 把暂存组件换入状态目录，返回把改动前文件换回的回退函数；换入中途失败时
// 已换入的文件随即换回。成功后调用方仍需 Discard 清理暂存目录（含被换下的旧文件）。
func (s *Staged) Commit(p paths.Paths) (func() error, error) {
	var swaps []fileSwap
	if s.Kernel != "" {
		version := filepath.Join(s.dir, "sing-box.version")
		if err := os.WriteFile(version, []byte(s.Version+"\n"), 0o644); err != nil {
			return nil, err
		}
		swaps = append(swaps, fileSwap{s.Kernel, p.SingBoxBin}, fileSwap{version, p.SingBoxVersion})
	}
	if s.Geosite != "" {
		swaps = append(swaps, fileSwap{s.Geosite, p.GeositeCN})
	}
	if s.GeoIP != "" {
		swaps = append(swaps, fileSwap{s.GeoIP, p.GeoIPCN})
	}
	restore, err := swapIn(s.dir, swaps)
	if err != nil {
		return nil, err
	}
	if s.Kernel != "" {
		execx.Ok(i18n.T("内核已部署: ") + s.Version)
	}
	if s.Geosite != "" || s.GeoIP != "" {
		execx.Ok(i18n.T("geo 数据已更新"))
	}
	return restore, nil
}

// Discard 删除暂存目录。
func (s *Staged) Discard() {
	if s != nil && s.dir != "" {
		os.RemoveAll(s.dir)
	}
}

type fileSwap struct{ src, dst string }

type swapped struct {
	dst, backup string // backup 为空表示 dst 原本不存在
}

// swapIn 依次把 src 改名为 dst，原 dst 先改名备份到 dir。
func swapIn(dir string, swaps []fileSwap) (func() error, error) {
	var done []swapped
	restore := func() error {
		var firstErr error
		for i := len(done) - 1; i >= 0; i-- {
			if err := done[i].undo(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	for i, sw := range swaps {
		item := swapped{dst: sw.dst}
		if _, err := os.Lstat(sw.dst); err == nil {
			item.backup = filepath.Join(dir, fmt.Sprintf("previous-%d-%s", i, filepath.Base(sw.dst)))
			if err := os.Rename(sw.dst, item.backup); err != nil {
				restore()
				return nil, err
			}
		}
		if err := os.Rename(sw.src, sw.dst); err != nil {
			if item.backup != "" {
				os.Rename(item.backup, sw.dst)
			}
			restore()
			return nil, err
		}
		done = append(done, item)
	}
	return restore, nil
}

func (s swapped) undo() error {
	if s.backup == "" {
		if err := os.Remove(s.dst); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.Rename(s.backup, s.dst)
}
