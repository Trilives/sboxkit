// Package fetchx 下载通道（对应 core.py 的 _Fetcher，net/http 取代 curl 子进程）：
// 「直连优先 → 代理兜底」。直连探测用的是一个通用探针（Google generate_204），
// 结果只决定"先试哪个通道"，不代表目标域名本身一定直连可达——像 GitHub 这种
// 在部分网络环境下被单独封锁/DNS 劫持的域名，直连探针可能通过而实际请求仍然
// 失败，因此任一通道失败后总会尝试另一通道，不会因为探针一次性通过就完全放弃
// 代理兜底。
package fetchx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/Trilives/sboxkit/internal/execx"
	"github.com/Trilives/sboxkit/internal/i18n"
)

const ProbeURL = "https://www.google.com/generate_204"

const (
	retryAttempts = 3
	retryDelay    = 2 * time.Second
)

// stallTimeout 下载过程中允许的最大"无新数据"间隔：ResponseHeaderTimeout 只
// 保护到收到响应头为止，之后 io.Copy/Decode 读取响应体本身没有任何超时——若
// 连接中途被静默丢弃（中间设备黑洞、CDN 抖动等），会永久卡死且不会重试。这里
// 给响应体的读取加一个"停滞看门狗"：每读到数据就重置计时器，真正无进展超过
// 此时长才判定为卡死并取消请求，不影响正常的慢速大文件下载。
// 变量而非常量，便于测试用更短的间隔验证行为。
var stallTimeout = 30 * time.Second

// httpStatusError 标记"确实收到了服务器响应，只是状态码不对"，用于和网络层
// 失败（超时、连接被重置、代理隧道异常等）区分——只有后者才值得提示用户"换个
// 节点试试"，前者（比如 404/403）换节点没有意义。
type httpStatusError struct {
	status int
	url    string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.status, e.url)
}

type Fetcher struct {
	Proxy    string // 兜底代理（直连不可用时才走）
	Token    string // GitHub Token（仅 ReadJSON 的 API 请求附带）
	directOK *bool
}

func New(proxy, token string) *Fetcher {
	return &Fetcher{Proxy: proxy, Token: token}
}

// clientFor 构造指定通道的 client：direct 显式绕过环境代理；proxy 走 f.Proxy。
func clientFor(channel, proxy string) *http.Client {
	tr := &http.Transport{
		Proxy:                 nil, // direct：绕过 http_proxy 等环境变量
		ResponseHeaderTimeout: 30 * time.Second,
	}
	if channel == "proxy" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: tr}
}

// DirectReachable 探测直连是否可用（结果缓存于本 Fetcher 生命周期）。
func (f *Fetcher) DirectReachable() bool {
	if f.directOK == nil {
		c := clientFor("direct", "")
		c.Timeout = 10 * time.Second
		resp, err := c.Get(ProbeURL)
		ok := err == nil && resp.StatusCode < 400
		if resp != nil {
			resp.Body.Close()
		}
		f.directOK = &ok
		if ok {
			execx.Info(i18n.T("直连可达，跳过代理。"))
		}
	}
	return *f.directOK
}

func (f *Fetcher) channels() []string {
	if f.Proxy == "" || os.Getenv("SBOXKIT_NO_PROXY") == "1" {
		return []string{"direct"}
	}
	if f.DirectReachable() {
		return []string{"direct", "proxy"}
	}
	return []string{"proxy", "direct"}
}

// do 按通道顺序执行 fn，首个成功即返回；全失败返回最后一个错误。
func (f *Fetcher) do(fn func(c *http.Client) error) error {
	chs := f.channels()
	var last error
	for i, ch := range chs {
		err := withRetry(func() error { return fn(clientFor(ch, f.Proxy)) })
		if err == nil {
			return nil
		}
		last = err
		if i < len(chs)-1 {
			execx.Warn(fmt.Sprintf(i18n.T("  %s 通道失败（%v），改直连重试…"), ch, err))
		}
	}
	var statusErr *httpStatusError
	if last != nil && !errors.As(last, &statusErr) {
		return fmt.Errorf("%w（%s）", last, f.failureHint())
	}
	return last
}

// failureHint 全部通道都失败、且不是"服务器给了明确状态码"这类失败（即网络层
// 失败：超时、连接被重置、代理隧道异常等）时附加的提示——这类失败换个代理节点
// 往往能绕过，值得单独提示；HTTP 4xx/5xx 换节点没有意义，不会走到这里。
func (f *Fetcher) failureHint() string {
	if f.Proxy != "" {
		return i18n.T("如持续失败，可能是当前代理节点质量不佳或被限速，建议更换节点/机场后重试。")
	}
	return i18n.T("如持续失败，请检查本机网络连接。")
}

func withRetry(fn func() error) error {
	var err error
	for i := 0; i < retryAttempts; i++ {
		if err = fn(); err == nil {
			return nil
		}
		if i < retryAttempts-1 {
			time.Sleep(retryDelay)
		}
	}
	return err
}

// ReadJSON 拉取 URL 并解码 JSON（用于 GitHub API；附带 Token）。
func (f *Fetcher) ReadJSON(rawURL string, v any) error {
	return f.do(func(c *http.Client) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "sboxkit")
		req.Header.Set("Accept", "application/vnd.github+json")
		if f.Token != "" {
			req.Header.Set("Authorization", "Bearer "+f.Token)
		}
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return &httpStatusError{status: resp.StatusCode, url: rawURL}
		}
		body := guardStall(resp.Body, cancel)
		defer body.Stop()
		return wrapStallErr(json.NewDecoder(body).Decode(v))
	})
}

// stallReader 包一层"停滞看门狗"：每次读到数据就把计时器往后推，读不到新
// 数据超过 idle 时长才取消请求，中断挂起的响应体读取——调用方通过
// errors.Is(err, context.Canceled)（见 wrapStallErr）识别是否真是停滞导致的失败，
// 而不是额外维护一个标志位。
type stallReader struct {
	r     io.Reader
	timer *time.Timer
}

func guardStall(r io.Reader, cancel context.CancelFunc) *stallReader {
	return &stallReader{r: r, timer: time.AfterFunc(stallTimeout, cancel)}
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 {
		s.timer.Reset(stallTimeout)
	}
	return n, err
}

func (s *stallReader) Stop() { s.timer.Stop() }

// wrapStallErr 把停滞看门狗触发的 context.Canceled 换成对用户有意义的提示；
// 其他错误原样透传。
func wrapStallErr(err error) error {
	if err != nil && errors.Is(err, context.Canceled) {
		return fmt.Errorf(i18n.T("下载连接停滞超过 %s 未收到新数据: %w"), stallTimeout, err)
	}
	return err
}

// FetchFile 下载到 path（支持 .part 断点续传语义：path 已有内容则尝试 Range 续传）。
// 不做完整性校验、不改名——由调用方（kernel）校验后落位。
func (f *Fetcher) FetchFile(rawURL, path string) error {
	return f.do(func(c *http.Client) error {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "sboxkit")
		var offset int64
		if st, err := os.Stat(path); err == nil && st.Size() > 0 {
			offset = st.Size()
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		var out *os.File
		switch {
		case resp.StatusCode == http.StatusPartialContent && offset > 0:
			out, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
		case resp.StatusCode < 400:
			out, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		default:
			return &httpStatusError{status: resp.StatusCode, url: rawURL}
		}
		if err != nil {
			return err
		}
		body := guardStall(resp.Body, cancel)
		defer body.Stop()
		if _, err := io.Copy(out, body); err != nil {
			out.Close()
			return wrapStallErr(err)
		}
		return out.Close()
	})
}
