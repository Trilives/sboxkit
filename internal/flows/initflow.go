// 初始化（首次部署）全流程（对应 flows/init.py）。
// 每个独立配置组件各自一个事务：某一步 ESC / 出错只回退它自己已应用的改动，
// 不会连带撤销更早已经成功提交的步骤（例如服务已注册启动后，后续下载资源
// 失败不应该把已经跑起来的服务也卸载掉）。语言选择由调用方在进入 Init 前
// 完成，不属于本流程的一部分。
package flows

import (
	"errors"
	"fmt"
	"os"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/errs"
	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/firewall"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/proxyenv"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
	"github.com/Trilives/sboxkit/internal/txn"
)

// Init 初始化流程。
func Init(p paths.Paths) error {
	execx.Header(i18n.T("初始化（首次部署）"))

	// 0. deb 种子接管（若从系统包安装，离线即可获得内核与基础规则）
	if _, err := kernel.SeedFromSystem(p); err != nil {
		execx.Warn(i18n.T("种子接管失败（不影响后续下载）：") + err.Error())
	}

	// 1. 部署设置：TUN / 局域网代理，独立一个事务。
	if err := initDeploymentSettings(p); err != nil {
		return err
	}

	// 2. 添加首个订阅（Clash / base64 / 本地 YAML 文件三选一），独立一个事务：
	// 这里取消/出错只回退订阅本身，不影响步骤 1 已保存的部署设置。
	ready, err := addInitialSubscription(p)
	if err != nil {
		return err
	}
	if !ready {
		execx.Info(i18n.T("已跳过订阅与服务注册，结束初始化。设置已保存，") +
			i18n.T("稍后可在主菜单「订阅 → 添加订阅」补配并启动服务。"))
		return nil
	}

	// 3. 注册并启动 systemd 服务，独立一个事务：deb 安装已内置内核与基础规则，
	// 优先直接使用；非 deb / 资源缺失场景才在启动前下载兜底。一旦服务成功启动，
	// 后续可选步骤（更新资源/自愈/定时器）失败都不应该把它卸载回退。
	installed, err := registerService(p)
	if err != nil {
		return err
	}
	if !installed {
		return nil
	}

	// 4. 服务已经跑起来；是否顺带更新内核/geo 数据属于锦上添花，失败只提示，
	// 不影响已经成功启动的服务。
	if err := optionalPostStartUpdate(p); err != nil && !errors.Is(err, errs.ErrCancelled) {
		execx.Warn(fmt.Sprintf(i18n.T("更新内核/geo 数据失败（服务仍按原资源正常运行）：%v"), err))
	}

	// 5. 可选增强：网络自愈 / 每周更新，各自独立，互不影响、也不影响服务本身。
	optionalExtras()

	// 6. 提示是否临时切换节点
	ok, err := tui.Confirm(i18n.T("配置已就绪，是否现在切换节点？"), false)
	if err != nil {
		return err
	}
	if ok {
		if err := NodeSwitchLive(p, p.ConfigFile, ""); err != nil {
			return err
		}
	}

	execx.Ok(i18n.T("初始化完成。"))
	printAccessHint(p)
	return nil
}

// initDeploymentSettings 步骤 1：TUN / 局域网代理，独立事务。
func initDeploymentSettings(p paths.Paths) error {
	return txn.Run(i18n.T("部署设置"), func(t *txn.Transaction) error {
		cfg := config.Load(p)
		// TUN 模式：全局透明代理；关则纯代理，需各 App 自设代理
		enableTun, err := tui.Confirm(i18n.T("启用 TUN 模式？（整机流量自动走代理；否=纯代理，需各 App 手动设代理）"),
			cfg.EnableTun)
		if err != nil {
			return err
		}
		cfg.EnableTun = enableTun
		lanProxy, err := tui.Confirm(i18n.T("开启局域网代理？（让局域网其他主机可用本机作为代理，监听 0.0.0.0:7890）"),
			cfg.LanProxy)
		if err != nil {
			return err
		}
		cfg.LanProxy = lanProxy
		if err := t.BackupFile(p.CustomizeFile); err != nil {
			return err
		}
		if err := config.Save(p, cfg); err != nil {
			return err
		}

		// TUN 关闭=纯代理：可选把代理变量写入 bashrc
		if !enableTun {
			ok, err := tui.Confirm(i18n.T("把代理环境变量写入 ~/.bashrc？（新开终端自动走 127.0.0.1:7890）"), true)
			if err != nil {
				return err
			}
			if ok {
				if err := t.BackupFile(proxyenv.TargetBashrc()); err != nil {
					return err
				}
				if _, err := proxyenv.Write(); err != nil {
					return err
				}
			}
		}

		// 局域网代理需放行防火墙端口
		if lanProxy {
			ok, err := tui.Confirm(i18n.T("更新防火墙放行 7890 端口？"), true)
			if err != nil {
				return err
			}
			if ok {
				t.AddUndo(i18n.T("撤销防火墙放行 7890"), func() error { firewall.Revoke(firewall.ProxyPort); return nil })
				firewall.Allow(firewall.ProxyPort)
			}
		}
		return nil
	})
}

// addInitialSubscription 步骤 2：独立事务；ready=false 表示用户主动跳过
// （非取消/出错），Init 据此结束流程而不注册服务。
func addInitialSubscription(p paths.Paths) (bool, error) {
	ready := false
	err := txn.Run(i18n.T("添加订阅"), func(t *txn.Transaction) error {
		r, err := initialConfigSource(p, t)
		ready = r
		return err
	})
	return ready, err
}

// registerService 步骤 3：独立事务；installed=false 表示用户取消（已回退），
// Init 据此结束流程而不继续后续可选步骤。
func registerService(p paths.Paths) (bool, error) {
	installed := false
	err := txn.Run(i18n.T("注册服务"), func(t *txn.Transaction) error {
		if err := ensureStartupResources(p); err != nil {
			return err
		}
		t.AddUndo(i18n.T("卸载服务 sing-box"), func() error { return sysd.Remove(p, sysd.DefaultName, true) })
		if err := sysd.Install(p, sysd.DefaultName, true); err != nil {
			return err
		}
		installed = true
		return nil
	})
	return installed, err
}

// initialConfigSource 添加首个订阅。若状态目录（/var/lib/sboxkit）里已有
// 订阅记录——典型场景是运行 migrate-runtime-dir.sh 后重新初始化，订阅数据本
// 就没被清理过——询问是否直接复用现有订阅，跳过重新添加。否则与「配置变更 →
// 添加订阅」共用同一个三选一来源选择器（Clash / base64 / 本地 YAML 文件），
// 本地文件此时也是作为一个真正的订阅条目创建，而不是走单独的「本地文件覆盖」
// 直接改写路径。
func initialConfigSource(p paths.Paths, t *txn.Transaction) (bool, error) {
	if err := t.BackupFile(p.ConfigFile); err != nil {
		return false, err
	}
	if err := t.BackupFile(p.ActiveFile); err != nil {
		return false, err
	}

	if existing := subscription.ListAll(p); len(existing) > 0 {
		useLocal, err := tui.Confirm(
			fmt.Sprintf(i18n.T("检测到本地已有 %d 个订阅记录，是否直接使用现有订阅？"), len(existing)), true)
		if err != nil {
			return false, err
		}
		if useLocal {
			target := existing[0].Name
			if active := subscription.GetActive(p); active != nil {
				target = active.Name
			}
			if err := subscription.Switch(p, target); err != nil {
				return false, err
			}
			execx.Ok(fmt.Sprintf(i18n.T("已使用现有订阅：%s"), target))
			return true, nil
		}
	}

	info, err := askNewSubscription()
	if err != nil {
		return false, err
	}
	if info == nil {
		return false, nil
	}
	sub, err := subscription.Add(p, info.Name, info.URL, info.SourceType, info.ApplyOverlay, true, info.FetchViaProxy)
	if err != nil {
		return false, err
	}
	t.AddUndo(i18n.T("删除订阅 ")+sub.Name, func() error { return subscription.RemoveSub(p, sub.Name) })
	return true, nil
}

func startupResourcesReady(p paths.Paths) bool {
	if _, err := os.Stat(p.SingBoxBin); err != nil {
		return false
	}
	if _, err := os.Stat(p.GeositeCN); err != nil {
		return false
	}
	if _, err := os.Stat(p.GeoIPCN); err != nil {
		return false
	}
	return true
}

func ensureStartupResources(p paths.Paths) error {
	if startupResourcesReady(p) {
		execx.Info(i18n.T("使用本地内核与基础规则启动服务（系统包种子或既有资源）。"))
		return nil
	}
	execx.Warn(i18n.T("未找到本地内核或基础规则；非 .deb 安装/种子缺失时需要先下载才能启动服务。"))
	ok, err := tui.Confirm(i18n.T("现在下载内核和基础规则以便启动服务？"), true)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s", i18n.T("缺少 sing-box 内核或基础规则，无法注册并启动服务"))
	}
	ensureGithubToken(p)
	if _, err := kernel.DownloadAll(p, kernel.Options{}); err != nil {
		return err
	}
	return nil
}

func optionalPostStartUpdate(p paths.Paths) error {
	ok, err := tui.Confirm(i18n.T("服务已启动。现在下载/更新内核和 geo 数据？（内置 Web 面板已随服务部署，浏览器访问 http://host:9090/ui/ 即可查看/切换节点）"), false)
	if err != nil || !ok {
		return err
	}
	execx.Info(i18n.T("下载/更新内核 / geo 数据…"))
	ensureGithubToken(p)
	if _, err := kernel.DownloadAll(p, kernel.Options{Force: true}); err != nil {
		return err
	}
	execx.Info(i18n.T("已更新资源，重新部署运行时并重启服务…"))
	return sysd.Install(p, sysd.DefaultName, true)
}

// optionalExtras 步骤 5：网络自愈 / 每周更新各自独立一个事务，互不影响，
// 也不影响此前已经成功注册启动的服务；任一项失败只警告、不中断另一项。
func optionalExtras() {
	if err := installResilienceIfWanted(); err != nil && !errors.Is(err, errs.ErrCancelled) {
		execx.Warn(fmt.Sprintf(i18n.T("安装网络自愈失败：%v"), err))
	}
	if err := installTimerIfWanted(); err != nil && !errors.Is(err, errs.ErrCancelled) {
		execx.Warn(fmt.Sprintf(i18n.T("安装每周更新定时器失败：%v"), err))
	}
}

func installResilienceIfWanted() error {
	return txn.Run(i18n.T("网络自愈"), func(t *txn.Transaction) error {
		ok, err := tui.Confirm(i18n.T("安装网络切换自愈？"), true)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		t.AddUndo(i18n.T("卸载网络自愈"), func() error { return sysd.RemoveResilience(sysd.DefaultName) })
		return sysd.InstallResilience(sysd.ResilienceOptions{})
	})
}

func installTimerIfWanted() error {
	return txn.Run(i18n.T("每周更新定时器"), func(t *txn.Transaction) error {
		ok, err := tui.Confirm(i18n.T("安装每周自动更新定时器？"), false)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		t.AddUndo(i18n.T("卸载每周更新"), sysd.RemoveTimer)
		return sysd.InstallTimer("")
	})
}
