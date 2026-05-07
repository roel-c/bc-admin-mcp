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

type OptionToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestOptionToolSuite(t *testing.T) {
	suite.Run(t, new(OptionToolSuite))
}

func (s *OptionToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/options", "Options")
	s.prods.RegisterTools(s.reg)
	s.prods.RegisterOptionTools(s.reg)
}

func (s *OptionToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *OptionToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *OptionToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *OptionToolSuite) TestOptionList() {
	s.mockBC.EXPECT().ListProductOptions(gomock.Any(), 1).Return([]bigcommerce.ProductOption{
		{ID: 5, ProductID: 1, DisplayName: "Size", Type: "dropdown"},
	}, nil)

	result, err := s.callTool("catalog/products/options/list", map[string]any{
		"product_id": float64(1),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total_options"])
}

func (s *OptionToolSuite) TestOptionCreatePreview() {
	result, err := s.callTool("catalog/products/options/create", map[string]any{
		"product_id":   float64(1),
		"display_name": "Color",
		"type":         "swatch",
		"option_values": []any{
			map[string]any{"label": "Red"},
			map[string]any{"label": "Blue"},
		},
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *OptionToolSuite) TestOptionCreateExecute() {
	s.mockBC.EXPECT().CreateProductOption(gomock.Any(), 1, gomock.Any()).Return(&bigcommerce.ProductOption{
		ID: 10, ProductID: 1, DisplayName: "Color", Type: "swatch",
	}, nil)

	result, err := s.callTool("catalog/products/options/create", map[string]any{
		"product_id":   float64(1),
		"display_name": "Color",
		"type":         "swatch",
		"option_values": []any{
			map[string]any{"label": "Red"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *OptionToolSuite) TestOptionUpdatePreview() {
	result, err := s.callTool("catalog/products/options/update", map[string]any{
		"product_id":   float64(1),
		"option_id":    float64(5),
		"display_name": "Size Updated",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *OptionToolSuite) TestOptionDeletePreview() {
	s.mockBC.EXPECT().ListProductOptions(gomock.Any(), 1).Return([]bigcommerce.ProductOption{
		{ID: 5, ProductID: 1, DisplayName: "Size", Type: "dropdown", OptionValues: []bigcommerce.ProductOptionValue{{Label: "S"}, {Label: "M"}}},
	}, nil)

	result, err := s.callTool("catalog/products/options/delete", map[string]any{
		"product_id": float64(1),
		"option_id":  float64(5),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(2), data["value_count"])
}

func (s *OptionToolSuite) TestOptionDeleteExecute() {
	s.mockBC.EXPECT().DeleteProductOption(gomock.Any(), 1, 5).Return(nil)

	result, err := s.callTool("catalog/products/options/delete", map[string]any{
		"product_id": float64(1),
		"option_id":  float64(5),
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}
