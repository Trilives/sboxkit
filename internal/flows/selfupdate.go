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
	prompt := i18n.T("发现新版本 %s（当前 %s），现在更新？")
	if channel == selfupdate.Stable && selfupdate.Older(latest, currentVersion) {
		// 从更新的预览版切回稳定渠道：目标版本更旧，不是「新版本」。
		prompt = i18n.T("稳定版 %s 早于当前版本 %s，切换到稳定版？")
	}
	ok, err := tui.Confirm(fmt.Sprintf(prompt, latest, currentVersion), true)
	if err != nil || !ok {
		return err
	}
	tool, alreadyLatest, err := selfupdate.Prepare(p, currentVersion, channel)
	if err != nil {
		return err
	}
	if alreadyLatest {
		execx.Ok(fmt.Sprintf(i18n.T("已是最新版本（%s）。"), currentVersion))
		return nil
	}
	if err := runUpgrade(p, &upgrade{tool: tool}, upgradeMode{interactive: true, sync: true}); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("sboxkit 已更新到 %s，下次运行即生效（届时自动同步服务单元、Web 面板等运行时资源）。"), tool.Version))
	return nil
}

// doRollbackStable 回退到已记录的稳定版：与更新一样先校验该版本的生成器与当前内核的组合。
func doRollbackStable(p paths.Paths) error {
	tool, err := selfupdate.PrepareStable(p)
	if err != nil {
		return err
	}
	if err := runUpgrade(p, &upgrade{tool: tool}, upgradeMode{interactive: true, sync: true}); err != nil {
		return err
	}
	execx.Ok(fmt.Sprintf(i18n.T("已回退到稳定版 %s，下次运行即生效（届时自动同步服务单元、Web 面板等运行时资源）。"), tool.Version))
	return nil
}
