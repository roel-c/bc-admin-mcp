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

// --- Product Create Tests ---

type CreateToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestCreateToolSuite(t *testing.T) {
	suite.Run(t, new(CreateToolSuite))
}

func (s *CreateToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.prods.RegisterTools(s.reg)
}

func (s *CreateToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *CreateToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CreateToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *CreateToolSuite) TestCreatePreview() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"name":  "New Widget",
		"price": float64(29.99),
		"sku":   "NW-001",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	product := data["product"].(map[string]any)
	s.Equal("New Widget", product["name"])
	s.Equal(float64(29.99), product["price"])
	s.Equal("NW-001", product["sku"])
	s.Equal("physical", product["type"])
}

func (s *CreateToolSuite) TestCreateExecute() {
	s.mockBC.EXPECT().CreateProduct(gomock.Any(), gomock.Any()).Return(&bigcommerce.Product{
		ID: 999, Name: "New Widget", SKU: "NW-001", Price: 29.99, IsVisible: true,
	}, nil)

	result, err := s.callTool("catalog/products/create", map[string]any{
		"name":      "New Widget",
		"price":     float64(29.99),
		"sku":       "NW-001",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
	product := data["product"].(map[string]any)
	s.Equal(float64(999), product["id"])
	s.Equal("New Widget", product["name"])
}

func (s *CreateToolSuite) TestCreateRequiresName() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"price": float64(10),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CreateToolSuite) TestCreateDigitalType() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"name": "E-Book",
		"type": "digital",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	product := data["product"].(map[string]any)
	s.Equal("digital", product["type"])
}

func (s *CreateToolSuite) TestCreateInvalidType() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"name": "Bad",
		"type": "imaginary",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *CreateToolSuite) TestCreateWithCategories() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"name":         "Cat Widget",
		"category_ids": []any{float64(10), float64(20)},
	})
	s.NoError(err)
	data := s.parseJSON(result)
	product := data["product"].(map[string]any)
	cats := product["categories"].([]any)
	s.Len(cats, 2)
}

func (s *CreateToolSuite) TestCreateWithSEOFields() {
	result, err := s.callTool("catalog/products/create", map[string]any{
		"name":             "SEO Widget",
		"page_title":       "Buy SEO Widget",
		"meta_description": "Best widget for SEO",
		"search_keywords":  "widget,seo,best",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	product := data["product"].(map[string]any)
	s.Equal("Buy SEO Widget", product["page_title"])
	s.Equal("Best widget for SEO", product["meta_description"])
	s.Equal("widget,seo,best", product["search_keywords"])
}
