// Package tundns 代 sing-box 向 systemd-resolved 登记 TUN 链路的 DNS。
//
// sing-box 以 root 运行时，TUN 启动后经 D-Bus 把链路 DNS 设为 TUN 网段内的合成地址、
// 路由域设为 ~. 并设为默认 DNS 路由，系统解析因此进入 TUN；以非特权身份（DynamicUser）
// 运行时 systemd-resolved 拒绝这些调用，sing-box 也不报错，系统解析仍走物理网卡上的
// DNS。系统单元以 root 执行的 ExecStartPost 调用本命令补上这一步（见 sysd/tundns.go）。
package tundns

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	// linkWait 等待 sing-box 建好并启用 TUN 链路、systemd-resolved 认出它的最长时间：sing-box 先初始化
	// 出站与规则集才启动 TUN；须短于 systemd 默认的 TimeoutStartSec（90 秒）。
	linkWait     = time.Minute
	pollInterval = 200 * time.Millisecond
	// commandTimeout 单次 systemctl / resolvectl 调用的上限。
	commandTimeout = 10 * time.Second
	// maxLinkName Linux 链路名上限（IFNAMSIZ - 1）。
	maxLinkName = 15
)

var errLinkDown = errors.New("link is missing, down or not a TUN device")

// sysClassNet 内核导出网络设备属性的目录；测试中替换。
var sysClassNet = "/sys/class/net"

// system 本命令依赖的外部状态，测试中替换。
type system struct {
	resolvedActive func() bool
	linkUp         func(name string) bool // 链路存在、已启用且是 TUN 设备
	resolvectl     func(args ...string) (string, error)
	now            func() time.Time
	sleep          func(time.Duration)
}

var host = system{
	resolvedActive: resolvedIsActive,
	linkUp:         tunLinkUp,
	resolvectl:     runResolvectl,
	now:            time.Now,
	sleep:          time.Sleep,
}

// Run 执行 `sboxkit tun-dns --dev <链路> --dns <地址>...`，返回进程退出码。
func Run(args []string) int {
	link, servers, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tun-dns:", err)
		return 2
	}
	if err := register(host, link, servers); err != nil {
		fmt.Fprintln(os.Stderr, "tun-dns:", err)
		return 1
	}
	return 0
}

func parseArgs(args []string) (string, []string, error) {
	flags := flag.NewFlagSet("tun-dns", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	link := flags.String("dev", "", "TUN link name")
	var servers []string
	flags.Func("dns", "DNS server address on the TUN link (repeatable)", func(value string) error {
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return err
		}
		servers = append(servers, addr.String())
		return nil
	})
	if err := flags.Parse(args); err != nil {
		return "", nil, err
	}
	switch {
	case flags.NArg() > 0:
		return "", nil, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	case !ValidLinkName(*link):
		return "", nil, fmt.Errorf("invalid link name %q", *link)
	case len(servers) == 0:
		return "", nil, errors.New("missing --dns address")
	}
	return *link, servers, nil
}

// ValidLinkName 可安全写进服务单元、传给 resolvectl 的链路名：内核要求不超过 15 字节且不含
// /、: 与空白，这里只允许字母、数字和 _ . -，且不以 - 开头（否则会被当成命令选项）。
func ValidLinkName(name string) bool {
	if name == "" || len(name) > maxLinkName || name == "." || name == ".." || name[0] == '-' {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.-", r)) {
			return false
		}
	}
	return true
}

// register 等 TUN 链路就绪后登记 DNS。systemd-resolved 未运行时系统解析不经过它，无需登记。
// 链路一直没就绪或登记失败返回错误：ExecStartPost 失败使服务启动失败并由 systemd 重启，
// 切换运行身份时 sysd.verifyIdentitySwitch 据此改回 root，由 sing-box 自行登记。
func register(sys system, link string, servers []string) error {
	if !sys.resolvedActive() {
		return nil
	}
	deadline := sys.now().Add(linkWait)
	for {
		err := apply(sys, link, servers)
		if err == nil {
			return nil
		}
		if !sys.now().Before(deadline) {
			return fmt.Errorf("register DNS of %s with systemd-resolved: %w", link, err)
		}
		sys.sleep(pollInterval)
	}
}

// apply 与 sing-box 以 root 运行时的登记一致：链路 DNS、路由域 ~.、默认 DNS 路由。链路刚
// 出现时 systemd-resolved 可能还没认出它，失败由 register 重试。
func apply(sys system, link string, servers []string) error {
	if !sys.linkUp(link) {
		return errLinkDown
	}
	for _, args := range [][]string{
		append([]string{"dns", link}, servers...),
		{"domain", link, "~."},
		{"default-route", link, "yes"},
	} {
		if _, err := sys.resolvectl(args...); err != nil {
			return err
		}
	}
	return nil
}

// ResolvedActive systemd-resolved 是否在运行（含启动中）。
func ResolvedActive() bool { return host.resolvedActive() }

// Ensure 供 watchdog 使用：TUN 链路已启用且 systemd-resolved 在运行时，确认链路仍按 apply 的方式
// 登记着 DNS，没有则补登记一次（不等待链路出现）；返回是否补登记过。
func Ensure(link string, servers []string) (bool, error) { return ensure(host, link, servers) }

func ensure(sys system, link string, servers []string) (bool, error) {
	if !sys.resolvedActive() || !sys.linkUp(link) {
		return false, nil
	}
	ok, err := registered(sys, link, servers)
	if err != nil || ok {
		return false, err
	}
	return true, apply(sys, link, servers)
}

// registered 链路当前的 DNS、路由域与默认路由设置是否包含 apply 的登记。
func registered(sys system, link string, servers []string) (bool, error) {
	settings := map[string][]string{}
	for _, name := range []string{"dns", "domain", "default-route"} {
		out, err := sys.resolvectl(name, link)
		if err != nil {
			return false, err
		}
		settings[name] = linkValues(out)
	}
	for _, server := range servers {
		if !slices.Contains(settings["dns"], server) {
			return false, nil
		}
	}
	return slices.Contains(settings["domain"], "~.") && slices.Equal(settings["default-route"], []string{"yes"}), nil
}

// linkValues 解析 `resolvectl dns|domain|default-route <链路>` 的输出「Link 7 (singbox): 值…」。
func linkValues(out string) []string {
	_, values, _ := strings.Cut(strings.TrimSpace(out), "):")
	return strings.Fields(values)
}

func resolvedIsActive() bool {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "systemctl", "is-active", "systemd-resolved.service").Output()
	return runningState(strings.TrimSpace(string(out)))
}

// runningState `systemctl is-active` 的输出是否表示运行中；正在启动的 systemd-resolved 也算，
// 随后的登记失败会重试到它就绪。
func runningState(state string) bool {
	switch state {
	case "active", "activating", "reloading":
		return true
	}
	return false
}

// tunLinkUp 链路已启用且是 TUN 设备。接口名来自运行时配置，可能出自原样信任的原生订阅，
// 只登记 TUN，配置不能借此改写物理网卡等其他链路的 DNS。
func tunLinkUp(name string) bool {
	return linkIsUp(name) && isTun(name)
}

func linkIsUp(name string) bool {
	iface, err := net.InterfaceByName(name)
	return err == nil && iface.Flags&net.FlagUp != 0
}

// isTun 内核只为 tun/tap 设备导出 tun_flags。
func isTun(name string) bool {
	_, err := os.Stat(filepath.Join(sysClassNet, name, "tun_flags"))
	return err == nil
}

func runResolvectl(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "resolvectl", args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolvectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
