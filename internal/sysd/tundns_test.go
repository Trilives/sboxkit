package sysd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

// generatedTun 与 converter 生成的 TUN 入站一致。
const generatedTun = `{"inbounds":[{"type":"tun","tag":"tun-in","interface_name":"singbox","address":["172.19.0.1/30"],"auto_route":true},{"type":"mixed","listen_port":7890}]}`

func TestTunDNSLinksMatchWhatSingBoxRegistersAsRoot(t *testing.T) {
	for raw, want := range map[string][]tunDNSLink{
		generatedTun: {{Dev: "singbox", Servers: []string{"172.19.0.2"}}},
		`{"inbounds":[{"type":"tun","interface_name":"tun0","address":["fdfe:dcba:9876::1/126","172.18.0.1/30","172.20.0.1/30"]}]}`: {
			{Dev: "tun0", Servers: []string{"172.18.0.2", "fdfe:dcba:9876::2"}},
		},
		`{"inbounds":[{"type":"tun","interface_name":"tun0","address":"10.0.0.1/24"}]}`:                {{Dev: "tun0", Servers: []string{"10.0.0.2"}}},
		`{"inbounds":[{"type":"tun","interface_name":"tun0","address":["10.0.0.1/32","fd00::1/64"]}]}`: {{Dev: "tun0", Servers: []string{"fd00::2"}}},
		`{"inbounds":[{"type":"mixed","listen_port":7890}]}`:                                           nil,
		`{}`: nil,
	} {
		got, err := tunDNSLinks(decode(t, raw))
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("tunDNSLinks(%s) = %+v, %v; want %+v", raw, got, err, want)
		}
	}
}

func TestTunDNSLinksRejectsLinksItCannotRegister(t *testing.T) {
	for _, raw := range []string{
		`{"inbounds":[{"type":"tun","address":["172.19.0.1/30"]}]}`,
		`{"inbounds":[{"type":"tun","interface_name":"-x","address":["172.19.0.1/30"]}]}`,
		`{"inbounds":[{"type":"tun","interface_name":"tun\n0","address":["172.19.0.1/30"]}]}`,
		`{"inbounds":[{"type":"tun","interface_name":"tun0","address":["172.19.0.1/32"]}]}`,
		`{"inbounds":[{"type":"tun","interface_name":"tun0"}]}`,
	} {
		if links, err := tunDNSLinks(decode(t, raw)); err == nil {
			t.Errorf("tunDNSLinks(%s) = %+v, want an error", raw, links)
		}
	}
}

func TestChooseIdentityKeepsRootWhenTunDNSCannotBeRegistered(t *testing.T) {
	stubSystemd(t, 255)
	p := paths.FromRoot(t.TempDir())

	if id, reason := chooseIdentity(p, unitScope{}, decode(t, generatedTun)); id != identityDynamic || reason != "" {
		t.Fatalf("generated TUN: %v (%q), want the dynamic user", id, reason)
	}
	unnamed := decode(t, `{"inbounds":[{"type":"tun","address":["172.19.0.1/30"]}]}`)
	if id, reason := chooseIdentity(p, unitScope{}, unnamed); id != identityRoot || reason == "" {
		t.Fatalf("unnamed TUN: %v (%q), want root with an explanation", id, reason)
	}
	// 每个链路的钩子依次各等至多 1 分钟，合计可能超出启动时限而被误判为非 root 起不来。
	two := decode(t, `{"inbounds":[{"type":"tun","interface_name":"tun0","address":["172.18.0.1/30"]},{"type":"tun","interface_name":"tun1","address":["172.20.0.1/30"]}]}`)
	if id, reason := chooseIdentity(p, unitScope{}, two); id != identityRoot || reason == "" {
		t.Fatalf("two TUN inbounds: %v (%q), want root with an explanation", id, reason)
	}
}

func TestWithRuntimeIdentityListsTunDNSOnlyForTheDynamicUser(t *testing.T) {
	stubSystemd(t, 255)
	p := paths.FromRoot(t.TempDir())
	staged := filepath.Join(t.TempDir(), "staged.json")
	rt := rtPathsFor(unitScope{}, DefaultName)

	if err := os.WriteFile(staged, []byte(generatedTun), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := withRuntimeIdentity(p, rt, staged)
	if err != nil || got.Identity != identityDynamic || len(got.TunDNS) != 1 || got.TunDNS[0].Dev != "singbox" {
		t.Fatalf("dynamic: %+v, %v", got, err)
	}

	process := `{"inbounds":[{"type":"tun","interface_name":"singbox","address":["172.19.0.1/30"]}],"route":{"rules":[{"process_name":["aria2c"],"outbound":"DIRECT"}]}}`
	if err := os.WriteFile(staged, []byte(process), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = withRuntimeIdentity(p, rt, staged)
	if err != nil || got.Identity != identityRoot || got.TunDNS != nil {
		t.Fatalf("root: %+v, %v; a root sing-box registers its own DNS", got, err)
	}
}

func TestDynamicTunUnitRegistersDNSAsRootAfterStart(t *testing.T) {
	stubServiceExecutable(t, "/usr/bin/sboxkit")
	rt := rtPathsFor(unitScope{UnitDir: systemUnitDir, RuntimeDir: paths.RuntimeDir}, DefaultName)
	rt.Identity = identityDynamic
	rt.TunDNS = []tunDNSLink{{Dev: "singbox", Servers: []string{"172.19.0.2", "fdfe:dcba:9876::2"}}}

	unit, err := renderUnit(DefaultName, rt)
	if err != nil {
		t.Fatal(err)
	}

	want := "\nExecStart=/var/lib/sboxkit-runtime/sing-box run -c ${CREDENTIALS_DIRECTORY}/config.json\n" +
		`ExecStartPost=+"/usr/bin/sboxkit" tun-dns --dev singbox --dns 172.19.0.2 --dns fdfe:dcba:9876::2` + "\n"
	if !strings.Contains(unit, want) {
		t.Fatalf("dynamic TUN unit missing the DNS hook:\n%s", unit)
	}
}

func TestOnlyDynamicTunUnitsCarryTheDNSHook(t *testing.T) {
	stubServiceExecutable(t, "/usr/bin/sboxkit")
	base := rtPathsFor(unitScope{UnitDir: systemUnitDir, RuntimeDir: paths.RuntimeDir}, DefaultName)
	links := []tunDNSLink{{Dev: "singbox", Servers: []string{"172.19.0.2"}}}

	asRoot := base
	asRoot.TunDNS = links // verifyIdentitySwitch 退回 root 时沿用同一份 runtimePaths
	noTun := base
	noTun.Identity = identityDynamic
	for label, rt := range map[string]runtimePaths{"root": asRoot, "dynamic without TUN": noTun} {
		unit, err := renderUnit(DefaultName, rt)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(unit, "ExecStartPost") {
			t.Errorf("%s unit must not register DNS:\n%s", label, unit)
		}
	}
}

func TestNeedsIdentityCheckWhenTheDynamicUnitChanges(t *testing.T) {
	stubServiceExecutable(t, "/usr/bin/sboxkit")
	rt := rtPathsFor(unitScope{UnitDir: t.TempDir(), RuntimeDir: paths.RuntimeDir}, DefaultName)
	rt.Identity = identityDynamic

	if !needsIdentityCheck(DefaultName, rt) {
		t.Fatal("a first dynamic unit must be checked")
	}
	withoutHook, err := renderUnit(DefaultName, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rt.Unit, []byte(withoutHook), 0o644); err != nil {
		t.Fatal(err)
	}
	if needsIdentityCheck(DefaultName, rt) {
		t.Fatal("an unchanged dynamic unit was already checked")
	}
	rt.TunDNS = []tunDNSLink{{Dev: "singbox", Servers: []string{"172.19.0.2"}}}
	if !needsIdentityCheck(DefaultName, rt) {
		t.Fatal("adding the DNS hook must be checked: a failing hook falls back to root")
	}
	asRoot := rt
	asRoot.Identity = identityRoot
	if needsIdentityCheck(DefaultName, asRoot) {
		t.Fatal("root units are not checked")
	}
}
