package sysd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUserTunDNSUnitRunsOnlyResolvectlWhenTheLinkAppears(t *testing.T) {
	link := tunDNSLink{Dev: "singbox", Servers: []string{"172.19.0.2", "fdfe:dcba:9876::2"}}
	device, ok := netDeviceUnit(link.Dev)
	if !ok {
		t.Fatal("singbox must map to a device unit")
	}

	text := userTunDNSUnitText(1000, link, device, "/usr/bin/resolvectl", true)

	for _, want := range []string{
		"\nBindsTo=sys-subsystem-net-devices-singbox.device\n",
		"\nAfter=sys-subsystem-net-devices-singbox.device systemd-resolved.service\n",
		// 接口名可能出自原样信任的原生订阅：同名的物理网卡不是 TUN，不得改写它的 DNS。
		"\nConditionPathExists=/sys/class/net/singbox/tun_flags\n",
		"\nType=oneshot\n",
		"\nRestart=on-failure\nRestartSec=1\n",
		"\nExecStart=\"/usr/bin/resolvectl\" dns singbox 172.19.0.2 fdfe:dcba:9876::2\n",
		"\nExecStart=\"/usr/bin/resolvectl\" domain singbox ~.\n",
		"\nExecStart=\"/usr/bin/resolvectl\" default-route singbox yes\n",
		"\nWantedBy=sys-subsystem-net-devices-singbox.device\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("unit missing %q:\n%s", strings.TrimSpace(want), text)
		}
	}
	if strings.Contains(text, "sboxkit tun-dns") || strings.Contains(text, "User=") {
		t.Fatalf("the root unit must not run the user's sboxkit:\n%s", text)
	}
	// systemd 244 之前 Type=oneshot 带 Restart= 的单元拒绝加载。
	if old := userTunDNSUnitText(1000, link, device, "/usr/bin/resolvectl", false); strings.Contains(old, "Restart=") {
		t.Fatalf("units for systemd < %d must not use Restart=:\n%s", minOneshotRestartSystemd, old)
	}
}

func TestStaleUserTunDNSListsUnitsTheConfigNoLongerNeeds(t *testing.T) {
	dir := "/etc/systemd/system/"
	installed := []string{dir + "sboxkit-tun-dns-1000-singbox.service", dir + "sboxkit-tun-dns-1000-tun0.service"}

	stale := staleUserTunDNS(1000, installed, []tunDNSLink{{Dev: "singbox", Servers: []string{"172.19.0.2"}}})
	if want := installed[1:]; !reflect.DeepEqual(stale, want) {
		t.Fatalf("stale = %q, want %q", stale, want)
	}
	if stale := staleUserTunDNS(1000, installed, nil); !reflect.DeepEqual(stale, installed) {
		t.Fatalf("TUN off: stale = %q, want every unit", stale)
	}
}

func TestNetDeviceUnitOnlyForNamesThatNeedNoEscaping(t *testing.T) {
	for dev, want := range map[string]bool{"singbox": true, "tun0": true, "sb_tun.1": true, "sb-tun": false, ".tun": false} {
		if _, got := netDeviceUnit(dev); got != want {
			t.Errorf("netDeviceUnit(%q) ok = %v, want %v", dev, got, want)
		}
	}
}

func TestPlanUserTunDNSInstallsOnlyMissingOrChangedUnits(t *testing.T) {
	links := []tunDNSLink{
		{Dev: "singbox", Servers: []string{"172.19.0.2"}},
		{Dev: "tun0", Servers: []string{"10.0.0.2"}},
		{Dev: "sb-tun", Servers: []string{"10.1.0.2"}},
	}
	upToDate := userTunDNSUnitText(1000, links[0], "sys-subsystem-net-devices-singbox.device", "/usr/bin/resolvectl", true)
	installed := map[string]string{userTunDNSUnitName(1000, "singbox"): upToDate}

	units, warnings := planUserTunDNS(1000, links, "/usr/bin/resolvectl", true, func(name string) (string, bool) {
		text, ok := installed[name]
		return text, ok
	})

	var names []string
	for _, unit := range units {
		names = append(names, unit.Name)
	}
	if want := []string{"sboxkit-tun-dns-1000-tun0.service"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("units = %q, want %q", names, want)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "sb-tun") {
		t.Fatalf("warnings = %q, want one for sb-tun", warnings)
	}

	units, _ = planUserTunDNS(1000, links[:1], "/usr/bin/resolvectl", true, func(string) (string, bool) { return upToDate, false })
	if len(units) != 1 {
		t.Fatal("an installed but disabled unit must be re-enabled")
	}
}

func TestEnsureTunDNSFromChecksEveryLinkOfTheRuntimeConfig(t *testing.T) {
	orig := tunDNSEnsure
	t.Cleanup(func() { tunDNSEnsure = orig })
	var checked []string
	tunDNSEnsure = func(link string, servers []string) (bool, error) {
		checked = append(checked, link+" "+strings.Join(servers, ","))
		return true, nil
	}
	cfg := filepath.Join(t.TempDir(), "sing-box.json")
	if err := os.WriteFile(cfg, []byte(generatedTun), 0o600); err != nil {
		t.Fatal(err)
	}

	ensureTunDNSFrom(cfg)
	ensureTunDNSFrom(filepath.Join(t.TempDir(), "missing.json"))

	if want := []string{"singbox 172.19.0.2"}; !reflect.DeepEqual(checked, want) {
		t.Fatalf("checked = %q, want %q", checked, want)
	}
}
