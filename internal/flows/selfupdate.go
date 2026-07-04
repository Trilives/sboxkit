// sboxkit 自更新交互流程（对应 internal/selfupdate 的下载 / 校验 / 切版本）。
package flows

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/selfupdate"
	"github.com/Trilives/sboxkit/internal/tui"
)

// SelfUpdateFlow 更新 sboxkit 自身：可选稳定版 / 预览版渠道；若曾经用稳定渠道
// 更新过、且当前不在那个版本上，额外提供「回退到稳定版」选项。
func SelfUpdateFlow(p paths.Paths, currentVersion string) error {
	options := []string{i18n.T("更新到稳定版"), i18n.T("更新到预览版（尝鲜，可能不稳定）")}
	if stableVer, ok := selfupdate.LastStableVersion(p); ok && stableVer != currentVersion {
		options = append(options, fmt.Sprintf(i18n.T("回退到稳定版 %s"), stableVer))
	}
	i, err := tui.Select(i18n.T("更新 sboxkit 自身"), options, tui.SelectOpts{BackLabel: i18n.T("返回上层")})
	if err != nil {
		return nil
	}
	switch i {
	case 0:
		return doSelfUpdate(p, currentVersion, selfupdate.Stable)
	case 1:
		return doSelfUpdate(p, currentVersion, selfupdate.Preview)
	case 2:
		return doRollbackStable(p)
	}
	return nil
}

func doSelfUpdate(p paths.Paths, currentVersion string, channel selfupdate.Channel) error {
	execx.Info(fmt.Sprintf(i18n.T("当前版本：%s，正在查询最新版本…"), currentVersion))
	latest, err := selfupdate.LatestVersion(p, channel)
	if err != nil {
		return err
	}
	if latest == currentVersion {
		execx.Ok(fmt.Sprintf(i18n.T("已是最新版本（%s）。"), currentVersion))
		return nil
	}
	ok, err := tui.Confirm(fmt.Sprintf(i18n.T("发现新版本 %s（当前 %s），现在更新？"), latest, currentVersion), true)
	if err != nil || !ok {
		return err
	}
	version, alreadyLatest, err := selfupdate.Update(p, currentVersion, channel)
	if err != nil {
		return err
	}
	if alreadyLatest {
		execx.Ok(fmt.Sprintf(i18n.T("已是最新版本（%s）。"), version))
		return nil
	}
	execx.Ok(fmt.Sprintf(i18n.T("sboxkit 已更新到 %s，下次运行即生效。"), version))
	return nil
}

func doRollbackStable(p paths.Paths) error {
	version, err := selfupdate.RollbackToStable(p)
	if err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("已回退到稳定版 %s，下次运行即生效。"), version))
	return nil
}
