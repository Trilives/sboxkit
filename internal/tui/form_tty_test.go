package tui

import (
	"fmt"
	"os"
	"testing"
)

// TestFormTTYPreview is an opt-in tmux/manual smoke harness. Normal test runs
// skip it; CI or maintainers can set SBOXKIT_FORM_PREVIEW=1 and drive keys.
func TestFormTTYPreview(t *testing.T) {
	if os.Getenv("SBOXKIT_FORM_PREVIEW") != "1" {
		t.Skip("manual TTY form smoke harness")
	}
	fields := []FormField{
		{Key: "proxy", Label: "下载代理", Kind: FormText, Placeholder: "http://127.0.0.1:7890"},
		{Key: "port", Label: "代理端口", Kind: FormText, Value: "7890"},
		{Key: "tun", Label: "启用 TUN", Kind: FormBool, Value: "true"},
		{Key: "lan", Label: "局域网代理", Kind: FormBool},
		{Key: "firewall", Label: "放行端口", Kind: FormBool, Value: "true", Enabled: func(v FormResult) bool { return v.Bool("lan") }},
		{Key: "shell", Label: "shell 代理", Kind: FormBool, Value: "true", Enabled: func(v FormResult) bool { return !v.Bool("tun") }},
		{Key: "uid", Label: "直连 UID", Kind: FormText, Enabled: func(v FormResult) bool { return v.Bool("tun") }},
		{Key: "process", Label: "直连进程", Kind: FormText, Enabled: func(v FormResult) bool { return v.Bool("tun") }},
		{Key: "name", Label: "订阅名称", Kind: FormText, Value: "default"},
		{Key: "source", Label: "订阅类型", Kind: FormChoice, Value: "Clash", Options: []string{"Clash", "sing-box"}},
		{Key: "url", Label: "订阅地址", Kind: FormText},
		{Key: "fetch", Label: "代理拉取", Kind: FormBool},
		{Key: "customize", Label: "统一重建", Kind: FormBool, Value: "true"},
	}
	result, err := Form("sboxkit 初始化", fields, FormOpts{SubmitLabel: "开始初始化", Hint: "详细配置请启动后在配置里面设置"})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("FORM_RESULT=%v\n", result)
}
