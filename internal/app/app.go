package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/download"
	"github.com/Trilives/sboxkit/internal/nettest"
	"github.com/Trilives/sboxkit/internal/node"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/proxyenv"
	"github.com/Trilives/sboxkit/internal/subscription"
	"github.com/Trilives/sboxkit/internal/system"
	"github.com/Trilives/sboxkit/internal/version"
	"github.com/Trilives/sboxkit/internal/webui"
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

func runUI(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit ui serve [--root DIR] [--addr ADDR]")
		return 0
	}
	if args[0] != "serve" {
		return fail(stderr, "unknown ui command: %s", args[0])
	}
	root, rest := parseRoot(args[1:])
	addr := valueFlag(rest, "--addr", "127.0.0.1:8790")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Fprintf(stdout, "sboxkit ui listening on http://%s\n", addr)
	if err := webui.NewServer(paths.FromRoot(root)).ListenAndServe(ctx, addr); err != nil {
		return fail(stderr, "serve ui: %v", err)
	}
	return 0
}

func runConfig(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit config [show|set] [--root DIR] [--key KEY --value VALUE]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	p := paths.FromRoot(root)
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil {
		return fail(stderr, "load config: %v", err)
	}
	switch args[0] {
	case "show":
		data, _ := jsonMarshalIndent(cfg)
		fmt.Fprintln(stdout, string(data))
	case "set":
		key := valueFlag(rest, "--key", "")
		value := valueFlag(rest, "--value", "")
		if key == "" {
			return fail(stderr, "--key is required")
		}
		if err := config.SetField(&cfg, key, value); err != nil {
			return fail(stderr, "set config: %v", err)
		}
		if err := config.Save(p.CustomizeFile, cfg); err != nil {
			return fail(stderr, "save config: %v", err)
		}
		fmt.Fprintf(stdout, "updated %s\n", key)
	default:
		return fail(stderr, "unknown config command: %s", args[0])
	}
	return 0
}

func runNode(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit node [list|switch] [--api URL] [--secret TOKEN] [--group Proxy --name NODE]")
		return 0
	}
	_, rest := parseRoot(args[1:])
	client := node.NewClient(valueFlag(rest, "--api", "http://127.0.0.1:9090"), valueFlag(rest, "--secret", ""))
	switch args[0] {
	case "list":
		groups, err := client.Groups(context.Background())
		if err != nil {
			return fail(stderr, "list nodes: %v", err)
		}
		for _, group := range groups {
			fmt.Fprintf(stdout, "%s\tcurrent=%s\tchoices=%s\n", group.Name, group.Now, strings.Join(group.All, ","))
		}
	case "switch":
		group := valueFlag(rest, "--group", "Proxy")
		name := valueFlag(rest, "--name", "")
		if name == "" {
			return fail(stderr, "--name is required")
		}
		if err := client.Switch(context.Background(), group, name); err != nil {
			return fail(stderr, "switch node: %v", err)
		}
		fmt.Fprintf(stdout, "%s switched to %s\n", group, name)
	default:
		return fail(stderr, "unknown node command: %s", args[0])
	}
	return 0
}

func runInit(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: sboxkit init [--root DIR] [--download] [--proxy URL] [--no-tun] [--write-proxy-env|--no-write-proxy-env] [--proxy-env-file PATH]")
		return 0
	}
	root, rest := parseRoot(args)
	return initState(root, rest, stdout, stderr)
}

func runInitWizard(reader *bufio.Reader, stdout io.Writer, stderr io.Writer) int {
	enableTun := askYesNo(reader, stdout, "是否启用 TUN 模式？[是/否，默认是]：", true)
	args := []string{}
	if !enableTun {
		args = append(args, "--no-tun")
		if askYesNo(reader, stdout, "TUN 已关闭，是否将 Shell 代理环境变量写入 ~/.bashrc？[是/否，默认否]：", false) {
			args = append(args, "--write-proxy-env")
		} else {
			args = append(args, "--no-write-proxy-env")
		}
	}
	return initState("", args, stdout, stderr)
}

func initState(root string, rest []string, stdout io.Writer, stderr io.Writer) int {
	p := paths.FromRoot(root)
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil {
		return fail(stderr, "load config: %v", err)
	}
	if hasFlag(rest, "--no-tun") {
		cfg.EnableTun = false
	}
	if err := p.EnsureStateDirs(); err != nil {
		return fail(stderr, "create state directories: %v", err)
	}
	if err := config.Save(p.CustomizeFile, cfg); err != nil {
		return fail(stderr, "save config: %v", err)
	}
	if hasFlag(rest, "--download") {
		if proxy := valueFlag(rest, "--proxy", ""); proxy != "" {
			cfg.DownloadProxy = proxy
		}
		if err := download.DownloadRuntimeAssets(context.Background(), p, cfg, false); err != nil {
			return fail(stderr, "download assets: %v", err)
		}
	}
	if !cfg.EnableTun && hasFlag(rest, "--write-proxy-env") {
		target := valueFlag(rest, "--proxy-env-file", "")
		if err := proxyenv.Write(target); err != nil {
			return fail(stderr, "write proxy environment: %v", err)
		}
		if target == "" {
			target = proxyenv.TargetBashrc()
		}
		fmt.Fprintf(stdout, "proxy environment written to %s\n", target)
	}
	fmt.Fprintf(stdout, "initialized sboxkit state at %s\n", p.StateDir)
	return 0
}

func runProxyEnv(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit proxy-env [write|remove] [--file PATH]")
		return 0
	}
	_, rest := parseRoot(args[1:])
	target := valueFlag(rest, "--file", "")
	if target == "" {
		target = valueFlag(rest, "--proxy-env-file", "")
	}
	switch args[0] {
	case "write":
		if err := proxyenv.Write(target); err != nil {
			return fail(stderr, "write proxy environment: %v", err)
		}
		if target == "" {
			target = proxyenv.TargetBashrc()
		}
		fmt.Fprintf(stdout, "proxy environment written to %s\n", target)
	case "remove":
		if err := proxyenv.Remove(target); err != nil {
			return fail(stderr, "remove proxy environment: %v", err)
		}
		if target == "" {
			target = proxyenv.TargetBashrc()
		}
		fmt.Fprintf(stdout, "proxy environment removed from %s\n", target)
	default:
		return fail(stderr, "unknown proxy-env command: %s", args[0])
	}
	return 0
}

func runUpdate(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(stdout, "Usage: sboxkit update [--root DIR] [--force] [--proxy URL] [--core] [--sync-service]")
		return 0
	}
	root, rest := parseRoot(args)
	p := paths.FromRoot(root)
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil {
		return fail(stderr, "load config: %v", err)
	}
	force := hasFlag(rest, "--force")
	if proxy := valueFlag(rest, "--proxy", ""); proxy != "" {
		cfg.DownloadProxy = proxy
	}
	if hasFlag(rest, "--core") {
		if err := download.DownloadAll(context.Background(), p, cfg, force); err != nil {
			return fail(stderr, "update assets: %v", err)
		}
	} else if err := download.DownloadRuntimeAssets(context.Background(), p, cfg, force); err != nil {
		return fail(stderr, "update assets: %v", err)
	}
	manager := subscription.NewManager(p, cfg)
	if active, _ := manager.Active(); active != nil {
		if _, err := manager.Rebuild(active.Name); err != nil {
			return fail(stderr, "rebuild active subscription: %v", err)
		}
		fmt.Fprintf(stdout, "active subscription rebuilt: %s\n", active.Name)
	}
	if hasFlag(rest, "--sync-service") {
		printServiceTrafficWarning(stdout)
		if err := system.NewService(p, system.OSRunner{UseSudo: true}).SyncAndRestart(context.Background()); err != nil {
			return fail(stderr, "sync service: %v", err)
		}
		fmt.Fprintln(stdout, "service synced and restarted")
	}
	fmt.Fprintln(stdout, "assets updated")
	return 0
}

func runSub(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit sub [add|list|switch|remove|refresh|rebuild] [--url URL|--file PATH] [--proxy URL] [options]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	p := paths.FromRoot(root)
	cfg, err := config.Load(p.CustomizeFile)
	if err != nil {
		return fail(stderr, "load config: %v", err)
	}
	manager := subscription.NewManager(p, cfg)
	switch args[0] {
	case "add":
		name := valueFlag(rest, "--name", "sub")
		rawURL := valueFlag(rest, "--url", "")
		filePath := valueFlag(rest, "--file", "")
		source := subscription.SourceKind(valueFlag(rest, "--source", string(subscription.SourceClash)))
		if filePath != "" && !hasExplicitValue(rest, "--source") {
			source = ""
		}
		if proxy := valueFlag(rest, "--proxy", ""); proxy != "" {
			cfg.DownloadProxy = proxy
			manager = subscription.NewManager(p, cfg)
		}
		if rawURL == "" && filePath == "" {
			return fail(stderr, "--url or --file is required")
		}
		if rawURL != "" && filePath != "" {
			return fail(stderr, "--url and --file cannot be used together")
		}
		if filePath != "" {
			sub, err := manager.AddFile(name, filePath, source, !hasFlag(rest, "--passthrough"), !hasFlag(rest, "--no-active"))
			if err != nil {
				return fail(stderr, "add config file: %v", err)
			}
			fmt.Fprintf(stdout, "config file %s ready: %d nodes\n", sub.Name, sub.LastNodeCount)
			return 0
		}
		sub, err := manager.Add(name, rawURL, source, !hasFlag(rest, "--passthrough"), !hasFlag(rest, "--no-active"))
		if err != nil {
			return fail(stderr, "add subscription: %v", err)
		}
		fmt.Fprintf(stdout, "subscription %s ready: %d nodes\n", sub.Name, sub.LastNodeCount)
	case "list":
		subs, err := manager.List()
		if err != nil {
			return fail(stderr, "list subscriptions: %v", err)
		}
		for _, sub := range subs {
			fmt.Fprintf(stdout, "%s\t%s\t%d\n", sub.Name, sub.SourceType, sub.LastNodeCount)
		}
	case "switch":
		name := valueFlag(rest, "--name", "")
		if name == "" {
			return fail(stderr, "--name is required")
		}
		if err := manager.Switch(name); err != nil {
			return fail(stderr, "switch subscription: %v", err)
		}
		fmt.Fprintf(stdout, "active subscription: %s\n", name)
	case "remove":
		name := valueFlag(rest, "--name", "")
		if name == "" {
			return fail(stderr, "--name is required")
		}
		if err := manager.Remove(name); err != nil {
			return fail(stderr, "remove subscription: %v", err)
		}
		fmt.Fprintf(stdout, "removed subscription: %s\n", name)
	case "refresh":
		name := valueFlag(rest, "--name", "")
		if name == "" {
			return fail(stderr, "--name is required")
		}
		if proxy := valueFlag(rest, "--proxy", ""); proxy != "" {
			cfg.DownloadProxy = proxy
			manager = subscription.NewManager(p, cfg)
		}
		sub, err := manager.Refresh(name)
		if err != nil {
			return fail(stderr, "refresh subscription: %v", err)
		}
		fmt.Fprintf(stdout, "subscription %s refreshed: %d nodes\n", sub.Name, sub.LastNodeCount)
	case "rebuild":
		name := valueFlag(rest, "--name", "")
		if name == "" {
			return fail(stderr, "--name is required")
		}
		sub, err := manager.Rebuild(name)
		if err != nil {
			return fail(stderr, "rebuild subscription: %v", err)
		}
		fmt.Fprintf(stdout, "subscription %s rebuilt: %d nodes\n", sub.Name, sub.LastNodeCount)
	default:
		return fail(stderr, "unknown sub command: %s", args[0])
	}
	return 0
}

func runService(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit service [install|sync|remove|status] [--root DIR] [--no-start]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	svc := system.NewService(paths.FromRoot(root), system.OSRunner{UseSudo: true})
	ctx := context.Background()
	var err error
	if serviceCommandStartsOrRestarts(args[0], rest) {
		printServiceTrafficWarning(stdout)
	}
	switch args[0] {
	case "install":
		err = svc.Install(ctx, !hasFlag(rest, "--no-start"))
	case "sync":
		err = svc.SyncAndRestart(ctx)
	case "remove":
		err = svc.Remove(ctx, true)
	case "status":
		err = svc.Status(ctx)
	default:
		return fail(stderr, "unknown service command: %s", args[0])
	}
	if err != nil {
		return fail(stderr, "service %s: %v", args[0], err)
	}
	return 0
}

func serviceCommandStartsOrRestarts(cmd string, rest []string) bool {
	switch cmd {
	case "install":
		return !hasFlag(rest, "--no-start")
	case "sync":
		return true
	default:
		return false
	}
}

func serviceTrafficWarning() string {
	return "警告：如果 TUN 或路由变更影响当前会话，启动或重启 sboxkit 可能会中断当前 SSH 连接。"
}

func printServiceTrafficWarning(stdout io.Writer) {
	fmt.Fprintln(stdout, serviceTrafficWarning())
}

func runTimer(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit timer [install|remove] [--binary PATH]")
		return 0
	}
	root, rest := parseRoot(args[1:])
	p := paths.FromRoot(root)
	binary := valueFlag(rest, "--binary", "/usr/bin/sboxkit")
	runner := system.OSRunner{UseSudo: true}
	var err error
	if args[0] == "install" {
		err = system.InstallUpdateTimer(context.Background(), runner, p.StateDir, binary, "", "")
	} else if args[0] == "remove" {
		err = system.RemoveUpdateTimer(context.Background(), runner)
	} else {
		return fail(stderr, "unknown timer command: %s", args[0])
	}
	if err != nil {
		return fail(stderr, "timer %s: %v", args[0], err)
	}
	return 0
}

func runResilience(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(stdout, "Usage: sboxkit resilience [install|remove]")
		return 0
	}
	root, _ := parseRoot(args[1:])
	p := paths.FromRoot(root)
	runner := system.OSRunner{UseSudo: true}
	var err error
	if args[0] == "install" {
		err = system.InstallResilience(context.Background(), runner, p.StateDir, "2min", 20, "singbox")
	} else if args[0] == "remove" {
		err = system.RemoveResilience(context.Background(), runner)
	} else {
		return fail(stderr, "unknown resilience command: %s", args[0])
	}
	if err != nil {
		return fail(stderr, "resilience %s: %v", args[0], err)
	}
	return 0
}

func runUninstall(args []string, stdout io.Writer, stderr io.Writer) int {
	root, rest := parseRoot(args)
	p := paths.FromRoot(root)
	runner := system.OSRunner{UseSudo: true}
	var failed bool
	if !hasFlag(rest, "--keep-system") {
		if err := system.RemoveResilience(context.Background(), runner); err != nil {
			fmt.Fprintf(stderr, "remove resilience: %v\n", err)
			failed = true
		}
		if err := system.RemoveUpdateTimer(context.Background(), runner); err != nil {
			fmt.Fprintf(stderr, "remove update timer: %v\n", err)
			failed = true
		}
		if err := system.NewService(p, runner).Remove(context.Background(), true); err != nil {
			fmt.Fprintf(stderr, "remove service/runtime: %v\n", err)
			failed = true
		}
	}
	if hasFlag(rest, "--purge-state") {
		if err := os.RemoveAll(p.StateDir); err != nil {
			return fail(stderr, "remove state: %v", err)
		}
		fmt.Fprintf(stdout, "removed user state: %s\n", p.StateDir)
	}
	if failed {
		printPackageRemovalHint(stdout)
		return 1
	}
	fmt.Fprintln(stdout, "sboxkit-managed service, timers, resilience hooks, and runtime files removed.")
	printPackageRemovalHint(stdout)
	return 0
}

func printPackageRemovalHint(stdout io.Writer) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Debian 包本身由 apt 管理。")
	fmt.Fprintln(stdout, "移除已安装的 sboxkit 二进制和包元数据：")
	fmt.Fprintln(stdout, "  sudo apt remove sboxkit")
	fmt.Fprintln(stdout, "如需连同配置文件一起移除：")
	fmt.Fprintln(stdout, "  sudo apt purge sboxkit")
}

func runNettest(stdout io.Writer, proxy string) {
	results := nettest.Run(context.Background(), nil, proxy)
	fmt.Fprint(stdout, nettest.Format(results))
}

func parseRoot(args []string) (string, []string) {
	root := ""
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--root" && i+1 < len(args) {
			root = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--root=") {
			root = strings.TrimPrefix(args[i], "--root=")
			continue
		}
		rest = append(rest, args[i])
	}
	if root == "" {
		root = paths.DefaultRoot()
	} else if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	return root, rest
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func valueFlag(args []string, flag string, fallback string) string {
	prefix := flag + "="
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return fallback
}

func hasExplicitValue(args []string, flag string) bool {
	prefix := flag + "="
	for i, arg := range args {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
		if arg == flag && i+1 < len(args) {
			return true
		}
	}
	return false
}

func askYesNo(reader *bufio.Reader, stdout io.Writer, prompt string, fallback bool) bool {
	fmt.Fprint(stdout, prompt)
	line, err := reader.ReadString('\n')
	if err != nil {
		return fallback
	}
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "y", "yes", "true", "是", "对", "好", "1", "t", "ok":
		return true
	case "n", "no", "false", "否", "不", "错", "0", "f", "off":
		return false
	default:
		return fallback
	}
}

func fail(stderr io.Writer, format string, args ...any) int {
	fmt.Fprintf(stderr, format+"\n", args...)
	return 1
}

func jsonMarshalIndent(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

func isTerminal(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}
