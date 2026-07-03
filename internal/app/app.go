package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Trilives/sboxkit/internal/version"
)

var plannedCommands = map[string]string{
	"init":       "initialize sboxkit",
	"modify":     "show modification commands for subscriptions, service, timers, and resilience",
	"sub":        "manage subscriptions",
	"config":     "show or update customize config",
	"node":       "list or switch runtime proxy nodes through Clash API",
	"proxy-env":  "write or remove shell proxy environment variables",
	"service":    "install, sync, remove, or inspect sboxkit.service",
	"timer":      "install or remove weekly update timer",
	"resilience": "install or remove network resilience watchdog",
	"ui":         "serve the built-in management WebUI",
	"nettest":    "test latency and exit IP through the local proxy",
	"uninstall":  "remove service, timers, hooks, assets, and state",
	"update":     "update optional rule-set assets",
}

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	return runWithFileLog(args, stdout, stderr, func(logStderr io.Writer) int {
		return run(args, stdout, logStderr)
	})
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		if isTerminal(os.Stdin) {
			return runInteractive(stdout, stderr)
		}
		printHelp(stdout)
		return 0
	}

	cmd := args[0]
	switch cmd {
	case "-h", "--help", "help":
		printHelp(stdout)
		return 0
	case "version":
		fmt.Fprintf(stdout, "sboxkit %s (%s, %s)\n", version.Version, version.Commit, version.Date)
		return 0
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "update":
		return runUpdate(args[1:], stdout, stderr)
	case "modify":
		fmt.Fprintln(stdout, "Use: sboxkit sub|service|timer|resilience [command]")
		return 0
	case "sub":
		return runSub(args[1:], stdout, stderr)
	case "config":
		return runConfig(args[1:], stdout, stderr)
	case "node":
		return runNode(args[1:], stdout, stderr)
	case "proxy-env":
		return runProxyEnv(args[1:], stdout, stderr)
	case "service":
		return runService(args[1:], stdout, stderr)
	case "timer":
		return runTimer(args[1:], stdout, stderr)
	case "resilience":
		return runResilience(args[1:], stdout, stderr)
	case "ui":
		return runUI(args[1:], stdout, stderr)
	case "uninstall":
		return runUninstall(args[1:], stdout, stderr)
	case "nettest":
		runNettest(stdout, valueFlag(args[1:], "--proxy", "127.0.0.1:7890"))
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", cmd)
		return 2
	}
}

func runInteractive(stdout io.Writer, stderr io.Writer) int {
	if isTerminal(os.Stdin) {
		if code, ok := runTTYInteractive(stderr); ok {
			return code
		}
	}
	return runNumberedInteractive(stdout, stderr)
}

func runNumberedInteractive(stdout io.Writer, stderr io.Writer) int {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintln(stdout, "\nsboxkit")
		fmt.Fprintln(stdout, "  1) 初始化")
		fmt.Fprintln(stdout, "  2) 更新资源")
		fmt.Fprintln(stdout, "  3) 查看订阅")
		fmt.Fprintln(stdout, "  4) 显示配置")
		fmt.Fprintln(stdout, "  5) 网络测试")
		fmt.Fprintln(stdout, "  6) 服务状态")
		fmt.Fprintln(stdout, "  0) 退出")
		fmt.Fprint(stdout, "选择：")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Fprintln(stdout)
			return 0
		}
		switch strings.TrimSpace(line) {
		case "1":
			_ = runInitWizard(reader, stdout, stderr)
		case "2":
			_ = runUpdate(nil, stdout, stderr)
		case "3":
			_ = runSub([]string{"list"}, stdout, stderr)
		case "4":
			_ = runConfig([]string{"show"}, stdout, stderr)
		case "5":
			runNettest(stdout, "127.0.0.1:7890")
		case "6":
			_ = runService([]string{"status"}, stdout, stderr)
		case "0", "q", "quit", "exit":
			return 0
		default:
			fmt.Fprintln(stderr, "未知选项")
		}
	}
}

func printHelp(stdout io.Writer) {
	fmt.Fprintln(stdout, "sboxkit - Linux CLI/TUI deployment manager for sing-box")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Usage:")
	fmt.Fprintln(stdout, "  sboxkit [command]")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Commands:")
	for _, cmd := range []string{"init", "modify", "sub", "config", "node", "proxy-env", "service", "timer", "resilience", "ui", "nettest", "uninstall", "update"} {
		fmt.Fprintf(stdout, "  %-10s %s\n", cmd, plannedCommands[cmd])
	}
	fmt.Fprintln(stdout, "  version    print build version")
	fmt.Fprintln(stdout, "  help       print this help")
}
