package shared

import (
	"fmt"
	"strings"
)

// ValidMetafieldPermissionSets are the BigCommerce metafield permission_set values
// (Management API). Shared by every tool that exposes metafield writes.
var ValidMetafieldPermissionSets = map[string]bool{
	"app_only":            true,
	"read":                true,
	"write":               true,
	"read_and_sf_access":  true,
	"write_and_sf_access": true,
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
	if !ValidMetafieldPermissionSets[s] {
		return "", fmt.Errorf(
			"permission_set must be one of: app_only, read, write, read_and_sf_access, write_and_sf_access",
		)
	}
	return s, nil
}

// AppOnlyMetafieldPermissionNote is shown in previews where new metafields default
// to app_only and the caller may need a Storefront-readable permission instead.
const AppOnlyMetafieldPermissionNote = "New metafields default to app_only. Use read_and_sf_access or write_and_sf_access for Storefront-readable values."
