package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type AssignmentParamsSuite struct {
	suite.Suite
}

func TestAssignmentParamsSuite(t *testing.T) {
	suite.Run(t, new(AssignmentParamsSuite))
}

func (s *AssignmentParamsSuite) TestValidInput() {
	args := map[string]any{
		"product_ids":  []any{float64(111), float64(222)},
		"category_ids": []any{float64(408), float64(508)},
	}
	p, err := catalog.ParseAssignmentParams(args)
	s.NoError(err)
	s.Equal([]int{111, 222}, p.ProductIDs)
	s.Equal([]int{408, 508}, p.CategoryIDs)
}

func (s *AssignmentParamsSuite) TestMissingProductIDs() {
	args := map[string]any{
		"category_ids": []any{float64(1)},
	}
	_, err := catalog.ParseAssignmentParams(args)
	s.Error(err)
	s.Contains(err.Error(), "product_ids")
}

func (s *AssignmentParamsSuite) TestMissingCategoryIDs() {
	args := map[string]any{
		"product_ids": []any{float64(1)},
	}
	_, err := catalog.ParseAssignmentParams(args)
	s.Error(err)
	s.Contains(err.Error(), "category_ids")
}

func (s *AssignmentParamsSuite) TestEmptyProductIDs() {
	args := map[string]any{
		"product_ids":  []any{},
		"category_ids": []any{float64(1)},
	}
	_, err := catalog.ParseAssignmentParams(args)
	s.Error(err)
}

func (s *AssignmentParamsSuite) TestEmptyCategoryIDs() {
	args := map[string]any{
		"product_ids":  []any{float64(1)},
		"category_ids": []any{},
	}
	_, err := catalog.ParseAssignmentParams(args)
	s.Error(err)
}
