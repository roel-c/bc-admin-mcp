package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type BulkSingleProductVariantSelectionSuite struct {
	suite.Suite
}

func TestBulkSingleProductVariantSelectionSuite(t *testing.T) {
	suite.Run(t, new(BulkSingleProductVariantSelectionSuite))
}

func (s *BulkSingleProductVariantSelectionSuite) TestByVariantIDs() {
	sel, err := catalog.ParseBulkSingleProductVariantSelection(map[string]any{
		"variant_ids": []any{float64(1), float64(2)},
	})
	s.NoError(err)
	s.Equal("", sel.SKUContains)
	s.Equal([]int{1, 2}, sel.VariantIDs)
}

func (s *BulkSingleProductVariantSelectionSuite) TestByVariantSKUContains() {
	sel, err := catalog.ParseBulkSingleProductVariantSelection(map[string]any{
		"variant_sku_contains": "-XYZ-",
	})
	s.NoError(err)
	s.Equal("-XYZ-", sel.SKUContains)
	s.Empty(sel.VariantIDs)
}

func (s *BulkSingleProductVariantSelectionSuite) TestRejectBothTargetingModes() {
	_, err := catalog.ParseBulkSingleProductVariantSelection(map[string]any{
		"variant_ids":          []any{float64(1)},
		"variant_sku_contains": "X",
	})
	s.Error(err)
	s.Contains(err.Error(), "only one of")
}

func (s *BulkSingleProductVariantSelectionSuite) TestRejectNeither() {
	_, err := catalog.ParseBulkSingleProductVariantSelection(map[string]any{})
	s.Error(err)
	s.Contains(err.Error(), "variant_ids or variant_sku_contains")
}
