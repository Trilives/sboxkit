package converter

import (
	"fmt"
	"strings"
)

// tlsConfig builds the shared sing-box TLS block used by every TLS-capable
// protocol (converter.md §3 asks for a single shared TLS module). It also owns
// Reality and uTLS fingerprint mapping.
func tlsConfig(proxy map[string]any, defaultEnabled bool) map[string]any {
	serverName := firstString(proxy, "servername", "server_name", "sni")
	insecure, hasInsecure := proxy["skip-cert-verify"]
	alpnValue := proxy["alpn"]
	// Clash `client-fingerprint` is the uTLS fingerprint (chrome/firefox/…).
	// Clash `fingerprint` is a *certificate* pin (SHA-256), a different concept
	// from sing-box's utls fingerprint, so it is intentionally NOT mapped here
	// (converter.md §3). Mapping it onto utls would yield a broken handshake.
	clientFingerprint := firstString(proxy, "client-fingerprint")
	reality, hasReality := realityBlock(proxy)
	certificatePath := firstString(proxy, "ca", "certificate_path")
	certificate := firstString(proxy, "ca-str", "certificate")
	clientCertificatePath := firstString(proxy, "client-cert", "client_certificate_path")
	clientCertificate := firstString(proxy, "client-cert-str", "client_certificate")
	clientKeyPath := firstString(proxy, "client-key", "client_key_path")
	clientKey := firstString(proxy, "client-key-str", "client_key")

	explicit := parseBool(defaultValue(proxy["tls"], defaultEnabled))
	hasMaterial := serverName != "" || alpnValue != nil || clientFingerprint != "" || hasReality ||
		certificatePath != "" || certificate != "" || clientCertificatePath != "" ||
		clientCertificate != "" || clientKeyPath != "" || clientKey != ""

	if !explicit && !hasMaterial && !hasInsecure {
		return nil
	}

	tls := map[string]any{"enabled": explicit || hasMaterial}
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
	if clientFingerprint != "" {
		tls["utls"] = map[string]any{"enabled": true, "fingerprint": clientFingerprint}
	}
	if hasReality {
		tls["reality"] = reality
	}
	addIf(tls, "certificate_path", certificatePath)
	addIf(tls, "certificate", certificate)
	addIf(tls, "client_certificate_path", clientCertificatePath)
	addIf(tls, "client_certificate", clientCertificate)
	addIf(tls, "client_key_path", clientKeyPath)
	addIf(tls, "client_key", clientKey)
	return tls
}

// realityBlock maps Clash `reality-opts` onto sing-box `tls.reality`. Without a
// public key there is no usable Reality config, so it reports absent.
func realityBlock(proxy map[string]any) (map[string]any, bool) {
	opts, ok := normalizeMap(proxy["reality-opts"])
	if !ok {
		return nil, false
	}
	publicKey := firstString(opts, "public-key", "public_key")
	if publicKey == "" {
		return nil, false
	}
	reality := map[string]any{"enabled": true, "public_key": publicKey}
	if shortID := firstString(opts, "short-id", "short_id"); shortID != "" {
		reality["short_id"] = shortID
	}
	return reality, true
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
		// converter.md §4: unsupported transports (xhttp/mkcp/…) must be rejected,
		// never silently downgraded to TCP.
		return fmt.Errorf("unsupported transport %s", network)
	}
	return nil
}

// addMultiplex carries Clash `smux` onto sing-box `multiplex`, but only when the
// source explicitly enables it (converter.md §6: default off, never auto-enable).
func addMultiplex(proxy map[string]any, outbound map[string]any) {
	opts, ok := normalizeMap(proxy["smux"])
	if !ok || !parseBool(opts["enabled"]) {
		return
	}
	mux := map[string]any{"enabled": true}
	addIf(mux, "protocol", asString(opts["protocol"]))
	for src, dst := range map[string]string{
		"max-connections": "max_connections",
		"min-streams":     "min_streams",
		"max-streams":     "max_streams",
	} {
		if n := asInt(opts[src]); n > 0 {
			mux[dst] = n
		}
	}
	if _, has := opts["padding"]; has {
		mux["padding"] = parseBool(opts["padding"])
	}
	outbound["multiplex"] = mux
}
