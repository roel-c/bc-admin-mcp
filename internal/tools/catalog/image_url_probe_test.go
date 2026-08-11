package catalog_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ImageProbeSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestImageProbeSuite(t *testing.T) {
	suite.Run(t, new(ImageProbeSuite))
}

func (s *ImageProbeSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.prods.RegisterTools(s.reg)
}

func (s *ImageProbeSuite) TearDownTest() { s.ctrl.Finish() }

func (s *ImageProbeSuite) callCreate(args map[string]any) *mcp.CallToolResult {
	def := s.reg.GetTool("catalog/products/create")
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: "catalog/products/create", Arguments: args}}
	result, err := def.Handler(context.Background(), req)
	s.NoError(err)
	return result
}

func (s *ImageProbeSuite) resultText(result *mcp.CallToolResult) string {
	s.Require().NotEmpty(result.Content)
	return result.Content[0].(mcp.TextContent).Text
}

func (s *ImageProbeSuite) TestCreatePreviewRejectsUnreachableImageURL() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	result := s.callCreate(map[string]any{
		"name": "Bad Image Product",
		"images": []any{
			map[string]any{"image_url": srv.URL + "/missing.jpg", "is_thumbnail": true},
		},
	})
	s.True(result.IsError)
	s.Contains(s.resultText(result), "image_url")
	s.Contains(s.resultText(result), "404")
}

func (s *ImageProbeSuite) TestCreatePreviewAcceptsReachableImageURL() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := s.callCreate(map[string]any{
		"name": "Good Image Product",
		"images": []any{
			map[string]any{"image_url": srv.URL + "/ok.jpg", "is_thumbnail": true},
		},
	})
	s.False(result.IsError)
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(s.resultText(result)), &data))
	s.Equal("pending_confirmation", data["status"])
}
