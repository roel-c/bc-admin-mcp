package catalog_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ChannelToolsSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	ct     *catalog.ChannelTools
	reg    *discovery.Registry
}

func TestChannelToolsSuite(t *testing.T) {
	suite.Run(t, new(ChannelToolsSuite))
}

func (s *ChannelToolsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.ct = catalog.NewChannelTools(s.mockBC)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/channels", "Channels")
	s.reg.RegisterCategory("catalog/channels/listings", "Listings")
	s.ct.RegisterTools(s.reg)
}

func (s *ChannelToolsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ChannelToolsSuite) callTool(args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool("catalog/channels/list")
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "catalog/channels/list", Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *ChannelToolsSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *ChannelToolsSuite) TestListSummariesMultiStorefrontSignal() {
	s.mockBC.EXPECT().ListStoreChannels(gomock.Any(), map[string]string(nil)).Return([]bigcommerce.StoreChannel{
		{ID: 1, Name: "Default", Type: "storefront", Platform: "bigcommerce", Status: "active"},
		{ID: 2, Name: "EU Storefront", Type: "storefront", Platform: "bigcommerce", Status: "active"},
		{ID: 3, Name: "Amazon", Type: "marketplace", Platform: "amazon", Status: "connected"},
	}, nil)

	result, err := s.callTool(map[string]any{})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(3), data["total"])
	s.Equal(float64(2), data["active_storefront_channel_count"])
	s.Equal(true, data["multi_storefront_likely"])
}

func (s *ChannelToolsSuite) TestListPassesTypeFilter() {
	s.mockBC.EXPECT().ListStoreChannels(gomock.Any(), map[string]string{"type": "storefront"}).Return([]bigcommerce.StoreChannel{
		{ID: 1, Name: "Default", Type: "storefront", Status: "active"},
	}, nil)

	result, err := s.callTool(map[string]any{"type": "storefront"})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
}

func (s *ChannelToolsSuite) callCategoryTreesTool(args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool("catalog/channels/category_trees")
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "catalog/channels/category_trees", Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *ChannelToolsSuite) TestCategoryTreesAllStores() {
	s.mockBC.EXPECT().ListCategoryTrees(gomock.Any(), map[string]string(nil)).Return([]bigcommerce.CategoryTree{
		{ID: 10, Name: "Default", Channels: []int{1}},
	}, nil)

	result, err := s.callCategoryTreesTool(map[string]any{})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
	trees := data["trees"].([]any)
	s.Len(trees, 1)
}

func (s *ChannelToolsSuite) TestCategoryTreesFilteredByChannel() {
	s.mockBC.EXPECT().ListCategoryTrees(gomock.Any(), map[string]string{"channel_id:in": "2"}).Return([]bigcommerce.CategoryTree{
		{ID: 20, Name: "EU", Channels: []int{2}},
	}, nil)

	result, err := s.callCategoryTreesTool(map[string]any{"channel_id": float64(2)})
	s.NoError(err)
	s.False(result.IsError)
	data := s.parseJSON(result)
	s.Equal(float64(1), data["total"])
}

func (s *ChannelToolsSuite) callListingTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: toolPath, Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *ChannelToolsSuite) TestListingsList() {
	s.mockBC.EXPECT().ListChannelListings(gomock.Any(), 5, gomock.Any()).Return([]bigcommerce.ChannelListing{
		{ChannelID: 5, ListingID: 100, ProductID: 1, State: "active"},
	}, nil)

	res, err := s.callListingTool("catalog/channels/listings/list", map[string]any{"channel_id": float64(5)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(5), data["channel_id"])
	s.Equal(float64(1), data["total"])
}

func (s *ChannelToolsSuite) TestListingsCreatePreview() {
	validJSON := `[{"product_id":1,"state":"active","variants":[{"product_id":1,"variant_id":10,"state":"active"}]}]`
	res, err := s.callListingTool("catalog/channels/listings/create", map[string]any{
		"channel_id":    float64(3),
		"listings_json": validJSON,
		"confirmed":     false,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
}

func (s *ChannelToolsSuite) TestListingsCreateMissingVariantsError() {
	res, err := s.callListingTool("catalog/channels/listings/create", map[string]any{
		"channel_id":    float64(3),
		"listings_json": `[{"product_id":1,"state":"active"}]`,
		"confirmed":     true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *ChannelToolsSuite) TestListingsUpdateConfirmed() {
	s.mockBC.EXPECT().UpdateChannelListings(gomock.Any(), 4, gomock.Any()).Return([]byte(`{"data":[]}`), nil)

	body := `[{"listing_id":99,"product_id":1,"state":"disabled","variants":[{"product_id":1,"variant_id":2,"state":"disabled"}]}]`
	res, err := s.callListingTool("catalog/channels/listings/update", map[string]any{
		"channel_id":    float64(4),
		"listings_json": body,
		"confirmed":     true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("completed", data["status"])
}

func (s *ChannelToolsSuite) TestListingsCreateRejectsInvalidState() {
	body := `[{"product_id":1,"state":"published","variants":[{"product_id":1,"variant_id":2,"state":"active"}]}]`
	res, err := s.callListingTool("catalog/channels/listings/create", map[string]any{
		"channel_id":    float64(4),
		"listings_json": body,
		"confirmed":     true,
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "is not a valid BigCommerce listing state")
}

func (s *ChannelToolsSuite) TestListingsCreateRejectsInvalidVariantState() {
	body := `[{"product_id":1,"state":"active","variants":[{"product_id":1,"variant_id":2,"state":"NOT_REAL"}]}]`
	res, err := s.callListingTool("catalog/channels/listings/create", map[string]any{
		"channel_id":    float64(4),
		"listings_json": body,
		"confirmed":     true,
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "is not a valid BigCommerce variant listing state")
}
