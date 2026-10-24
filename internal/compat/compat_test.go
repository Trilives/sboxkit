package compat

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
)

func TestMain(m *testing.M) {
	i18n.SetLang(i18n.ZH)
	os.Exit(m.Run())
}

// fakeKernel 写一个最小的 sing-box 替身：check 拒绝含 REJECT 的配置，
// 通过时把收到的配置与工作目录记录到 dir 下供断言。
func fakeKernel(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sing-box")
	script := `#!/bin/sh
case "$1" in
version) echo "sing-box version ` + version + `"; echo; echo "Environment: go1.25 linux/amd64" ;;
check)
  if grep -q REJECT "$3"; then
    printf '\033[31mFATAL\033[0m[0000] initialize dns router: legacy rule rejected\n' >&2
    exit 1
  fi
  cp "$3" "` + dir + `/checked.json"
  pwd > "` + dir + `/checked.cwd"
  ls > "` + dir + `/checked.ls"
  ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestCheckAcceptsConfigTheKernelAccepts(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	bin := fakeKernel(t, "1.14.3")

	if err := Check(p, []byte(`{"log":{}}`), Bundle{Kernel: bin}); err != nil {
		t.Fatalf("check: %v", err)
	}
}

func TestCheckReportsKernelVersionAndCleanOutput(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	bin := fakeKernel(t, "1.14.3")

	err := Check(p, []byte(`{"log":{"level":"REJECT"}}`), Bundle{Kernel: bin})
	var rejected *Error
	if !errors.As(err, &rejected) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if rejected.Kernel != "1.14.3" {
		t.Fatalf("kernel version = %q", rejected.Kernel)
	}
	if strings.Contains(rejected.Output, "\x1b[") || !strings.Contains(rejected.Output, "FATAL[0000] initialize dns router") {
		t.Fatalf("output should be the kernel message without ANSI codes: %q", rejected.Output)
	}
	if !strings.Contains(err.Error(), "sing-box 1.14.3") {
		t.Fatalf("message should name the kernel version: %v", err)
	}
	removed := &Error{Kernel: "1.14.3", Output: "x is deprecated in sing-box 1.14.0"}
	if !strings.Contains(removed.Error(), removed.Hint()) {
		t.Fatalf("message should carry the remedy: %v", removed)
	}
}

// compat-check 的 --kernel 等参数可以是相对路径；内核在校验目录里运行，必须先转绝对路径。
func TestCheckResolvesRelativeBundlePaths(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	bin := fakeKernel(t, "1.14.3")
	geosite := filepath.Join(filepath.Dir(bin), "geosite-cn.srs")
	if err := os.WriteFile(geosite, []byte("srs"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Dir(bin))
	bundle := Bundle{Kernel: "./sing-box", Geosite: "geosite-cn.srs"}
	config := `{"route":{"rule_set":[{"type":"local","tag":"geosite-cn","path":"` + p.GeositeCN + `"}]}}`

	if err := Check(p, []byte(config), bundle); err != nil {
		t.Fatalf("check with relative bundle paths: %v", err)
	}
	ruleSets := readChecked(t, bin)["route"].(map[string]any)["rule_set"].([]any)
	if got := ruleSets[0].(map[string]any)["path"]; got != geosite {
		t.Fatalf("geosite path = %v, want absolute %s", got, geosite)
	}
}

func TestCheckRedirectsStateRuleSetsToBundleFiles(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	bin := fakeKernel(t, "1.14.3")
	staged := t.TempDir()
	bundle := Bundle{Kernel: bin, Geosite: filepath.Join(staged, "geosite-cn.srs"), GeoIP: filepath.Join(staged, "geoip-cn.srs")}
	for _, file := range []string{bundle.Geosite, bundle.GeoIP} {
		if err := os.WriteFile(file, []byte("srs"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	config := `{"route":{"rule_set":[
		{"type":"local","tag":"geosite-cn","path":"` + p.GeositeCN + `"},
		{"type":"local","tag":"geoip-cn","path":"` + p.GeoIPCN + `"},
		{"type":"remote","tag":"other","url":"https://example.invalid/x.srs"}]}}`

	if err := Check(p, []byte(config), bundle); err != nil {
		t.Fatalf("check: %v", err)
	}
	checked := readChecked(t, bin)
	ruleSets := checked["route"].(map[string]any)["rule_set"].([]any)
	if got := ruleSets[0].(map[string]any)["path"]; got != bundle.Geosite {
		t.Fatalf("geosite path = %v, want %s", got, bundle.Geosite)
	}
	if got := ruleSets[1].(map[string]any)["path"]; got != bundle.GeoIP {
		t.Fatalf("geoip path = %v, want %s", got, bundle.GeoIP)
	}
	if got := ruleSets[2].(map[string]any)["url"]; got != "https://example.invalid/x.srs" {
		t.Fatalf("unrelated rule-set changed: %v", ruleSets[2])
	}
}

// 系统模式运行时目录仅 root 可读，而 check 会读取面板目录：面板与缓存库改指校验目录。
func TestCheckRedirectsRuntimeManagedPathsIntoPrivateDirectory(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	bin := fakeKernel(t, "1.14.3")
	config := `{"experimental":{
		"cache_file":{"enabled":true,"path":"/var/lib/sboxkit-runtime/cache.db"},
		"clash_api":{"external_controller":"127.0.0.1:9090","external_ui":"/var/lib/sboxkit-runtime/ui"}}}`

	if err := Check(p, []byte(config), Bundle{Kernel: bin}); err != nil {
		t.Fatalf("check: %v", err)
	}
	exp := readChecked(t, bin)["experimental"].(map[string]any)
	cwd, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "checked.cwd"))
	if err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSpace(string(cwd))
	if got := exp["clash_api"].(map[string]any)["external_ui"]; got != filepath.Join(dir, "ui") {
		t.Fatalf("external_ui = %v, want inside %s", got, dir)
	}
	if got := exp["cache_file"].(map[string]any)["path"]; got != filepath.Join(dir, "cache.db") {
		t.Fatalf("cache path = %v, want inside %s", got, dir)
	}
}

// 运行时以运行时目录为工作目录（其中有 geosite-cn.srs / geoip-cn.srs），
// 校验目录同样放置这两个文件，导入配置里的相对路径与运行时解析一致。
func TestCheckMirrorsRuntimeWorkingDirectoryRuleSets(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	bin := fakeKernel(t, "1.14.3")
	if err := os.MkdirAll(p.Ruleset, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{p.GeositeCN, p.GeoIPCN} {
		if err := os.WriteFile(file, []byte("srs"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	bundle := StateBundle(p)
	bundle.Kernel = bin
	if err := Check(p, []byte(`{}`), bundle); err != nil {
		t.Fatalf("check: %v", err)
	}
	listing, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "checked.ls"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"geosite-cn.srs", "geoip-cn.srs"} {
		if !strings.Contains(string(listing), want) {
			t.Fatalf("check working directory lacks %s: %s", want, listing)
		}
	}
	cwd, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "checked.cwd"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(strings.TrimSpace(string(cwd))); !os.IsNotExist(err) {
		t.Fatalf("private check directory with node credentials was not removed: %v", err)
	}
}

func TestCheckReportsMissingKernel(t *testing.T) {
	p := paths.FromRoot(t.TempDir())
	err := Check(p, []byte(`{}`), StateBundle(p))
	if !errors.Is(err, ErrKernelMissing) {
		t.Fatalf("expected ErrKernelMissing, got %v", err)
	}
}

func TestKernelVersionReadsFirstLine(t *testing.T) {
	if got := KernelVersion(fakeKernel(t, "1.14.3")); got != "1.14.3" {
		t.Fatalf("KernelVersion = %q", got)
	}
	if got := KernelVersion(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Fatalf("missing kernel version = %q", got)
	}
}

func TestAtLeastComparesReleaseVersions(t *testing.T) {
	cases := []struct {
		version, min string
		want, ok     bool
	}{
		{"1.14.3", "1.14.0", true, true},
		{"v1.14.0", "1.14.0", true, true},
		{"1.13.19", "1.14.0", false, true},
		{"1.14.0-beta.2", "1.14.0", true, true},
		{"2.0.0", "1.14.0", true, true},
		{"1.9.10", "1.10.0", false, true},
		{"", "1.14.0", false, false},
		{"bundled", "1.14.0", false, false},
	}
	for _, tc := range cases {
		got, ok := AtLeast(tc.version, tc.min)
		if got != tc.want || ok != tc.ok {
			t.Errorf("AtLeast(%q, %q) = %v, %v; want %v, %v", tc.version, tc.min, got, ok, tc.want, tc.ok)
		}
	}
}

// 输出取自真实内核（sing-box 1.13.21 / 1.14.3 校验 v0.3.1 与当前生成器的配置）。
func TestErrorHintClassifiesCommonCauses(t *testing.T) {
	cases := []struct {
		name, output, want string
	}{
		{"deprecated syntax", "Legacy Address Filter Fields in DNS rules is deprecated in sing-box 1.14.0 and will be removed in sing-box 1.16.0, checkout documentation for migration", "sboxkit"},
		{"new kernel requires match_response", "initialize dns router: validate dns rule[1]: Response Match Fields (ip_cidr, ip_is_private, ip_accept_any, response_rcode, response_answer, response_ns, response_extra) require match_response to be enabled", "sboxkit"},
		{"old kernel lacks DNS rule action", "decode config at /tmp/sboxkit-check-1/config.json: dns.rules[1]: unknown DNS rule action: evaluate", "内核"},
		{"old kernel lacks route rule action", "decode config at /tmp/x.json: route.rules[7]: unknown rule action: resolve", "内核"},
		{"old kernel lacks field", `decode config at /tmp/x.json: dns.rules[0]: json: unknown field "match_response"`, "内核"},
		{"unrelated failure", "listen tcp: address already in use", ""},
	}
	for _, c := range cases {
		hint := (&Error{Output: c.output}).Hint()
		if c.want == "" && hint != "" || c.want != "" && !strings.Contains(hint, c.want) {
			t.Errorf("%s: hint %q, want it to mention %q", c.name, hint, c.want)
		}
	}
}

func readChecked(t *testing.T, bin string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(filepath.Dir(bin), "checked.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}
