package shared

import (
	"fmt"
	"strconv"
)

// SearchFilter declares how a single tool parameter maps to a BigCommerce
// query parameter. The Kind field controls type-checking at extraction time.
type SearchFilter struct {
	ToolKey string // parameter name exposed to the LLM
	BCKey   string // BigCommerce query parameter key (e.g. "name:like")
	Kind    string // "string", "number", or "bool"
}

// ExtractFilters reads tool arguments through a SearchFilter table, returning
// the corresponding BigCommerce query parameters. It returns an error if a
// provided argument has the wrong type.
func ExtractFilters(args map[string]any, filters []SearchFilter) (map[string]string, error) {
	params := make(map[string]string)
	for _, f := range filters {
		v, ok := args[f.ToolKey]
		if !ok {
			continue
		}
		switch f.Kind {
		case "string":
			s, sOk := v.(string)
			if !sOk {
				return nil, fmt.Errorf("%s must be a string", f.ToolKey)
			}
			if s != "" {
				params[f.BCKey] = s
			}
		case "number":
			n, nOk := v.(float64)
			if !nOk {
				return nil, fmt.Errorf("%s must be a number", f.ToolKey)
			}
			if n == float64(int(n)) {
				params[f.BCKey] = fmt.Sprintf("%.0f", n)
			} else {
				params[f.BCKey] = strconv.FormatFloat(n, 'f', -1, 64)
			}
		case "bool":
			b, bOk := v.(bool)
			if !bOk {
				return nil, fmt.Errorf("%s must be a boolean", f.ToolKey)
			}
			params[f.BCKey] = strconv.FormatBool(b)
		}
	}
	return params, nil
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
