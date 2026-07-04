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
	if tls := tlsConfig(proxy, false); len(tls) > 0 {
		out["tls"] = tls
	}
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
	out["uuid"] = asString(proxy["uuid"])
	if flow := asString(proxy["flow"]); flow != "" {
		out["flow"] = flow
	}
	if tls := tlsConfig(proxy, false); len(tls) > 0 {
		out["tls"] = tls
	}
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
	if tls := tlsConfig(proxy, true); len(tls) > 0 {
		out["tls"] = tls
	}
	return out, nil
}

func convertTUIC(proxy map[string]any, tag string) (map[string]any, error) {
	if err := requireFields(proxy, "server", "uuid", "password"); err != nil {
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

func tlsConfig(proxy map[string]any, defaultEnabled bool) map[string]any {
	enabled := parseBool(defaultValue(proxy["tls"], defaultEnabled))
	serverName := firstString(proxy, "servername", "server_name", "sni")
	insecure, hasInsecure := proxy["skip-cert-verify"]
	alpnValue := proxy["alpn"]
	fingerprint := firstString(proxy, "client-fingerprint", "fingerprint")
	certificatePath := firstString(proxy, "ca", "certificate_path")
	certificate := firstString(proxy, "ca-str", "certificate")
	clientCertificatePath := firstString(proxy, "client-cert", "client_certificate_path")
	clientCertificate := firstString(proxy, "client-cert-str", "client_certificate")
	clientKeyPath := firstString(proxy, "client-key", "client_key_path")
	clientKey := firstString(proxy, "client-key-str", "client_key")

	if !enabled && serverName == "" && !hasInsecure && alpnValue == nil && fingerprint == "" &&
		certificatePath == "" && certificate == "" && clientCertificatePath == "" &&
		clientCertificate == "" && clientKeyPath == "" && clientKey == "" {
		return nil
	}

	tls := map[string]any{"enabled": enabled || serverName != "" || alpnValue != nil || fingerprint != "" || certificatePath != "" || certificate != "" ||
		clientCertificatePath != "" || clientCertificate != "" || clientKeyPath != "" || clientKey != ""}
	if serverName != "" {
		tls["server_name"] = serverName
	}
	if hasInsecure {
		tls["insecure"] = parseBool(insecure)
	}
	if alpn := stringSlice(alpnValue); len(alpn) > 0 {
		tls["alpn"] = alpn
	} else if text := asString(alpnValue); text != "" {
		tls["alpn"] = splitComma(text)
	}
	if fingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": fingerprint}
	}
	addIf(tls, "certificate_path", certificatePath)
	addIf(tls, "certificate", certificate)
	addIf(tls, "client_certificate_path", clientCertificatePath)
	addIf(tls, "client_certificate", clientCertificate)
	addIf(tls, "client_key_path", clientKeyPath)
	addIf(tls, "client_key", clientKey)
	return tls
}

func addSupportedTransport(proxy map[string]any, outbound map[string]any) error {
	network := strings.ToLower(asString(proxy["network"]))
	if network == "" || network == "tcp" || network == "raw" {
		return nil
	}
	switch network {
	case "ws", "websocket":
		opts, _ := normalizeMap(proxy["ws-opts"])
		transport := map[string]any{"type": "ws"}
		addIf(transport, "path", asString(opts["path"]))
		if headers, ok := normalizeMap(opts["headers"]); ok && len(headers) > 0 {
			clean := map[string]string{}
			for k, v := range headers {
				clean[k] = asString(v)
			}
			transport["headers"] = clean
		}
		outbound["transport"] = transport
	case "grpc":
		opts, _ := normalizeMap(proxy["grpc-opts"])
		transport := map[string]any{"type": "grpc"}
		addIf(transport, "service_name", firstString(opts, "grpc-service-name", "serviceName", "service_name"))
		outbound["transport"] = transport
	case "httpupgrade", "http-upgrade":
		opts, _ := normalizeMap(proxy["httpupgrade-opts"])
		transport := map[string]any{"type": "httpupgrade"}
		addIf(transport, "path", asString(opts["path"]))
		if hosts := stringSlice(opts["host"]); len(hosts) > 0 {
			transport["host"] = hosts
		} else if host := asString(opts["host"]); host != "" {
			transport["host"] = []string{host}
		}
		outbound["transport"] = transport
	default:
		return fmt.Errorf("unsupported transport %s", network)
	}
	return nil
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
	return map[string]any{"type": typ, "tag": tag, "server": server, "server_port": port}, nil
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
