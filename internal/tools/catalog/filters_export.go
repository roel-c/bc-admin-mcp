package catalog

import "github.com/roel-c/bc-admin-mcp/internal/tools/shared"

// SearchFilter is an alias for the shared declarative filter mapping type.
type SearchFilter = shared.SearchFilter

// ExtractFilters reads tool arguments through a SearchFilter table.
func ExtractFilters(args map[string]any, filters []SearchFilter) (map[string]string, error) {
	return shared.ExtractFilters(args, filters)
}

// HasDataFilterBCParams reports whether bcParams contains at least one data filter.
func HasDataFilterBCParams(bcParams map[string]string, filters []SearchFilter, nonDataToolKeys map[string]bool) bool {
	return shared.HasDataFilterBCParams(bcParams, filters, nonDataToolKeys)
}
