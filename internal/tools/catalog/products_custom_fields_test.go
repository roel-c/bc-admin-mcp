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

type CustomFieldToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestCustomFieldToolSuite(t *testing.T) {
	suite.Run(t, new(CustomFieldToolSuite))
}

func (s *CustomFieldToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/custom_fields", "Custom Fields")
	s.prods.RegisterTools(s.reg)
	s.prods.RegisterCustomFieldTools(s.reg)
}

func (s *CustomFieldToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *CustomFieldToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CustomFieldToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *CustomFieldToolSuite) TestCustomFieldList() {
	s.mockBC.EXPECT().ListProductCustomFields(gomock.Any(), 1).Return([]bigcommerce.ProductCustomField{
		{ID: 1, Name: "Material", Value: "Cotton"},
	}, nil)

	result, err := s.callTool("catalog/products/custom_fields/list", map[string]any{
		"product_id": float64(1),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total_custom_fields"])
}

func (s *CustomFieldToolSuite) TestCustomFieldSetCreatePreview() {
	s.mockBC.EXPECT().ListProductCustomFields(gomock.Any(), 1).Return(nil, nil)

	result, err := s.callTool("catalog/products/custom_fields/set", map[string]any{
		"product_id": float64(1),
		"name":       "Origin",
		"value":      "USA",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal("create", data["action"])
}

func (s *CustomFieldToolSuite) TestCustomFieldSetUpdatePreview() {
	s.mockBC.EXPECT().ListProductCustomFields(gomock.Any(), 1).Return([]bigcommerce.ProductCustomField{
		{ID: 5, Name: "Origin", Value: "China"},
	}, nil)

	result, err := s.callTool("catalog/products/custom_fields/set", map[string]any{
		"product_id": float64(1),
		"name":       "Origin",
		"value":      "USA",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal("update", data["action"])
	s.Equal("China", data["old_value"])
}

func (s *CustomFieldToolSuite) TestCustomFieldSetCreateExecute() {
	s.mockBC.EXPECT().ListProductCustomFields(gomock.Any(), 1).Return(nil, nil)
	s.mockBC.EXPECT().CreateProductCustomField(gomock.Any(), 1, gomock.Any()).Return(&bigcommerce.ProductCustomField{
		ID: 10, Name: "Origin", Value: "USA",
	}, nil)

	result, err := s.callTool("catalog/products/custom_fields/set", map[string]any{
		"product_id": float64(1),
		"name":       "Origin",
		"value":      "USA",
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal("created", data["action"])
}

func (s *CustomFieldToolSuite) TestCustomFieldSetUpdateExecute() {
	s.mockBC.EXPECT().ListProductCustomFields(gomock.Any(), 1).Return([]bigcommerce.ProductCustomField{
		{ID: 5, Name: "Origin", Value: "China"},
	}, nil)
	s.mockBC.EXPECT().UpdateProductCustomField(gomock.Any(), 1, 5, gomock.Any()).Return(&bigcommerce.ProductCustomField{
		ID: 5, Name: "Origin", Value: "USA",
	}, nil)

	result, err := s.callTool("catalog/products/custom_fields/set", map[string]any{
		"product_id": float64(1),
		"name":       "Origin",
		"value":      "USA",
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal("updated", data["action"])
}

func (s *CustomFieldToolSuite) TestCustomFieldDeleteByName() {
	s.mockBC.EXPECT().ListProductCustomFields(gomock.Any(), 1).Return([]bigcommerce.ProductCustomField{
		{ID: 5, Name: "Origin", Value: "USA"},
	}, nil).Times(2)

	result, err := s.callTool("catalog/products/custom_fields/delete", map[string]any{
		"product_id": float64(1),
		"name":       "Origin",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal(float64(5), data["custom_field_id"])
}

func (s *CustomFieldToolSuite) TestCustomFieldDeleteExecute() {
	s.mockBC.EXPECT().DeleteProductCustomField(gomock.Any(), 1, 5).Return(nil)

	result, err := s.callTool("catalog/products/custom_fields/delete", map[string]any{
		"product_id":      float64(1),
		"custom_field_id": float64(5),
		"confirmed":       true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}
