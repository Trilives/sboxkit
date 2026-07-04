package paths

import (
	"strings"
	"testing"
)

func TestSboxkitHomeWins(t *testing.T) {
	t.Setenv("SBOXKIT_HOME", "/tmp/cdhome")
	if got := Detect().State; got != "/tmp/cdhome" {
		t.Fatalf("State = %s, SBOXKIT_HOME 应最优先", got)
	}
}

func TestFixedDefault(t *testing.T) {
	t.Setenv("SBOXKIT_HOME", "")
	if got := Detect().State; got != DefaultStateRoot {
		t.Fatalf("State = %s, 默认应为固定目录 %s", got, DefaultStateRoot)
	}
}

func TestDerivedPaths(t *testing.T) {
	t.Setenv("SBOXKIT_HOME", "/tmp/cd")
	p := Detect()
	checks := map[string]string{
		p.ConfigFile:    "/tmp/cd/config.json",
		p.CustomizeFile: "/tmp/cd/customize.json",
		p.ActiveFile:    "/tmp/cd/active",
		p.SingBoxBin:    "/tmp/cd/bin/sing-box",
		p.GeositeCN:     "/tmp/cd/ruleset/geosite-cn.srs",
	}
	for got, want := range checks {
		if got != want {
			t.Errorf("路径 = %s, 期望 %s", got, want)
		}
	}
	if !strings.HasPrefix(p.SubscriptionDir("Hua"), "/tmp/cd/subscriptions") {
		t.Error("SubscriptionDir 应位于 subscriptions/ 下")
	}
}
