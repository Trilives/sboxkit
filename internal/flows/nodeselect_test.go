package flows

import "testing"

func TestNodeLabelsMarkCurrentSelection(t *testing.T) {
	labels := nodeMenuLabels([]string{"sg-01", "sg-02"}, map[string]int{"sg-01": 30}, "sg-02", true)
	if labels[0] != "sg-01   30ms" {
		t.Fatalf("unexpected first label %q", labels[0])
	}
	if labels[1] != "sg-02   当前" {
		t.Fatalf("current node label missing marker: %#v", labels)
	}
}

func TestSelectorCurrentPrefersFirstRealNode(t *testing.T) {
	group := map[string]any{
		"outbounds": []string{"DIRECT", "Traffic: info", "sg-02", "sg-01"},
	}
	if got := selectorCurrent(group); got != "sg-02" {
		t.Fatalf("selectorCurrent = %q, want sg-02", got)
	}
}
