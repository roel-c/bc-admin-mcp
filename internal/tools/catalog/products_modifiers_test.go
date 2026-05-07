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

type ModifierToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestModifierToolSuite(t *testing.T) {
	suite.Run(t, new(ModifierToolSuite))
}

func (s *ModifierToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/modifiers", "Modifiers")
	s.prods.RegisterTools(s.reg)
	s.prods.RegisterModifierTools(s.reg)
}

func (s *ModifierToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *ModifierToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *ModifierToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *ModifierToolSuite) TestModifierList() {
	s.mockBC.EXPECT().ListProductModifiers(gomock.Any(), 1).Return([]bigcommerce.ProductModifier{
		{ID: 3, ProductID: 1, DisplayName: "Engraving", Type: "text"},
	}, nil)

	result, err := s.callTool("catalog/products/modifiers/list", map[string]any{
		"product_id": float64(1),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total_modifiers"])
}

func (s *ModifierToolSuite) TestModifierCreatePreview() {
	result, err := s.callTool("catalog/products/modifiers/create", map[string]any{
		"product_id":   float64(1),
		"display_name": "Gift Message",
		"type":         "multi_line_text",
		"required":     false,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *ModifierToolSuite) TestModifierCreateExecute() {
	s.mockBC.EXPECT().CreateProductModifier(gomock.Any(), 1, gomock.Any()).Return(&bigcommerce.ProductModifier{
		ID: 20, ProductID: 1, DisplayName: "Gift Message", Type: "multi_line_text",
	}, nil)

	result, err := s.callTool("catalog/products/modifiers/create", map[string]any{
		"product_id":   float64(1),
		"display_name": "Gift Message",
		"type":         "multi_line_text",
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *ModifierToolSuite) TestModifierDeletePreview() {
	s.mockBC.EXPECT().ListProductModifiers(gomock.Any(), 1).Return([]bigcommerce.ProductModifier{
		{ID: 3, ProductID: 1, DisplayName: "Engraving", Type: "text"},
	}, nil)

	result, err := s.callTool("catalog/products/modifiers/delete", map[string]any{
		"product_id":  float64(1),
		"modifier_id": float64(3),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal("Engraving", data["display_name"])
}

func (s *ModifierToolSuite) TestModifierDeleteExecute() {
	s.mockBC.EXPECT().DeleteProductModifier(gomock.Any(), 1, 3).Return(nil)

	result, err := s.callTool("catalog/products/modifiers/delete", map[string]any{
		"product_id":  float64(1),
		"modifier_id": float64(3),
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}
