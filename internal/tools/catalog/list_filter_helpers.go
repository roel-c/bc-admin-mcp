package catalog

import "fmt"

// ReadListAllBoolean returns args[key] when it is a bool; otherwise false.
func ReadListAllBoolean(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// HasDataFilterBCParams reports whether bcParams contains at least one query key that
// is not solely a "non-data" filter (e.g. sort) according to filters + nonDataToolKeys.
// Pass nil or empty nonDataToolKeys if every filter key counts as a data constraint.
func HasDataFilterBCParams(bcParams map[string]string, filters []SearchFilter, nonDataToolKeys map[string]bool) bool {
	for bcKey := range bcParams {
		isNonData := false
		if nonDataToolKeys != nil {
			for _, f := range filters {
				if f.BCKey == bcKey && nonDataToolKeys[f.ToolKey] {
					isNonData = true
					break
				}
			}
		}
		if !isNonData {
			return true
		}
	}
	return false
}

// ErrInvalidBCSort returns a non-nil error if params["sort"] is set and not in allowed.
func ErrInvalidBCSort(params map[string]string, allowed map[string]bool, validOptionsHint string) error {
	if sortVal, ok := params["sort"]; ok {
		if !allowed[sortVal] {
			return fmt.Errorf("invalid sort field %q — %s", sortVal, validOptionsHint)
		}
	}
	return nil
}
