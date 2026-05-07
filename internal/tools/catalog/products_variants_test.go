package catalog_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type VariantToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestVariantToolSuite(t *testing.T) {
	suite.Run(t, new(VariantToolSuite))
}

func (s *VariantToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/variants", "Variants")
	s.prods.RegisterTools(s.reg)
	s.prods.RegisterVariantTools(s.reg)
}

func (s *VariantToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *VariantToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *VariantToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *VariantToolSuite) TestVariantList() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 1).Return([]bigcommerce.Variant{
		{ID: 100, ProductID: 1, SKU: "V1", Price: 19.99},
		{ID: 101, ProductID: 1, SKU: "V2", Price: 24.99},
	}, nil)

	result, err := s.callTool("catalog/products/variants/list", map[string]any{
		"product_id": float64(1),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(2), data["total_variants"])
}

func (s *VariantToolSuite) TestVariantCreatePreview() {
	result, err := s.callTool("catalog/products/variants/create", map[string]any{
		"product_id": float64(1),
		"sku":        "NEW-V",
		"price":      float64(29.99),
		"option_values": []any{
			map[string]any{"option_display_name": "Size", "label": "Large"},
		},
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *VariantToolSuite) TestVariantCreateExecute() {
	price := float64(29.99)
	s.mockBC.EXPECT().CreateVariant(gomock.Any(), 1, gomock.Any()).Return(&bigcommerce.ProductVariantFull{
		ID: 200, ProductID: 1, SKU: "NEW-V", Price: &price,
	}, nil)

	result, err := s.callTool("catalog/products/variants/create", map[string]any{
		"product_id": float64(1),
		"sku":        "NEW-V",
		"price":      float64(29.99),
		"option_values": []any{
			map[string]any{"option_display_name": "Size", "label": "Large"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *VariantToolSuite) TestVariantUpdatePreview() {
	price := float64(19.99)
	s.mockBC.EXPECT().GetVariant(gomock.Any(), 1, 100).Return(&bigcommerce.ProductVariantFull{
		ID: 100, ProductID: 1, SKU: "V1", Price: &price,
	}, nil)

	result, err := s.callTool("catalog/products/variants/update", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
		"price":      float64(24.99),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *VariantToolSuite) TestVariantUpdateExecute() {
	newPrice := float64(24.99)
	s.mockBC.EXPECT().UpdateVariant(gomock.Any(), 1, 100, gomock.Any()).Return(&bigcommerce.ProductVariantFull{
		ID: 100, ProductID: 1, Price: &newPrice,
	}, nil)

	result, err := s.callTool("catalog/products/variants/update", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
		"price":      float64(24.99),
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *VariantToolSuite) TestVariantDeletePreview() {
	price := float64(19.99)
	s.mockBC.EXPECT().GetVariant(gomock.Any(), 1, 100).Return(&bigcommerce.ProductVariantFull{
		ID: 100, ProductID: 1, SKU: "V1", Price: &price,
	}, nil)

	result, err := s.callTool("catalog/products/variants/delete", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *VariantToolSuite) TestVariantDeleteExecute() {
	s.mockBC.EXPECT().DeleteVariant(gomock.Any(), 1, 100).Return(nil)

	result, err := s.callTool("catalog/products/variants/delete", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *VariantToolSuite) TestVariantUpdateNoFieldsError() {
	result, err := s.callTool("catalog/products/variants/update", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(100),
	})
	s.NoError(err)
	s.True(result.IsError)
}
