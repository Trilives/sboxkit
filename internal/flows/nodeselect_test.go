package flows

import (
	"errors"
	"testing"
)

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

func TestSelectorCurrentSkipsSubgroups(t *testing.T) {
	group := map[string]any{
		"outbounds": []string{"Auto", "sg-02"},
	}
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "Auto", "type": "urltest"},
		},
	}
	if got := selectorCurrent(group, cfg); got != "sg-02" {
		t.Fatalf("selectorCurrent = %q, want sg-02", got)
	}
}

func TestCurrentNodePrefersRuntimeAndFallsBackOnFailure(t *testing.T) {
	group := map[string]any{
		"tag":       "Proxy",
		"outbounds": []string{"sg-config", "sg-other"},
	}
	cfg := map[string]any{"outbounds": []any{}}

	runtime := func(group string) (string, error) {
		if group != "Proxy" {
			t.Fatalf("group = %q, want Proxy", group)
		}
		return "sg-runtime", nil
	}
	if got := currentNode(group, cfg, runtime); got != "sg-runtime" {
		t.Fatalf("currentNode runtime = %q, want sg-runtime", got)
	}

	failed := func(string) (string, error) { return "", errors.New("unreachable") }
	if got := currentNode(group, cfg, failed); got != "sg-config" {
		t.Fatalf("currentNode fallback = %q, want sg-config", got)
	}
}
