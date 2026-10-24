package flows

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Trilives/sboxkit/internal/compat"
	"github.com/Trilives/sboxkit/internal/kernel"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/selfupdate"
)

// kernelScript sing-box 替身：check 拒绝含任一 marker 的配置。
func kernelScript(version string, markers ...string) string {
	script := "#!/bin/sh\ncase \"$1\" in\nversion) echo \"sing-box version " + version + "\" ;;\ncheck)\n"
	for _, m := range markers {
		script += "  if grep -q '" + m + "' \"$3\"; then echo \"FATAL[0000] " + m + " is deprecated in sing-box " + version + "\" >&2; exit 1; fi\n"
	}
	return script + "  ;;\nesac\n"
}

type upgradeFixture struct {
	p      paths.Paths
	staged *kernel.Staged
}

// newUpgradeFixture 隔离的用户模式状态（服务未安装）：state 内核 1.14.3 全部接受，
// 生效订阅 Hua 原文含节点 good-node，生效配置为 deployed；暂存内核 1.15.0 拒绝 markers。
func newUpgradeFixture(t *testing.T, deployed string, markers ...string) upgradeFixture {
	t.Helper()
	root := t.TempDir()
	t.Setenv("SBOXKIT_USER_MODE", "1")
	t.Setenv("SBOXKIT_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	p := paths.Detect()
	if err := p.EnsureStateDirs(); err != nil {
		t.Fatal(err)
	}
	raw := "proxies:\n  - {name: good-node, type: ss, server: 1.2.3.4, port: 443, cipher: aes-128-gcm, password: pw}\n"
	meta, _ := json.Marshal(map[string]any{"name": "Hua", "url": "/unused", "source_type": "local"})
	dir := p.SubscriptionDir("Hua")
	writeAll(t, map[string]string{
		p.SingBoxBin:                      kernelScript("1.14.3"),
		p.SingBoxVersion:                  "1.14.3\n",
		filepath.Join(dir, "meta.json"):   string(meta),
		filepath.Join(dir, "raw.yaml"):    raw,
		filepath.Join(dir, "config.json"): deployed,
		p.ConfigFile:                      deployed,
		p.ActiveFile:                      "Hua\n",
	})
	if err := os.Chmod(p.SingBoxBin, 0o755); err != nil {
		t.Fatal(err)
	}
	staged, err := kernel.NewStaging(p)
	if err != nil {
		t.Fatal(err)
	}
	staged.Kernel, staged.Version = filepath.Join(staged.Dir(), "sing-box"), "1.15.0"
	writeAll(t, map[string]string{staged.Kernel: kernelScript("1.15.0", markers...)})
	if err := os.Chmod(staged.Kernel, 0o755); err != nil {
		t.Fatal(err)
	}
	return upgradeFixture{p: p, staged: staged}
}

func writeAll(t *testing.T, files map[string]string) {
	t.Helper()
	for path, content := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// stubConfirm 依次给出 answers，并记录问过的问题。
func stubConfirm(t *testing.T, answers ...bool) *[]string {
	t.Helper()
	var asked []string
	orig := confirmUpgrade
	confirmUpgrade = func(prompt string, _ bool) (bool, error) {
		asked = append(asked, prompt)
		if len(answers) == 0 {
			t.Fatalf("unexpected question: %s", prompt)
		}
		answer := answers[0]
		answers = answers[1:]
		return answer, nil
	}
	t.Cleanup(func() { confirmUpgrade = orig })
	return &asked
}

func (f upgradeFixture) assertKernel(t *testing.T, version string) {
	t.Helper()
	if got := stateKernelVersion(f.p); got != version {
		t.Fatalf("state kernel version = %q, want %q", got, version)
	}
	if out := compat.KernelVersion(f.p.SingBoxBin); out != version {
		t.Fatalf("state kernel binary reports %q, want %q", out, version)
	}
}

func (f upgradeFixture) activeConfig(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(f.p.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func assertRefused(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "已放弃本次更新") {
		t.Fatalf("expected the update to be refused, got %v", err)
	}
}

func TestUnattendedAssetUpdateRefusedWhenDeployedConfigIsRejected(t *testing.T) {
	f := newUpgradeFixture(t, `{"legacy-marker":true}`, "legacy-marker")

	err := runUpgrade(f.p, &upgrade{assets: f.staged}, upgradeMode{interactive: false, sync: true})

	assertRefused(t, err)
	f.assertKernel(t, "1.14.3")
	if got := f.activeConfig(t); got != `{"legacy-marker":true}` {
		t.Fatalf("active config changed: %q", got)
	}
	if _, err := os.Stat(f.staged.Dir()); !os.IsNotExist(err) {
		t.Fatalf("staged kernel left behind: %v", err)
	}
}

func TestAssetUpdateRegeneratesActiveConfigWhenUserAgrees(t *testing.T) {
	f := newUpgradeFixture(t, `{"legacy-marker":true}`, "legacy-marker")
	asked := stubConfirm(t, true)

	if err := runUpgrade(f.p, &upgrade{assets: f.staged}, upgradeMode{interactive: true, sync: true}); err != nil {
		t.Fatal(err)
	}

	f.assertKernel(t, "1.15.0")
	active := f.activeConfig(t)
	if !strings.Contains(active, "good-node") || strings.Contains(active, "legacy-marker") {
		t.Fatalf("active config not regenerated: %s", active)
	}
	stored, _ := os.ReadFile(filepath.Join(f.p.SubscriptionDir("Hua"), "config.json"))
	if string(stored) != active {
		t.Fatal("subscription config and active config diverged")
	}
	if len(*asked) != 1 || !strings.Contains((*asked)[0], "重新生成") {
		t.Fatalf("questions = %v", *asked)
	}
}

func TestAssetUpdateRefusedWhenUserDeclinesRegeneration(t *testing.T) {
	f := newUpgradeFixture(t, `{"legacy-marker":true}`, "legacy-marker")
	stubConfirm(t, false)

	err := runUpgrade(f.p, &upgrade{assets: f.staged}, upgradeMode{interactive: true, sync: true})

	assertRefused(t, err)
	f.assertKernel(t, "1.14.3")
	if got := f.activeConfig(t); got != `{"legacy-marker":true}` {
		t.Fatalf("active config changed: %q", got)
	}
}

// 新内核拒绝当前生成器的输出：询问是否同时更新 sboxkit；没有更新的版本时放弃。
func TestAssetUpdateOffersSboxkitWhenGeneratorIsIncompatible(t *testing.T) {
	f := newUpgradeFixture(t, `{"deployed":true}`, "good-node")
	asked := stubConfirm(t, true)
	orig := prepareTool
	prepareTool = func(paths.Paths) (*selfupdate.Prepared, bool, error) { return nil, true, nil }
	t.Cleanup(func() { prepareTool = orig })

	err := runUpgrade(f.p, &upgrade{assets: f.staged}, upgradeMode{interactive: true, sync: true})

	assertRefused(t, err)
	f.assertKernel(t, "1.14.3")
	if len(*asked) != 1 || !strings.Contains((*asked)[0], "sboxkit") {
		t.Fatalf("questions = %v", *asked)
	}
}

// geoOnlyStaging 只暂存两份规则集（不含内核）。
func geoOnlyStaging(t *testing.T, p paths.Paths) *kernel.Staged {
	t.Helper()
	geo, err := kernel.NewStaging(p)
	if err != nil {
		t.Fatal(err)
	}
	geo.Geosite, geo.GeoIP = filepath.Join(geo.Dir(), "geosite-cn.srs"), filepath.Join(geo.Dir(), "geoip-cn.srs")
	writeAll(t, map[string]string{geo.Geosite: "srs", geo.GeoIP: "srs"})
	return geo
}

// 只更新规则集，但当前生成器要求更新的内核：补充的应是内核而不是 sboxkit。
func TestGeoUpdateOffersKernelWhenGeneratorNeedsNewerKernel(t *testing.T) {
	f := newUpgradeFixture(t, `{"deployed":true}`)
	writeAll(t, map[string]string{f.p.SingBoxBin: kernelScript("1.13.9"), f.p.SingBoxVersion: "1.13.9\n"})
	geo := geoOnlyStaging(t, f.p)
	asked := stubConfirm(t, true)
	origStage, origTool := stageCore, prepareTool
	stageCore = func(_ paths.Paths, s *kernel.Staged) error {
		s.Kernel, s.Version = f.staged.Kernel, f.staged.Version
		return nil
	}
	prepareTool = func(paths.Paths) (*selfupdate.Prepared, bool, error) {
		t.Fatal("offered a sboxkit update for a kernel that is too old")
		return nil, false, nil
	}
	t.Cleanup(func() { stageCore, prepareTool = origStage, origTool })

	if err := runUpgrade(f.p, &upgrade{assets: geo}, upgradeMode{interactive: true, sync: true}); err != nil {
		t.Fatal(err)
	}

	f.assertKernel(t, "1.15.0")
	if len(*asked) != 1 || !strings.Contains((*asked)[0], "内核") {
		t.Fatalf("questions = %v", *asked)
	}
}

// 补充的内核下载失败（升级不可用）：放弃本次更新并说明原组合不变，同时保留失败原因。
func TestUpdateRefusedWhenComplementCannotBeFetched(t *testing.T) {
	f := newUpgradeFixture(t, `{"deployed":true}`)
	writeAll(t, map[string]string{f.p.SingBoxBin: kernelScript("1.13.9"), f.p.SingBoxVersion: "1.13.9\n"})
	stubConfirm(t, true)
	orig := stageCore
	stageCore = func(paths.Paths, *kernel.Staged) error { return errors.New("HTTP 401") }
	t.Cleanup(func() { stageCore = orig })

	err := runUpgrade(f.p, &upgrade{assets: geoOnlyStaging(t, f.p)}, upgradeMode{interactive: true, sync: true})

	assertRefused(t, err)
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("download failure hidden: %v", err)
	}
	f.assertKernel(t, "1.13.9")
}

func TestCompatibleAssetUpdateIsApplied(t *testing.T) {
	f := newUpgradeFixture(t, `{"deployed":true}`)

	if err := runUpgrade(f.p, &upgrade{assets: f.staged}, upgradeMode{interactive: false, sync: true}); err != nil {
		t.Fatal(err)
	}

	f.assertKernel(t, "1.15.0")
	if got := f.activeConfig(t); got != `{"deployed":true}` {
		t.Fatalf("compatible update must not rewrite the active config: %q", got)
	}
}

// stableTool 记录一个可回退的稳定版并以它为升级目标（只切换 current，不迁移可执行文件）。
func stableTool(t *testing.T, p paths.Paths) (*selfupdate.Prepared, string) {
	t.Helper()
	versions := filepath.Join(p.State, "sboxkit-versions")
	running := filepath.Join(versions, "0.4.0", "sboxkit")
	stable := filepath.Join(versions, "0.3.2", "sboxkit")
	writeAll(t, map[string]string{running: "#!/bin/sh\necho sboxkit 0.4.0\n", stable: "#!/bin/sh\necho sboxkit 0.3.2\n"})
	for _, bin := range []string{running, stable} {
		if err := os.Chmod(bin, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(running, filepath.Join(versions, "current")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stable, filepath.Join(versions, "last-stable")); err != nil {
		t.Fatal(err)
	}
	tool, err := selfupdate.PrepareStable(p)
	if err != nil {
		t.Fatal(err)
	}
	return tool, running
}

func stubToolCheck(t *testing.T, report compat.Report, cfg []byte, err error) {
	t.Helper()
	orig := toolCompatCheck
	toolCompatCheck = func(*selfupdate.Prepared, paths.Paths, compat.Bundle) (compat.Report, []byte, error) {
		return report, cfg, err
	}
	t.Cleanup(func() { toolCompatCheck = orig })
}

func currentTarget(t *testing.T, p paths.Paths) string {
	t.Helper()
	target, err := os.Readlink(filepath.Join(p.State, "sboxkit-versions", "current"))
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func TestSboxkitUpdateRefusedWhenGeneratorNeedsKernelUserDeclines(t *testing.T) {
	f := newUpgradeFixture(t, `{"deployed":true}`)
	tool, running := stableTool(t, f.p)
	stubToolCheck(t, compat.Report{Generator: "0.3.2", MinKernel: "1.15.0", Kernel: "1.14.3", Subscription: "Hua",
		Error: "unknown rule action: evaluate"}, nil, nil)
	asked := stubConfirm(t, false)

	err := runUpgrade(f.p, &upgrade{tool: tool}, upgradeMode{interactive: true, sync: true})

	assertRefused(t, err)
	if got := currentTarget(t, f.p); got != running {
		t.Fatalf("current switched to %s despite the refusal", got)
	}
	if len(*asked) != 1 || !strings.Contains((*asked)[0], "内核") {
		t.Fatalf("questions = %v", *asked)
	}
}

func TestUnverifiableSboxkitUpdateNeedsExplicitConsent(t *testing.T) {
	f := newUpgradeFixture(t, `{"deployed":true}`)
	tool, running := stableTool(t, f.p)
	stubToolCheck(t, compat.Report{}, nil, selfupdate.ErrNoCompatCheck)
	stubConfirm(t, false)

	assertRefused(t, runUpgrade(f.p, &upgrade{tool: tool}, upgradeMode{interactive: true, sync: true}))
	if got := currentTarget(t, f.p); got != running {
		t.Fatalf("current switched without consent: %s", got)
	}

	stubConfirm(t, true)
	if err := runUpgrade(f.p, &upgrade{tool: tool}, upgradeMode{interactive: true, sync: true}); err != nil {
		t.Fatal(err)
	}
	if got := currentTarget(t, f.p); got != tool.Bin {
		t.Fatalf("current = %s, want %s", got, tool.Bin)
	}
}

func TestCheckGeneratorStatesMinimumKernel(t *testing.T) {
	f := newUpgradeFixture(t, `{"deployed":true}`)
	writeAll(t, map[string]string{f.p.SingBoxBin: kernelScript("1.13.9")})

	report, cfg := checkGenerator(f.p, compat.StateBundle(f.p))

	if report.Compatible || cfg != nil || !strings.Contains(report.Explain(), "1.14.0") || !strings.Contains(report.Explain(), "1.13.9") {
		t.Fatalf("report should name the required and the actual kernel: %+v", report)
	}
}

func TestRunCompatCheckWritesReportAndRegeneratedConfig(t *testing.T) {
	f := newUpgradeFixture(t, `{"deployed":true}`)
	dir := t.TempDir()
	reportPath, outPath := filepath.Join(dir, "report.json"), filepath.Join(dir, "config.json")
	var stderr bytes.Buffer

	code := RunCompatCheck(f.p, []string{"--kernel", f.staged.Kernel, "--geosite", f.p.GeositeCN, "--geoip", f.p.GeoIPCN,
		"--report", reportPath, "--out", outPath}, &stderr)

	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	report, err := compat.ReadReport(reportPath)
	if err != nil || !report.Compatible || report.Subscription != "Hua" || report.Kernel != "1.15.0" {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if cfg, err := os.ReadFile(outPath); err != nil || !strings.Contains(string(cfg), "good-node") {
		t.Fatalf("regenerated config = %q, %v", cfg, err)
	}
	if st, err := os.Stat(outPath); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("regenerated config with node credentials must be private: %v %v", st, err)
	}
}

func TestRunCompatCheckRejectsMissingReportPath(t *testing.T) {
	var stderr bytes.Buffer
	if code := RunCompatCheck(paths.FromRoot(t.TempDir()), nil, &stderr); code != 2 {
		t.Fatalf("exit %d, want usage error", code)
	}
	if stderr.Len() == 0 {
		t.Fatal("usage should be printed")
	}
}
