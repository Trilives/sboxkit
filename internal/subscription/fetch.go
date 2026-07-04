// 拉取订阅原始内容（对应 fetch.py，net/http 取代 curl）。
//
// 机场常按 User-Agent 决定返回的订阅格式，故按来源类型设置合适的 UA。
// 可选经局域网 download_proxy 下载（覆盖出海慢的机场）。
package subscription

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

// clash 用 clash-verge UA，促使机场返回 Clash.Meta 专属字段（如 hysteria2/vless reality）；
// sing-box 用 sing-box UA，促使机场返回其原生订阅格式。
var userAgents = map[string]string{
	"clash":    "clash-verge/v2.0.0",
	"sing-box": "sing-box/1.13.0",
	"base64":   "v2rayN/6.0",
}

const (
	fetchRetries     = 3
	fetchRetryDelay  = 2 * time.Second
	fetchDialTimeout = 10 * time.Second
	// 单次尝试超时：链接/代理不可达时应快速失败并重试，而不是让用户等一次
	// 就长达数分钟——之前 120s×3 次的组合在代理不通时会让整个操作看起来像
	// 卡死。绝大多数订阅原文体积很小，30s 对正常网络绰绰有余。
	fetchTimeout = 30 * time.Second
)

// Fetch 下载订阅内容，返回原始字节。
func Fetch(rawURL, sourceType, proxy string) ([]byte, error) {
	ua, ok := userAgents[sourceType]
	if !ok {
		ua = "Mozilla/5.0"
	}
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: fetchDialTimeout}).DialContext,
		TLSHandshakeTimeout: fetchDialTimeout,
	}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	client := &http.Client{Transport: tr, Timeout: fetchTimeout}

	var lastErr error
	for i := 0; i < fetchRetries; i++ {
		if i > 0 {
			execx.Warn(fmt.Sprintf(i18n.T("拉取失败（%v），第 %d/%d 次重试…"), lastErr, i+1, fetchRetries))
			time.Sleep(fetchRetryDelay)
		}
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", ua)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode >= 400 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, rawURL)
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return data, nil
	}
	return nil, fmt.Errorf(i18n.T("拉取订阅失败: %w"), lastErr)
}
