package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type CategoryProductsParamsSuite struct {
	suite.Suite
}

func TestCategoryProductsParamsSuite(t *testing.T) {
	suite.Run(t, new(CategoryProductsParamsSuite))
}

func (s *CategoryProductsParamsSuite) TestAcceptsCategoryID() {
	args := map[string]any{"category_id": float64(408)}
	p, err := catalog.ParseCategoryProductsParams(args)
	s.NoError(err)
	s.Equal(408, p.CategoryID)
}

func (s *CategoryProductsParamsSuite) TestAcceptsCategoryName() {
	args := map[string]any{"category_name": "Shop All"}
	p, err := catalog.ParseCategoryProductsParams(args)
	s.NoError(err)
	s.Equal("Shop All", p.CategoryName)
}

func (s *CategoryProductsParamsSuite) TestRejectBothIDAndName() {
	args := map[string]any{"category_id": float64(1), "category_name": "X"}
	_, err := catalog.ParseCategoryProductsParams(args)
	s.Error(err)
	s.Contains(err.Error(), "mutually exclusive")
}

func (s *CategoryProductsParamsSuite) TestRejectNeitherIDNorName() {
	args := map[string]any{}
	_, err := catalog.ParseCategoryProductsParams(args)
	s.Error(err)
	s.Contains(err.Error(), "provide either")
}

func (s *CategoryProductsParamsSuite) TestAcceptsOptionalParams() {
	args := map[string]any{
		"category_id":    float64(408),
		"limit":          float64(10),
		"sort":           "price",
		"sort_direction": "desc",
	}
	p, err := catalog.ParseCategoryProductsParams(args)
	s.NoError(err)
	s.Equal(10, p.Limit)
	s.Equal("price", p.Sort)
	s.Equal("desc", p.SortDirection)
}

func (s *CategoryProductsParamsSuite) TestRejectInvalidSort() {
	args := map[string]any{"category_id": float64(1), "sort": "invalid"}
	_, err := catalog.ParseCategoryProductsParams(args)
	s.Error(err)
}
