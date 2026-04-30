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

type ChannelSummarySuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	cache  *session.Store
	prods  *catalog.Products
	reg    *discovery.Registry
}

func TestChannelSummarySuite(t *testing.T) {
	suite.Run(t, new(ChannelSummarySuite))
}

func (s *ChannelSummarySuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.cache = session.NewStore(60 * time.Second)
	s.prods = catalog.NewProducts(s.mockBC, s.cache)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/products", "Products")
	s.reg.RegisterCategory("catalog/products/channel_assignments", "Channel assignments")
	s.prods.RegisterTools(s.reg)
}

func (s *ChannelSummarySuite) TearDownTest() { s.ctrl.Finish() }

func (s *ChannelSummarySuite) callTool(args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool("catalog/products/channel_summary")
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: def.Path, Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *ChannelSummarySuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *ChannelSummarySuite) TestRequiresProductIDs() {
	res, err := s.callTool(map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *ChannelSummarySuite) TestCapsProductIDs() {
	res, err := s.callTool(map[string]any{
		"product_ids": []any{
			float64(1), float64(2), float64(3), float64(4), float64(5), float64(6),
		},
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *ChannelSummarySuite) TestSummaryAggregatesAssignmentsAndListings() {
	s.mockBC.EXPECT().ListStoreChannels(gomock.Any(), gomock.Any()).Return([]bigcommerce.StoreChannel{
		{ID: 1, Name: "Default", Type: "storefront", Status: "active"},
		{ID: 2, Name: "EU", Type: "storefront", Status: "active"},
		{ID: 5, Name: "Amazon", Type: "marketplace", Status: "connected"},
	}, nil)

	s.mockBC.EXPECT().
		ListProductChannelAssignments(gomock.Any(), map[string]string{"product_id:in": "100"}).
		Return([]bigcommerce.ProductChannelAssignment{
			{ProductID: 100, ChannelID: 1},
			{ProductID: 100, ChannelID: 5},
		}, nil)

	s.mockBC.EXPECT().
		ListChannelListings(gomock.Any(), 1, map[string]string{"product_id:in": "100"}).
		Return([]bigcommerce.ChannelListing{
			{ChannelID: 1, ProductID: 100, ListingID: 11, State: "active"},
		}, nil)
	s.mockBC.EXPECT().
		ListChannelListings(gomock.Any(), 5, map[string]string{"product_id:in": "100"}).
		Return([]bigcommerce.ChannelListing{
			{ChannelID: 5, ProductID: 100, ListingID: 99, State: "pending", Name: "Amazon-only override"},
		}, nil)

	res, err := s.callTool(map[string]any{
		"product_ids": []any{float64(100)},
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)

	s.Equal(float64(2), data["channels_queried"])
	products := data["products"].([]any)
	s.Require().Len(products, 1)
	prod := products[0].(map[string]any)
	s.Equal(float64(100), prod["product_id"])

	listings := prod["listings_by_channel"].(map[string]any)
	s.Contains(listings, "1")
	s.Contains(listings, "5")
	amazon := listings["5"].(map[string]any)
	s.Equal("pending", amazon["state"])
	s.Equal("Amazon-only override", amazon["name"])
}

func (s *ChannelSummarySuite) TestSummaryFlagsAssignmentWithoutListing() {
	s.mockBC.EXPECT().ListStoreChannels(gomock.Any(), gomock.Any()).Return([]bigcommerce.StoreChannel{
		{ID: 1, Name: "Default", Type: "storefront", Status: "active"},
	}, nil)
	s.mockBC.EXPECT().
		ListProductChannelAssignments(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.ProductChannelAssignment{
			{ProductID: 7, ChannelID: 1},
		}, nil)
	s.mockBC.EXPECT().
		ListChannelListings(gomock.Any(), 1, gomock.Any()).
		Return([]bigcommerce.ChannelListing{}, nil)

	res, err := s.callTool(map[string]any{"product_ids": []any{float64(7)}})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	prod := data["products"].([]any)[0].(map[string]any)
	missing := prod["channels_assigned_without_listing"].([]any)
	s.Len(missing, 1)
	s.Equal(float64(1), missing[0])
}

func (s *ChannelSummarySuite) TestSummaryFlagsListingWithoutAssignmentWhenIncluded() {
	s.mockBC.EXPECT().ListStoreChannels(gomock.Any(), gomock.Any()).Return([]bigcommerce.StoreChannel{
		{ID: 9, Name: "Legacy", Type: "storefront"},
	}, nil)
	s.mockBC.EXPECT().
		ListProductChannelAssignments(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.ProductChannelAssignment{}, nil)
	s.mockBC.EXPECT().
		ListChannelListings(gomock.Any(), 9, gomock.Any()).
		Return([]bigcommerce.ChannelListing{
			{ChannelID: 9, ProductID: 4, ListingID: 50, State: "active"},
		}, nil)

	res, err := s.callTool(map[string]any{
		"product_ids":                 []any{float64(4)},
		"include_unassigned_channels": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	prod := data["products"].([]any)[0].(map[string]any)
	stranded := prod["channels_with_listing_but_no_assignment"].([]any)
	s.Len(stranded, 1)
	s.Equal(float64(9), stranded[0])
}
