package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"
)

type Client struct {
	BaseURL string
	Secret  string
	HTTP    *http.Client
}

type Proxy struct {
	Name string   `json:"name"`
	Type string   `json:"type"`
	Now  string   `json:"now"`
	All  []string `json:"all"`
}

func NewClient(baseURL string, secret string) *Client {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:9090"
	}
	return &Client{BaseURL: baseURL, Secret: secret, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) Groups(ctx context.Context) ([]Proxy, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/proxies", nil)
	if err != nil {
		return nil, err
	}
	c.authorize(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("clash API HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Proxies map[string]Proxy `json:"proxies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	groups := []Proxy{}
	for _, proxy := range payload.Proxies {
		if len(proxy.All) > 0 {
			groups = append(groups, proxy)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	return groups, nil
}

func (c *Client) Switch(ctx context.Context, group string, node string) error {
	body, _ := json.Marshal(map[string]string{"name": node})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.BaseURL+"/proxies/"+url.PathEscape(group), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	c.authorize(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("switch node: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) authorize(req *http.Request) {
	if c.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.Secret)
	}
}
