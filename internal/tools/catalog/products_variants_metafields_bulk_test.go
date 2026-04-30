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

type BulkVariantMetafieldIDsSuite struct {
	suite.Suite
}

func TestBulkVariantMetafieldIDsSuite(t *testing.T) {
	suite.Run(t, new(BulkVariantMetafieldIDsSuite))
}

func (s *BulkVariantMetafieldIDsSuite) TestValidDedupes() {
	args := map[string]any{"variant_ids": []any{float64(1), float64(1), float64(2)}}
	ids, err := catalog.ParseBulkVariantMetafieldVariantIDs(args)
	s.NoError(err)
	s.Equal([]int{1, 2}, ids)
}

func (s *BulkVariantMetafieldIDsSuite) TestRejectsEmpty() {
	_, err := catalog.ParseBulkVariantMetafieldVariantIDs(map[string]any{
		"variant_ids": []any{},
	})
	s.Error(err)
	s.Contains(err.Error(), "non-empty")
}

func (s *BulkVariantMetafieldIDsSuite) TestRejectsMissingKey() {
	_, err := catalog.ParseBulkVariantMetafieldVariantIDs(map[string]any{})
	s.Error(err)
	s.Contains(err.Error(), "variant_ids is required")
}

func (s *BulkVariantMetafieldIDsSuite) TestRejectsNonNumberElement() {
	_, err := catalog.ParseBulkVariantMetafieldVariantIDs(map[string]any{
		"variant_ids": []any{"x"},
	})
	s.Error(err)
	s.Contains(err.Error(), "must be a number")
}

func (s *BulkVariantMetafieldIDsSuite) TestRejectsNonPositive() {
	_, err := catalog.ParseBulkVariantMetafieldVariantIDs(map[string]any{
		"variant_ids": []any{float64(0)},
	})
	s.Error(err)
	s.Contains(err.Error(), "positive")
}

func (s *BulkVariantMetafieldIDsSuite) TestRejectsOverMax() {
	arr := make([]any, 51)
	for i := range arr {
		arr[i] = float64(i + 1)
	}
	_, err := catalog.ParseBulkVariantMetafieldVariantIDs(map[string]any{
		"variant_ids": arr,
	})
	s.Error(err)
	s.Contains(err.Error(), "exceeds maximum")
}

type VariantMetafieldBulkToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestVariantMetafieldBulkToolSuite(t *testing.T) {
	suite.Run(t, new(VariantMetafieldBulkToolSuite))
}

func (s *VariantMetafieldBulkToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/variants", "Variants")
	s.reg.RegisterCategory("catalog/products/variants/metafields", "Variant metafields")
	s.prods.RegisterVariantMetafieldBulkTools(s.reg)
}

func (s *VariantMetafieldBulkToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *VariantMetafieldBulkToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *VariantMetafieldBulkToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *VariantMetafieldBulkToolSuite) TestBulkSetPreview() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 10).Return([]bigcommerce.Variant{
		{ID: 100, ProductID: 10, SKU: "A", Price: 1},
		{ID: 101, ProductID: 10, SKU: "B", Price: 2},
	}, nil)
	s.mockBC.EXPECT().ListVariantMetafields(gomock.Any(), 10, 100).Return(nil, nil)
	s.mockBC.EXPECT().ListVariantMetafields(gomock.Any(), 10, 101).Return([]bigcommerce.Metafield{
		{ID: 7, Namespace: "ns", Key: "k", Value: "old", PermissionSet: "app_only"},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_set", map[string]any{
		"product_id":  float64(10),
		"variant_ids": []any{float64(100), float64(101)},
		"namespace":   "ns",
		"key":         "k",
		"value":       "new",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(10), data["product_id"])
	s.Contains(data["message"].(string), "2 variant")
}

func (s *VariantMetafieldBulkToolSuite) TestBulkSetRejectsVariantNotOnProduct() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 10).Return([]bigcommerce.Variant{
		{ID: 100, ProductID: 10, SKU: "A", Price: 1},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_set", map[string]any{
		"product_id":  float64(10),
		"variant_ids": []any{float64(999)},
		"namespace":   "ns",
		"key":         "k",
		"value":       "v",
	})
	s.NoError(err)
	text := result.Content[0].(mcp.TextContent).Text
	s.Contains(text, "not found on product")
}

func (s *VariantMetafieldBulkToolSuite) TestBulkSetPreviewByVariantSKUContains() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 10).Return([]bigcommerce.Variant{
		{ID: 1, ProductID: 10, SKU: "P-ABC-RED", Price: 1},
		{ID: 2, ProductID: 10, SKU: "P-DEF-BLUE", Price: 2},
		{ID: 3, ProductID: 10, SKU: "P-XYZ-SMALL", Price: 3},
	}, nil)
	s.mockBC.EXPECT().ListVariantMetafields(gomock.Any(), 10, 3).Return(nil, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_set", map[string]any{
		"product_id":           float64(10),
		"variant_sku_contains": "xyz",
		"namespace":            "viz",
		"key":                  "url",
		"value":                "https://example.com/g.svg",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal("xyz", data["variant_sku_contains"])
	s.Equal(float64(1), data["variant_count"])
}

func (s *VariantMetafieldBulkToolSuite) TestBulkDeletePreview() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 3).Return([]bigcommerce.Variant{
		{ID: 20, ProductID: 3, SKU: "V", Price: 1},
	}, nil)
	s.mockBC.EXPECT().ListVariantMetafields(gomock.Any(), 3, 20).Return([]bigcommerce.Metafield{
		{ID: 50, Namespace: "x", Key: "y", Value: "z"},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_delete", map[string]any{
		"product_id":  float64(3),
		"variant_ids": []any{float64(20)},
		"namespace":   "x",
		"key":         "y",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(50), data["per_variant"].([]any)[0].(map[string]any)["metafield_id"])
}

type CrossProductVariantMetafieldBulkSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestCrossProductVariantMetafieldBulkSuite(t *testing.T) {
	suite.Run(t, new(CrossProductVariantMetafieldBulkSuite))
}

func (s *CrossProductVariantMetafieldBulkSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/variants", "Variants")
	s.reg.RegisterCategory("catalog/products/variants/metafields", "Variant metafields")
	s.prods.RegisterVariantMetafieldBulkTools(s.reg)
}

func (s *CrossProductVariantMetafieldBulkSuite) TearDownTest() { s.ctrl.Finish() }

func (s *CrossProductVariantMetafieldBulkSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CrossProductVariantMetafieldBulkSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *CrossProductVariantMetafieldBulkSuite) TestBulkSetProductsPreviewSKUContains() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 10).Return([]bigcommerce.Variant{
		{ID: 100, ProductID: 10, SKU: "SKU-XYZ-1", Price: 1},
		{ID: 101, ProductID: 10, SKU: "OTHER", Price: 2},
	}, nil)
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 20).Return([]bigcommerce.Variant{
		{ID: 200, ProductID: 20, SKU: "no-match-here", Price: 1},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_set_products", map[string]any{
		"product_ids":          []any{float64(10), float64(20)},
		"variant_scope":        "sku_contains",
		"variant_sku_contains": "xyz",
		"namespace":            "viz",
		"key":                  "graph",
		"value":                "u",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total_variant_operations"])
	s.Equal("xyz", data["variant_sku_contains"])
}

func (s *CrossProductVariantMetafieldBulkSuite) TestBulkSetProductsSKUContainsNoMatches() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 1).Return([]bigcommerce.Variant{
		{ID: 10, ProductID: 1, SKU: "AAA", Price: 1},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_set_products", map[string]any{
		"product_ids":          []any{float64(1)},
		"variant_scope":        "sku_contains",
		"variant_sku_contains": "nomatch",
		"namespace":            "n",
		"key":                  "k",
		"value":                "v",
	})
	s.NoError(err)
	s.True(result.IsError)
	s.Contains(result.Content[0].(mcp.TextContent).Text, "no variants matched")
}

func (s *CrossProductVariantMetafieldBulkSuite) TestBulkSetProductsPreviewAllVariants() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 10).Return([]bigcommerce.Variant{
		{ID: 100, ProductID: 10, SKU: "A", Price: 1},
		{ID: 101, ProductID: 10, SKU: "B", Price: 2},
	}, nil)
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 20).Return([]bigcommerce.Variant{
		{ID: 200, ProductID: 20, SKU: "C", Price: 3},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_set_products", map[string]any{
		"product_ids":   []any{float64(10), float64(20)},
		"variant_scope": "all_variants",
		"namespace":     "viz",
		"key":           "graph_ref",
		"value":         "https://cdn.example.com/chart.svg",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(3), data["total_variant_operations"])
	s.Equal("all_variants", data["variant_scope"])
}

func (s *CrossProductVariantMetafieldBulkSuite) TestBulkSetProductsFirstVariantOnly() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 5).Return([]bigcommerce.Variant{
		{ID: 50, ProductID: 5, SKU: "X", Price: 1},
		{ID: 51, ProductID: 5, SKU: "Y", Price: 2},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_set_products", map[string]any{
		"product_ids":   []any{float64(5)},
		"variant_scope": "first_variant_only",
		"namespace":     "n",
		"key":           "k",
		"value":         "v",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	per := data["per_product"].([]any)
	s.Len(per, 1)
	row := per[0].(map[string]any)
	ids := row["variant_ids"].([]any)
	s.Len(ids, 1)
	s.Equal(float64(50), ids[0])
}

func (s *CrossProductVariantMetafieldBulkSuite) TestBulkSetProductsRejectsInvalidScope() {
	result, err := s.callTool("catalog/products/variants/metafields/bulk_set_products", map[string]any{
		"product_ids":   []any{float64(1)},
		"variant_scope": "every_sku",
		"namespace":     "n",
		"key":           "k",
		"value":         "v",
	})
	s.NoError(err)
	s.True(result.IsError)
	s.Contains(result.Content[0].(mcp.TextContent).Text, "variant_scope")
}

func (s *CrossProductVariantMetafieldBulkSuite) TestBulkSetProductsRejectsOverMaxOperations() {
	many := make([]bigcommerce.Variant, 501)
	for i := range many {
		many[i] = bigcommerce.Variant{ID: i + 1, ProductID: 1, SKU: "S", Price: 1}
	}
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 1).Return(many, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_set_products", map[string]any{
		"product_ids":   []any{float64(1)},
		"variant_scope": "all_variants",
		"namespace":     "n",
		"key":           "k",
		"value":         "v",
	})
	s.NoError(err)
	s.True(result.IsError)
	s.Contains(result.Content[0].(mcp.TextContent).Text, "exceeds maximum")
}

func (s *CrossProductVariantMetafieldBulkSuite) TestBulkDeleteProductsPreview() {
	s.mockBC.EXPECT().ListVariantsForProduct(gomock.Any(), 7).Return([]bigcommerce.Variant{
		{ID: 70, ProductID: 7, SKU: "Z", Price: 1},
	}, nil)

	result, err := s.callTool("catalog/products/variants/metafields/bulk_delete_products", map[string]any{
		"product_ids":   []any{float64(7)},
		"variant_scope": "all_variants",
		"namespace":     "viz",
		"key":           "graph_ref",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(1), data["total_variant_operations"])
}
