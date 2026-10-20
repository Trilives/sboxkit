package tui

import (
	"fmt"

	"github.com/Trilives/sboxkit/internal/execx"
)

// Plain-mode forms deliberately reuse the existing line-oriented components:
// scripts can feed one answer per enabled field without terminal control codes.
func formPlain(fields []FormField) (FormResult, error) {
	fields = cloneFormFields(fields)
	for i := range fields {
		field := &fields[i]
		if !fieldEnabled(*field, formValues(fields)) {
			continue
		}
		original := field.Value
		for {
			var err error
			switch field.Kind {
			case FormBool:
				var value bool
				value, err = Confirm(field.Label, field.Value == "true")
				field.Value = boolString(value)
			case FormChoice:
				var idx int
				labels := make([]string, len(field.Options))
				for i, option := range field.Options {
					labels[i] = choiceLabel(*field, option)
				}
				idx, err = Select(field.Label, labels, SelectOpts{Initial: stringIndex(field.Options, field.Value)})
				if err == nil {
					field.Value = field.Options[idx]
				}
			default:
				field.Value, err = Ask(field.Label, AskOpts{Default: field.Value, AllowEmpty: !field.Required})
			}
			if err != nil {
				return nil, err
			}
			if field.Validate == nil {
				break
			}
			invalid := field.Validate(field.Value)
			if invalid == nil {
				break
			}
			// Explain the rejection and re-ask with the last accepted value as the
			// default, so pressing Enter cannot resubmit the rejected input.
			execx.Warn(fmt.Sprintf("%s: %v", field.Label, invalid))
			field.Value = original
		}
	}
	if err := validateForm(fields); err != nil {
		return nil, err
	}
	return formValues(fields), nil
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func stringIndex(items []string, value string) int {
	for i, item := range items {
		if item == value {
			return i
		}
	}
	return 0
}
