package converter

import (
	"fmt"
	"strconv"
	"strings"
)

func normalizePort(value any) int {
	port := asInt(value)
	if port < 1 || port > 65535 {
		return 0
	}
	return port
}

func normalizeMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[asString(k)] = v
		}
		return out, true
	default:
		return nil, false
	}
}

func asString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func asInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(typed))
		return i
	default:
		return 0
	}
}

func parseBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on", "tls":
			return true
		default:
			return false
		}
	default:
		return value != nil
	}
}

func firstValue(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := m[key]; ok && value != nil && asString(value) != "" {
			return value
		}
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	return asString(firstValue(m, keys...))
}

func defaultValue(value any, fallback any) any {
	if value == nil || asString(value) == "" {
		return fallback
	}
	return value
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := asString(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	default:
		return nil
	}
}

func splitComma(text string) []string {
	parts := strings.Split(text, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func addIf(m map[string]any, key string, value string) {
	if value != "" {
		m[key] = value
	}
}

// bandwidthMbps parses a Clash bandwidth value ("100", "100 Mbps", "1 Gbps")
// into an integer megabits/sec for sing-box up_mbps/down_mbps.
func bandwidthMbps(value any) int {
	s := strings.ToLower(strings.TrimSpace(asString(value)))
	if s == "" {
		return 0
	}
	factor := 1.0
	for _, unit := range []struct {
		suffix string
		mult   float64
	}{
		{"gbps", 1000}, {"mbps", 1}, {"kbps", 0.001},
		{"g", 1000}, {"m", 1}, {"k", 0.001},
	} {
		if strings.HasSuffix(s, unit.suffix) {
			s = strings.TrimSpace(strings.TrimSuffix(s, unit.suffix))
			factor = unit.mult
			break
		}
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(n * factor)
}

// serverPortRanges converts a Clash port-hopping spec ("443,8000-9000") into
// sing-box server_ports entries ("443:443", "8000:9000").
func serverPortRanges(spec string) []string {
	out := []string{}
	for _, part := range splitComma(spec) {
		if strings.Contains(part, "-") {
			out = append(out, strings.Replace(part, "-", ":", 1))
		} else {
			out = append(out, part+":"+part)
		}
	}
	return out
}

// msToDuration renders a millisecond integer as a sing-box duration string
// ("10000" -> "10s"), keeping millisecond precision when not a whole second.
func msToDuration(value any) string {
	ms := asInt(value)
	if ms <= 0 {
		return ""
	}
	if ms%1000 == 0 {
		return strconv.Itoa(ms/1000) + "s"
	}
	return strconv.Itoa(ms) + "ms"
}
