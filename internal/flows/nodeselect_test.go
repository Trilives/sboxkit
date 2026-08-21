package flows

import (
	"errors"
	"slices"
	"testing"

	"github.com/Trilives/sboxkit/internal/config"
	"github.com/Trilives/sboxkit/internal/i18n"
	"github.com/Trilives/sboxkit/internal/paths"
	"github.com/Trilives/sboxkit/internal/tui"
)

func TestPickGroupRequiresConfiguredMainSelector(t *testing.T) {
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "Small", "type": "selector", "outbounds": []string{"sg-01"}},
			map[string]any{"tag": "Largest", "type": "selector", "outbounds": []string{"sg-01", "sg-02"}},
		},
	}

	group, err := pickGroup(cfg, "", "Proxy", nil)
	if err != nil {
		t.Fatalf("pickGroup returned an error: %v", err)
	}
	if group != nil {
		t.Fatalf("pickGroup guessed %q instead of rejecting an unrecognized main selector", group["tag"])
	}
}

func TestPickGroupUsesConfiguredMainSelector(t *testing.T) {
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "Proxy", "type": "selector", "outbounds": []string{"sg-01"}},
			map[string]any{"tag": "Largest", "type": "selector", "outbounds": []string{"sg-01", "sg-02"}},
		},
	}

	group, err := pickGroup(cfg, "", "Proxy", nil)
	if err != nil {
		t.Fatalf("pickGroup returned an error: %v", err)
	}
	if group["tag"] != "Proxy" {
		t.Fatalf("pickGroup selected %q, want Proxy", group["tag"])
	}
}

func TestPickGroupValidatesSelectorAndForcedGroup(t *testing.T) {
	if _, err := pickGroup(map[string]any{}, "", "Proxy", nil); err == nil {
		t.Fatal("expected a config without selectors to be rejected")
	}

	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "Manual", "type": "selector"},
		},
	}
	group, err := pickGroup(cfg, "Manual", "Proxy", nil)
	if err != nil || group["tag"] != "Manual" {
		t.Fatalf("forced selector = %#v, err = %v", group, err)
	}
	if _, err := pickGroup(cfg, "Missing", "Proxy", nil); err == nil {
		t.Fatal("expected a missing forced selector to be rejected")
	}
}

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

func TestPickGroupUsesMainGroupKeywordsWhenDefaultTagMissing(t *testing.T) {
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "🚀 节点选择", "type": "selector"},
			map[string]any{"tag": "Auto", "type": "selector"},
		},
	}
	group, err := pickGroup(cfg, "", "Proxy", []string{"节点选择"})
	if err != nil {
		t.Fatalf("pickGroup returned error: %v", err)
	}
	if tag := group["tag"]; tag != "🚀 节点选择" {
		t.Fatalf("pickGroup tag = %v, want 主选择组", tag)
	}
}

func TestPickGroupReturnsNilWhenMainGroupIsNotRecognized(t *testing.T) {
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "Manual", "type": "selector"},
			map[string]any{"tag": "Auto", "type": "selector"},
		},
	}
	group, err := pickGroup(cfg, "", "Proxy", []string{"节点选择"})
	if err != nil {
		t.Fatalf("pickGroup returned error: %v", err)
	}
	if group != nil {
		t.Fatalf("pickGroup = %#v, want nil when no selector matches", group)
	}
}

func TestAddMainGroupKeywordRequiresAMatchingSelector(t *testing.T) {
	settings := config.Defaults()
	settings.MainGroupKeywords = []string{"Proxy", "Select"}
	selectors := []map[string]any{
		{"tag": "🚀 节点选择", "type": "selector"},
	}

	updated, added := addMainGroupKeyword(settings, selectors, "节点选择")
	if !added {
		t.Fatal("expected matching keyword to be added")
	}
	if !slices.Equal(updated.MainGroupKeywords, []string{"节点选择", "Proxy", "Select"}) {
		t.Fatalf("main_group_keywords = %#v", updated.MainGroupKeywords)
	}
	if !slices.Equal(settings.MainGroupKeywords, []string{"Proxy", "Select"}) {
		t.Fatalf("input settings were mutated: %#v", settings.MainGroupKeywords)
	}

	unchanged, added := addMainGroupKeyword(settings, selectors, "missing")
	if added {
		t.Fatal("non-matching keyword should not be saved")
	}
	if !slices.Equal(unchanged.MainGroupKeywords, settings.MainGroupKeywords) {
		t.Fatalf("non-matching input changed settings: %#v", unchanged.MainGroupKeywords)
	}
}

func TestPromptMainGroupKeywordSavesDirectInputAndAllowsBlank(t *testing.T) {
	p := nodeSelectTestPaths(t)
	selectors := []map[string]any{{"tag": "🚀 节点选择", "type": "selector"}}
	matchingInput := func(string, tui.AskOpts) (string, error) { return "节点选择", nil }

	added, err := promptMainGroupKeywordWith(p, selectors, matchingInput)
	if err != nil {
		t.Fatalf("promptMainGroupKeywordWith returned an error: %v", err)
	}
	if !added {
		t.Fatal("expected matching direct input to be saved")
	}
	if got := config.Load(p).MainGroupKeywords; len(got) == 0 || got[0] != "节点选择" {
		t.Fatalf("saved main-group keywords = %#v", got)
	}

	before := config.Load(p).MainGroupKeywords
	blankInput := func(string, tui.AskOpts) (string, error) { return "", nil }
	added, err = promptMainGroupKeywordWith(p, selectors, blankInput)
	if err != nil {
		t.Fatalf("blank input returned an error: %v", err)
	}
	if added {
		t.Fatal("blank input should skip adding a keyword")
	}
	if got := config.Load(p).MainGroupKeywords; !slices.Equal(got, before) {
		t.Fatalf("blank input changed main-group keywords: %#v", got)
	}
}

func TestResolveTargetGroupRechecksWithSavedKeyword(t *testing.T) {
	p := nodeSelectTestPaths(t)
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "🚀 节点选择", "type": "selector", "outbounds": []string{"sg-01"}},
			map[string]any{"tag": "Auto", "type": "selector", "outbounds": []string{"sg-01"}},
		},
	}

	target, err := resolveTargetGroup(p, cfg, "", func(string, tui.AskOpts) (string, error) {
		return "节点选择", nil
	})
	if err != nil {
		t.Fatalf("resolveTargetGroup returned error: %v", err)
	}
	if tag := target["tag"]; tag != "🚀 节点选择" {
		t.Fatalf("resolved target = %v, want 主选择组", tag)
	}
	if got := config.Load(p).MainGroupKeywords; len(got) == 0 || got[0] != "节点选择" {
		t.Fatalf("saved main-group keywords = %#v", got)
	}
}

func TestResolveTargetGroupReturnsMissingErrorWithoutMatchingKeyword(t *testing.T) {
	p := nodeSelectTestPaths(t)
	cfg := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "Manual", "type": "selector", "outbounds": []string{"sg-01"}},
			map[string]any{"tag": "Auto", "type": "selector", "outbounds": []string{"sg-02"}},
		},
	}
	before := config.Load(p).MainGroupKeywords

	target, err := resolveTargetGroup(p, cfg, "", func(string, tui.AskOpts) (string, error) {
		return "missing", nil
	})
	if err == nil || err.Error() != i18n.T("未识别到主选择组，无法切换节点") {
		t.Fatalf("error = %v, want missing-main-group error", err)
	}
	if target != nil {
		t.Fatalf("resolved target = %#v, want nil", target)
	}
	if got := config.Load(p).MainGroupKeywords; !slices.Equal(got, before) {
		t.Fatalf("non-matching input changed main-group keywords: %#v", got)
	}
}

func TestResolveTargetGroupUsesStoredGroupAndPropagatesPromptErrors(t *testing.T) {
	p := nodeSelectTestPaths(t)
	configured := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "Proxy", "type": "selector", "outbounds": []string{"sg-01"}},
		},
	}
	target, err := resolveTargetGroup(p, configured, "", func(string, tui.AskOpts) (string, error) {
		t.Fatal("prompt should not run for the configured main selector")
		return "", nil
	})
	if err != nil || target["tag"] != "Proxy" {
		t.Fatalf("resolved target = %#v, err = %v", target, err)
	}

	unrecognized := map[string]any{
		"outbounds": []any{
			map[string]any{"tag": "Manual", "type": "selector", "outbounds": []string{"sg-01"}},
		},
	}
	wantErr := errors.New("prompt failed")
	if _, err := resolveTargetGroup(p, unrecognized, "", func(string, tui.AskOpts) (string, error) {
		return "", wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("prompt error = %v, want %v", err, wantErr)
	}
}

func TestCollectMembersOnlyUsesMainSelectorChoices(t *testing.T) {
	main := map[string]any{"tag": "Proxy", "type": "selector", "outbounds": []string{"Singapore-01", "Auto", "DIRECT"}}
	cfg := map[string]any{
		"outbounds": []any{
			main,
			map[string]any{"tag": "Auto", "type": "urltest", "outbounds": []string{"Singapore-01", "unrelated"}},
			map[string]any{"tag": "Singapore-01", "type": "shadowsocks"},
			map[string]any{"tag": "unrelated", "type": "shadowsocks"},
		},
	}

	buckets, subgroups := collectMembers(cfg, main)
	if !slices.Equal(buckets["sg"], []string{"Singapore-01"}) {
		t.Fatalf("main selector choices = %#v", buckets)
	}
	if !slices.Equal(subgroups, []string{"Auto"}) {
		t.Fatalf("main selector subgroups = %#v", subgroups)
	}
}

func nodeSelectTestPaths(t *testing.T) paths.Paths {
	t.Helper()
	t.Setenv("SBOXKIT_HOME", t.TempDir())
	p := paths.Detect()
	if err := config.Save(p, config.Defaults()); err != nil {
		t.Fatalf("save default config: %v", err)
	}
	return p
}
