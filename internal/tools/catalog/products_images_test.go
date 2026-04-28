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

type ImageToolSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestImageToolSuite(t *testing.T) {
	suite.Run(t, new(ImageToolSuite))
}

func (s *ImageToolSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/images", "Images")
	s.prods.RegisterTools(s.reg)
	s.prods.RegisterImageTools(s.reg)
}

func (s *ImageToolSuite) TearDownTest() { s.ctrl.Finish() }

func (s *ImageToolSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: toolPath, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *ImageToolSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *ImageToolSuite) TestImageList() {
	s.mockBC.EXPECT().ListProductImages(gomock.Any(), 1).Return([]bigcommerce.ProductImage{
		{ID: 10, ProductID: 1, IsThumbnail: true},
		{ID: 11, ProductID: 1},
	}, nil)

	result, err := s.callTool("catalog/products/images/list", map[string]any{
		"product_id": float64(1),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal(float64(2), data["total_images"])
}

func (s *ImageToolSuite) TestImageAddPreview() {
	result, err := s.callTool("catalog/products/images/add", map[string]any{
		"product_id":   float64(1),
		"image_url":    "https://example.com/img.png",
		"is_thumbnail": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *ImageToolSuite) TestImageAddExecute() {
	s.mockBC.EXPECT().CreateProductImage(gomock.Any(), 1, gomock.Any()).Return(&bigcommerce.ProductImage{
		ID: 100, ProductID: 1,
	}, nil)

	result, err := s.callTool("catalog/products/images/add", map[string]any{
		"product_id": float64(1),
		"image_url":  "https://example.com/photo.jpg",
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}

func (s *ImageToolSuite) TestImageAddInvalidURL() {
	result, err := s.callTool("catalog/products/images/add", map[string]any{
		"product_id": float64(1),
		"image_url":  "https://example.com/file.pdf",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *ImageToolSuite) TestImageDeletePreview() {
	s.mockBC.EXPECT().ListProductImages(gomock.Any(), 1).Return([]bigcommerce.ProductImage{
		{ID: 10, ProductID: 1, ImageFile: "img.jpg", IsThumbnail: true},
	}, nil)

	result, err := s.callTool("catalog/products/images/delete", map[string]any{
		"product_id": float64(1),
		"image_id":   float64(10),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *ImageToolSuite) TestImageDeleteExecute() {
	s.mockBC.EXPECT().DeleteProductImage(gomock.Any(), 1, 10).Return(nil)

	result, err := s.callTool("catalog/products/images/delete", map[string]any{
		"product_id": float64(1),
		"image_id":   float64(10),
		"confirmed":  true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
}
