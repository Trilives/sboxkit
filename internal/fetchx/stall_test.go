package fetchx

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestStallGuardUnblocksRead 验证 guardStall 确实能中断一个卡死的响应体读取
// （模拟连接被静默丢弃：服务端写了几个字节后就再也不发数据也不关闭连接）。
func TestStallGuardUnblocksRead(t *testing.T) {
	orig := stallTimeout
	stallTimeout = 200 * time.Millisecond
	defer func() { stallTimeout = orig }()

	done := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done() // respond to client disconnect instead of sleeping forever
		close(done)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	guard := guardStall(resp.Body, cancel)
	defer guard.Stop()

	start := time.Now()
	_, copyErr := io.Copy(io.Discard, guard)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("stall guard did not unblock the read promptly, took %s", elapsed)
	}
	if !errors.Is(copyErr, context.Canceled) {
		t.Fatalf("expected errors.Is(err, context.Canceled), got %v", copyErr)
	}
	if wrapped := wrapStallErr(copyErr); errors.Is(wrapped, context.Canceled) == false {
		t.Fatalf("wrapStallErr should still satisfy errors.Is(context.Canceled), got %v", wrapped)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler never observed the client disconnect")
	}
}
