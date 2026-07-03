package node

import (
	"encoding/json"
	"fmt"
	"os"
)

func ReorderSelectorConfig(path string, group string, selected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	outbounds, ok := doc["outbounds"].([]any)
	if !ok {
		return fmt.Errorf("config outbounds must be an array")
	}
	for _, item := range outbounds {
		outbound, ok := item.(map[string]any)
		if !ok || outbound["type"] != "selector" || outbound["tag"] != group {
			continue
		}
		next, err := moveSelectedFirst(stringSlice(outbound["outbounds"]), selected)
		if err != nil {
			return err
		}
		outbound["outbounds"] = next
		outbound["default"] = selected
		encoded, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(path, append(encoded, '\n'), 0o644)
	}
	return fmt.Errorf("selector group %q not found", group)
}

func moveSelectedFirst(values []string, selected string) ([]string, error) {
	next := make([]string, 0, len(values))
	found := false
	for _, value := range values {
		if value == selected {
			found = true
			continue
		}
		next = append(next, value)
	}
	if !found {
		return nil, fmt.Errorf("node %q not found in selector", selected)
	}
	return append([]string{selected}, next...), nil
}

func stringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok {
			out = append(out, text)
		}
	}
	return out
}
