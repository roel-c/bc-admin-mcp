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

type BrandFilterTableSuite struct {
	suite.Suite
}

func TestBrandFilterTableSuite(t *testing.T) {
	suite.Run(t, new(BrandFilterTableSuite))
}

func (s *BrandFilterTableSuite) TestAllEntriesHaveNonEmptyKeys() {
	for _, f := range catalog.BrandSearchFilters {
		s.NotEmpty(f.ToolKey)
		s.NotEmpty(f.BCKey, "BCKey for %s", f.ToolKey)
	}
}

func (s *BrandFilterTableSuite) TestAllEntriesHaveValidKind() {
	validKinds := map[string]bool{"string": true, "number": true, "bool": true}
	for _, f := range catalog.BrandSearchFilters {
		s.True(validKinds[f.Kind], "invalid Kind %q for %s", f.Kind, f.ToolKey)
	}
}

func (s *BrandFilterTableSuite) TestNoDuplicateToolKeys() {
	seen := make(map[string]bool)
	for _, f := range catalog.BrandSearchFilters {
		s.False(seen[f.ToolKey], "duplicate ToolKey: %s", f.ToolKey)
		seen[f.ToolKey] = true
	}
}

func (s *BrandFilterTableSuite) TestNoDuplicateBCKeys() {
	seen := make(map[string]bool)
	for _, f := range catalog.BrandSearchFilters {
		s.False(seen[f.BCKey], "duplicate BCKey: %s", f.BCKey)
		seen[f.BCKey] = true
	}
}

type BrandExtractFiltersSuite struct {
	suite.Suite
}

func TestBrandExtractFiltersSuite(t *testing.T) {
	suite.Run(t, new(BrandExtractFiltersSuite))
}

func (s *BrandExtractFiltersSuite) TestNameLikeExtraction() {
	args := map[string]any{"name_like": "Acme"}
	params, err := catalog.ExtractFilters(args, catalog.BrandSearchFilters)
	s.NoError(err)
	s.Equal("Acme", params["name:like"])
}

func (s *BrandExtractFiltersSuite) TestSortParamsNotCountedAsSoleFilter() {
	args := map[string]any{"sort": "name", "sort_direction": "asc"}
	params, err := catalog.ExtractFilters(args, catalog.BrandSearchFilters)
	s.NoError(err)
	s.Equal("name", params["sort"])
	s.Equal("asc", params["direction"])
}

type BrandCreateParamsSuite struct {
	suite.Suite
}

func TestBrandCreateParamsSuite(t *testing.T) {
	suite.Run(t, new(BrandCreateParamsSuite))
}

func (s *BrandCreateParamsSuite) TestMinimalValid() {
	p, err := catalog.ParseBrandCreateParams(map[string]any{"name": "Acme"})
	s.NoError(err)
	s.Equal("Acme", p.Payload.Name)
	s.False(p.Confirmed)
}

func (s *BrandCreateParamsSuite) TestCustomURLSetsPayload() {
	p, err := catalog.ParseBrandCreateParams(map[string]any{
		"name":       "Acme",
		"custom_url": "/brands/acme/",
	})
	s.NoError(err)
	s.Require().NotNil(p.Payload.CustomURL)
	s.Equal("/brands/acme/", p.Payload.CustomURL.URL)
}

func (s *BrandCreateParamsSuite) TestMissingNameReturnsError() {
	_, err := catalog.ParseBrandCreateParams(map[string]any{"page_title": "x"})
	s.Error(err)
	s.Contains(err.Error(), "name is required")
}

type BrandUpdateParamsSuite struct {
	suite.Suite
}

func TestBrandUpdateParamsSuite(t *testing.T) {
	suite.Run(t, new(BrandUpdateParamsSuite))
}

func (s *BrandUpdateParamsSuite) TestRequiresPositiveBrandID() {
	_, err := catalog.ParseBrandUpdateParams(map[string]any{
		"brand_id": float64(0),
		"name":     "X",
	})
	s.Error(err)
	s.Contains(err.Error(), "positive")
}

func (s *BrandUpdateParamsSuite) TestPageTitlePointer() {
	p, err := catalog.ParseBrandUpdateParams(map[string]any{
		"brand_id":   float64(5),
		"page_title": "New title",
	})
	s.NoError(err)
	s.Equal(5, p.BrandID)
	s.Require().NotNil(p.Update.PageTitle)
	s.Equal("New title", *p.Update.PageTitle)
}

type BrandHandlerSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	brands *catalog.Brands
	reg    *discovery.Registry
}

func TestBrandHandlerSuite(t *testing.T) {
	suite.Run(t, new(BrandHandlerSuite))
}

func (s *BrandHandlerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.brands = catalog.NewBrands(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/brands", "Brands")
	s.reg.RegisterCategory("catalog/brands/metafields", "Brand metafields")
	s.brands.RegisterTools(s.reg)
}

func (s *BrandHandlerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *BrandHandlerSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found", toolPath)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      toolPath,
			Arguments: args,
		},
	}
	return def.Handler(context.Background(), req)
}

func (s *BrandHandlerSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *BrandHandlerSuite) TestListRequiresFilterOrListAll() {
	result, err := s.callTool("catalog/brands/list", map[string]any{})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *BrandHandlerSuite) TestListAllCallsSearch() {
	s.mockBC.EXPECT().SearchBrands(gomock.Any(), gomock.Any()).Return([]bigcommerce.Brand{
		{ID: 1, Name: "Acme"},
	}, nil)

	result, err := s.callTool("catalog/brands/list", map[string]any{"list_all": true})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
}

func (s *BrandHandlerSuite) TestListRejectsInvalidSort() {
	result, err := s.callTool("catalog/brands/list", map[string]any{
		"name_like": "x",
		"sort":      "invalid_sort",
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *BrandHandlerSuite) TestGetReturnsBrand() {
	s.mockBC.EXPECT().GetBrand(gomock.Any(), 7).Return(&bigcommerce.Brand{
		ID: 7, Name: "Acme",
	}, nil)

	result, err := s.callTool("catalog/brands/get", map[string]any{"brand_id": float64(7)})
	s.NoError(err)
	s.False(result.IsError)
}

func (s *BrandHandlerSuite) TestCreatePreview() {
	result, err := s.callTool("catalog/brands/create", map[string]any{"name": "NewCo"})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
	brand := data["brand"].(map[string]any)
	s.Equal("NewCo", brand["name"])
}

func (s *BrandHandlerSuite) TestCreateExecute() {
	s.mockBC.EXPECT().CreateBrand(gomock.Any(), gomock.Any()).Return(&bigcommerce.Brand{
		ID: 100, Name: "NewCo",
	}, nil)

	result, err := s.callTool("catalog/brands/create", map[string]any{
		"name":      "NewCo",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
}

func (s *BrandHandlerSuite) TestUpdateRequiresField() {
	result, err := s.callTool("catalog/brands/update", map[string]any{
		"brand_id": float64(1),
	})
	s.NoError(err)
	s.True(result.IsError)
}

func (s *BrandHandlerSuite) TestUpdatePreview() {
	result, err := s.callTool("catalog/brands/update", map[string]any{
		"brand_id":   float64(3),
		"page_title": "T",
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal("preview", data["status"])
}

func (s *BrandHandlerSuite) TestUpdateExecute() {
	s.mockBC.EXPECT().UpdateBrand(gomock.Any(), 3, gomock.Any()).Return(&bigcommerce.Brand{
		ID: 3, Name: "Acme",
	}, nil)

	result, err := s.callTool("catalog/brands/update", map[string]any{
		"brand_id":  float64(3),
		"name":      "Acme Renamed",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("updated", data["status"])
}

func (s *BrandHandlerSuite) TestMetafieldsListByBrandID() {
	s.mockBC.EXPECT().ListBrandMetafields(gomock.Any(), 8).Return([]bigcommerce.Metafield{
		{ID: 1, Namespace: "ns", Key: "k", Value: "v"},
	}, nil)

	result, err := s.callTool("catalog/brands/metafields/list", map[string]any{
		"brand_id": float64(8),
	})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(8), data["brand_id"])
	s.Equal(float64(1), data["total"])
}

func (s *BrandHandlerSuite) TestMetafieldsListResolvesBrandName() {
	s.mockBC.EXPECT().SearchBrands(gomock.Any(), map[string]string{"name": "Acme"}).
		Return([]bigcommerce.Brand{{ID: 20, Name: "Acme"}}, nil)
	s.mockBC.EXPECT().ListBrandMetafields(gomock.Any(), 20).Return(nil, nil)

	result, err := s.callTool("catalog/brands/metafields/list", map[string]any{
		"brand_name": "Acme",
	})
	s.NoError(err)
	s.False(result.IsError)
}

func (s *BrandHandlerSuite) TestMetafieldsSetPreview() {
	s.mockBC.EXPECT().ListBrandMetafields(gomock.Any(), 1).Return(nil, nil)

	result, err := s.callTool("catalog/brands/metafields/set", map[string]any{
		"brand_id":  float64(1),
		"namespace": "my_app",
		"key":       "flag",
		"value":     "1",
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
	s.Equal("create", data["action"])
}

func (s *BrandHandlerSuite) TestMetafieldsSetExecuteCreates() {
	s.mockBC.EXPECT().ListBrandMetafields(gomock.Any(), 1).Return(nil, nil)
	s.mockBC.EXPECT().CreateBrandMetafield(gomock.Any(), 1, gomock.Any()).Return(&bigcommerce.Metafield{
		ID: 50, Namespace: "my_app", Key: "flag", Value: "1",
	}, nil)

	result, err := s.callTool("catalog/brands/metafields/set", map[string]any{
		"brand_id":  float64(1),
		"namespace": "my_app",
		"key":       "flag",
		"value":     "1",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("created", data["status"])
}

func (s *BrandHandlerSuite) TestMetafieldsSetExecuteUpdates() {
	existing := []bigcommerce.Metafield{{ID: 77, Namespace: "my_app", Key: "flag", Value: "0"}}
	s.mockBC.EXPECT().ListBrandMetafields(gomock.Any(), 1).Return(existing, nil)
	s.mockBC.EXPECT().UpdateBrandMetafield(gomock.Any(), 1, 77, gomock.Any()).Return(&bigcommerce.Metafield{
		ID: 77, Namespace: "my_app", Key: "flag", Value: "1",
	}, nil)

	result, err := s.callTool("catalog/brands/metafields/set", map[string]any{
		"brand_id":  float64(1),
		"namespace": "my_app",
		"key":       "flag",
		"value":     "1",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("updated", data["status"])
}

func (s *BrandHandlerSuite) TestMetafieldsDeletePreview() {
	result, err := s.callTool("catalog/brands/metafields/delete", map[string]any{
		"brand_id":     float64(2),
		"metafield_id": float64(9),
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("pending_confirmation", data["status"])
}

func (s *BrandHandlerSuite) TestMetafieldsDeleteExecute() {
	s.mockBC.EXPECT().DeleteBrandMetafield(gomock.Any(), 2, 9).Return(nil)

	result, err := s.callTool("catalog/brands/metafields/delete", map[string]any{
		"brand_id":     float64(2),
		"metafield_id": float64(9),
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(result)
	s.Equal("deleted", data["status"])
}
