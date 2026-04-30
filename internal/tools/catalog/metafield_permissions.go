package catalog

import (
	"fmt"
	"strings"
)

// validMetafieldPermissionSets are allowed BigCommerce catalog metafield
// permission_set values (Management API). Shared by category, product, variant, and brand metafield tools.
var validMetafieldPermissionSets = map[string]bool{
	"app_only":             true,
	"read":                 true,
	"write":                true,
	"read_and_sf_access":   true,
	"write_and_sf_access":  true,
}

// ParseOptionalPermissionSet extracts and validates permission_set from tool args.
// Empty return means the caller omitted the field (apply resource-specific default).
func ParseOptionalPermissionSet(args map[string]any) (string, error) {
	v, ok := args["permission_set"]
	if !ok {
		return "", nil
	}
	s, sOk := v.(string)
	if !sOk {
		return "", fmt.Errorf("permission_set must be a string")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !validMetafieldPermissionSets[s] {
		return "", fmt.Errorf(
			"permission_set must be one of: app_only, read, write, read_and_sf_access, write_and_sf_access",
		)
	}
	return s, nil
}
