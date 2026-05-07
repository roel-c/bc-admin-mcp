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

type VariantMetafieldSetParamsSuite struct {
	suite.Suite
}

func TestVariantMetafieldSetParamsSuite(t *testing.T) {
	suite.Run(t, new(VariantMetafieldSetParamsSuite))
}

func (s *VariantMetafieldSetParamsSuite) TestByProductIDAndVariantID() {
	args := map[string]any{
		"product_id": float64(10),
		"variant_id": float64(200),
		"namespace":  "app",
		"key":        "k1",
		"value":      "v1",
	}
	p, err := catalog.ParseVariantMetafieldSetParams(args)
	s.NoError(err)
	s.Equal(10, p.ProductID)
	s.Equal(200, p.VariantID)
	s.Equal("", p.VariantSKU)
	s.Equal("app", p.Namespace)
}

func (s *VariantMetafieldSetParamsSuite) TestByProductSKUAndVariantSKU() {
	args := map[string]any{
		"sku":         "P-SKU",
		"variant_sku": "V-SKU",
		"namespace":   "n",
		"key":         "k",
		"value":       "v",
	}
	p, err := catalog.ParseVariantMetafieldSetParams(args)
	s.NoError(err)
	s.Equal("P-SKU", p.SKU)
	s.Equal("V-SKU", p.VariantSKU)
	s.Equal(0, p.VariantID)
}

func (s *VariantMetafieldSetParamsSuite) TestRejectVariantIDAndVariantSKUTogether() {
	args := map[string]any{
		"product_id":  float64(1),
		"variant_id":  float64(2),
		"variant_sku": "X",
		"namespace":   "n",
		"key":         "k",
		"value":       "v",
	}
	_, err := catalog.ParseVariantMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "only one of")
}

func (s *VariantMetafieldSetParamsSuite) TestRejectMissingVariantLocator() {
	args := map[string]any{
		"product_id": float64(1),
		"namespace":  "n",
		"key":        "k",
		"value":      "v",
	}
	_, err := catalog.ParseVariantMetafieldSetParams(args)
	s.Error(err)
	s.Contains(err.Error(), "variant_id or variant_sku")
}

type VariantMetafieldDeleteParamsSuite struct {
	suite.Suite
}

func TestVariantMetafieldDeleteParamsSuite(t *testing.T) {
	suite.Run(t, new(VariantMetafieldDeleteParamsSuite))
}

func (s *VariantMetafieldDeleteParamsSuite) TestByMetafieldID() {
	args := map[string]any{
		"product_id":   float64(1),
		"variant_id":   float64(9),
		"metafield_id": float64(44),
	}
	p, err := catalog.ParseVariantMetafieldDeleteParams(args)
	s.NoError(err)
	s.Equal(44, p.MetafieldID)
	s.Equal(9, p.VariantID)
}

func (s *VariantMetafieldDeleteParamsSuite) TestByNamespaceKeyAndVariantSKU() {
	args := map[string]any{
		"product_name": "Widget",
		"variant_sku":  "W-RED",
		"namespace":    "ns",
		"key":          "k",
	}
	p, err := catalog.ParseVariantMetafieldDeleteParams(args)
	s.NoError(err)
	s.Equal("W-RED", p.VariantSKU)
	s.Equal("ns", p.Namespace)
	s.Equal("k", p.Key)
}

type VariantMetafieldToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestVariantMetafieldToolSuite(t *testing.T) {
	suite.Run(t, new(VariantMetafieldToolSuite))
}

func (s *VariantMetafieldToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/variants", "Variants")
	s.reg.RegisterCategory("catalog/products/variants/metafields", "Variant metafields")
	s.prods.RegisterVariantMetafieldTools(s.reg)
}

func (s *VariantMetafieldToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *VariantMetafieldToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *VariantMetafieldToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *VariantMetafieldToolSuite) TestListByVariantID() {
	s.mockBC.EXPECT().GetVariant(gomock.Any(), 1, 50).Return(&bigcommerce.ProductVariantFull{
		ID:        50,
		ProductID: 1,
	}, nil)
	s.mockBC.EXPECT().ListVariantMetafields(gomock.Any(), 1, 50).Return([]bigcommerce.Metafield{
		{ID: 7, Namespace: "n", Key: "k", Value: "x"},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/list", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(50),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["product_id"])
	s.Equal(float64(50), data["variant_id"])
	s.Equal(float64(1), data["total"])
}

func (s *VariantMetafieldToolSuite) TestListByVariantSKU() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 2).Return([]bigcommerce.Variant{
		{ID: 10, ProductID: 2, SKU: "V-A", Price: 1},
		{ID: 11, ProductID: 2, SKU: "V-B", Price: 2},
	}, nil)
	s.mockBC.EXPECT().ListVariantMetafields(gomock.Any(), 2, 11).Return([]bigcommerce.Metafield{}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/list", map[string]any{
		"product_id":  float64(2),
		"variant_sku": "V-B",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(0), data["total"])
}

func (s *VariantMetafieldToolSuite) TestSetPreview() {
	s.mockBC.EXPECT().GetVariant(gomock.Any(), 1, 5).Return(&bigcommerce.ProductVariantFull{ID: 5, ProductID: 1}, nil)
	s.mockBC.EXPECT().ListVariantMetafields(gomock.Any(), 1, 5).Return(nil, nil)

	result, err := s.callTool("catalog/products/variants/metafields/set", map[string]any{
		"product_id": float64(1),
		"variant_id": float64(5),
		"namespace":  "ns",
		"key":        "flag",
		"value":      "1",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Contains(data["message"].(string), "Will create")
}

func (s *VariantMetafieldToolSuite) TestDeletePreviewByNamespaceKey() {
	s.mockBC.EXPECT().GetVariant(gomock.Any(), 3, 9).Return(&bigcommerce.ProductVariantFull{ID: 9, ProductID: 3}, nil)
	s.mockBC.EXPECT().ListVariantMetafields(gomock.Any(), 3, 9).Return([]bigcommerce.Metafield{
		{ID: 100, Namespace: "a", Key: "b", Value: "old"},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/delete", map[string]any{
		"product_id": float64(3),
		"variant_id": float64(9),
		"namespace":  "a",
		"key":        "b",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(100), data["metafield_id"])
	s.Equal("pending_confirmation", data["status"])
}
