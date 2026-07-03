package subscription

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

var userAgents = map[SourceKind]string{
	SourceClash:   "clash-verge/v2.0.0",
	SourceSingBox: "sing-box/1.13.0",
	SourceBase64:  "v2rayN/6.0",
}

func Fetch(rawURL string, source SourceKind, proxy string) ([]byte, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != "" {
		proxyURL, err := url.Parse(proxy)
		if err != nil {
			return nil, fmt.Errorf("parse proxy URL: %w", err)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	client := &http.Client{Transport: transport, Timeout: 120 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent(source))

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch subscription: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read subscription: %w", err)
	}
	return data, nil
}

func userAgent(source SourceKind) string {
	if ua, ok := userAgents[source]; ok {
		return ua
	}
	return "Mozilla/5.0"
}
