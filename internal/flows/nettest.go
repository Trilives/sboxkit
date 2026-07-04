// 网络测试（对应 flows/nettest.py，net/http 取代 curl）：测主流流媒体 / 站点 / AI
// 服务的延迟（TTFB），并探测 OpenAI / Claude 等的出口 IP（经本地 sing-box 代理）。
//
// 优先经本地 mixed 入站 127.0.0.1:7890 走代理（即「走代理后的真实体验」）；
// 代理端口未监听时回退直连并标注。出口 IP 借 Cloudflare 边缘的 /cdn-cgi/trace。
package flows

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/sysd"
	"github.com/Trilives/sboxkit/internal/tui"
)

const (
	nettestProxyHost = "127.0.0.1"
	nettestProxyPort = 7890
	nettestUA        = "Mozilla/5.0 (X11; Linux x86_64) sboxkit-nettest"
	nettestTimeout   = 10 * time.Second
)

type latTarget struct {
	cat, name, url string
}

var latencyTargets = []latTarget{
	{"流媒体", "Netflix", "https://www.netflix.com/title/80018499"},
	{"流媒体", "YouTube", "https://www.youtube.com/generate_204"},
	{"流媒体", "Disney+", "https://www.disneyplus.com/"},
	{"流媒体", "TikTok", "https://www.tiktok.com/"},
	{"流媒体", "Spotify", "https://www.spotify.com/"},
	{"站点", "Google", "https://www.google.com/generate_204"},
	{"站点", "GitHub", "https://github.com/"},
	{"站点", "Cloudflare", "https://www.cloudflare.com/cdn-cgi/trace"},
	{"站点", "Wikipedia", "https://en.wikipedia.org/"},
	{"AI", "OpenAI", "https://chat.openai.com/cdn-cgi/trace"},
	{"AI", "Claude", "https://claude.ai/cdn-cgi/trace"},
	{"AI", "Gemini", "https://gemini.google.com/"},
}

var traceTargets = []latTarget{
	{"", "OpenAI", "https://chat.openai.com/cdn-cgi/trace"},
	{"", "Claude", "https://claude.ai/cdn-cgi/trace"},
	{"", "Cloudflare", "https://www.cloudflare.com/cdn-cgi/trace"},
}

type latResult struct {
	ms   int
	ok   bool
	code string
}

func proxyUp() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", nettestProxyHost, nettestProxyPort), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func nettestClient(viaProxy bool) *http.Client {
	tr := &http.Transport{Proxy: nil}
	if viaProxy {
		u, _ := url.Parse(fmt.Sprintf("http://%s:%d", nettestProxyHost, nettestProxyPort))
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr, Timeout: nettestTimeout}
}

// latency 单目标 TTFB（time to first response byte）与状态码。
func latency(client *http.Client, rawURL string) latResult {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return latResult{code: "ERR"}
	}
	req.Header.Set("User-Agent", nettestUA)
	start := time.Now()
	var ttfb time.Duration
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() { ttfb = time.Since(start) },
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	resp, err := client.Do(req)
	if err != nil {
		return latResult{code: "ERR"}
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<14))
	resp.Body.Close()
	if ttfb == 0 {
		ttfb = time.Since(start)
	}
	return latResult{ms: int(ttfb.Milliseconds()), ok: true, code: fmt.Sprint(resp.StatusCode)}
}

// cfTrace 拉取 /cdn-cgi/trace 并解析 k=v 字段。
func cfTrace(client *http.Client, rawURL string) map[string]string {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", nettestUA)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
	if err != nil {
		return nil
	}
	fields := map[string]string{}
	for _, line := range strings.Split(string(body), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			fields[k] = v
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// runPool 并发跑 worker(i)，带 TTY 进度。
func runPool(n int, worker func(i int), label string) {
	tty := term.IsTerminal(int(os.Stdout.Fd()))
	if !tty {
		execx.Info(fmt.Sprintf(i18n.T("%s（%d 项）…"), label, n))
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, min(12, n))
	done := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			worker(i)
			mu.Lock()
			done++
			if tty {
				fmt.Printf("\r\033[K  %s… %d/%d", label, done, n)
			}
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	if tty {
		fmt.Print("\r\033[K")
	}
}

func fmtMs(r latResult) string {
	if !r.ok {
		return i18n.T("超时")
	}
	return fmt.Sprintf("%dms", r.ms)
}

// fileLocations 主要文件/目录位置一览（对应「网络测试 / 诊断」聚合的排障信息）。
func fileLocations(p paths.Paths) []struct{ label, path string } {
	return []struct{ label, path string }{
		{i18n.T("生效配置"), p.ConfigFile},
		{i18n.T("定制层"), p.CustomizeFile},
		{i18n.T("生效订阅名"), p.ActiveFile},
		{i18n.T("订阅目录"), p.Subscriptions},
		{i18n.T("sing-box 内核"), p.SingBoxBin},
		{i18n.T("基础规则目录"), p.Ruleset},
		{i18n.T("Web UI 目录"), p.UI},
		{i18n.T("下载缓存目录"), p.Downloads},
		{i18n.T("systemd 单元"), "/etc/systemd/system/" + sysd.DefaultName + ".service"},
	}
}

// FileLocationsTool 显示主要文件/目录位置一览（「工具」菜单的一项）。
func FileLocationsTool(p paths.Paths) error {
	execx.Header(i18n.T("主要文件位置"))
	for _, r := range fileLocations(p) {
		fmt.Printf("  %-12s %s\n", r.label+":", r.path)
	}
	fmt.Println()
	tui.Pause(i18n.T("回车返回主菜单… "))
	return nil
}

// ToolsMenu 「工具」菜单：网络测试 / 主要文件位置 / 信息，未来可继续添加其它排障工具。
func ToolsMenu(p paths.Paths) error {
	idx := 0
	for {
		options := []string{i18n.T("网络测试"), i18n.T("主要文件位置"), i18n.T("信息")}
		i, err := tui.Select(i18n.T("工具"), options, tui.SelectOpts{BackLabel: i18n.T("返回上层"), Initial: idx})
		if err != nil {
			return nil
		}
		idx = i
		var terr error
		switch i {
		case 0:
			terr = Nettest()
		case 1:
			terr = FileLocationsTool(p)
		case 2:
			terr = InfoTool(p)
		}
		if terr != nil {
			execx.Error(terr.Error())
		}
	}
}

// Nettest 网络测试全流程：延迟测试 + 出口 IP。
func Nettest() error {
	execx.Header(i18n.T("网络测试"))
	viaProxy := proxyUp()
	proxyURL := fmt.Sprintf("http://%s:%d", nettestProxyHost, nettestProxyPort)
	if viaProxy {
		execx.Info(fmt.Sprintf(i18n.T("经本地代理 %s 测试（走 sing-box 出口）。"), proxyURL))
	} else {
		execx.Warn(fmt.Sprintf(i18n.T("本地代理 %s 未监听，改用直连测试（结果不代表代理体验）。"), proxyURL))
	}
	client := nettestClient(viaProxy)

	// 1. 延迟
	lat := make([]latResult, len(latencyTargets))
	runPool(len(latencyTargets), func(i int) { lat[i] = latency(client, latencyTargets[i].url) }, i18n.T("延迟测试"))
	fmt.Println()
	lastCat := ""
	for i, t := range latencyTargets {
		if t.cat != lastCat {
			execx.Info(fmt.Sprintf(i18n.T("【%s】"), i18n.T(t.cat)))
			lastCat = t.cat
		}
		mark := "✗"
		if lat[i].ok {
			mark = "✓"
		}
		fmt.Printf("  %s %-12s %8s  (HTTP %s)\n", mark, t.name, fmtMs(lat[i]), lat[i].code)
	}

	// 2. 出口 IP（OpenAI / Claude / Cloudflare）
	fmt.Println()
	execx.Info(fmt.Sprintf(i18n.T("【%s】"), i18n.T("出口 IP / 落地")))
	traces := make([]map[string]string, len(traceTargets))
	runPool(len(traceTargets), func(i int) { traces[i] = cfTrace(client, traceTargets[i].url) }, i18n.T("出口探测"))
	for i, t := range traceTargets {
		f := traces[i]
		if f == nil || f["ip"] == "" {
			fmt.Printf(i18n.T("  ✗ %-12s 探测失败\n"), t.name)
			continue
		}
		loc := f["loc"]
		if loc == "" {
			loc = "?"
		}
		extra := ""
		if f["colo"] != "" {
			extra = fmt.Sprintf("  [%s]", f["colo"])
		}
		fmt.Printf(i18n.T("  ✓ %-12s %-22s 落地 %s%s\n"), t.name, f["ip"], loc, extra)
	}

	fmt.Println()
	execx.Ok(i18n.T("网络测试完成。"))
	tui.Pause(i18n.T("回车返回主菜单… "))
	return nil
}
