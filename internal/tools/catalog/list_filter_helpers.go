package catalog

import (
	"fmt"

	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ReadListAllBoolean returns args[key] when it is a bool; otherwise false.
func ReadListAllBoolean(args map[string]any, key string) bool {
	return shared.ReadBool(args, key)
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
