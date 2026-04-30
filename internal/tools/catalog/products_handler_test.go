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

type ProductHandlerSuite struct {
	suite.Suite
	ctrl  *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestProductHandlerSuite(t *testing.T) {
	suite.Run(t, new(ProductHandlerSuite))
}

func (s *ProductHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.prods.RegisterTools(s.reg)
}

func (s *ProductHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ProductHandlerSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found in registry", toolPath)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolPath,
			Arguments: args,
		},
	}
	return def.Handler(context.Background(), req)
}

func (s *ProductHandlerSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

// --- Product Search Tests ---

func (s *ProductHandlerSuite) TestSearchReturnsProducts() {
	s.mockBC.EXPECT().SearchProducts(gomock.Any(), gomock.Any()).Return([]bigcommerce.Product{
		{ID: 1, Name: "Widget", SKU: "W-001", Price: 19.99},
		{ID: 2, Name: "Gadget", SKU: "G-001", Price: 29.99},
	}, nil)

	result, err := s.callTool("catalog/products/search", map[string]any{
		"name_like": "et",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal(float64(2), data["total_products"])
	products := data["products"].([]any)
	s.Len(products, 2)
	first := products[0].(map[string]any)
	s.Equal("Widget", first["name"])
}

func (s *ProductHandlerSuite) TestSearchRequiresFilter() {
	result, err := s.callTool("catalog/products/search", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ProductHandlerSuite) TestSearchRejectsSortOnly() {
	result, err := s.callTool("catalog/products/search", map[string]any{
		"sort": "name",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ProductHandlerSuite) TestSearchWithChannelIDsAppliesFilter() {
	s.mockBC.EXPECT().SearchProducts(gomock.Any(), gomock.AssignableToTypeOf(map[string]string{})).
		DoAndReturn(func(_ context.Context, params map[string]string) ([]bigcommerce.Product, error) {
			s.Equal("1,3", params["channel_id:in"])
			return []bigcommerce.Product{{ID: 5, Name: "Channel widget"}}, nil
		})

	result, err := s.callTool("catalog/products/search", map[string]any{
		"channel_ids": []any{float64(1), float64(3)},
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total_products"])
}

func (s *ProductHandlerSuite) TestSearchChannelIDsExceedingLimit() {
	ids := make([]any, 0, 21)
	for i := 1; i <= 21; i++ {
		ids = append(ids, float64(i))
	}
	result, err := s.callTool("catalog/products/search", map[string]any{
		"channel_ids": ids,
	})
	s.NoError(err)
	s.True(result.IsError)
}

// --- Product Get Tests ---

func (s *ProductHandlerSuite) TestGetReturnsProductWithVariants() {
	s.mockBC.EXPECT().GetProduct(gomock.Any(), 42).Return(&bigcommerce.Product{
		ID: 42, Name: "Deluxe Widget", Price: 49.99,
	}, nil)
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 42).Return([]bigcommerce.Variant{
		{ID: 100, ProductID: 42, Price: 54.99},
		{ID: 101, ProductID: 42, Price: 0},
	}, nil)

	result, err := s.callTool("catalog/products/get", map[string]any{
		"product_id": float64(42),
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal(true, data["has_variant_pricing"])
	s.Equal(float64(2), data["variant_count"])
}

func (s *ProductHandlerSuite) TestGetDetectsNoVariantPricing() {
	s.mockBC.EXPECT().GetProduct(gomock.Any(), 42).Return(&bigcommerce.Product{
		ID: 42, Name: "Simple Widget", Price: 19.99,
	}, nil)
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 42).Return([]bigcommerce.Variant{
		{ID: 100, ProductID: 42, Price: 0},
	}, nil)

	result, err := s.callTool("catalog/products/get", map[string]any{
		"product_id": float64(42),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(false, data["has_variant_pricing"])
}

func (s *ProductHandlerSuite) TestGetRequiresProductID() {
	result, err := s.callTool("catalog/products/get", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

// --- Assign Categories Tests ---

func (s *ProductHandlerSuite) TestAssignCategoriesPreview() {
	result, err := s.callTool("catalog/products/assign_categories", map[string]any{
		"product_ids":  []any{float64(1), float64(2)},
		"category_ids": []any{float64(10), float64(20)},
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(4), data["total_assignments"])
}

func (s *ProductHandlerSuite) TestAssignCategoriesExecute() {
	s.mockBC.EXPECT().UpsertCategoryAssignments(gomock.Any(), gomock.Any()).Return(nil)

	result, err := s.callTool("catalog/products/assign_categories", map[string]any{
		"product_ids":  []any{float64(1)},
		"category_ids": []any{float64(10)},
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}
