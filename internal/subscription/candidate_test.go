package subscription

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestMain(m *testing.M) {
	i18n.SetLang(i18n.ZH)
	os.Exit(m.Run())
}

// isolatedState 用户模式 + 临时单元目录：服务视为未安装，不会触及真实 systemd 单元。
func isolatedState(t *testing.T) paths.Paths {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SBOXKIT_USER_MODE", "1")
	t.Setenv("SBOXKIT_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	p := paths.Detect()
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	return p
}

// installKernel 在 state 放一个 sing-box 替身：check 拒绝含 marker 的配置。
func installKernel(t *testing.T, p paths.Paths, marker string) {
	t.Helper()
	script := `#!/bin/sh
case "$1" in
version) echo "sing-box version 1.14.3" ;;
check) if grep -q '` + marker + `' "$3"; then echo "FATAL[0000] Legacy Address Filter Fields in DNS rules is deprecated in sing-box 1.14.0" >&2; exit 1; fi ;;
esac
`
	if err := os.WriteFile(p.SingBoxBin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func clashRaw(node string) string {
	return "proxies:\n  - {name: " + node + ", type: ss, server: 1.2.3.4, port: 443, cipher: aes-128-gcm, password: pw}\n"
}

// activeLocalSub 写一个已生效的本地来源订阅：原文 / 配置 / meta / state 配置 / active。
func activeLocalSub(t *testing.T, p paths.Paths, name, raw string) *Subscription {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source.yaml")
	sub := &Subscription{Name: name, URL: source, SourceType: "local", CreatedAt: now(), UpdatedAt: "before"}
	meta, err := json.Marshal(sub)
	if err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		source:              raw,
		rawFile(p, sub):     raw,
		configFile(p, name): "deployed-config",
		metaFile(p, name):   string(meta),
		p.ConfigFile:        "deployed-config",
		p.ActiveFile:        name + "\n",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return sub
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func assertRejectedByKernel(t *testing.T, err error) {
	t.Helper()
	var rejected *compat.Error
	if !errors.As(err, &rejected) || rejected.Kernel != "1.14.3" {
		t.Fatalf("expected the kernel verdict, got %v", err)
	}
	if !strings.Contains(err.Error(), "保持不变") || !strings.Contains(err.Error(), "sboxkit") {
		t.Fatalf("error should state what was kept and how to fix it: %v", err)
	}
}

func TestRebuildRejectedByKernelKeepsSubscriptionAndActiveConfig(t *testing.T) {
	p := isolatedState(t)
	installKernel(t, p, "legacy-node")
	sub := activeLocalSub(t, p, "Hua", clashRaw("legacy-node"))
	metaBefore, _ := os.ReadFile(metaFile(p, sub.Name))

	_, err := Rebuild(p, sub.Name)

	assertRejectedByKernel(t, err)
	assertFile(t, configFile(p, sub.Name), "deployed-config")
	assertFile(t, p.ConfigFile, "deployed-config")
	assertFile(t, metaFile(p, sub.Name), string(metaBefore))
	assertFile(t, p.ActiveFile, "Hua\n")
}

func TestRefreshRejectedByKernelKeepsStoredOriginal(t *testing.T) {
	p := isolatedState(t)
	installKernel(t, p, "legacy-node")
	sub := activeLocalSub(t, p, "Hua", clashRaw("good-node"))
	if err := os.WriteFile(sub.URL, []byte(clashRaw("legacy-node")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Refresh(p, sub.Name)

	assertRejectedByKernel(t, err)
	assertFile(t, rawFile(p, sub), clashRaw("good-node"))
	assertFile(t, p.ConfigFile, "deployed-config")
}

func TestAddRejectedByKernelCreatesNothing(t *testing.T) {
	p := isolatedState(t)
	installKernel(t, p, "legacy-node")
	activeLocalSub(t, p, "Hua", clashRaw("good-node"))
	source := filepath.Join(t.TempDir(), "new.yaml")
	if err := os.WriteFile(source, []byte(clashRaw("legacy-node")), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Add(p, "New", source, "local", false, true, false)

	assertRejectedByKernel(t, err)
	if _, err := os.Stat(p.SubscriptionDir("New")); !os.IsNotExist(err) {
		t.Fatalf("rejected subscription was stored: %v", err)
	}
	assertFile(t, p.ActiveFile, "Hua\n")
	assertFile(t, p.ConfigFile, "deployed-config")
}

func TestSwitchRejectedByKernelKeepsActiveSubscription(t *testing.T) {
	p := isolatedState(t)
	installKernel(t, p, "legacy-node")
	activeLocalSub(t, p, "Hua", clashRaw("good-node"))
	old := activeLocalSub(t, p, "Old", clashRaw("legacy-node"))
	if err := os.WriteFile(configFile(p, old.Name), []byte(`{"outbounds":[{"tag":"legacy-node"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ActiveFile, []byte("Hua\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Switch(p, old.Name)

	assertRejectedByKernel(t, err)
	if !strings.Contains(err.Error(), "重新生成配置") {
		t.Fatalf("switch failure should suggest regenerating the subscription: %v", err)
	}
	assertFile(t, p.ActiveFile, "Hua\n")
	assertFile(t, p.ConfigFile, "deployed-config")
}

func TestRebuildAcceptedByKernelReplacesActiveConfig(t *testing.T) {
	p := isolatedState(t)
	installKernel(t, p, "legacy-node")
	sub := activeLocalSub(t, p, "Hua", clashRaw("good-node"))

	if _, err := Rebuild(p, sub.Name); err != nil {
		t.Fatalf("compatible rebuild rejected: %v", err)
	}
	got, err := os.ReadFile(p.ConfigFile)
	if err != nil || !strings.Contains(string(got), "good-node") {
		t.Fatalf("active config not regenerated: %q, %v", got, err)
	}
	if stored, _ := os.ReadFile(configFile(p, sub.Name)); string(stored) != string(got) {
		t.Fatal("subscription config and active config diverged")
	}
	if meta := Get(p, sub.Name); meta == nil || meta.UpdatedAt == "before" || meta.LastNodeCount != 1 {
		t.Fatalf("meta not updated: %+v", meta)
	}
}

// 首次初始化下载内核前无法校验：照常生成，注册服务时由 sysd 再校验。
func TestAddWithoutKernelDefersValidation(t *testing.T) {
	p := isolatedState(t)
	source := filepath.Join(t.TempDir(), "new.yaml")
	if err := os.WriteFile(source, []byte(clashRaw("good-node")), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Add(p, "New", source, "local", false, true, false); err != nil {
		t.Fatalf("add without a kernel: %v", err)
	}
	assertFile(t, p.ActiveFile, "New\n")
}

func TestGenerateUsesStoredOriginalWithoutWriting(t *testing.T) {
	p := isolatedState(t)
	sub := activeLocalSub(t, p, "Hua", clashRaw("good-node"))

	cfgJSON, err := Generate(p, sub.Name)

	if err != nil || !strings.Contains(string(cfgJSON), "good-node") {
		t.Fatalf("generate = %q, %v", cfgJSON, err)
	}
	assertFile(t, configFile(p, sub.Name), "deployed-config")
}
