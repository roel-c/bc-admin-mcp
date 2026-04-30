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

type VariantGlobalFilterTableSuite struct {
	suite.Suite
}

func TestVariantGlobalFilterTableSuite(t *testing.T) {
	suite.Run(t, new(VariantGlobalFilterTableSuite))
}

func (s *VariantGlobalFilterTableSuite) TestNoDuplicateBCKeys() {
	seen := make(map[string]bool)
	for _, f := range catalog.VariantGlobalSearchFilters {
		s.False(seen[f.BCKey], "duplicate BCKey %s", f.BCKey)
		seen[f.BCKey] = true
	}
}

type GlobalVariantHandlerSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	gv     *catalog.GlobalVariants
	reg    *discovery.Registry
}

func TestGlobalVariantHandlerSuite(t *testing.T) {
	suite.Run(t, new(GlobalVariantHandlerSuite))
}

func (s *GlobalVariantHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.gv = catalog.NewGlobalVariants(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/variants", "Global variants")
	s.gv.RegisterTools(s.reg)
}

func (s *GlobalVariantHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *GlobalVariantHandlerSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: toolPath, Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *GlobalVariantHandlerSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *GlobalVariantHandlerSuite) TestListRequiresFilterOrListAll() {
	result, err := s.callTool("catalog/variants/list", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *GlobalVariantHandlerSuite) TestListByProductIDs() {
	s.mockBC.EXPECT().SearchVariants(gomock.Any(), gomock.Any()).Return([]bigcommerce.Variant{
		{ID: 1, ProductID: 10, SKU: "A", Price: 9.99},
	}, nil)

	result, err := s.callTool("catalog/variants/list", map[string]any{
		"product_ids": []any{float64(10)},
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
}

func (s *GlobalVariantHandlerSuite) TestListRejectsInvalidSort() {
	result, err := s.callTool("catalog/variants/list", map[string]any{
		"sku":  "x",
		"sort": "not_a_field",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *GlobalVariantHandlerSuite) TestBulkPreview() {
	result, err := s.callTool("catalog/variants/bulk_update", map[string]any{
		"updates": []any{
			map[string]any{"variant_id": float64(1), "price": float64(19.99)},
		},
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *GlobalVariantHandlerSuite) TestBulkExecute() {
	s.mockBC.EXPECT().BatchUpdateVariants(gomock.Any(), gomock.Any()).Return(&bigcommerce.BatchResult{
		Succeeded: 1,
		Failed:    0,
	}, nil)

	result, err := s.callTool("catalog/variants/bulk_update", map[string]any{
		"updates": []any{
			map[string]any{"variant_id": float64(1), "price": float64(19.99)},
		},
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("completed", data["status"])
	s.Equal(float64(1), data["succeeded"])
}

func (s *GlobalVariantHandlerSuite) TestBulkRejectsEmptyChangeRow() {
	result, err := s.callTool("catalog/variants/bulk_update", map[string]any{
		"updates": []any{
			map[string]any{"variant_id": float64(1)},
		},
	})
	s.NoError(err)
	s.True(result.IsError)
}
