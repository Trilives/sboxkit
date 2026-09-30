package i18n

// 用户模式（tar.gz 安装包、systemd 用户单元、随包内核）相关文案。
func init() {
	register(map[string]string{
		"无法连接 systemd 用户实例（systemctl --user），请在已登录的用户会话中运行，或确认 XDG_RUNTIME_DIR 已设置": "Cannot reach the systemd user instance (systemctl --user); run from a logged-in user session or make sure XDG_RUNTIME_DIR is set",
		"未能开启 linger：注销后用户级服务会停止、开机也不会自动启动。可手动执行 sudo loginctl enable-linger ":      "Could not enable linger: the user service will stop at logout and will not start at boot. You can run: sudo loginctl enable-linger ",
		"已开启 linger：用户级服务在注销后继续运行，并随开机启动。":                                          "Linger enabled: the user service keeps running after logout and starts at boot.",
		"检测到系统级服务 %s，它与用户模式用户级服务会争抢相同端口；请先在系统版 sboxkit 中暂停或卸载。":                     "System service %s detected; it competes with the user mode service for the same ports. Pause or uninstall it from the system-mode sboxkit first.",
		"用户模式用户级服务不安装 NetworkManager 钩子（需要 root），由 watchdog 定时器兜底。":                 "User mode does not install the NetworkManager hook (requires root); the watchdog timer covers recovery.",
		"已从用户模式安装包接管 %d 个种子文件（离线可用；后续可在线更新）。":                                       "Took over %d seed files from the user mode bundle (works offline; can be updated online later).",
		"运行时目录": "Runtime directory",
		"用户模式无法替换 %s（%v）；请把用户模式安装包放在当前用户可写的位置":                   "User mode cannot replace %s (%v); keep the user mode bundle somewhere writable by the current user",
		"用户模式状态目录不可写：%s（请把用户模式安装包解压到当前用户可写的位置，或设置 SBOXKIT_HOME）": "User mode state directory is not writable: %s (extract the user mode bundle somewhere writable by the current user, or set SBOXKIT_HOME)",
		"用户模式安装包已附带内核与基础规则集，跳过初始化后的自动下载；可稍后在「工具 → 更新」中更新。":       "The user mode bundle ships the core and base rule-sets, so the post-init download is skipped; update later from Tools → Update.",
		"（用户模式不可用）":   " (unavailable in user mode)",
		"（需 sudo 授权）": " (asks for sudo)",
		"用户模式不会修改防火墙；如本机启用了防火墙，请自行放行 %d 端口。":                            "User mode does not modify the firewall; if a firewall is active, allow port %d yourself.",
		"用户模式启用 TUN：应用配置时会请求一次 sudo，为 sing-box 授予网络管理能力（setcap）。":       "Enabling TUN in user mode: applying the config asks for sudo once to grant sing-box network-admin capabilities (setcap).",
		"配置启用了 TUN：将通过 sudo 为运行时 sing-box 授予网络管理能力（setcap），之后重启无需再次授权。": "The config enables TUN: sudo is needed once to grant the runtime sing-box network-admin capabilities (setcap); later restarts need no further authorization.",
		"为 sing-box 授予 TUN 所需能力":                                              "Grant sing-box the capabilities TUN needs",
		"启用 TUN 需要 setcap 命令（Debian/Ubuntu: libcap2-bin，Fedora/Arch: libcap）": "TUN needs the setcap command (Debian/Ubuntu: libcap2-bin, Fedora/Arch: libcap)",
		"运行时目录 %s 所在文件系统以 nosuid 挂载，TUN 所需的文件能力不会生效；请把用户模式安装包放到其他位置": "The runtime directory %s is on a nosuid mount, so the file capabilities TUN needs would be ignored; move the user mode bundle elsewhere",
		"为 %s 授予网络能力失败：文件系统可能不支持扩展属性":                                "Failed to grant network capabilities to %s: the filesystem may not support extended attributes",
		"新内核已下载，但启用 TUN 需要 sudo 为其授予网络能力，当前无法交互授权：服务继续使用旧内核（规则集与配置已更新）。请稍后在 sboxkit 中执行『运行时管理 → 服务设置 → 重载服务』完成切换。": "A new core was downloaded, but TUN needs sudo to grant it network capabilities and no interactive authorization is possible now: the service keeps the old core (rule-sets and config were updated). Run \"Runtime management → Service settings → Reload service\" in sboxkit later to switch.",
		"已下载的新内核尚未被服务使用：请执行『运行时管理 → 服务设置 → 重载服务』（启用 TUN 时需一次 sudo 授权）。":                                            "The downloaded core is not in use by the service yet: run \"Runtime management → Service settings → Reload service\" (asks for sudo once when TUN is enabled).",
		"记录 linger 状态失败（卸载时不会自动关闭 linger）：%v":                                "Failed to record the linger state (uninstall will not turn linger off automatically): %v",
		"未找到 loginctl，无法关闭 sboxkit 开启的 linger；可手动执行 loginctl disable-linger": "loginctl not found, cannot turn off the linger sboxkit enabled; run loginctl disable-linger manually",
		"关闭 linger 失败，可手动执行 loginctl disable-linger：%w":                      "Failed to turn off linger; run loginctl disable-linger manually: %w",
		"已关闭 sboxkit 开启的 linger（恢复安装前的登录会话设置）。":                              "Turned off the linger sboxkit enabled (login session settings restored to their pre-install state).",
		"用户模式请以安装包目录的属主（UID %d）运行，不要使用 sudo 或 root；启用 TUN 时 sboxkit 会单独请求一次 sudo。需要系统级部署请安装 .deb（系统模式）。": "Run user mode as the owner of the bundle directory (UID %d), not with sudo or as root; sboxkit asks for sudo separately, once, when TUN is enabled. For a system-wide deployment install the .deb (system mode).",
		"系统模式服务 %s 正在运行，会占用用户模式服务所需的端口；请先在系统模式 sboxkit 中暂停或卸载（sudo sboxkit pause）后再试。":                   "The system mode service %s is running and holds the ports the user mode service needs; pause or uninstall it from system mode sboxkit first (sudo sboxkit pause), then retry.",
		"网络自愈（watchdog）": "Network self-healing (watchdog)",
		"用户模式：移除全部项目后，再删除安装包目录 %s 即可彻底清除。":           "User mode: after removing every item, delete the bundle directory %s to remove sboxkit completely.",
		"提示：暂停为运行时停止，下次开机（已开启 linger）或重新登录后会自动恢复运行。": "Tip: pausing only stops the running service; it resumes automatically at the next boot (with linger) or the next login.",
	})
}
