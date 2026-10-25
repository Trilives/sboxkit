package tundns

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeSystem struct {
	active     bool
	upAfter    int               // linkUp 第几次调用起返回 true；< 0 表示链路始终不出现
	failDNS    int               // 前几次 resolvectl dns 调用失败
	current    map[string]string // 查询类 resolvectl 调用（只给链路名）的输出
	linkChecks int
	calls      []string
	sleeps     int
	clock      time.Time
}

func (f *fakeSystem) system() system {
	return system{
		resolvedActive: func() bool { return f.active },
		linkUp: func(string) bool {
			f.linkChecks++
			return f.upAfter >= 0 && f.linkChecks > f.upAfter
		},
		resolvectl: func(args ...string) (string, error) {
			if len(args) == 2 {
				return f.current[args[0]], nil
			}
			if args[0] == "dns" && f.failDNS > 0 {
				f.failDNS--
				return "", errors.New("Link 42 not known")
			}
			f.calls = append(f.calls, strings.Join(args, " "))
			return "", nil
		},
		now: func() time.Time { return f.clock },
		sleep: func(d time.Duration) {
			f.sleeps++
			f.clock = f.clock.Add(d)
		},
	}
}

func TestRegisterSetsTheSameLinkDNSAsARootSingBox(t *testing.T) {
	fake := &fakeSystem{active: true, upAfter: 3}

	if err := register(fake.system(), "singbox", []string{"172.19.0.2"}); err != nil {
		t.Fatal(err)
	}

	want := []string{"dns singbox 172.19.0.2", "domain singbox ~.", "default-route singbox yes"}
	if !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("resolvectl calls = %q, want %q", fake.calls, want)
	}
	if fake.sleeps != 3 {
		t.Fatalf("sleeps = %d, want to poll until the link is up", fake.sleeps)
	}
}

func TestRegisterRetriesUntilResolvedKnowsTheLink(t *testing.T) {
	fake := &fakeSystem{active: true, failDNS: 2}

	if err := register(fake.system(), "singbox", []string{"172.19.0.2", "fdfe:dcba:9876::2"}); err != nil {
		t.Fatal(err)
	}

	if len(fake.calls) != 3 || fake.calls[0] != "dns singbox 172.19.0.2 fdfe:dcba:9876::2" {
		t.Fatalf("resolvectl calls = %q, want one successful registration after the retries", fake.calls)
	}
}

func TestRegisterSkipsHostsWithoutResolved(t *testing.T) {
	fake := &fakeSystem{active: false, upAfter: -1}

	if err := register(fake.system(), "singbox", []string{"172.19.0.2"}); err != nil {
		t.Fatalf("register without systemd-resolved = %v, want nothing to do", err)
	}
	if fake.linkChecks != 0 || len(fake.calls) != 0 {
		t.Fatal("nothing must be touched when systemd-resolved is not running")
	}
}

func TestRegisterFailsWhenTheLinkNeverComesUp(t *testing.T) {
	fake := &fakeSystem{active: true, upAfter: -1}

	err := register(fake.system(), "singbox", []string{"172.19.0.2"})

	// 失败必须让 ExecStartPost 失败：否则服务显示 active，系统解析却没有进入 TUN。
	if !errors.Is(err, errLinkDown) {
		t.Fatalf("register = %v, want the link error", err)
	}
	if want := int(linkWait/pollInterval) + 1; fake.linkChecks != want || len(fake.calls) != 0 {
		t.Fatalf("link checks = %d, calls = %q", fake.linkChecks, fake.calls)
	}
}

func TestParseArgs(t *testing.T) {
	link, servers, err := parseArgs([]string{"--dev", "singbox", "--dns", "172.19.0.2", "--dns", "fdfe:dcba:9876::2"})
	if err != nil || link != "singbox" || !reflect.DeepEqual(servers, []string{"172.19.0.2", "fdfe:dcba:9876::2"}) {
		t.Fatalf("parseArgs = %q %q %v", link, servers, err)
	}
	for _, args := range [][]string{
		{"--dev", "singbox"},
		{"--dns", "172.19.0.2"},
		{"--dev", "singbox", "--dns", "example.com"},
		{"--dev", "-x", "--dns", "172.19.0.2"},
		{"--dev", "singbox", "--dns", "172.19.0.2", "extra"},
	} {
		if _, _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%q) accepted invalid arguments", args)
		}
	}
}

func TestValidLinkName(t *testing.T) {
	for name, want := range map[string]bool{
		"singbox":          true,
		"tun0":             true,
		"sb_tun.1-a":       true,
		"":                 false,
		"exactly16chars__": false,
		"-dns":             false,
		"..":               false,
		"a b":              false,
		"a/b":              false,
		"a:b":              false,
		"a\nb":             false,
		"tun%i":            false,
	} {
		if got := ValidLinkName(name); got != want {
			t.Errorf("ValidLinkName(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestRunRejectsBadArgumentsBeforeTouchingTheHost(t *testing.T) {
	if code := Run([]string{"--dev", "singbox"}); code != 2 {
		t.Fatalf("Run without --dns = %d, want usage error 2", code)
	}
}

func TestLinkIsUp(t *testing.T) {
	if !linkIsUp("lo") {
		t.Fatal("loopback must be reported as up")
	}
	if linkIsUp("sbk-missing0") {
		t.Fatal("a missing link must not be reported as up")
	}
	if tunLinkUp("lo") {
		t.Fatal("only TUN links may be registered, not loopback")
	}
}

func TestIsTunChecksTheKernelTunFlags(t *testing.T) {
	orig := sysClassNet
	t.Cleanup(func() { sysClassNet = orig })
	sysClassNet = t.TempDir()
	for name, attr := range map[string]string{"singbox": "tun_flags", "eth0": "carrier"} {
		if err := os.MkdirAll(filepath.Join(sysClassNet, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sysClassNet, name, attr), []byte("1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if !isTun("singbox") {
		t.Fatal("a link with tun_flags is a TUN device")
	}
	if isTun("eth0") || isTun("missing0") {
		t.Fatal("links without tun_flags must not be treated as TUN")
	}
}

func TestRunningStateCountsAStartingResolved(t *testing.T) {
	for state, want := range map[string]bool{
		"active": true, "activating": true, "reloading": true,
		"inactive": false, "failed": false, "deactivating": false, "": false,
	} {
		if got := runningState(state); got != want {
			t.Errorf("runningState(%q) = %v, want %v", state, got, want)
		}
	}
}

func TestEnsureLeavesARegisteredLinkAlone(t *testing.T) {
	fake := &fakeSystem{active: true, current: map[string]string{
		"dns":           "Link 7 (singbox): 172.19.0.2\n",
		"domain":        "Link 7 (singbox): ~.\n",
		"default-route": "Link 7 (singbox): yes\n",
	}}

	repaired, err := ensure(fake.system(), "singbox", []string{"172.19.0.2"})

	if err != nil || repaired || len(fake.calls) != 0 {
		t.Fatalf("ensure = %v, %v, calls %q; want nothing to do", repaired, err, fake.calls)
	}
}

func TestEnsureReregistersALinkThatLostItsDNS(t *testing.T) {
	for name, current := range map[string]map[string]string{
		"nothing set":     {"dns": "Link 7 (singbox):", "domain": "Link 7 (singbox):", "default-route": "Link 7 (singbox): no"},
		"other server":    {"dns": "Link 7 (singbox): 192.168.5.1", "domain": "Link 7 (singbox): ~.", "default-route": "Link 7 (singbox): yes"},
		"no route domain": {"dns": "Link 7 (singbox): 172.19.0.2", "domain": "Link 7 (singbox):", "default-route": "Link 7 (singbox): yes"},
	} {
		fake := &fakeSystem{active: true, current: current}

		repaired, err := ensure(fake.system(), "singbox", []string{"172.19.0.2"})

		if err != nil || !repaired || len(fake.calls) != 3 {
			t.Errorf("%s: ensure = %v, %v, calls %q; want a re-registration", name, repaired, err, fake.calls)
		}
	}
}

func TestEnsureSkipsWhenThereIsNothingToRegister(t *testing.T) {
	for name, fake := range map[string]*fakeSystem{
		"resolved not running": {active: false},
		"link not up":          {active: true, upAfter: -1},
	} {
		if repaired, err := ensure(fake.system(), "singbox", []string{"172.19.0.2"}); err != nil || repaired || len(fake.calls) != 0 {
			t.Errorf("%s: ensure = %v, %v, calls %q", name, repaired, err, fake.calls)
		}
	}
}

func TestLinkValues(t *testing.T) {
	if got := linkValues("Link 7 (singbox): 172.19.0.2 fdfe:dcba:9876::2\n"); !reflect.DeepEqual(got, []string{"172.19.0.2", "fdfe:dcba:9876::2"}) {
		t.Fatalf("linkValues = %q", got)
	}
	if got := linkValues("Link 7 (singbox):\n"); len(got) != 0 {
		t.Fatalf("empty setting parsed as %q", got)
	}
}
