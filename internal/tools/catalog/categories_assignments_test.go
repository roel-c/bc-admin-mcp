package catalog_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
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

// TestRejectsTooManyProducts guards the per-call product cap (max 100)
// so a single PUT body can't carry an unbounded list.
func (s *AssignmentParamsSuite) TestRejectsTooManyProducts() {
	pids := make([]any, 101)
	for i := range pids {
		pids[i] = float64(i + 1)
	}
	args := map[string]any{
		"product_ids":  pids,
		"category_ids": []any{float64(1)},
	}
	_, err := catalog.ParseAssignmentParams(args)
	s.Require().Error(err)
	s.Contains(err.Error(), "product_ids")
}

// TestRejectsTooManyCategories guards the per-call category cap (max 50).
func (s *AssignmentParamsSuite) TestRejectsTooManyCategories() {
	cids := make([]any, 51)
	for i := range cids {
		cids[i] = float64(i + 1)
	}
	args := map[string]any{
		"product_ids":  []any{float64(1)},
		"category_ids": cids,
	}
	_, err := catalog.ParseAssignmentParams(args)
	s.Require().Error(err)
	s.Contains(err.Error(), "category_ids")
}

// TestRejectsTooManyPairs guards the Cartesian (product × category) cap of
// 500 pairs so a misbehaving agent can't fan out to thousands of assignment
// rows in one PUT (which would 422 on body size). Mirrors the channel
// assignments cap and aligns assign_categories with unassign_categories.
func (s *AssignmentParamsSuite) TestRejectsTooManyPairs() {
	pids := make([]any, 60) // 60 × 10 = 600 pairs > 500
	for i := range pids {
		pids[i] = float64(i + 1)
	}
	cids := make([]any, 10)
	for i := range cids {
		cids[i] = float64(1000 + i)
	}
	args := map[string]any{
		"product_ids":  pids,
		"category_ids": cids,
	}
	_, err := catalog.ParseAssignmentParams(args)
	s.Require().Error(err)
	s.Contains(err.Error(), "pairs")
}

// TestAcceptsExactlyMaxPairs locks the cap at 500 so future refactors can't
// silently change the boundary.
func (s *AssignmentParamsSuite) TestAcceptsExactlyMaxPairs() {
	pids := make([]any, 50) // 50 × 10 = 500 pairs (exactly the cap)
	for i := range pids {
		pids[i] = float64(i + 1)
	}
	cids := make([]any, 10)
	for i := range cids {
		cids[i] = float64(1000 + i)
	}
	args := map[string]any{
		"product_ids":  pids,
		"category_ids": cids,
	}
	_, err := catalog.ParseAssignmentParams(args)
	s.NoError(err)
}

type UnassignCategoriesSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	p      *catalog.Products
	reg    *discovery.Registry
}

func TestUnassignCategoriesSuite(t *testing.T) {
	suite.Run(t, new(UnassignCategoriesSuite))
}

func (s *UnassignCategoriesSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.p = catalog.NewProducts(s.mockBC, nil)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/channel_assignments", "Channel assignments")
	s.p.RegisterTools(s.reg)
}

func (s *UnassignCategoriesSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *UnassignCategoriesSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *UnassignCategoriesSuite) tool() *discovery.ToolDef {
	def := s.reg.GetTool("catalog/products/unassign_categories")
	s.Require().NotNil(def)
	return def
}

func (s *UnassignCategoriesSuite) TestPreviewWithoutConfirm() {
	def := s.tool()
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: def.Path,
			Arguments: map[string]any{
				"product_ids":  []any{float64(10), float64(11)},
				"category_ids": []any{float64(50)},
			},
		},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(2), data["product_count"])
	s.Equal(float64(1), data["category_count"])
}

func (s *UnassignCategoriesSuite) TestRequiresCategoryIDs() {
	def := s.tool()
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: def.Path,
			Arguments: map[string]any{
				"product_ids": []any{float64(10)},
			},
		},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.True(res.IsError)
}

func (s *UnassignCategoriesSuite) TestConfirmedDeletes() {
	s.mockBC.EXPECT().
		DeleteCategoryAssignmentsByFilter(gomock.Any(), []int{10}, []int{50, 51}).
		Return(nil)

	def := s.tool()
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: def.Path,
			Arguments: map[string]any{
				"product_ids":  []any{float64(10)},
				"category_ids": []any{float64(50), float64(51)},
				"confirmed":    true,
			},
		},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("completed", data["status"])
}
