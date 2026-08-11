package converter

// mergeCustomFakeIPFilters appends user-maintained exclusions to the source
// subscription's FakeIP baseline. A subscription without FakeIP remains
// unchanged: an exclusion list alone must not silently enable FakeIP.
func mergeCustomFakeIPFilters(source sourceOptions, custom []string) (sourceOptions, []string) {
	if source.FakeIP == nil {
		return source, nil
	}
	customRules, skipped := clashFakeIPFilterRules(custom)
	fake := *source.FakeIP
	fake.ExclusionRules = mergeFakeIPExclusionRules(fake.ExclusionRules, customRules)
	source.FakeIP = &fake
	return source, skipped
}

// mergeFakeIPExclusionRules consolidates the domain matchers supported by the
// preservation layer. All generated rules share the same remote DNS action,
// so combining their match fields preserves OR semantics while making
// ordering and de-duplication deterministic.
func mergeFakeIPExclusionRules(groups ...[]map[string]any) []map[string]any {
	keys := []string{"domain", "domain_suffix", "domain_keyword", "domain_regex"}
	values := make(map[string][]string, len(keys))
	seen := make(map[string]map[string]bool, len(keys))
	for _, key := range keys {
		seen[key] = map[string]bool{}
	}
	for _, rules := range groups {
		for _, rule := range rules {
			for _, key := range keys {
				for _, value := range stringSlice(rule[key]) {
					if value == "" || seen[key][value] {
						continue
					}
					seen[key][value] = true
					values[key] = append(values[key], value)
				}
			}
		}
	}
	rule := map[string]any{"action": "route", "server": remoteDNSTag}
	for _, key := range keys {
		if len(values[key]) > 0 {
			rule[key] = values[key]
		}
	}
	if len(rule) == 2 {
		return nil
	}
	return []map[string]any{rule}
}
