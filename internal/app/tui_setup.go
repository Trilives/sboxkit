package app

import "fmt"

func runTUIFirstSetup(s *tuiSession) bool {
	return commandAction("首次初始化", func(s *tuiSession) int {
		args := []string{}
		if !s.confirm("是否启用 TUN 模式？", true) {
			args = append(args, "--no-tun")
			if s.confirm("TUN 已关闭，是否将 Shell 代理变量写入 ~/.bashrc？", false) {
				args = append(args, "--write-proxy-env")
			} else {
				args = append(args, "--no-write-proxy-env")
			}
		}
		code := initState("", args, s.stdout, s.stderr)
		if code != 0 {
			return code
		}
		if s.confirm("现在导入订阅或本地配置吗？", true) {
			if s.confirm("是否从链接导入？", true) {
				code = runTUIAddRemoteSubscriptionCommand(s)
			} else {
				code = runTUIAddLocalConfigCommand(s)
			}
			if code != 0 {
				return code
			}
		}
		if s.confirm("现在安装并启动 sboxkit.service 吗？", true) && s.confirmServiceTrafficRisk("安装并启动 sboxkit.service") {
			code = runService([]string{"install"}, s.stdout, s.stderr)
			if code != 0 {
				return code
			}
			fmt.Fprintln(s.stdout, "\n服务已启动，正在通过本地代理下载可选规则集，然后重启服务。")
			code = runUpdate(firstSetupPostStartUpdateArgs(), s.stdout, s.stderr)
		}
		return code
	})(s)
}

func firstSetupPostStartUpdateArgs() []string {
	return []string{"--proxy", "http://127.0.0.1:7890", "--sync-service"}
}
