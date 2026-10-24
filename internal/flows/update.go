package flows

import (
	"errors"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

// updateMenuFlow aggregates the independent core, geo and sboxkit update
// channels under Tools. The embedded Web UI follows the sboxkit binary.
func updateMenuFlow(p paths.Paths, currentVersion string) error {
	options := []string{i18n.T("内核"), i18n.T("geo 数据"), i18n.T("sboxkit 自身")}
	handlers := []func() error{
		func() error { return updateCoreOnly(p) },
		func() error { return updateGeoOnly(p) },
		func() error { return SelfUpdateFlow(p, currentVersion) },
	}
	idx := 0
	for {
		i, err := tui.Select(i18n.T("更新"), options, tui.SelectOpts{BackLabel: i18n.T("返回上层"), Initial: idx})
		if err != nil {
			return nil
		}
		idx = i
		if err := handlers[i](); err != nil {
			if errors.Is(err, errs.ErrCancelled) {
				continue
			}
			execx.Error(err.Error())
		}
	}
}

func updateCoreOnly(p paths.Paths) error {
	ensureGithubToken(p)
	return UpdateAssets(p, kernel.StageOptions{Core: true, Force: true}, true, true)
}

func updateGeoOnly(p paths.Paths) error {
	ensureGithubToken(p)
	return UpdateAssets(p, kernel.StageOptions{Geo: true, Force: true}, true, true)
}

// UpdateAssets 更新内核和/或规则集（菜单「内核」「geo 数据」、`sboxkit update`、
// `sboxkit assets-update` 与初始化后的自动更新共用）：最新版先暂存，校验组合通过后
// 才换入状态目录并同步服务。interactive=false 时凡需用户决定的情形一律放弃更新；
// sync=false 时只更新状态目录、不碰服务。
func UpdateAssets(p paths.Paths, opts kernel.StageOptions, interactive, sync bool) error {
	staged, err := kernel.Stage(p, opts)
	if err != nil {
		return err
	}
	return runUpgrade(p, &upgrade{assets: staged}, upgradeMode{interactive: interactive, sync: sync})
}
