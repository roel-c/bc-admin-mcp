package catalog_test

import (
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
)

type ReorderParamsSuite struct {
	suite.Suite
}

func TestReorderParamsSuite(t *testing.T) {
	suite.Run(t, new(ReorderParamsSuite))
}

func (s *ReorderParamsSuite) TestMinimalWithIDs() {
	args := map[string]any{
		"category_ids": []any{float64(508), float64(408), float64(409)},
	}
	p, err := catalog.ParseReorderParams(args)
	s.NoError(err)
	s.Equal([]int{508, 408, 409}, p.CategoryIDs)
	s.Equal(0, p.StartSortOrder)
	s.Equal(10, p.Increment)
}

func (s *ReorderParamsSuite) TestWithNames() {
	args := map[string]any{
		"category_names": []any{"Shop All", "All Wholesale"},
	}
	p, err := catalog.ParseReorderParams(args)
	s.NoError(err)
	s.Equal([]string{"Shop All", "All Wholesale"}, p.CategoryNames)
}

func (s *ReorderParamsSuite) TestCustomStartAndIncrement() {
	args := map[string]any{
		"category_ids":     []any{float64(1), float64(2)},
		"start_sort_order": float64(100),
		"increment":        float64(5),
	}
	p, err := catalog.ParseReorderParams(args)
	s.NoError(err)
	s.Equal(100, p.StartSortOrder)
	s.Equal(5, p.Increment)
}

func (s *ReorderParamsSuite) TestRejectBothIDsAndNames() {
	args := map[string]any{
		"category_ids":   []any{float64(1)},
		"category_names": []any{"X"},
	}
	_, err := catalog.ParseReorderParams(args)
	s.Error(err)
	s.Contains(err.Error(), "not both")
}

func (s *ReorderParamsSuite) TestRejectEmpty() {
	args := map[string]any{}
	_, err := catalog.ParseReorderParams(args)
	s.Error(err)
}

func (s *ReorderParamsSuite) TestRejectZeroIncrement() {
	args := map[string]any{
		"category_ids": []any{float64(1)},
		"increment":    float64(0),
	}
	_, err := catalog.ParseReorderParams(args)
	s.Error(err)
	s.Contains(err.Error(), "at least 1")
}
