package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// ---------------------------------------------------------------------------
// SearchFilter table validation
// ---------------------------------------------------------------------------

type SearchFilterTableSuite struct {
	suite.Suite
}

func TestSearchFilterTableSuite(t *testing.T) {
	suite.Run(t, new(SearchFilterTableSuite))
}

func (s *SearchFilterTableSuite) TestAllEntriesHaveNonEmptyKeys() {
	for _, f := range catalog.ProductSearchFilters {
		s.NotEmpty(f.ToolKey, "ToolKey must not be empty")
		s.NotEmpty(f.BCKey, "BCKey must not be empty for %s", f.ToolKey)
	}
}

func (s *SearchFilterTableSuite) TestAllEntriesHaveValidKind() {
	validKinds := map[string]bool{"string": true, "number": true, "bool": true}
	for _, f := range catalog.ProductSearchFilters {
		s.True(validKinds[f.Kind], "invalid Kind %q for filter %s", f.Kind, f.ToolKey)
	}
}

func (s *SearchFilterTableSuite) TestNoDuplicateToolKeys() {
	seen := make(map[string]bool)
	for _, f := range catalog.ProductSearchFilters {
		s.False(seen[f.ToolKey], "duplicate ToolKey: %s", f.ToolKey)
		seen[f.ToolKey] = true
	}
}

func (s *SearchFilterTableSuite) TestNoDuplicateBCKeys() {
	seen := make(map[string]bool)
	for _, f := range catalog.ProductSearchFilters {
		s.False(seen[f.BCKey], "duplicate BCKey: %s", f.BCKey)
		seen[f.BCKey] = true
	}
}

// ---------------------------------------------------------------------------
// ExtractFilters
// ---------------------------------------------------------------------------

type ExtractFiltersSuite struct {
	suite.Suite
}

func TestExtractFiltersSuite(t *testing.T) {
	suite.Run(t, new(ExtractFiltersSuite))
}

func (s *ExtractFiltersSuite) TestStringExtraction() {
	args := map[string]any{"name_like": "Testing Product"}
	params, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Equal("Testing Product", params["name:like"])
}

func (s *ExtractFiltersSuite) TestNumberExtraction() {
	args := map[string]any{"category_id": float64(408)}
	params, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Equal("408", params["categories:in"])
}

func (s *ExtractFiltersSuite) TestBoolExtraction() {
	args := map[string]any{"is_visible": true}
	params, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Equal("true", params["is_visible"])
}

func (s *ExtractFiltersSuite) TestBoolFalseExtraction() {
	args := map[string]any{"is_visible": false}
	params, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Equal("false", params["is_visible"])
}

func (s *ExtractFiltersSuite) TestMultipleFilters() {
	args := map[string]any{
		"name_like":  "Shirt",
		"price_min":  float64(10),
		"price_max":  float64(50),
		"is_visible": true,
	}
	params, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Equal("Shirt", params["name:like"])
	s.Equal("10", params["price:min"])
	s.Equal("50", params["price:max"])
	s.Equal("true", params["is_visible"])
}

func (s *ExtractFiltersSuite) TestUnknownKeysIgnored() {
	args := map[string]any{"unknown_field": "value", "name_like": "Test"}
	params, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Len(params, 1)
	s.Equal("Test", params["name:like"])
}

func (s *ExtractFiltersSuite) TestEmptyStringSkipped() {
	args := map[string]any{"name_like": ""}
	params, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Empty(params)
}

func (s *ExtractFiltersSuite) TestWrongTypeStringReturnsError() {
	args := map[string]any{"name_like": 123}
	_, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.Error(err)
	s.Contains(err.Error(), "name_like must be a string")
}

func (s *ExtractFiltersSuite) TestWrongTypeNumberReturnsError() {
	args := map[string]any{"category_id": "not-a-number"}
	_, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.Error(err)
	s.Contains(err.Error(), "category_id must be a number")
}

func (s *ExtractFiltersSuite) TestWrongTypeBoolReturnsError() {
	args := map[string]any{"is_visible": "yes"}
	_, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.Error(err)
	s.Contains(err.Error(), "is_visible must be a boolean")
}

func (s *ExtractFiltersSuite) TestEmptyArgsReturnsEmptyParams() {
	args := map[string]any{}
	params, err := catalog.ExtractFilters(args, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Empty(params)
}

func (s *ExtractFiltersSuite) TestNilArgsReturnsEmptyParams() {
	params, err := catalog.ExtractFilters(nil, catalog.ProductSearchFilters)
	s.NoError(err)
	s.Empty(params)
}

// ---------------------------------------------------------------------------
// Standalone test for the empty-filter guard logic
// ---------------------------------------------------------------------------

func TestEmptyFilterGuard(t *testing.T) {
	nonFilterKeys := map[string]bool{
		"sort": true, "sort_direction": true, "include_fields": true,
	}

	tests := []struct {
		name      string
		args      map[string]any
		expectErr bool
	}{
		{
			name:      "no args triggers guard",
			args:      map[string]any{},
			expectErr: true,
		},
		{
			name:      "only sort is not a filter",
			args:      map[string]any{"sort": "name"},
			expectErr: true,
		},
		{
			name:      "sort + direction still not a filter",
			args:      map[string]any{"sort": "name", "sort_direction": "asc"},
			expectErr: true,
		},
		{
			name:      "name_like is a real filter",
			args:      map[string]any{"name_like": "Test"},
			expectErr: false,
		},
		{
			name:      "category_id is a real filter",
			args:      map[string]any{"category_id": float64(23)},
			expectErr: false,
		},
		{
			name:      "filter + sort passes",
			args:      map[string]any{"keyword": "shoes", "sort": "price"},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params, err := catalog.ExtractFilters(tc.args, catalog.ProductSearchFilters)
			if err != nil {
				t.Fatalf("unexpected extraction error: %v", err)
			}

			hasFilter := false
			for bcKey := range params {
				isNonFilter := false
				for _, f := range catalog.ProductSearchFilters {
					if f.BCKey == bcKey && nonFilterKeys[f.ToolKey] {
						isNonFilter = true
						break
					}
				}
				if !isNonFilter {
					hasFilter = true
					break
				}
			}

			if tc.expectErr {
				assert.False(t, hasFilter, "expected no real filters for args %v", tc.args)
			} else {
				assert.True(t, hasFilter, "expected at least one real filter for args %v", tc.args)
			}
		})
	}
}
