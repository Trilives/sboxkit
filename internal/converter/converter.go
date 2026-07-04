// Package converter 把 Clash / sing-box 原生 / base64 订阅原文转换为 sing-box
// 的 JSON 配置（对应 Python 参考实现 subscription/convert.py）。
//
// 三个入口：ClashToSingBox（Clash YAML，现场生成整份配置）、SingBoxDirect
// （sing-box 原生 JSON，直接信任或按同一套规则重建）、BuildSingBoxConfig
// （给定已转换的节点 outbound 列表，组装 inbounds/outbounds/route/dns/experimental）。
//
// 文件划分：本文件是入口 + 校验；protocols.go 是各协议 outbound 转换；
// sections.go 是 inbounds/dns/route/outbounds/experimental 各段的组装；
// typeconv.go 是与业务无关的标量/类型转换 helper。
package converter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"gopkg.in/yaml.v3"
)

var reservedTags = map[string]bool{
	"Proxy": true, "AI": true, "Streaming": true, "Direct": true, "Auto": true,
	"SG-Auto": true, "SG-Fallback": true, "HK-Auto": true, "HK-Fallback": true,
	"Fallback": true, "DIRECT": true, "BLOCK": true, "DNS": true,
}

type Config struct {
	Log          map[string]any   `json:"log"`
	DNS          map[string]any   `json:"dns"`
	Inbounds     []map[string]any `json:"inbounds"`
	Outbounds    []map[string]any `json:"outbounds"`
	Route        Route            `json:"route"`
	Experimental Experimental     `json:"experimental"`
}

type Route struct {
	AutoDetectInterface bool             `json:"auto_detect_interface,omitempty"`
	DefaultResolver     map[string]any   `json:"default_domain_resolver,omitempty"`
	Rules               []map[string]any `json:"rules,omitempty"`
	RuleSet             []map[string]any `json:"rule_set,omitempty"`
	Final               string           `json:"final,omitempty"`
}

type Experimental struct {
	ClashAPI ClashAPI `json:"clash_api"`
}

type ClashAPI struct {
	ExternalController               string `json:"external_controller"`
	ExternalUI                       string `json:"external_ui,omitempty"`
	DefaultMode                      string `json:"default_mode"`
	AccessControlAllowPrivateNetwork bool   `json:"access_control_allow_private_network,omitempty"`
}

type Info map[string]any

func ClashToSingBox(yamlText string, cfg config.Config, p paths.Paths) (Config, Info, error) {
	var root map[string]any
	if err := yaml.Unmarshal([]byte(yamlText), &root); err != nil {
		return Config{}, nil, fmt.Errorf("parse clash YAML: %w", err)
	}

	rawProxies, ok := root["proxies"].([]any)
	if !ok || len(rawProxies) == 0 {
		return Config{}, nil, errors.New("subscription is missing non-empty proxies list")
	}

	used := cloneReservedTags()
	converted := make([]map[string]any, 0, len(rawProxies))
	skipped := map[string]int{}
	for _, raw := range rawProxies {
		proxy, ok := normalizeMap(raw)
		if !ok {
			skipped["proxy is not a mapping"]++
			continue
		}
		outbound, reason := convertProxy(proxy, used)
		if reason != "" {
			skipped[reason]++
			continue
		}
		converted = append(converted, outbound)
	}
	if len(converted) == 0 {
		return Config{}, nil, errors.New("no nodes converted successfully")
	}

	result, info, err := BuildSingBoxConfig(converted, cfg, p)
	if err != nil {
		return Config{}, nil, err
	}
	info["total"] = len(rawProxies)
	info["converted"] = len(converted)
	info["skipped"] = skipped
	return result, info, nil
}

func SingBoxDirect(raw string, cfg config.Config, p paths.Paths, customize bool) (Config, Info, error) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return Config{}, nil, fmt.Errorf("parse sing-box JSON: %w", err)
	}
	if !customize {
		ensureClashAPI(doc, cfg, p)
		var result Config
		data, _ := json.Marshal(doc)
		if err := json.Unmarshal(data, &result); err != nil {
			return Config{}, nil, fmt.Errorf("decode sing-box passthrough: %w", err)
		}
		return result, Info{"mode": "passthrough"}, nil
	}

	rawOutbounds, _ := doc["outbounds"].([]any)
	nodes := make([]map[string]any, 0, len(rawOutbounds))
	for _, rawOutbound := range rawOutbounds {
		outbound, ok := normalizeMap(rawOutbound)
		if !ok {
			continue
		}
		typ, _ := outbound["type"].(string)
		tag, _ := outbound["tag"].(string)
		if tag == "" || isGeneratedOutboundType(typ) {
			continue
		}
		nodes = append(nodes, outbound)
	}
	if len(nodes) == 0 {
		return Config{}, nil, errors.New("sing-box config has no reusable node outbounds")
	}
	result, info, err := BuildSingBoxConfig(nodes, cfg, p)
	if err != nil {
		return Config{}, nil, err
	}
	info["mode"] = "customized"
	return result, info, nil
}

func BuildSingBoxConfig(nodes []map[string]any, cfg config.Config, p paths.Paths) (Config, Info, error) {
	outbounds, info := buildOutbounds(nodes, cfg)
	final := cfg.DefaultOutbound
	if (final == "SG-Auto" || final == "SG-Fallback") && !boolInfo(info, "has_sg_auto") {
		final = "Proxy"
	}
	if (final == "HK-Auto" || final == "HK-Fallback") && !boolInfo(info, "has_hk_auto") {
		final = "Proxy"
	}
	result := Config{
		Log:          map[string]any{"level": "warning"},
		DNS:          buildDNS(cfg, p),
		Inbounds:     buildInbounds(cfg),
		Outbounds:    outbounds,
		Route:        buildRoute(final, cfg, p),
		Experimental: buildExperimental(cfg, p),
	}
	if err := Validate(result); err != nil {
		return Config{}, nil, err
	}
	return result, info, nil
}

func Validate(c Config) error {
	if len(c.Inbounds) == 0 {
		return errors.New("inbounds must be non-empty")
	}
	if len(c.Outbounds) == 0 {
		return errors.New("outbounds must be non-empty")
	}
	if c.Route.Final == "" {
		return errors.New("route final is required")
	}
	if c.Experimental.ClashAPI.ExternalController == "" || c.Experimental.ClashAPI.ExternalUI == "" {
		return errors.New("clash API external_controller and external_ui are required")
	}

	tags := map[string]bool{}
	for _, outbound := range c.Outbounds {
		typ, _ := outbound["type"].(string)
		tag, _ := outbound["tag"].(string)
		if typ == "" || tag == "" {
			return errors.New("each outbound must have type and tag")
		}
		if tags[tag] {
			return fmt.Errorf("duplicate outbound tag %q", tag)
		}
		tags[tag] = true
	}
	for _, outbound := range c.Outbounds {
		typ, _ := outbound["type"].(string)
		if typ != "selector" && typ != "urltest" {
			continue
		}
		tag, _ := outbound["tag"].(string)
		refs := stringSlice(outbound["outbounds"])
		if len(refs) == 0 {
			return fmt.Errorf("%s outbounds must be non-empty", tag)
		}
		for _, ref := range refs {
			if !tags[ref] {
				return fmt.Errorf("%s references missing outbound %s", tag, ref)
			}
		}
	}
	if !tags[c.Route.Final] {
		return fmt.Errorf("route final references missing outbound %s", c.Route.Final)
	}
	return nil
}

func cloneReservedTags() map[string]bool {
	out := make(map[string]bool, len(reservedTags))
	for key, value := range reservedTags {
		out[key] = value
	}
	return out
}
