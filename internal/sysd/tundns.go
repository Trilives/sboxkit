// 非特权系统服务的 TUN DNS：sing-box 以 DynamicUser 运行时，systemd-resolved 拒绝它为 TUN
// 链路登记 DNS，系统解析仍走物理网卡（见 tundns 包）。主单元为每个 TUN 入站加一行以 root
// 执行（+ 前缀）的 ExecStartPost，sing-box 建好链路后由 sboxkit 代为登记；链路随 sing-box
// 退出而消失，systemd-resolved 同时丢弃这些设置，无需撤销。
package sysd

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/tundns"
)

// tunDNSLink 一个 TUN 入站的链路名，以及 sing-box 以 root 运行时会为它登记的 DNS 地址。
type tunDNSLink struct {
	Dev     string
	Servers []string
}

// 可在测试中替换。
var tunDNSEnsure = tundns.Ensure

// tunDNSLinks 列出运行时配置中各 TUN 入站需要代为登记的链路。无法确定链路名或 DNS 地址时
// 报错，由 chooseIdentity 改用 root，让 sing-box 自行登记。
func tunDNSLinks(doc map[string]any) ([]tunDNSLink, error) {
	inbounds, _ := doc["inbounds"].([]any)
	var links []tunDNSLink
	for _, raw := range inbounds {
		inbound, _ := raw.(map[string]any)
		if inbound["type"] != "tun" {
			continue
		}
		link, err := tunDNSLinkOf(inbound)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	return links, nil
}

func tunDNSLinkOf(inbound map[string]any) (tunDNSLink, error) {
	dev, _ := inbound["interface_name"].(string)
	if !tundns.ValidLinkName(dev) {
		return tunDNSLink{}, fmt.Errorf(i18n.T("TUN 接口名 %q 无法用于登记 DNS（须显式指定，且只含字母、数字和 _ . -）"), dev)
	}
	servers := tunDNSServers(listValues(inbound["address"]))
	if len(servers) == 0 {
		return tunDNSLink{}, fmt.Errorf(i18n.T("TUN 接口 %s 的地址段内没有可作 DNS 的地址"), dev)
	}
	return tunDNSLink{Dev: dev, Servers: servers}, nil
}

// tunDNSServers 与 sing-tun 的推导一致：首个 IPv4、首个 IPv6 地址各自的下一个地址，须仍在
// 地址段内（如 172.19.0.1/30 → 172.19.0.2）。
func tunDNSServers(prefixes []string) []string {
	var inet4, inet6 *netip.Prefix
	for _, raw := range prefixes {
		prefix, err := netip.ParsePrefix(raw)
		switch {
		case err != nil:
		case prefix.Addr().Is4() && inet4 == nil:
			inet4 = &prefix
		case prefix.Addr().Is6() && inet6 == nil:
			inet6 = &prefix
		}
	}
	var servers []string
	for _, prefix := range []*netip.Prefix{inet4, inet6} {
		if prefix == nil {
			continue
		}
		if next := prefix.Addr().Next(); prefix.Contains(next) {
			servers = append(servers, next.String())
		}
	}
	return servers
}

// listValues sing-box 的列表字段也接受单个字符串。
func listValues(v any) []string {
	switch val := v.(type) {
	case string:
		return []string{val}
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// tunDNSCommand 主单元 ExecStartPost 调用的 sboxkit；没有需要代为登记的链路时为空。
func tunDNSCommand(rt runtimePaths) (string, error) {
	if rt.Identity != identityDynamic || len(rt.TunDNS) == 0 {
		return "", nil
	}
	exe, err := serviceExecutable(rt.Scope)
	if err != nil {
		return "", err
	}
	return systemdExecArg(exe), nil
}

// runtimeTunDNSLinks 读取运行时配置中的 TUN 链路（见 tunDNSLinks）。
func runtimeTunDNSLinks(path string) ([]tunDNSLink, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return tunDNSLinks(doc)
}

// ensureTunDNS watchdog 每轮确认各 TUN 链路的系统 DNS 仍登记在 systemd-resolved 中，丢失时补登记，
// 不论 sing-box 以哪种身份运行（钩子与 sing-box 自己都只在启动时登记一次）。用户模式的 watchdog
// 不是 root，无权登记，跳过。
func ensureTunDNS(service string) {
	s := currentScope()
	if s.User {
		return
	}
	ensureTunDNSFrom(filepath.Join(s.RuntimeDir, service+".json"))
}

func ensureTunDNSFrom(runtimeConfig string) {
	links, err := runtimeTunDNSLinks(runtimeConfig)
	if err != nil {
		return
	}
	for _, link := range links {
		repaired, err := tunDNSEnsure(link.Dev, link.Servers)
		switch {
		case err != nil:
			logHealthcheck("classification=tun_dns link=%s action=reregister_failed error=%q", link.Dev, err.Error())
		case repaired:
			logHealthcheck("classification=tun_dns link=%s action=reregistered", link.Dev)
		}
	}
}
