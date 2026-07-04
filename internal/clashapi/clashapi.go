// Package clashapi Clash API 客户端（对应 node_select.py 的 urllib 部分）：
// 版本探测 / 实时切组 / 并发测延迟。
//
// sing-box 的 experimental.clash_api 与 mihomo 的 external-controller 走同一套
// HTTP 接口（/proxies、/proxies/{group}、/proxies/{name}/delay），协议逻辑不用
// 改；唯一的差异是控制器信息在配置里的位置——mihomo 是顶层 external-controller /
// secret 平级字段，sing-box 嵌在 experimental.clash_api 之下。
package clashapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DelayURL       = "https://www.gstatic.com/generate_204"
	DelayTimeoutMS = 5000
)

type Client struct {
	Base   string
	secret string
	http   *http.Client
}

// FromConfig 从运行时配置提取 experimental.clash_api.external_controller / secret；
// 无控制器返回 nil。
func FromConfig(cfg map[string]any) *Client {
	experimental, _ := cfg["experimental"].(map[string]any)
	api, _ := experimental["clash_api"].(map[string]any)
	controller, _ := api["external_controller"].(string)
	if controller == "" {
		return nil
	}
	host, port := controller, "9090"
	if i := strings.LastIndex(controller, ":"); i >= 0 {
		host, port = controller[:i], controller[i+1:]
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	secret, _ := api["secret"].(string)
	return &Client{
		Base:   fmt.Sprintf("http://%s:%s", host, port),
		secret: secret,
		http:   &http.Client{Transport: &http.Transport{Proxy: nil}},
	}
}

func (c *Client) req(method, path string, body []byte, timeout time.Duration) (*http.Response, error) {
	var rd *bytes.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, c.Base+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	cl := *c.http
	cl.Timeout = timeout
	return cl.Do(req)
}

// Reachable API 是否可达（GET /version）。
func (c *Client) Reachable() bool {
	resp, err := c.req(http.MethodGet, "/version", nil, 2*time.Second)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}

// Switch 实时切换 group 的选中节点（PUT /proxies/{group}）。
func (c *Client) Switch(group, node string) error {
	body, _ := json.Marshal(map[string]string{"name": node})
	resp, err := c.req(http.MethodPut, "/proxies/"+url.PathEscape(group), body, 4*time.Second)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

// Delay 测某节点延迟（ms）；失败/超时返回 (0, false)。
func (c *Client) Delay(name string) (int, bool) {
	q := url.Values{"url": {DelayURL}, "timeout": {fmt.Sprint(DelayTimeoutMS)}}
	resp, err := c.req(http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay?"+q.Encode(),
		nil, DelayTimeoutMS*time.Millisecond+2*time.Second)
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, false
	}
	var out struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Delay == 0 {
		return 0, false
	}
	return out.Delay, true
}
