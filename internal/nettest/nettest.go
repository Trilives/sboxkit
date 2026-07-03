package nettest

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type Target struct {
	Name string
	URL  string
}

type Result struct {
	Name    string
	URL     string
	Status  int
	TTFB    time.Duration
	Error   string
	Proxied bool
}

type Trace struct {
	IP      string
	Country string
	Colo    string
}

var DefaultTargets = []Target{
	{"Google", "https://www.google.com/generate_204"},
	{"GitHub", "https://github.com/"},
	{"Cloudflare", "https://www.cloudflare.com/cdn-cgi/trace"},
	{"OpenAI", "https://chatgpt.com/cdn-cgi/trace"},
	{"Claude", "https://claude.ai/"},
	{"YouTube", "https://www.youtube.com/generate_204"},
	{"Netflix", "https://www.netflix.com/"},
}

func Run(ctx context.Context, targets []Target, proxyAddr string) []Result {
	if len(targets) == 0 {
		targets = DefaultTargets
	}
	proxied := proxyReachable(proxyAddr)
	client := httpClient(proxyAddr, proxied)

	results := make([]Result, len(targets))
	var wg sync.WaitGroup
	for i, target := range targets {
		wg.Add(1)
		go func(i int, target Target) {
			defer wg.Done()
			results[i] = probe(ctx, client, target, proxied)
		}(i, target)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	return results
}

func Format(results []Result) string {
	var b strings.Builder
	for _, result := range results {
		if result.Error != "" {
			fmt.Fprintf(&b, "%-12s ERROR %s\n", result.Name, result.Error)
			continue
		}
		mode := "direct"
		if result.Proxied {
			mode = "proxy"
		}
		fmt.Fprintf(&b, "%-12s %4d %6dms %s\n", result.Name, result.Status, result.TTFB.Milliseconds(), mode)
	}
	return b.String()
}

func probe(ctx context.Context, client *http.Client, target Target, proxied bool) Result {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return Result{Name: target.Name, URL: target.URL, Error: err.Error(), Proxied: proxied}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{Name: target.Name, URL: target.URL, Error: err.Error(), Proxied: proxied}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return Result{Name: target.Name, URL: target.URL, Status: resp.StatusCode, TTFB: time.Since(start), Proxied: proxied}
}

func httpClient(proxyAddr string, proxied bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxied {
		proxyURL, _ := url.Parse("http://" + proxyAddr)
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}
}

func proxyReachable(proxyAddr string) bool {
	if proxyAddr == "" {
		proxyAddr = "127.0.0.1:7890"
	}
	conn, err := net.DialTimeout("tcp", proxyAddr, 700*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func parseTrace(text string) Trace {
	trace := Trace{}
	for _, line := range strings.Split(text, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "ip":
			trace.IP = value
		case "loc":
			trace.Country = value
		case "colo":
			trace.Colo = value
		}
	}
	return trace
}
