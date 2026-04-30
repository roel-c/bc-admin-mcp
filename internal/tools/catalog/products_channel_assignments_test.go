package catalog_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ProductChannelAssignmentsSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	p      *catalog.Products
	reg    *discovery.Registry
}

func TestProductChannelAssignmentsSuite(t *testing.T) {
	suite.Run(t, new(ProductChannelAssignmentsSuite))
}

func (s *ProductChannelAssignmentsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.p = catalog.NewProducts(s.mockBC, nil)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/channel_assignments", "Channel assignments")
	s.p.RegisterTools(s.reg)
}

func (s *ProductChannelAssignmentsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ProductChannelAssignmentsSuite) getTool(path string) *discovery.ToolDef {
	def := s.reg.GetTool(path)
	s.Require().NotNil(def)
	return def
}

func (s *ProductChannelAssignmentsSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *ProductChannelAssignmentsSuite) TestListRequiresFilter() {
	def := s.getTool("catalog/products/channel_assignments/list")
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: def.Path, Arguments: map[string]any{}},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.True(res.IsError)
}

func (s *ProductChannelAssignmentsSuite) TestListWithProductFilter() {
	s.mockBC.EXPECT().ListProductChannelAssignments(gomock.Any(), map[string]string{
		"product_id:in": "1,2",
	}).Return([]bigcommerce.ProductChannelAssignment{
		{ProductID: 1, ChannelID: 10},
		{ProductID: 2, ChannelID: 10},
	}, nil)

	def := s.getTool("catalog/products/channel_assignments/list")
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: def.Path,
			Arguments: map[string]any{
				"product_ids": []any{float64(1), float64(2)},
			},
		},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(2), data["total"])
}

func (s *ProductChannelAssignmentsSuite) TestAssignPreview() {
	def := s.getTool("catalog/products/channel_assignments/assign")
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: def.Path,
			Arguments: map[string]any{
				"product_ids":  []any{float64(5)},
				"channel_ids":  []any{float64(1), float64(2)},
				"confirmed":    false,
			},
		},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(2), data["total_assignments"])
}

func (s *ProductChannelAssignmentsSuite) TestAssignConfirmed() {
	s.mockBC.EXPECT().UpsertProductChannelAssignments(gomock.Any(), []bigcommerce.ProductChannelAssignment{
		{ProductID: 7, ChannelID: 3},
	}).Return(nil)

	def := s.getTool("catalog/products/channel_assignments/assign")
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: def.Path,
			Arguments: map[string]any{
				"product_ids": []any{float64(7)},
				"channel_ids": []any{float64(3)},
				"confirmed":   true,
			},
		},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("completed", data["status"])
}

func (s *ProductChannelAssignmentsSuite) TestRemovePreview() {
	def := s.getTool("catalog/products/channel_assignments/remove")
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: def.Path,
			Arguments: map[string]any{
				"product_ids": []any{float64(9)},
				"channel_ids": []any{float64(2)},
				"confirmed":   false,
			},
		},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
}

func (s *ProductChannelAssignmentsSuite) TestRemoveConfirmed() {
	s.mockBC.EXPECT().DeleteProductChannelAssignments(gomock.Any(), []int{9}, []int{2}).Return(nil)

	def := s.getTool("catalog/products/channel_assignments/remove")
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: def.Path,
			Arguments: map[string]any{
				"product_ids": []any{float64(9)},
				"channel_ids": []any{float64(2)},
				"confirmed":   true,
			},
		},
	}
	res, err := def.Handler(context.Background(), req)
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("completed", data["status"])
}
