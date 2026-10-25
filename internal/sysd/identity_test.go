package sysd

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/paths"
)

func stubSystemd(t *testing.T, version int) {
	t.Helper()
	orig := systemdVersion
	systemdVersion = func() int { return version }
	t.Cleanup(func() { systemdVersion = orig })
}

func decode(t *testing.T, raw string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestChooseIdentityRunsSystemServiceUnprivilegedByDefault(t *testing.T) {
	stubSystemd(t, 255)
	p := paths.FromRoot(t.TempDir())
	plain := decode(t, `{"route":{"rules":[{"domain_suffix":["cn"],"outbound":"DIRECT"}]}}`)

	if id, reason := chooseIdentity(p, unitScope{}, plain); id != identityDynamic || reason != "" {
		t.Fatalf("chooseIdentity = %v (%q), want the dynamic user", id, reason)
	}
	if id, _ := chooseIdentity(p, unitScope{User: true}, plain); id != identityRoot {
		t.Fatal("user units keep their own user and file capabilities")
	}
}

func TestChooseIdentityKeepsRootWhenItIsNeeded(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	process := decode(t, `{"route":{"rules":[{"type":"logical","mode":"or","rules":[{"process_name":["tailscaled"]}],"outbound":"DIRECT"}]}}`)
	plain := decode(t, `{}`)

	stubSystemd(t, 255)
	if id, reason := chooseIdentity(p, unitScope{}, process); id != identityRoot || reason == "" {
		t.Fatalf("process rules: %v (%q), want root with an explanation", id, reason)
	}
	stubSystemd(t, 245)
	if id, reason := chooseIdentity(p, unitScope{}, plain); id != identityRoot || reason == "" {
		t.Fatalf("systemd 245: %v (%q), want root: LoadCredential needs 247", id, reason)
	}
	stubSystemd(t, 255)
	if err := os.MkdirAll(p.State, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fallbackMarker(p), []byte(Version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if id, _ := chooseIdentity(p, unitScope{}, plain); id != identityRoot {
		t.Fatal("after a failed switch this version must stay on root")
	}
	if err := os.WriteFile(fallbackMarker(p), []byte("0.0.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if id, _ := chooseIdentity(p, unitScope{}, plain); id != identityDynamic {
		t.Fatal("a fallback recorded by an older version must be retried")
	}
}

func TestUsesProcessRulesFindsRulesAnywhere(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"dns":{"rules":[{"process_path":["/usr/bin/x"]}]}}`:                                    true,
		`{"route":{"rule_set":[{"type":"inline","rules":[{"process_path_regex":["^/opt/"]}]}]}}`: true,
		`{"route":{"rules":[{"user_id":[1000],"outbound":"DIRECT"}]},"outbounds":[{"tag":"p"}]}`: false,
		`{"route":{"rules":[{"process_name":[],"outbound":"DIRECT"}]}}`:                          false,
		`{"route":{"rules":[{"process_name":"curl","outbound":"DIRECT"}]}}`:                      true,
	} {
		if got := usesProcessRules(decode(t, raw)); got != want {
			t.Errorf("usesProcessRules(%s) = %v, want %v", raw, got, want)
		}
	}
}

func TestDynamicUnitReadsConfigThroughCredentialsAndWritesOnlyItsCache(t *testing.T) {
	rt := rtPathsFor(unitScope{UnitDir: systemUnitDir, RuntimeDir: paths.RuntimeDir}, DefaultName)
	rt.Identity = identityDynamic

	unit, err := renderUnit(DefaultName, rt)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"\nDynamicUser=yes\n",
		"\nLoadCredential=config.json:/var/lib/sboxkit-runtime/sing-box.json\n",
		"\nExecStart=/var/lib/sboxkit-runtime/sing-box run -c ${CREDENTIALS_DIRECTORY}/config.json\n",
		"\nProtectSystem=strict\n",
		"\nStateDirectory=sboxkit-cache\n",
		"\nAmbientCapabilities=CAP_NET_ADMIN CAP_NET_BIND_SERVICE CAP_NET_RAW\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("dynamic unit missing %q:\n%s", strings.TrimSpace(want), unit)
		}
	}
	if strings.Contains(unit, "User=root") {
		t.Fatalf("dynamic unit still runs as root:\n%s", unit)
	}
}

func TestRootUnitKeepsSameCacheDirectory(t *testing.T) {
	rt := rtPathsFor(unitScope{UnitDir: systemUnitDir, RuntimeDir: paths.RuntimeDir}, DefaultName)

	unit, err := renderUnit(DefaultName, rt)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"\nUser=root\n", "\nStateDirectory=sboxkit-cache\n", "run -c /var/lib/sboxkit-runtime/sing-box.json\n"} {
		if !strings.Contains(unit, want) {
			t.Errorf("root unit missing %q:\n%s", strings.TrimSpace(want), unit)
		}
	}
	if rt.CacheDB != "/var/lib/sboxkit-cache/cache.db" {
		t.Fatalf("system cache = %s, want the StateDirectory used by both identities", rt.CacheDB)
	}
	if strings.Contains(unit, "DynamicUser") {
		t.Fatal("root unit must not enable DynamicUser")
	}
}

func TestDropLogOutputRemovesOnlyTheFileTarget(t *testing.T) {
	doc := decode(t, `{"log":{"level":"info","output":"/etc/cron.d/x"}}`)

	if !dropLogOutput(doc) {
		t.Fatal("log.output not reported as dropped")
	}
	log := doc["log"].(map[string]any)
	if _, ok := log["output"]; ok || log["level"] != "info" {
		t.Fatalf("log = %v, want output removed and level kept", log)
	}
	if dropLogOutput(decode(t, `{"log":{"level":"warn"}}`)) {
		t.Fatal("nothing to drop reported as a change")
	}
}

func stubIdentityChecks(t *testing.T, healthy ...bool) *[]serviceIdentity {
	t.Helper()
	origHealthy, origRewrite, origDelay := unitHealthy, rewriteUnit, settleDelay
	t.Cleanup(func() { unitHealthy, rewriteUnit, settleDelay = origHealthy, origRewrite, origDelay })
	settleDelay = 0
	unitHealthy = func(unitScope, string) bool {
		next := healthy[0]
		healthy = healthy[1:]
		return next
	}
	var rewrites []serviceIdentity
	rewriteUnit = func(_ string, rt runtimePaths) error { rewrites = append(rewrites, rt.Identity); return nil }
	return &rewrites
}

func dynamicRuntime(t *testing.T) (paths.Paths, runtimePaths) {
	t.Helper()
	p := paths.FromRoot(t.TempDir())
	if err := os.MkdirAll(p.State, 0o755); err != nil {
		t.Fatal(err)
	}
	rt := rtPathsFor(unitScope{}, DefaultName)
	rt.Identity = identityDynamic
	return p, rt
}

func TestVerifyIdentitySwitchKeepsAHealthyUnprivilegedService(t *testing.T) {
	p, rt := dynamicRuntime(t)
	rewrites := stubIdentityChecks(t, true)

	if err := verifyIdentitySwitch(p, DefaultName, rt, nil); err != nil || len(*rewrites) != 0 {
		t.Fatalf("healthy switch: err=%v rewrites=%v, want nothing to do", err, *rewrites)
	}
}

func TestVerifyIdentitySwitchFallsBackToRootAndRemembersIt(t *testing.T) {
	p, rt := dynamicRuntime(t)
	rewrites := stubIdentityChecks(t, false, true)

	if err := verifyIdentitySwitch(p, DefaultName, rt, nil); err != nil {
		t.Fatal(err)
	}

	if len(*rewrites) != 1 || (*rewrites)[0] != identityRoot {
		t.Fatalf("rewrites = %v, want one restart as root", *rewrites)
	}
	if !fellBack(p) {
		t.Fatal("fallback not recorded: every later sync would retry the failing identity")
	}
}

func TestVerifyIdentitySwitchTreatsAFailedStartJobAsUnhealthy(t *testing.T) {
	p, rt := dynamicRuntime(t)
	rewrites := stubIdentityChecks(t, true)

	// ExecStartPost（代为登记 TUN DNS）失败时 systemctl restart 本身报错，服务不会进入 active。
	if err := verifyIdentitySwitch(p, DefaultName, rt, errors.New("Job for sing-box.service failed")); err != nil {
		t.Fatal(err)
	}

	if len(*rewrites) != 1 || (*rewrites)[0] != identityRoot {
		t.Fatalf("rewrites = %v, want one restart as root", *rewrites)
	}
	if !fellBack(p) {
		t.Fatal("fallback not recorded after a failed start job")
	}
}

func TestVerifyIdentitySwitchKeepsSandboxWhenRootFailsToo(t *testing.T) {
	p, rt := dynamicRuntime(t)
	rewrites := stubIdentityChecks(t, false, false)

	// 两种身份都起不来：报错交给调用方回退到切换前的配置，而不是报告同步成功。
	if err := verifyIdentitySwitch(p, DefaultName, rt, nil); err == nil {
		t.Fatal("a service that fails under both identities was reported as synced")
	}

	if got := *rewrites; len(got) != 2 || got[0] != identityRoot || got[1] != identityDynamic {
		t.Fatalf("rewrites = %v, want root then back to the dynamic user", got)
	}
	if fellBack(p) {
		t.Fatal("a failure unrelated to the identity must not disable the sandbox for this version")
	}
}

func stubUnitProps(t *testing.T, snapshots ...map[string]string) {
	t.Helper()
	origProps, origDelay := unitProps, settleDelay
	t.Cleanup(func() { unitProps, settleDelay = origProps, origDelay })
	settleDelay = 0
	unitProps = func(unitScope, string) (map[string]string, error) {
		next := snapshots[0]
		snapshots = snapshots[1:]
		return next, nil
	}
}

func TestUnitSettlesComparesRestartsSinceOurOwnRestart(t *testing.T) {
	cases := []struct {
		name          string
		before, after map[string]string
		want          bool
	}{
		// systemctl restart 不清零之前失败累积的 NRestarts：只看等待期间有没有新增。
		{"earlier failures do not count", map[string]string{"ActiveState": "active", "NRestarts": "17"},
			map[string]string{"ActiveState": "active", "NRestarts": "17"}, true},
		{"restarted while settling", map[string]string{"ActiveState": "active", "NRestarts": "0"},
			map[string]string{"ActiveState": "active", "NRestarts": "1"}, false},
		{"waiting to restart", map[string]string{"ActiveState": "active", "NRestarts": "2"},
			map[string]string{"ActiveState": "activating", "NRestarts": "2"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stubUnitProps(t, tc.before, tc.after)
			if got := unitSettles(unitScope{}, DefaultName); got != tc.want {
				t.Fatalf("unitSettles = %v, want %v", got, tc.want)
			}
		})
	}
}
