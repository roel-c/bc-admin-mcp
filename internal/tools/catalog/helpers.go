package catalog

import "fmt"

const (
	sessionKeyDefault       = "default"
	cacheKeyProductUpdate   = "product_update"
	cacheKeyProductDelete   = "product_delete"
)

// parseFloat64SliceToPositiveInts converts a JSON array ([]any of float64) to
// []int, requiring every element to be a positive integer (> 0).
func parseFloat64SliceToPositiveInts(v any, fieldName string) ([]int, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", fieldName)
	}
	out := make([]int, 0, len(raw))
	for _, item := range raw {
		f, fOk := item.(float64)
		if !fOk {
			return nil, fmt.Errorf("each %s entry must be a number", fieldName)
		}
		id := int(f)
		if id <= 0 {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

// parseFloat64SliceToNonNegativeInts converts a JSON array ([]any of float64)
// to []int, allowing zero but dropping negative values.
func parseFloat64SliceToNonNegativeInts(v any, fieldName string) ([]int, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", fieldName)
	}
	out := make([]int, 0, len(raw))
	for _, item := range raw {
		f, fOk := item.(float64)
		if !fOk {
			return nil, fmt.Errorf("each %s entry must be a number", fieldName)
		}
		id := int(f)
		if id < 0 {
			continue
		}
		out = append(out, id)
	}
	return out, nil
}

// parseStringSlice converts a JSON array ([]any of strings) to []string.
func parseStringSlice(v any, fieldName string) ([]string, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", fieldName)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, sOk := item.(string)
		if !sOk {
			return nil, fmt.Errorf("each %s entry must be a string", fieldName)
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}
