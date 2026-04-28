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

type CreateExpandedSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestCreateExpandedSuite(t *testing.T) {
	suite.Run(t, new(CreateExpandedSuite))
}

func (s *CreateExpandedSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.prods.RegisterTools(s.reg)
}

func (s *CreateExpandedSuite) TearDownTest() { s.ctrl.Finish() }

func (s *CreateExpandedSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CreateExpandedSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *CreateExpandedSuite) TestCreatePreviewWithAllFields() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"name":               "Test Product",
		"type":               "physical",
		"weight":             float64(2.5),
		"price":              float64(29.99),
		"sku":                "TP-001",
		"description":        "A test product",
		"cost_price":         float64(15.00),
		"retail_price":       float64(34.99),
		"sale_price":         float64(27.99),
		"width":              float64(10),
		"height":             float64(5),
		"depth":              float64(3),
		"category_ids":       []any{float64(1), float64(2)},
		"brand_id":           float64(5),
		"is_visible":         true,
		"is_featured":        true,
		"page_title":         "Test Product Page",
		"meta_description":   "Meta desc",
		"search_keywords":    "test, product",
		"upc":                "123456789",
		"gtin":               "GT-123",
		"mpn":                "MP-001",
		"warranty":           "1 year",
		"condition":          "New",
		"availability":       "available",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	product := data["product"].(map[string]any)
	s.Equal("Test Product", product["name"])
	s.Equal(float64(29.99), product["price"])
	s.Equal("TP-001", product["sku"])
}

func (s *CreateExpandedSuite) TestCreateWithInlineImages() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"name":   "Image Product",
		"weight": float64(1.0),
		"images": []any{
			map[string]any{
				"image_url":    "https://example.com/img1.jpg",
				"is_thumbnail": true,
			},
			map[string]any{
				"image_url":   "https://example.com/img2.png",
				"description": "Alt text",
			},
		},
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	product := data["product"].(map[string]any)
	images := product["images"].([]any)
	s.Len(images, 2)
}

func (s *CreateExpandedSuite) TestCreateExecuteWithExpandedFields() {
	s.mockBC.EXPECT().CreateProduct(gomock.Any(), gomock.Any()).Return(&bigcommerce.Product{
		ID: 99, Name: "New Product", SKU: "NP-01", Price: 49.99,
	}, nil)

	result, err := s.callTool("catalog/products/create", map[string]any{
		"name":      "New Product",
		"weight":    float64(1.0),
		"price":     float64(49.99),
		"sku":       "NP-01",
		"upc":       "999888777",
		"warranty":  "2 years",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
}

func (s *CreateExpandedSuite) TestCreateNameRequired() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"weight": float64(1.0),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CreateExpandedSuite) TestCreateInvalidType() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"name": "Bad Type",
		"type": "subscription",
	})
	s.NoError(err)
	s.True(result.IsError)
}
