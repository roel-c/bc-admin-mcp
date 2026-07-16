package catalog

import (
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ParseOptionalPermissionSet validates permission_set from tool args. Wraps the
// shared implementation so existing call sites stay on the catalog package API.
func ParseOptionalPermissionSet(args map[string]any) (string, error) {
	return shared.ParseOptionalPermissionSet(args)
}
