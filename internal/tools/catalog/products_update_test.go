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

type UpdateToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestUpdateToolSuite(t *testing.T) {
	suite.Run(t, new(UpdateToolSuite))
}

func (s *UpdateToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.prods.RegisterTools(s.reg)
}

func (s *UpdateToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *UpdateToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *UpdateToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *UpdateToolSuite) TestUpdatePreviewSingleProduct() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget", Price: 19.99, IsVisible: true},
	}, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
		"is_visible":  false,
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(1), data["total_products"])
	fields := data["fields_updated"].([]any)
	s.Contains(fields, "price")
	s.Contains(fields, "is_visible")
}

func (s *UpdateToolSuite) TestUpdateExecuteSingleProduct() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{42}).Return([]bigcommerce.Product{
		{ID: 42, Name: "Widget", Price: 19.99},
	}, nil)

	// Preview first
	s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
	})

	s.mockBC.EXPECT().BatchUpdateProducts(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 1,
	}, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(42)},
		"price":       float64(24.99),
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal(float64(1), data["products_updated"])
}

func (s *UpdateToolSuite) TestUpdateByCategoryWithLimit() {
	prods := []bigcommerce.Product{
		{ID: 1, Name: "A", Price: 10},
		{ID: 2, Name: "B", Price: 20},
		{ID: 3, Name: "C", Price: 30},
	}
	s.mockBC.EXPECT().ListProductsByCategory(gomock.Any(), 5, gomock.Any()).Return(prods, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"category_id": float64(5),
		"limit":       float64(2),
		"name":        "Renamed",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(2), data["total_products"])
}

func (s *UpdateToolSuite) TestUpdateNoFieldsError() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids": []any{float64(1)},
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *UpdateToolSuite) TestUpdateNoTargetError() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"price": float64(10),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *UpdateToolSuite) TestUpdateMultipleTargetModesError() {
	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids":  []any{float64(1)},
		"sku":          "ABC",
		"price":        float64(10),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *UpdateToolSuite) TestUpdateSEOFields() {
	s.mockBC.EXPECT().GetProductsByIDs(gomock.Any(), []int{10}).Return([]bigcommerce.Product{
		{ID: 10, Name: "SEO Product", PageTitle: "Old"},
	}, nil)

	result, err := s.callTool("catalog/products/update", map[string]any{
		"product_ids":      []any{float64(10)},
		"page_title":       "New Title",
		"meta_description": "New Description",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	fields := data["fields_updated"].([]any)
	s.Contains(fields, "page_title")
	s.Contains(fields, "meta_description")
}
