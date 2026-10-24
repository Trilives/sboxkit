package compat

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/testkit"
)

// v0.3.1 生成器为 Hua 输出的 DNS 规则（hosts CNAME + FakeIP + 旧版 geoip-cn 地址过滤）
// 在 sing-box 1.14 起被启动校验拒绝；Check 必须在部署前复现这一失败。
func TestRealKernelRejectsV031HuaDNSRules(t *testing.T) {
	bin := os.Getenv("SING_BOX_BIN")
	if bin == "" {
		t.Skip("SING_BOX_BIN is not set")
	}
	p := paths.FromRoot(t.TempDir())
	seeds := testkit.RepoPath(t, "packaging/deps/rules")
	bundle := Bundle{Kernel: bin, Geosite: filepath.Join(seeds, "geosite-cn.srs"), GeoIP: filepath.Join(seeds, "geoip-cn.srs")}
	config := strings.NewReplacer("__GEOSITE__", p.GeositeCN, "__GEOIP__", p.GeoIPCN).
		Replace(testkit.ReadFixture(t, "testdata/compat/v0.3.1-hua-dns.json"))

	err := Check(p, []byte(config), bundle)
	var rejected *Error
	if !errors.As(err, &rejected) {
		t.Fatalf("v0.3.1 DNS rules must be rejected by %s, got %v", bin, err)
	}
	if !strings.Contains(rejected.Output, "Legacy Address Filter Fields") || rejected.Kernel == "" {
		t.Fatalf("unexpected rejection: %+v", rejected)
	}
	if !strings.Contains(rejected.Hint(), "sboxkit") {
		t.Fatalf("rejection should point at the generator: %q", rejected.Hint())
	}
}
