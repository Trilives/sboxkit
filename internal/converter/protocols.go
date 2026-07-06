package converter

import (
	"errors"
	"fmt"
	"strings"
)

func convertProxy(proxy map[string]any, used map[string]bool) (map[string]any, string) {
	name := strings.TrimSpace(asString(proxy["name"]))
	if name == "" {
		return nil, "missing name"
	}
	tag := makeSafeTag(name, used)
	typ := strings.ToLower(strings.TrimSpace(asString(proxy["type"])))

	var (
		out map[string]any
		err error
	)
	switch typ {
	case "anytls":
		out, err = convertAnyTLS(proxy, tag)
	case "trojan":
		out, err = convertTrojan(proxy, tag)
	case "ss", "shadowsocks":
		out, err = convertShadowsocks(proxy, tag)
	case "vmess":
		out, err = convertVMess(proxy, tag)
	case "vless":
		out, err = convertVLess(proxy, tag)
	case "hysteria2", "hy2":
		out, err = convertHysteria2(proxy, tag)
	case "tuic":
		out, err = convertTUIC(proxy, tag)
	case "socks", "socks5":
		out, err = convertSocks(proxy, tag)
	case "http":
		out, err = convertHTTP(proxy, tag)
	default:
		delete(used, tag)
		if typ == "" {
			typ = "unknown"
		}
		return nil, "unsupported type " + typ
	}
	if err != nil {
		delete(used, tag)
		return nil, err.Error()
	}
	return out, ""
}

func convertAnyTLS(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "anytls")
	if err != nil {
		return nil, err
	}
	out["password"] = asString(proxy["password"])
	out["tls"] = tlsConfig(proxy, true)
	return out, nil
}

func convertTrojan(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "trojan")
	if err != nil {
		return nil, err
	}
	out["password"] = asString(proxy["password"])
	if tls := tlsConfig(proxy, true); len(tls) > 0 {
		out["tls"] = tls
	}
	addMultiplex(proxy, out)
	return out, addSupportedTransport(proxy, out)
}

func convertShadowsocks(proxy map[string]any, tag string) (map[string]any, error) {
	method := firstString(proxy, "cipher", "method")
	if method == "" {
		return nil, errors.New("missing cipher")
	}
	if err := requireFields(proxy, "server", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "shadowsocks")
	if err != nil {
		return nil, err
	}
	out["method"] = method
	out["password"] = asString(proxy["password"])
	if _, ok := proxy["udp"]; ok && !parseBool(proxy["udp"]) {
		out["network"] = "tcp"
	}

	plugin := strings.ToLower(asString(proxy["plugin"]))
	if plugin == "" {
		return out, nil
	}
	opts, _ := normalizeMap(proxy["plugin-opts"])
	if plugin == "v2ray-plugin" {
		mode := strings.ToLower(asString(defaultValue(opts["mode"], "websocket")))
		if mode != "websocket" && mode != "quic" {
			return nil, fmt.Errorf("unsupported shadowsocks v2ray-plugin mode %s", mode)
		}
		pluginOpts := []string{"mode=" + mode}
		if parseBool(opts["tls"]) {
			pluginOpts = append(pluginOpts, "tls")
		}
		for _, key := range []string{"host", "path"} {
			if value := asString(opts[key]); value != "" {
				pluginOpts = append(pluginOpts, key+"="+value)
			}
		}
		out["plugin"] = "v2ray-plugin"
		out["plugin_opts"] = strings.Join(pluginOpts, ";")
		return out, nil
	}
	if plugin != "obfs" {
		return nil, fmt.Errorf("unsupported shadowsocks plugin %s", plugin)
	}
	mode := strings.ToLower(asString(defaultValue(opts["mode"], "http")))
	if mode != "http" && mode != "tls" {
		return nil, fmt.Errorf("unsupported shadowsocks obfs mode %s", mode)
	}
	pluginOpts := "obfs=" + mode
	if host := asString(opts["host"]); host != "" {
		pluginOpts += ";obfs-host=" + host
	}
	out["plugin"] = "obfs-local"
	out["plugin_opts"] = pluginOpts
	return out, nil
}

func convertVMess(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "uuid"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "vmess")
	if err != nil {
		return nil, err
	}
	out["uuid"] = asString(proxy["uuid"])
	out["security"] = asString(defaultValue(proxy["cipher"], "auto"))
	out["alter_id"] = asInt(defaultValue(firstValue(proxy, "alterId", "alter-id"), 0))
	if enc := firstString(proxy, "packet-encoding", "packet_encoding"); enc != "" {
		out["packet_encoding"] = enc
	}
	if _, ok := proxy["global-padding"]; ok {
		out["global_padding"] = parseBool(proxy["global-padding"])
	}
	if _, ok := proxy["authenticated-length"]; ok {
		out["authenticated_length"] = parseBool(proxy["authenticated-length"])
	}
	if tls := tlsConfig(proxy, false); len(tls) > 0 {
		out["tls"] = tls
	}
	addMultiplex(proxy, out)
	return out, addSupportedTransport(proxy, out)
}

func convertVLess(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "uuid"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "vless")
	if err != nil {
		return nil, err
	}
	// sing-box VLESS is always encryption "none"; a non-empty/non-none encryption
	// cannot be represented and must not be silently dropped (converter.md §5).
	if enc := strings.ToLower(strings.TrimSpace(asString(proxy["encryption"]))); enc != "" && enc != "none" {
		return nil, fmt.Errorf("unsupported vless encryption %s", enc)
	}
	out["uuid"] = asString(proxy["uuid"])
	if flow := strings.TrimSpace(asString(proxy["flow"])); flow != "" {
		if flow != "xtls-rprx-vision" {
			return nil, fmt.Errorf("unsupported vless flow %s", flow)
		}
		out["flow"] = flow
	}
	if enc := firstString(proxy, "packet-encoding", "packet_encoding"); enc != "" {
		out["packet_encoding"] = enc
	}
	if tls := tlsConfig(proxy, false); len(tls) > 0 {
		out["tls"] = tls
	}
	addMultiplex(proxy, out)
	return out, addSupportedTransport(proxy, out)
}

func convertHysteria2(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "hysteria2")
	if err != nil {
		return nil, err
	}
	out["password"] = asString(proxy["password"])
	if up := bandwidthMbps(proxy["up"]); up > 0 {
		out["up_mbps"] = up
	}
	if down := bandwidthMbps(proxy["down"]); down > 0 {
		out["down_mbps"] = down
	}
	// Port hopping: keep the ranges, don't collapse to the single server_port
	// (converter.md §Hysteria2).
	if ranges := serverPortRanges(firstString(proxy, "ports", "server_ports")); len(ranges) > 0 {
		out["server_ports"] = ranges
	}
	// Salamander obfuscation: silently dropping it produces a config that
	// handshakes but never passes traffic, so carry it faithfully.
	if obfsType := strings.TrimSpace(asString(proxy["obfs"])); obfsType != "" {
		obfs := map[string]any{"type": obfsType}
		addIf(obfs, "password", firstString(proxy, "obfs-password", "obfs_password"))
		out["obfs"] = obfs
	}
	if tls := tlsConfig(proxy, true); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, nil
}

func convertTUIC(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server"); err != nil {
		return nil, err
	}
	// TUIC v4 is token-based; only v5 (uuid + password) has a reliable sing-box
	// mapping, so reject v4 with a clear reason instead of a "missing uuid"
	// error (converter.md §TUIC).
	if asString(proxy["token"]) != "" && asString(proxy["uuid"]) == "" {
		return nil, errors.New("TUIC v4 (token auth) is not supported")
	}
	if err := requireFields(proxy, "uuid", "password"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "tuic")
	if err != nil {
		return nil, err
	}
	out["uuid"] = asString(proxy["uuid"])
	out["password"] = asString(proxy["password"])
	if cc := firstString(proxy, "congestion-controller", "congestion_control"); cc != "" {
		out["congestion_control"] = cc
	}
	if mode := firstString(proxy, "udp-relay-mode", "udp_relay_mode"); mode != "" {
		out["udp_relay_mode"] = mode
	}
	if _, ok := proxy["reduce-rtt"]; ok {
		out["zero_rtt_handshake"] = parseBool(proxy["reduce-rtt"])
	}
	if hb := msToDuration(firstValue(proxy, "heartbeat-interval", "heartbeat_interval")); hb != "" {
		out["heartbeat"] = hb
	}
	if tls := tlsConfig(proxy, true); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, nil
}

func convertSocks(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "socks")
	if err != nil {
		return nil, err
	}
	addAuth(proxy, out)
	return out, nil
}

func convertHTTP(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server"); err != nil {
		return nil, err
	}
	out, err := baseOutbound(proxy, tag, "http")
	if err != nil {
		return nil, err
	}
	addAuth(proxy, out)
	if tls := tlsConfig(proxy, false); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, nil
}

func baseOutbound(proxy map[string]any, tag string, typ string) (map[string]any, error) {
	server := asString(proxy["server"])
	port := normalizePort(firstValue(proxy, "server_port", "port"))
	if server == "" {
		return nil, errors.New("missing server")
	}
	if port == 0 {
		return nil, errors.New("missing or invalid port")
	}
	out := map[string]any{"type": typ, "tag": tag, "server": server, "server_port": port}
	// Universal dial fields (converter.md §2) — carried through rather than
	// silently dropped. `udp` stays protocol-specific because UDP-only
	// protocols (hysteria2/tuic) can't be forced to network:tcp.
	if _, ok := proxy["tfo"]; ok {
		out["tcp_fast_open"] = parseBool(proxy["tfo"])
	}
	if _, ok := proxy["mptcp"]; ok {
		out["tcp_multi_path"] = parseBool(proxy["mptcp"])
	}
	addIf(out, "bind_interface", firstString(proxy, "interface-name", "interface_name"))
	if mark := asInt(firstValue(proxy, "routing-mark", "routing_mark")); mark > 0 {
		out["routing_mark"] = mark
	}
	return out, nil
}

func requireFields(proxy map[string]any, fields ...string) error {
	for _, field := range fields {
		if asString(proxy[field]) == "" {
			return fmt.Errorf("missing %s", field)
		}
	}
	if normalizePort(firstValue(proxy, "server_port", "port")) == 0 {
		return errors.New("missing or invalid port")
	}
	return nil
}

func addAuth(proxy map[string]any, out map[string]any) {
	if username := asString(proxy["username"]); username != "" {
		out["username"] = username
	}
	if password := asString(proxy["password"]); password != "" {
		out["password"] = password
	}
}

func makeSafeTag(name string, used map[string]bool) string {
	tag := strings.TrimSpace(name)
	if tag == "" {
		tag = "node"
	}
	base := tag
	index := 1
	for used[tag] {
		tag = fmt.Sprintf("%s-%d", base, index)
		index++
	}
	used[tag] = true
	return tag
}
