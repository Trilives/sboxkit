// Package subscription 订阅处理：类型识别、最小改写（patch）、分流叠加（overlay）、
// 地区聚合组（regiongroups）等（对应 Python 版 subscription/ 子包）。
//
// 配置统一以 map[string]any 表达（yaml.v3 / encoding/json 的通用反序列化形态），
// 本文件提供跨来源（YAML 的 int、JSON 的 float64）的容错取值助手。
package subscription

import (
	"fmt"
	"strconv"
)

func anyToStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case int:
		return strconv.Itoa(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", x)
	}
}

// truthyVal 对应 Python bool(x) 的常见形态。
func truthyVal(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case float64:
		return x != 0
	case int:
		return x != 0
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

// truthy 对应 Python `bool(cfg.get(key, default))`。
func truthy(cfg map[string]any, key string, def bool) bool {
	v, ok := cfg[key]
	if !ok {
		return def
	}
	return truthyVal(v)
}

// strOr 对应 Python `str(v or default)`。
func strOr(v any, def string) string {
	if !truthyVal(v) {
		return def
	}
	return anyToStr(v)
}

func lenList(v any) int {
	if l, ok := v.([]any); ok {
		return len(l)
	}
	return 0
}

func strListOf(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, anyToStr(e))
		}
		return out
	}
	return nil
}

func intListOf(v any) []int {
	switch x := v.(type) {
	case []int:
		return x
	case []any:
		out := make([]int, 0, len(x))
		for _, e := range x {
			switch n := e.(type) {
			case int:
				out = append(out, n)
			case float64:
				out = append(out, int(n))
			case string:
				if i, err := strconv.Atoi(n); err == nil {
					out = append(out, i)
				}
			}
		}
		return out
	}
	return nil
}
