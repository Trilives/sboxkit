package tui

// Plain-mode forms deliberately reuse the existing line-oriented components:
// scripts can feed one answer per enabled field without terminal control codes.
func formPlain(fields []FormField) (FormResult, error) {
	fields = cloneFormFields(fields)
	for i := range fields {
		field := &fields[i]
		if !fieldEnabled(*field, formValues(fields)) {
			continue
		}
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
				field.Value, err = Ask(field.Label, AskOpts{Default: field.Value, AllowEmpty: true})
			}
			if err != nil {
				return nil, err
			}
			if field.Validate == nil || field.Validate(field.Value) == nil {
				break
			}
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
