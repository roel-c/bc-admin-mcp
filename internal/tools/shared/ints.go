package shared

import (
	"fmt"
	"strconv"
	"strings"
)

// JoinInts encodes a slice of integers as a comma-separated string suitable
// for BigCommerce `*:in` query parameters (e.g. id:in=1,2,3).
func JoinInts(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	b := make([]byte, 0, len(ids)*4)
	for i, id := range ids {
		if i > 0 {
			b = append(b, ',')
		}
		b = strconv.AppendInt(b, int64(id), 10)
	}
	return string(b)
}

// RequiredNonEmptyStringIDs reads args[key] as a JSON array of non-empty
// strings (e.g. UUIDs). Whitespace is trimmed; empty entries are rejected so
// the caller never silently drops an intended ID.
func RequiredNonEmptyStringIDs(args map[string]any, key string) ([]string, error) {
	raw, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
	out := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("each %s entry must be a string", key)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, fmt.Errorf("%s[%d] must be a non-empty string", key, i)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s must contain at least one id", key)
	}
	return out, nil
}

// OptionalStringIDs reads args[key] as a JSON array of non-empty strings.
// Returns (nil, nil) when args[key] is absent or nil. Empty entries are rejected.
func OptionalStringIDs(args map[string]any, key string) ([]string, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
	out := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("each %s entry must be a string", key)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, fmt.Errorf("%s[%d] must be a non-empty string", key, i)
		}
		out = append(out, s)
	}
	return out, nil
}
