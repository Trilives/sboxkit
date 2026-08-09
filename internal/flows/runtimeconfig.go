package flows

import "strconv"

// runtimeInboundState reads sing-box's real inbounds instead of looking for
// Clash-only top-level keys such as mixed-port/tun.
func runtimeInboundState(doc map[string]any) (mixedPort int, tun bool) {
	inbounds, _ := doc["inbounds"].([]any)
	for _, raw := range inbounds {
		inbound, _ := raw.(map[string]any)
		switch inbound["type"] {
		case "mixed":
			mixedPort = anyInt(inbound["listen_port"])
		case "tun":
			tun = true
		}
	}
	return mixedPort, tun
}

func runtimeClashAPISecret(doc map[string]any) string {
	experimental, _ := doc["experimental"].(map[string]any)
	clashAPI, _ := experimental["clash_api"].(map[string]any)
	secret, _ := clashAPI["secret"].(string)
	return secret
}

func anyInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}
