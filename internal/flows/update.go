package flows

import (
	"errors"

	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
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
	f, settings := kernel.NewFetcher(p)
	if _, err := kernel.UpdateCore(p, f, settings, true); err != nil {
		return err
	}
	return syncRestartIfInstalled(p)
}

func updateGeoOnly(p paths.Paths) error {
	ensureGithubToken(p)
	f, settings := kernel.NewFetcher(p)
	if err := kernel.UpdateGeodata(p, f, settings, true); err != nil {
		return err
	}
	return syncRestartIfInstalled(p)
}

func syncRestartIfInstalled(p paths.Paths) error {
	if fileExists(p.ConfigFile) && sysd.IsInstalled(sysd.DefaultName) {
		return sysd.SyncAndRestart(p, sysd.DefaultName)
	}
	return nil
}
