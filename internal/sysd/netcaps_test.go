package sysd

import (
	"encoding/binary"
	"os"
	"testing"
)

func vfsCapV2(effective bool, permitted uint64) []byte {
	raw := make([]byte, 20)
	magic := uint32(vfsCapRevision2)
	if effective {
		magic |= vfsCapFlagEffective
	}
	binary.LittleEndian.PutUint32(raw[0:4], magic)
	binary.LittleEndian.PutUint32(raw[4:8], uint32(permitted))
	binary.LittleEndian.PutUint32(raw[12:16], uint32(permitted>>32))
	return raw
}

func TestConfigNeedsNetAdminDetectsTunAndMarks(t *testing.T) {
	cases := map[string]struct {
		doc  map[string]any
		want bool
	}{
		"mixed only": {map[string]any{"inbounds": []any{map[string]any{"type": "mixed"}}}, false},
		"tun":        {map[string]any{"inbounds": []any{map[string]any{"type": "tun"}}}, true},
		"routing_mark": {map[string]any{"outbounds": []any{
			map[string]any{"type": "vless", "routing_mark": 255},
		}}, true},
		"default_mark": {map[string]any{"route": map[string]any{"default_mark": 1}}, true},
	}
	for name, tc := range cases {
		if got := configNeedsNetAdmin(tc.doc); got != tc.want {
			t.Errorf("%s: configNeedsNetAdmin = %v, want %v", name, got, tc.want)
		}
	}
}

func TestCapsSatisfiedRequiresEffectiveNetCapabilities(t *testing.T) {
	all := uint64(1<<capNetAdmin | 1<<capNetRaw | 1<<capNetBindService)

	if !capsSatisfied(vfsCapV2(true, all)) {
		t.Fatal("full effective net capability set rejected")
	}
	if capsSatisfied(vfsCapV2(false, all)) {
		t.Fatal("capabilities without the effective flag are useless for a non-capability-aware exec")
	}
	if capsSatisfied(vfsCapV2(true, 1<<capNetAdmin)) {
		t.Fatal("partial capability set accepted")
	}
	if capsSatisfied([]byte{1, 2, 3}) {
		t.Fatal("truncated xattr accepted")
	}
}

func TestEnsureNetCapabilitiesIsNoopWithoutTunOrInSystemScope(t *testing.T) {
	s := userModeScope(t)
	rt := rtPathsFor(s, DefaultName)
	if err := os.MkdirAll(rt.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rt.Config, []byte(`{"inbounds":[{"type":"mixed"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ensureNetCapabilities(rt); err != nil {
		t.Fatalf("user mode without TUN must not ask for privileges: %v", err)
	}
	if err := ensureNetCapabilities(runtimePaths{Config: "/nonexistent"}); err != nil {
		t.Fatalf("system scope relies on unit capabilities: %v", err)
	}
}
