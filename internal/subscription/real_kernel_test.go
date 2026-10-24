package subscription

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/testkit"
)

// Hua 特征：hosts CNAME + FakeIP + 依赖 geoip-cn 的 DNS 规则。
const huaFeatureSource = `
dns: {enhanced-mode: fake-ip}
hosts:
  router.lan: 192.168.1.1
  agent.example: origin.example
proxies:
  - {name: ss-01, type: ss, server: edge.provider.example, port: 443, cipher: aes-128-gcm, password: pw}
rules:
  - GEOIP,CN,DIRECT
  - MATCH,Proxy
`

// realKernelState 隔离状态目录，装入 SING_BOX_BIN 与随包规则集种子。
func realKernelState(t *testing.T) paths.Paths {
	t.Helper()
	bin := os.Getenv("SING_BOX_BIN")
	if bin == "" {
		t.Skip("SING_BOX_BIN is not set")
	}
	p := isolatedState(t)
	if err := os.Symlink(bin, p.SingBoxBin); err != nil {
		t.Fatal(err)
	}
	seeds := testkit.RepoPath(t, "packaging/deps/rules")
	for _, dst := range []string{p.GeositeCN, p.GeoIPCN} {
		data, err := os.ReadFile(filepath.Join(seeds, filepath.Base(dst)))
		if err != nil {
			t.Fatalf("read bundled rule-set (run scripts/fetch-deb-deps.sh): %v", err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// v0.3.1 为 Hua 生成的配置被 1.14 拒绝：切换到它必须失败且保持原生效订阅；
// 当前生成器从同一份 Hua 原文重建的配置通过真实内核校验后才部署。
func TestRealKernelGuardsSwitchAndRebuildOfHuaFeatures(t *testing.T) {
	p := realKernelState(t)
	hua := activeLocalSub(t, p, "Hua", huaFeatureSource)
	legacy := activeLocalSub(t, p, "Legacy", huaFeatureSource)
	fixture := strings.NewReplacer("__GEOSITE__", p.GeositeCN, "__GEOIP__", p.GeoIPCN).
		Replace(testkit.ReadFixture(t, "testdata/compat/v0.3.1-hua-dns.json"))
	if err := os.WriteFile(configFile(p, legacy.Name), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ActiveFile, []byte("Hua\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := Switch(p, legacy.Name)
	var rejected *compat.Error
	if !errors.As(err, &rejected) || !strings.Contains(rejected.Output, "Legacy Address Filter Fields") {
		t.Fatalf("switching to the v0.3.1 config must be rejected by the kernel, got %v", err)
	}
	assertFile(t, p.ActiveFile, "Hua\n")
	assertFile(t, p.ConfigFile, "deployed-config")

	if _, err := Rebuild(p, hua.Name); err != nil {
		t.Fatalf("current generator output rejected by %s: %v", os.Getenv("SING_BOX_BIN"), err)
	}
	active, err := os.ReadFile(p.ConfigFile)
	if err != nil || !strings.Contains(string(active), "match_response") {
		t.Fatalf("rebuilt Hua config not deployed: %v", err)
	}
	if err := compat.Check(p, active, compat.StateBundle(p)); err != nil {
		t.Fatalf("deployed config fails the real kernel: %v", err)
	}
}
