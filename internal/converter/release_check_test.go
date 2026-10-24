package converter

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/testkit"
)

func TestGeneratedConfigsPassReleaseSingBoxCheck(t *testing.T) {
	bin := os.Getenv("SING_BOX_BIN")
	if bin == "" {
		t.Skip("SING_BOX_BIN is not set")
	}
	cases := []struct {
		name string
		raw  string
		cfg  config.Config
	}{
		{name: "default TUN", raw: testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), cfg: config.Defaults()},
		{name: "bootstrap DoH", raw: testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), cfg: func() config.Config {
			cfg := config.Defaults()
			cfg.BootstrapDNSType = "https"
			cfg.BootstrapDNSPort = 443
			return cfg
		}()},
		{name: "bootstrap DHCP", raw: testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml"), cfg: func() config.Config {
			cfg := config.Defaults()
			cfg.BootstrapDNSType = "dhcp"
			return cfg
		}()},
		{name: "FakeIP", raw: `dns: {enhanced-mode: fake-ip, fake-ip-filter: ["*.lan"]}
proxies:
  - {name: ss-01, type: ss, server: edge.provider.example, port: 443, cipher: aes-128-gcm, password: pw}`,
			cfg: func() config.Config {
				cfg := config.Defaults()
				cfg.FakeIPFilter = []string{"+.user.example"}
				return cfg
			}(),
		},
		{name: "FakeIP hosts", raw: hostsAndFakeIPSource, cfg: config.Defaults()},
		{name: "hosts", raw: `hosts:
  router.lan: 192.168.1.1
  agent.example: origin.example
proxies:
  - {name: ss-01, type: ss, server: edge.provider.example, port: 443, cipher: aes-128-gcm, password: pw}`, cfg: config.Defaults()},
	}
	for _, enableTun := range []bool{false, true} {
		for _, enableFakeIP := range []bool{false, true} {
			raw := testkit.ReadFixture(t, "testdata/converter/clash-basic.yaml")
			if enableFakeIP {
				raw = "dns: {enhanced-mode: fake-ip, fake-ip-filter: [\"*.lan\"]}\n" + raw
			}
			cfg := config.Defaults()
			cfg.EnableTun = enableTun
			cases = append(cases, struct {
				name string
				raw  string
				cfg  config.Config
			}{
				name: "TUN " + boolName(enableTun) + " FakeIP " + boolName(enableFakeIP),
				raw:  raw,
				cfg:  cfg,
			})
		}
	}
	// Installed services always run with the bundled geo rule-sets, and sing-box
	// only validates IP rule-sets in DNS rules once it can load them.
	seeds := testkit.RepoPath(t, "packaging/deps/rules")
	for _, tc := range cases {
		for _, withRuleSets := range []bool{false, true} {
			t.Run(tc.name+" rule-sets "+boolName(withRuleSets), func(t *testing.T) {
				p := paths.FromRoot(t.TempDir())
				if withRuleSets {
					copyRuleSetSeeds(t, seeds, p)
				}
				checkGeneratedConfig(t, bin, tc.raw, tc.cfg, p)
			})
		}
	}
}

func checkGeneratedConfig(t *testing.T, bin, raw string, cfg config.Config, p paths.Paths) {
	t.Helper()
	result, _, err := ClashToSingBox(raw, cfg, p)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(bin, "check", "-c", file).CombinedOutput(); err != nil {
		t.Fatalf("sing-box check: %v\n%s", err, output)
	}
}

func copyRuleSetSeeds(t *testing.T, seeds string, p paths.Paths) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p.GeositeCN), 0o755); err != nil {
		t.Fatalf("create ruleset dir: %v", err)
	}
	for _, dst := range []string{p.GeositeCN, p.GeoIPCN} {
		data, err := os.ReadFile(filepath.Join(seeds, filepath.Base(dst)))
		if err != nil {
			t.Fatalf("read bundled rule-set (run scripts/fetch-deb-deps.sh): %v", err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}
}
