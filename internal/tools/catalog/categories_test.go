package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

// ---------------------------------------------------------------------------
// CategorySearchFilters table validation
// ---------------------------------------------------------------------------

type CategoryFilterTableSuite struct {
	suite.Suite
}

func TestCategoryFilterTableSuite(t *testing.T) {
	suite.Run(t, new(CategoryFilterTableSuite))
}

func (s *CategoryFilterTableSuite) TestAllEntriesHaveNonEmptyKeys() {
	for _, f := range catalog.CategorySearchFilters {
		s.NotEmpty(f.ToolKey, "ToolKey must not be empty")
		s.NotEmpty(f.BCKey, "BCKey must not be empty for %s", f.ToolKey)
	}
}

func (s *CategoryFilterTableSuite) TestAllEntriesHaveValidKind() {
	validKinds := map[string]bool{"string": true, "number": true, "bool": true}
	for _, f := range catalog.CategorySearchFilters {
		s.True(validKinds[f.Kind], "invalid Kind %q for filter %s", f.Kind, f.ToolKey)
	}
}

func (s *CategoryFilterTableSuite) TestNoDuplicateToolKeys() {
	seen := make(map[string]bool)
	for _, f := range catalog.CategorySearchFilters {
		s.False(seen[f.ToolKey], "duplicate ToolKey: %s", f.ToolKey)
		seen[f.ToolKey] = true
	}
}

func (s *CategoryFilterTableSuite) TestNoDuplicateBCKeys() {
	seen := make(map[string]bool)
	for _, f := range catalog.CategorySearchFilters {
		s.False(seen[f.BCKey], "duplicate BCKey: %s", f.BCKey)
		seen[f.BCKey] = true
	}
}

func (s *CategoryFilterTableSuite) TestNameLikeFilterExists() {
	found := false
	for _, f := range catalog.CategorySearchFilters {
		if f.ToolKey == "name_like" && f.BCKey == "name:like" {
			found = true
			break
		}
	}
	s.True(found, "name_like -> name:like filter must exist")
}

func (s *CategoryFilterTableSuite) TestParentIDFilterExists() {
	found := false
	for _, f := range catalog.CategorySearchFilters {
		if f.ToolKey == "parent_id" && f.BCKey == "parent_id:in" && f.Kind == "number" {
			found = true
			break
		}
	}
	s.True(found, "parent_id must map to parent_id:in for GET /v3/catalog/trees/categories")
}

func (s *CategoryFilterTableSuite) TestTreeIDFilterMapsToTreeIDIn() {
	found := false
	for _, f := range catalog.CategorySearchFilters {
		if f.ToolKey == "tree_id" && f.BCKey == "tree_id:in" && f.Kind == "number" {
			found = true
			break
		}
	}
	s.True(found, "tree_id must map to tree_id:in for GET /v3/catalog/trees/categories")
}

// ---------------------------------------------------------------------------
// ExtractFilters with CategorySearchFilters
// ---------------------------------------------------------------------------

type CategoryExtractFiltersSuite struct {
	suite.Suite
}

func TestCategoryExtractFiltersSuite(t *testing.T) {
	suite.Run(t, new(CategoryExtractFiltersSuite))
}

func (s *CategoryExtractFiltersSuite) TestNameLikeExtraction() {
	args := map[string]any{"name_like": "Apparel"}
	params, err := catalog.ExtractFilters(args, catalog.CategorySearchFilters)
	s.NoError(err)
	s.Equal("Apparel", params["name:like"])
}

func (s *CategoryExtractFiltersSuite) TestParentIDExtraction() {
	args := map[string]any{"parent_id": float64(0)}
	params, err := catalog.ExtractFilters(args, catalog.CategorySearchFilters)
	s.NoError(err)
	s.Equal("0", params["parent_id:in"])
}

func (s *CategoryExtractFiltersSuite) TestTreeIDExtraction() {
	args := map[string]any{"tree_id": float64(5)}
	params, err := catalog.ExtractFilters(args, catalog.CategorySearchFilters)
	s.NoError(err)
	s.Equal("5", params["tree_id:in"])
}

func (s *CategoryExtractFiltersSuite) TestIsVisibleExtraction() {
	args := map[string]any{"is_visible": false}
	params, err := catalog.ExtractFilters(args, catalog.CategorySearchFilters)
	s.NoError(err)
	s.Equal("false", params["is_visible"])
}

func (s *CategoryExtractFiltersSuite) TestMultipleCategoryFilters() {
	args := map[string]any{
		"name_like":  "Sale",
		"is_visible": true,
		"parent_id":  float64(10),
	}
	params, err := catalog.ExtractFilters(args, catalog.CategorySearchFilters)
	s.NoError(err)
	s.Len(params, 3)
	s.Equal("Sale", params["name:like"])
	s.Equal("true", params["is_visible"])
	s.Equal("10", params["parent_id:in"])
}

func (s *CategoryExtractFiltersSuite) TestWrongTypeReturnsError() {
	args := map[string]any{"parent_id": "not-a-number"}
	_, err := catalog.ExtractFilters(args, catalog.CategorySearchFilters)
	s.Error(err)
	s.Contains(err.Error(), "parent_id must be a number")
}

func (s *CategoryExtractFiltersSuite) TestEmptyArgsReturnsEmpty() {
	params, err := catalog.ExtractFilters(map[string]any{}, catalog.CategorySearchFilters)
	s.NoError(err)
	s.Empty(params)
}

// ---------------------------------------------------------------------------
// ParseCategoryCreateParams validation
// ---------------------------------------------------------------------------

type CategoryCreateParamsSuite struct {
	suite.Suite
}

func TestCategoryCreateParamsSuite(t *testing.T) {
	suite.Run(t, new(CategoryCreateParamsSuite))
}

func (s *CategoryCreateParamsSuite) TestMinimalValid() {
	args := map[string]any{"name": "Summer Sale"}
	p, err := catalog.ParseCategoryCreateParams(args)
	s.NoError(err)
	s.Equal("Summer Sale", p.Payload.Name)
	s.Equal(0, p.Payload.ParentID)
	s.False(p.Confirmed)
}

func (s *CategoryCreateParamsSuite) TestMissingNameReturnsError() {
	args := map[string]any{"description": "no name"}
	_, err := catalog.ParseCategoryCreateParams(args)
	s.Error(err)
	s.Contains(err.Error(), "name is required")
}

func (s *CategoryCreateParamsSuite) TestEmptyNameReturnsError() {
	args := map[string]any{"name": ""}
	_, err := catalog.ParseCategoryCreateParams(args)
	s.Error(err)
	s.Contains(err.Error(), "non-empty string")
}

func (s *CategoryCreateParamsSuite) TestAllOptionalFields() {
	args := map[string]any{
		"name":                 "Electronics",
		"parent_id":           float64(42),
		"description":         "All electronics",
		"is_visible":          true,
		"page_title":          "Shop Electronics",
		"meta_description":    "Buy electronics online",
		"search_keywords":     "phones,tablets",
		"sort_order":          float64(5),
		"default_product_sort": "price_asc",
	}
	p, err := catalog.ParseCategoryCreateParams(args)
	s.NoError(err)
	s.Equal("Electronics", p.Payload.Name)
	s.Equal(42, p.Payload.ParentID)
	s.Equal("All electronics", p.Payload.Description)
	s.NotNil(p.Payload.IsVisible)
	s.True(*p.Payload.IsVisible)
	s.Equal("Shop Electronics", p.Payload.PageTitle)
	s.Equal("Buy electronics online", p.Payload.MetaDescription)
	s.Equal("phones,tablets", p.Payload.SearchKeywords)
	s.Equal(5, p.Payload.SortOrder)
	s.Equal("price_asc", p.Payload.DefaultProductSort)
}

func (s *CategoryCreateParamsSuite) TestInvalidSortReturnsError() {
	args := map[string]any{
		"name":                 "Test",
		"default_product_sort": "random",
	}
	_, err := catalog.ParseCategoryCreateParams(args)
	s.Error(err)
	s.Contains(err.Error(), "invalid default_product_sort")
}

func (s *CategoryCreateParamsSuite) TestConfirmedFlag() {
	args := map[string]any{
		"name":      "Test",
		"confirmed": true,
	}
	p, err := catalog.ParseCategoryCreateParams(args)
	s.NoError(err)
	s.True(p.Confirmed)
}

func (s *CategoryCreateParamsSuite) TestParentIDWrongType() {
	args := map[string]any{
		"name":      "Test",
		"parent_id": "not-a-number",
	}
	_, err := catalog.ParseCategoryCreateParams(args)
	s.Error(err)
	s.Contains(err.Error(), "parent_id must be a number")
}

func (s *CategoryCreateParamsSuite) TestIsVisibleFalse() {
	args := map[string]any{
		"name":       "Hidden Category",
		"is_visible": false,
	}
	p, err := catalog.ParseCategoryCreateParams(args)
	s.NoError(err)
	s.NotNil(p.Payload.IsVisible)
	s.False(*p.Payload.IsVisible)
}

func (s *CategoryCreateParamsSuite) TestParentNameValid() {
	args := map[string]any{
		"name":        "Widgets",
		"parent_name": "Electronics",
	}
	p, err := catalog.ParseCategoryCreateParams(args)
	s.NoError(err)
	s.Equal("Widgets", p.Payload.Name)
	s.Equal("Electronics", p.ParentName)
	s.Equal(0, p.Payload.ParentID)
}

func (s *CategoryCreateParamsSuite) TestParentNameAndParentIDMutuallyExclusive() {
	args := map[string]any{
		"name":        "Widgets",
		"parent_name": "Electronics",
		"parent_id":   float64(42),
	}
	_, err := catalog.ParseCategoryCreateParams(args)
	s.Error(err)
	s.Contains(err.Error(), "mutually exclusive")
}

func (s *CategoryCreateParamsSuite) TestParentNameEmptyStringReturnsError() {
	args := map[string]any{
		"name":        "Widgets",
		"parent_name": "",
	}
	_, err := catalog.ParseCategoryCreateParams(args)
	s.Error(err)
	s.Contains(err.Error(), "non-empty string")
}

func (s *CategoryCreateParamsSuite) TestParentNameWrongTypeReturnsError() {
	args := map[string]any{
		"name":        "Widgets",
		"parent_name": float64(42),
	}
	_, err := catalog.ParseCategoryCreateParams(args)
	s.Error(err)
	s.Contains(err.Error(), "parent_name must be a non-empty string")
}

// ---------------------------------------------------------------------------
// ParseSingleDeleteParams validation
// ---------------------------------------------------------------------------

type SingleDeleteParamsSuite struct {
	suite.Suite
}

func TestSingleDeleteParamsSuite(t *testing.T) {
	suite.Run(t, new(SingleDeleteParamsSuite))
}

func (s *SingleDeleteParamsSuite) TestByID() {
	args := map[string]any{"category_id": float64(503)}
	p, err := catalog.ParseSingleDeleteParams(args)
	s.NoError(err)
	s.Equal([]int{503}, p.CategoryIDs)
	s.Empty(p.CategoryName)
	s.False(p.IncludeChildren)
	s.False(p.Confirmed)
}

func (s *SingleDeleteParamsSuite) TestByName() {
	args := map[string]any{"category_name": "Test"}
	p, err := catalog.ParseSingleDeleteParams(args)
	s.NoError(err)
	s.Empty(p.CategoryIDs)
	s.Equal("Test", p.CategoryName)
}

func (s *SingleDeleteParamsSuite) TestMissingBothReturnsError() {
	args := map[string]any{"confirmed": true}
	_, err := catalog.ParseSingleDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "provide either")
}

func (s *SingleDeleteParamsSuite) TestBothIDAndNameReturnsError() {
	args := map[string]any{
		"category_id":   float64(503),
		"category_name": "Test",
	}
	_, err := catalog.ParseSingleDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "mutually exclusive")
}

func (s *SingleDeleteParamsSuite) TestIncludeChildrenFlag() {
	args := map[string]any{
		"category_id":      float64(490),
		"include_children": true,
		"confirmed":        true,
	}
	p, err := catalog.ParseSingleDeleteParams(args)
	s.NoError(err)
	s.True(p.IncludeChildren)
	s.True(p.Confirmed)
}

func (s *SingleDeleteParamsSuite) TestCategoryIDWrongType() {
	args := map[string]any{"category_id": "not-a-number"}
	_, err := catalog.ParseSingleDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "category_id must be a number")
}

func (s *SingleDeleteParamsSuite) TestCategoryNameEmpty() {
	args := map[string]any{"category_name": ""}
	_, err := catalog.ParseSingleDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "non-empty string")
}

// ---------------------------------------------------------------------------
// ParseBulkDeleteParams validation
// ---------------------------------------------------------------------------

type BulkDeleteParamsSuite struct {
	suite.Suite
}

func TestBulkDeleteParamsSuite(t *testing.T) {
	suite.Run(t, new(BulkDeleteParamsSuite))
}

func (s *BulkDeleteParamsSuite) TestValid() {
	args := map[string]any{
		"category_ids": []any{float64(491), float64(492), float64(493)},
	}
	p, err := catalog.ParseBulkDeleteParams(args)
	s.NoError(err)
	s.Equal([]int{491, 492, 493}, p.CategoryIDs)
	s.False(p.IncludeChildren)
	s.False(p.Confirmed)
}

func (s *BulkDeleteParamsSuite) TestMissingIDsReturnsError() {
	args := map[string]any{"confirmed": true}
	_, err := catalog.ParseBulkDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "category_ids is required")
}

func (s *BulkDeleteParamsSuite) TestEmptyArrayReturnsError() {
	args := map[string]any{"category_ids": []any{}}
	_, err := catalog.ParseBulkDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "must not be empty")
}

func (s *BulkDeleteParamsSuite) TestWrongElementType() {
	args := map[string]any{"category_ids": []any{"not-a-number"}}
	_, err := catalog.ParseBulkDeleteParams(args)
	s.Error(err)
	s.Contains(err.Error(), "each category_id must be a number")
}

func (s *BulkDeleteParamsSuite) TestWithIncludeChildren() {
	args := map[string]any{
		"category_ids":     []any{float64(490)},
		"include_children": true,
		"confirmed":        true,
	}
	p, err := catalog.ParseBulkDeleteParams(args)
	s.NoError(err)
	s.True(p.IncludeChildren)
	s.True(p.Confirmed)
}
