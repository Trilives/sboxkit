package app

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const (
	keyEnter = "enter"
	keyEsc   = "esc"
	keyUp    = "up"
	keyDown  = "down"
	keyHome  = "home"
	keyEnd   = "end"
	keyPgUp  = "page-up"
	keyPgDn  = "page-down"
	keyOther = "other"
)

type tuiAction func(*tuiSession) bool

type tuiItem struct {
	Label  string
	Detail string
	Action tuiAction
}

type tuiSession struct {
	tty    *os.File
	reader *bufio.Reader
	stderr io.Writer
	status string
}

func runTTYInteractive(stderr io.Writer) (int, bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return 0, false
	}
	defer tty.Close()

	session := &tuiSession{
		tty:    tty,
		reader: bufio.NewReader(tty),
		stderr: tty,
		status: "Use arrow keys or j/k to move. Enter selects. Esc goes back.",
	}
	return session.run(), true
}

func (s *tuiSession) run() int {
	for {
		idx, ok := s.selectMenu("sboxkit", "Terminal UI for sing-box deployment", mainTUIItems())
		if !ok {
			s.clear()
			return 0
		}
		if mainTUIItems()[idx].Action(s) {
			s.clear()
			return 0
		}
	}
}

func mainTUIItems() []tuiItem {
	return []tuiItem{
		{"First setup wizard", "Initialize state, import a subscription, and optionally install the service", runTUIFirstSetup},
		{"Subscriptions", "Add, list, switch, refresh, rebuild, or remove subscriptions and local configs", submenu("Subscriptions", subscriptionTUIItems)},
		{"Service", "Install, sync, inspect, or remove sboxkit.service", submenu("Service", serviceTUIItems)},
		{"Runtime assets", "Download optional rules or update the packaged core cache", submenu("Runtime assets", updateTUIItems)},
		{"Configuration", "Show or edit customize.json, TUN, WebUI, and shell proxy settings", submenu("Configuration", configTUIItems)},
		{"Nodes", "List or switch selector nodes through the sing-box Clash API", submenu("Nodes", nodeTUIItems)},
		{"Network test", "Probe latency and exit IP through the local proxy", commandAction("Network test", func(s *tuiSession) int {
			runNettest(s.tty, "127.0.0.1:7890")
			return 0
		})},
		{"Timers and resilience", "Weekly update timer and network resilience watchdog", submenu("Timers and resilience", timerTUIItems)},
		{"Uninstall", "Remove system integration, with an optional state purge", submenu("Uninstall", uninstallTUIItems)},
		{"Help", "Print command-line help", commandAction("Help", func(s *tuiSession) int {
			printHelp(s.tty)
			return 0
		})},
		{"Quit", "Exit sboxkit", func(*tuiSession) bool { return true }},
	}
}

func subscriptionTUIItems() []tuiItem {
	return []tuiItem{
		{"List subscriptions", "Show saved subscriptions and node counts", commandAction("List subscriptions", func(s *tuiSession) int {
			return runSub([]string{"list"}, s.tty, s.stderr)
		})},
		{"Add remote URL", "Import Clash, sing-box, or base64 subscription URL", runTUIAddRemoteSubscription},
		{"Add local config file", "Copy a config.yaml/json into the fixed state directory", runTUIAddLocalConfig},
		{"Switch active subscription", "Select which saved subscription feeds the running config", promptCommand("Switch active subscription", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("Subscription name")
			if !ok {
				return nil, false
			}
			return []string{"switch", "--name", name}, true
		}, runSub)},
		{"Refresh subscription", "Fetch the latest remote content and rebuild config", promptCommand("Refresh subscription", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("Subscription name")
			if !ok {
				return nil, false
			}
			args := []string{"refresh", "--name", name}
			if proxy := s.promptDefault("Download proxy", ""); proxy != "" {
				args = append(args, "--proxy", proxy)
			}
			return args, true
		}, runSub)},
		{"Rebuild active config", "Rebuild from stored local/raw source without fetching", promptCommand("Rebuild subscription", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("Subscription name")
			if !ok {
				return nil, false
			}
			return []string{"rebuild", "--name", name}, true
		}, runSub)},
		{"Remove subscription", "Delete a saved subscription", promptCommand("Remove subscription", func(s *tuiSession) ([]string, bool) {
			name, ok := s.promptRequired("Subscription name")
			if !ok || !s.confirm("Remove subscription "+name+"?", false) {
				return nil, false
			}
			return []string{"remove", "--name", name}, true
		}, runSub)},
	}
}

func serviceTUIItems() []tuiItem {
	return []tuiItem{
		{"Install and start service", "Sync runtime files, install systemd unit, and restart service", commandAction("Install service", func(s *tuiSession) int {
			return runService([]string{"install"}, s.tty, s.stderr)
		})},
		{"Install without starting", "Install unit and runtime files but leave service stopped", commandAction("Install service without start", func(s *tuiSession) int {
			return runService([]string{"install", "--no-start"}, s.tty, s.stderr)
		})},
		{"Sync and restart", "Copy active config/assets into /etc/sboxkit and restart", commandAction("Sync service", func(s *tuiSession) int {
			return runService([]string{"sync"}, s.tty, s.stderr)
		})},
		{"Status", "Open systemctl status for sboxkit.service", commandAction("Service status", func(s *tuiSession) int {
			return runService([]string{"status"}, s.tty, s.stderr)
		})},
		{"Remove service", "Stop service and remove systemd runtime files", commandAction("Remove service", func(s *tuiSession) int {
			if !s.confirm("Remove sboxkit.service and /etc/sboxkit?", false) {
				return 0
			}
			return runService([]string{"remove"}, s.tty, s.stderr)
		})},
	}
}

func updateTUIItems() []tuiItem {
	return []tuiItem{
		{"Download optional rules through proxy", "Recommended after the service is running", commandAction("Update runtime assets", func(s *tuiSession) int {
			args := []string{"--proxy", s.promptDefault("Proxy URL", "http://127.0.0.1:7890")}
			if s.confirm("Sync assets to service and restart?", true) {
				args = append(args, "--sync-service")
			}
			return runUpdate(args, s.tty, s.stderr)
		})},
		{"Download optional rules direct", "Fetch large rule-set assets without a proxy", commandAction("Update runtime assets", func(s *tuiSession) int {
			return runUpdate(nil, s.tty, s.stderr)
		})},
		{"Update core cache and rules", "Download sing-box core into user state plus optional assets", commandAction("Update core and rules", func(s *tuiSession) int {
			args := []string{"--core"}
			if s.confirm("Force re-download?", false) {
				args = append(args, "--force")
			}
			return runUpdate(args, s.tty, s.stderr)
		})},
	}
}

func configTUIItems() []tuiItem {
	return []tuiItem{
		{"Show config", "Print current customize.json", commandAction("Show config", func(s *tuiSession) int {
			return runConfig([]string{"show"}, s.tty, s.stderr)
		})},
		{"Set config key", "Set any supported config field by key/value", promptCommand("Set config key", func(s *tuiSession) ([]string, bool) {
			key, ok := s.promptRequired("Key")
			if !ok {
				return nil, false
			}
			value, ok := s.promptRequired("Value")
			if !ok {
				return nil, false
			}
			return []string{"set", "--key", key, "--value", value}, true
		}, runConfig)},
		{"Enable TUN", "Set enable_tun=true", configSetAction("enable_tun", "true")},
		{"Disable TUN", "Set enable_tun=false and optionally write shell proxy env", commandAction("Disable TUN", func(s *tuiSession) int {
			code := runConfig([]string{"set", "--key", "enable_tun", "--value", "false"}, s.tty, s.stderr)
			if code == 0 && s.confirm("Write shell proxy variables to ~/.bashrc?", false) {
				code = runProxyEnv([]string{"write"}, s.tty, s.stderr)
			}
			return code
		})},
		{"Enable WebUI", "Set lan_panel=true; rebuild and sync after changing it", configSetAction("lan_panel", "true")},
		{"Disable WebUI", "Set lan_panel=false", configSetAction("lan_panel", "false")},
		{"Write shell proxy env", "Append managed proxy block to ~/.bashrc", commandAction("Write shell proxy env", func(s *tuiSession) int {
			return runProxyEnv([]string{"write"}, s.tty, s.stderr)
		})},
		{"Remove shell proxy env", "Remove managed proxy block from ~/.bashrc", commandAction("Remove shell proxy env", func(s *tuiSession) int {
			return runProxyEnv([]string{"remove"}, s.tty, s.stderr)
		})},
	}
}

func nodeTUIItems() []tuiItem {
	return []tuiItem{
		{"List nodes", "Read selector groups from the running Clash API", commandAction("List nodes", func(s *tuiSession) int {
			return runNode([]string{"list"}, s.tty, s.stderr)
		})},
		{"Switch node", "Switch a selector group without restarting sing-box", promptCommand("Switch node", func(s *tuiSession) ([]string, bool) {
			group := s.promptDefault("Group", "Proxy")
			name, ok := s.promptRequired("Node name")
			if !ok {
				return nil, false
			}
			return []string{"switch", "--group", group, "--name", name}, true
		}, runNode)},
	}
}

func timerTUIItems() []tuiItem {
	return []tuiItem{
		{"Install weekly update timer", "Install systemd timer for periodic updates", commandAction("Install timer", func(s *tuiSession) int {
			return runTimer([]string{"install", "--binary", "/usr/bin/sboxkit"}, s.tty, s.stderr)
		})},
		{"Remove weekly update timer", "Remove the update timer", commandAction("Remove timer", func(s *tuiSession) int {
			return runTimer([]string{"remove"}, s.tty, s.stderr)
		})},
		{"Install resilience watchdog", "Install network self-healing service/timer hooks", commandAction("Install resilience", func(s *tuiSession) int {
			return runResilience([]string{"install"}, s.tty, s.stderr)
		})},
		{"Remove resilience watchdog", "Remove network self-healing integration", commandAction("Remove resilience", func(s *tuiSession) int {
			return runResilience([]string{"remove"}, s.tty, s.stderr)
		})},
	}
}

func uninstallTUIItems() []tuiItem {
	return []tuiItem{
		{"Uninstall system integration", "Remove service, timer, resilience, and runtime files", commandAction("Uninstall", func(s *tuiSession) int {
			if !s.confirm("Uninstall system integration?", false) {
				return 0
			}
			return runUninstall(nil, s.tty, s.stderr)
		})},
		{"Uninstall and purge user state", "Also remove subscriptions, generated configs, downloads, and UI state", commandAction("Uninstall and purge state", func(s *tuiSession) int {
			if !s.confirm("Purge all sboxkit user state?", false) {
				return 0
			}
			return runUninstall([]string{"--purge-state"}, s.tty, s.stderr)
		})},
		{"Show apt package removal commands", "Explain how to remove the installed .deb package", commandAction("APT package removal", func(s *tuiSession) int {
			printPackageRemovalHint(s.tty)
			return 0
		})},
	}
}

func submenu(title string, items func() []tuiItem) tuiAction {
	return func(s *tuiSession) bool {
		for {
			idx, ok := s.selectMenu(title, "Esc returns to the previous menu", items())
			if !ok {
				return false
			}
			if items()[idx].Action(s) {
				return true
			}
		}
	}
}

func commandAction(title string, run func(*tuiSession) int) tuiAction {
	return func(s *tuiSession) bool {
		s.clear()
		fmt.Fprintf(s.tty, "== %s ==\n\n", title)
		code := run(s)
		if code != 0 {
			fmt.Fprintf(s.tty, "\nCommand exited with status %d.\n", code)
		}
		s.wait()
		return false
	}
}

func promptCommand(title string, build func(*tuiSession) ([]string, bool), run func([]string, io.Writer, io.Writer) int) tuiAction {
	return commandAction(title, func(s *tuiSession) int {
		args, ok := build(s)
		if !ok {
			fmt.Fprintln(s.tty, "Cancelled.")
			return 0
		}
		fmt.Fprintln(s.tty)
		return run(args, s.tty, s.stderr)
	})
}

func configSetAction(key string, value string) tuiAction {
	return commandAction("Set "+key, func(s *tuiSession) int {
		return runConfig([]string{"set", "--key", key, "--value", value}, s.tty, s.stderr)
	})
}

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
		code := initState("", args, s.tty, s.stderr)
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
		if s.confirm("Install and start sboxkit.service now?", true) {
			code = runService([]string{"install"}, s.tty, s.stderr)
		}
		return code
	})(s)
}

func runTUIAddRemoteSubscription(s *tuiSession) bool {
	return commandAction("Add remote subscription", runTUIAddRemoteSubscriptionCommand)(s)
}

func runTUIAddRemoteSubscriptionCommand(s *tuiSession) int {
	name := s.promptDefault("Name", "main")
	source := s.promptDefault("Source: clash, sing-box, or base64", "clash")
	url, ok := s.promptRequired("Subscription URL")
	if !ok {
		fmt.Fprintln(s.tty, "Cancelled.")
		return 0
	}
	args := []string{"add", "--name", name, "--source", source, "--url", url}
	if proxy := s.promptDefault("Download proxy", ""); proxy != "" {
		args = append(args, "--proxy", proxy)
	}
	if !s.confirm("Set as active subscription?", true) {
		args = append(args, "--no-active")
	}
	fmt.Fprintln(s.tty)
	return runSub(args, s.tty, s.stderr)
}

func runTUIAddLocalConfig(s *tuiSession) bool {
	return commandAction("Add local config file", runTUIAddLocalConfigCommand)(s)
}

func runTUIAddLocalConfigCommand(s *tuiSession) int {
	name := s.promptDefault("Name", "local")
	filePath, ok := s.promptRequired("Config file path")
	if !ok {
		fmt.Fprintln(s.tty, "Cancelled.")
		return 0
	}
	args := []string{"add", "--name", name, "--file", filePath}
	if source := s.promptDefault("Source override: clash, sing-box, base64, or blank for auto", ""); source != "" {
		args = append(args, "--source", source)
	}
	if s.confirm("Use sing-box config as passthrough?", false) {
		args = append(args, "--passthrough")
	}
	if !s.confirm("Set as active subscription?", true) {
		args = append(args, "--no-active")
	}
	fmt.Fprintln(s.tty)
	return runSub(args, s.tty, s.stderr)
}

func (s *tuiSession) selectMenu(title string, subtitle string, items []tuiItem) (int, bool) {
	if len(items) == 0 {
		return 0, false
	}
	restore, err := enterRawMode(s.tty)
	if err != nil {
		fmt.Fprintf(s.stderr, "enter raw terminal mode: %v\n", err)
		return 0, false
	}
	defer restore()

	selected := 0
	offset := 0
	for {
		rows, cols := terminalSize(s.tty)
		visible := rows - 10
		if visible < 5 {
			visible = 5
		}
		if visible > len(items) {
			visible = len(items)
		}
		offset = clampOffset(selected, offset, visible, len(items))
		s.renderMenu(title, subtitle, items, selected, offset, visible, cols)

		switch readKey(s.tty) {
		case keyEnter:
			s.showCursor()
			return selected, true
		case keyEsc:
			s.showCursor()
			return 0, false
		case keyUp:
			if selected > 0 {
				selected--
			}
		case keyDown:
			if selected < len(items)-1 {
				selected++
			}
		case keyHome:
			selected = 0
		case keyEnd:
			selected = len(items) - 1
		case keyPgUp:
			selected -= visible
			if selected < 0 {
				selected = 0
			}
		case keyPgDn:
			selected += visible
			if selected > len(items)-1 {
				selected = len(items) - 1
			}
		}
	}
}

func (s *tuiSession) renderMenu(title string, subtitle string, items []tuiItem, selected int, offset int, visible int, cols int) {
	if cols < 50 {
		cols = 50
	}
	width := cols - 4
	if width > 96 {
		width = 96
	}
	if width < 46 {
		width = 46
	}

	var b strings.Builder
	b.WriteString("\x1b[?25l\x1b[H\x1b[2J")
	b.WriteString("+" + strings.Repeat("-", width) + "+\n")
	b.WriteString("| " + padRight(title, width-2) + " |\n")
	if subtitle != "" {
		b.WriteString("| " + padRight(subtitle, width-2) + " |\n")
	}
	b.WriteString("+" + strings.Repeat("-", width) + "+\n")
	if offset > 0 {
		b.WriteString("| " + padRight("... "+strconv.Itoa(offset)+" more above", width-2) + " |\n")
	}
	for i := 0; i < visible; i++ {
		idx := offset + i
		if idx >= len(items) {
			break
		}
		item := items[idx]
		prefix := "  "
		if idx == selected {
			prefix = "> "
		}
		line := truncate(prefix+item.Label, width-2)
		if idx == selected {
			b.WriteString("| \x1b[7m" + padRight(line, width-2) + "\x1b[0m |\n")
		} else {
			b.WriteString("| " + padRight(line, width-2) + " |\n")
		}
		if item.Detail != "" && idx == selected {
			detail := truncate("  "+item.Detail, width-2)
			b.WriteString("| \x1b[2m" + padRight(detail, width-2) + "\x1b[0m |\n")
		}
	}
	if offset+visible < len(items) {
		b.WriteString("| " + padRight("... "+strconv.Itoa(len(items)-offset-visible)+" more below", width-2) + " |\n")
	}
	b.WriteString("+" + strings.Repeat("-", width) + "+\n")
	b.WriteString("Up/Down or j/k scroll  PgUp/PgDn jump  Enter select  Esc back  q quit\n")
	if s.status != "" {
		b.WriteString(s.status + "\n")
	}
	fmt.Fprint(s.tty, terminalLines(b.String()))
}

func (s *tuiSession) promptRequired(label string) (string, bool) {
	for {
		value := s.promptDefault(label, "")
		if value != "" {
			return value, true
		}
		if !s.confirm("Leave this prompt?", false) {
			continue
		}
		return "", false
	}
}

func (s *tuiSession) promptDefault(label string, fallback string) string {
	if fallback != "" {
		fmt.Fprintf(s.tty, "%s [%s]: ", label, fallback)
	} else {
		fmt.Fprintf(s.tty, "%s: ", label)
	}
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return fallback
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return fallback
	}
	return value
}

func (s *tuiSession) confirm(label string, fallback bool) bool {
	suffix := " [y/N]: "
	if fallback {
		suffix = " [Y/n]: "
	}
	fmt.Fprint(s.tty, label+suffix)
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return fallback
	}
}

func (s *tuiSession) wait() {
	fmt.Fprint(s.tty, "\nPress Enter to return to the menu...")
	_, _ = s.reader.ReadString('\n')
}

func (s *tuiSession) clear() {
	fmt.Fprint(s.tty, "\x1b[?25h\x1b[H\x1b[2J")
}

func (s *tuiSession) showCursor() {
	fmt.Fprint(s.tty, "\x1b[?25h")
}

func enterRawMode(tty *os.File) (func(), error) {
	current, err := sttyOutput(tty, "-g")
	if err != nil {
		return nil, err
	}
	mode := strings.TrimSpace(string(current))
	if err := sttyRun(tty, rawModeArgs()...); err != nil {
		return nil, err
	}
	restored := false
	return func() {
		if restored {
			return
		}
		restored = true
		_ = sttyRun(tty, mode)
		fmt.Fprint(tty, "\x1b[?25h")
	}, nil
}

func rawModeArgs() []string {
	return []string{"raw", "-echo", "min", "1", "time", "0"}
}

func escapeReadArgs() []string {
	return []string{"min", "0", "time", "1"}
}

func terminalLines(text string) string {
	return strings.ReplaceAll(text, "\n", "\r\n")
}

func sttyOutput(tty *os.File, args ...string) ([]byte, error) {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = tty
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("stty %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func sttyRun(tty *os.File, args ...string) error {
	cmd := exec.Command("stty", args...)
	cmd.Stdin = tty
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("stty %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func terminalSize(tty *os.File) (int, int) {
	out, err := sttyOutput(tty, "size")
	if err != nil {
		return 24, 80
	}
	fields := strings.Fields(string(out))
	if len(fields) != 2 {
		return 24, 80
	}
	rows, rowErr := strconv.Atoi(fields[0])
	cols, colErr := strconv.Atoi(fields[1])
	if rowErr != nil || colErr != nil || rows <= 0 || cols <= 0 {
		return 24, 80
	}
	return rows, cols
}

func readKey(tty *os.File) string {
	buf := make([]byte, 8)
	for {
		n, err := tty.Read(buf[:1])
		if err != nil {
			return keyEsc
		}
		if n == 0 {
			continue
		}
		switch buf[0] {
		case '\r', '\n':
			return keyEnter
		case 'q', 'Q':
			return keyEsc
		case 'j', 'J':
			return keyDown
		case 'k', 'K':
			return keyUp
		case 'g':
			return keyHome
		case 'G':
			return keyEnd
		case 0x1b:
			return readEscapeKey(tty, buf)
		}
		return keyOther
	}
}

func readEscapeKey(tty *os.File, buf []byte) string {
	_ = sttyRun(tty, escapeReadArgs()...)
	defer func() {
		_ = sttyRun(tty, rawModeArgs()...)
	}()

	n, err := tty.Read(buf[:1])
	if err != nil || n == 0 {
		return keyEsc
	}
	if buf[0] != '[' && buf[0] != 'O' {
		return keyEsc
	}
	n, err = tty.Read(buf[:1])
	if err != nil || n == 0 {
		return keyEsc
	}
	switch buf[0] {
	case 'A':
		return keyUp
	case 'B':
		return keyDown
	case 'H':
		return keyHome
	case 'F':
		return keyEnd
	case '5':
		_, _ = tty.Read(buf[:1])
		return keyPgUp
	case '6':
		_, _ = tty.Read(buf[:1])
		return keyPgDn
	default:
		return keyOther
	}
}

func clampOffset(selected int, offset int, visible int, total int) int {
	if selected < offset {
		offset = selected
	}
	if selected >= offset+visible {
		offset = selected - visible + 1
	}
	maxOffset := total - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	return offset
}

func padRight(value string, width int) string {
	value = truncate(value, width)
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func truncate(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}
