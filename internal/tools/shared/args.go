package shared

import (
	"fmt"
	"math"
)

// ReadBool returns args[key] when present and bool-typed; otherwise false.
func ReadBool(args map[string]any, key string) bool {
	if v, ok := args[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// ReadPositiveInt extracts a positive int (>0) from args[key]; the second
// return value is the human-friendly error to surface when the value is
// missing, not numeric, or non-positive.
func ReadPositiveInt(args map[string]any, key string) (int, error) {
	raw, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	f, ok := raw.(float64)
	if !ok {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	id := int(f)
	if id <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return id, nil
}
