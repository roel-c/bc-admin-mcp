package catalog_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type DeleteToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestDeleteToolSuite(t *testing.T) {
	suite.Run(t, new(DeleteToolSuite))
}

func (s *DeleteToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.prods.RegisterTools(s.reg)
}

func (s *DeleteToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *DeleteToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *DeleteToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *DeleteToolSuite) TestDeletePreview() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget", SKU: "WDG-01"},
	}, nil)

	result, err := s.callTool("catalog/products/delete", map[string]any{
		"product_ids": []any{float64(42)},
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(1), data["total_products"])
	s.Contains(data["message"], "PERMANENTLY DELETED")
}

func (s *DeleteToolSuite) TestDeleteExecute() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget"},
	}, nil)

	// Preview
	s.callTool("catalog/products/delete", map[string]any{
		"product_ids": []any{float64(42)},
	})

	s.mockBC.EXPECT().DeleteProducts(gomock.Any(), []int{42}).Return([]int{42}, nil)

	result, err := s.callTool("catalog/products/delete", map[string]any{
		"product_ids": []any{float64(42)},
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal(float64(1), data["products_deleted"])
}

func (s *DeleteToolSuite) TestDeleteNoTargetError() {
	result, err := s.callTool("catalog/products/delete", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *DeleteToolSuite) TestDeleteWithoutPreviewFails() {
	result, err := s.callTool("catalog/products/delete", map[string]any{
		"product_ids": []any{float64(99)},
		"confirmed":   true,
	})
	s.NoError(err)
	s.True(result.IsError)
}
