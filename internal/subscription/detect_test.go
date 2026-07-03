package subscription

import "testing"

func TestDetectClashYAML(t *testing.T) {
	kind, err := Detect([]byte("proxies:\n  - name: hk-01\n    type: vmess\n"))
	if err != nil {
		t.Fatalf("detect clash: %v", err)
	}
	if kind != SourceClash {
		t.Fatalf("expected clash, got %s", kind)
	}
}

func TestDetectSingBoxJSON(t *testing.T) {
	kind, err := Detect([]byte(`{"outbounds":[{"type":"direct","tag":"direct"}]}`))
	if err != nil {
		t.Fatalf("detect sing-box: %v", err)
	}
	if kind != SourceSingBox {
		t.Fatalf("expected sing-box, got %s", kind)
	}
}

func TestDetectBase64(t *testing.T) {
	kind, err := Detect([]byte("dm1lc3M6Ly9leGFtcGxl"))
	if err != nil {
		t.Fatalf("detect base64: %v", err)
	}
	if kind != SourceBase64 {
		t.Fatalf("expected base64, got %s", kind)
	}
}

func TestDetectRejectsEmptyContent(t *testing.T) {
	if _, err := Detect([]byte(" \n\t")); err == nil {
		t.Fatal("expected error for empty content")
	}
}
