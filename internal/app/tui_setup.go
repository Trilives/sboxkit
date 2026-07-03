package app

import "fmt"

func runTUIFirstSetup(s *tuiSession) bool {
	return commandAction("First setup wizard", func(s *tuiSession) int {
		args := []string{}
		if !s.confirm("Enable TUN mode?", true) {
			args = append(args, "--no-tun")
			if s.confirm("TUN is disabled. Write shell proxy variables to ~/.bashrc?", false) {
				args = append(args, "--write-proxy-env")
			} else {
				args = append(args, "--no-write-proxy-env")
			}
		}
		code := initState("", args, s.stdout, s.stderr)
		if code != 0 {
			return code
		}
		if s.confirm("Import a subscription or local config now?", true) {
			if s.confirm("Import from URL?", true) {
				code = runTUIAddRemoteSubscriptionCommand(s)
			} else {
				code = runTUIAddLocalConfigCommand(s)
			}
			if code != 0 {
				return code
			}
		}
		if s.confirm("Install and start sboxkit.service now?", true) && s.confirmServiceTrafficRisk("install and start sboxkit.service") {
			code = runService([]string{"install"}, s.stdout, s.stderr)
			if code != 0 {
				return code
			}
			fmt.Fprintln(s.stdout, "\nDownloading optional runtime rules through the running local proxy, then restarting the service.")
			code = runUpdate(firstSetupPostStartUpdateArgs(), s.stdout, s.stderr)
		}
		return code
	})(s)
}

func firstSetupPostStartUpdateArgs() []string {
	return []string{"--proxy", "http://127.0.0.1:7890", "--sync-service"}
}
