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

type PriceListsToolsSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockBigCommerceAPI
	tools  *catalog.PriceLists
	reg    *discovery.Registry
}

func TestPriceListsToolsSuite(t *testing.T) {
	suite.Run(t, new(PriceListsToolsSuite))
}

func (s *PriceListsToolsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceAPI(s.ctrl)
	s.tools = catalog.NewPriceLists(s.mockBC)
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("catalog", "Catalog")
	s.reg.RegisterCategory("catalog/pricelists", "Price lists")
	s.reg.RegisterCategory("catalog/pricelists/records", "Price list records")
	s.reg.RegisterCategory("catalog/pricelists/assignments", "Price list assignments")
	s.tools.RegisterTools(s.reg)
}

func (s *PriceListsToolsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *PriceListsToolsSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: path, Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *PriceListsToolsSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *PriceListsToolsSuite) TestListPriceListsPassesFilters() {
	s.mockBC.EXPECT().
		ListPriceLists(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, p bigcommerce.PriceListListParams) ([]bigcommerce.PriceList, error) {
			s.Equal("whole", p.NameLike)
			s.Equal(2, p.Page)
			s.Equal(25, p.Limit)
			return []bigcommerce.PriceList{{ID: 7, Name: "Wholesale", Active: true}}, nil
		})

	res, err := s.callTool("catalog/pricelists/list", map[string]any{
		"name_like": "whole",
		"page":      float64(2),
		"limit":     float64(25),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *PriceListsToolsSuite) TestCreatePriceListPreview() {
	res, err := s.callTool("catalog/pricelists/create", map[string]any{
		"name": "B2B List",
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("pending_confirmation", data["status"])
}

func (s *PriceListsToolsSuite) TestCreatePriceListConfirmed() {
	s.mockBC.EXPECT().
		CreatePriceList(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, payload bigcommerce.PriceListCreate) (*bigcommerce.PriceList, error) {
			s.Equal("B2B List", payload.Name)
			s.NotNil(payload.Active)
			s.Equal(true, *payload.Active)
			return &bigcommerce.PriceList{ID: 3, Name: payload.Name, Active: true}, nil
		})

	res, err := s.callTool("catalog/pricelists/create", map[string]any{
		"name":      "B2B List",
		"active":    true,
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("completed", data["status"])
}

func (s *PriceListsToolsSuite) TestListPriceListsRejectsPageAndCursorMix() {
	res, err := s.callTool("catalog/pricelists/list", map[string]any{
		"page":   float64(1),
		"before": "cursor_1",
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "page cannot be combined")
}

func (s *PriceListsToolsSuite) TestUpsertRecordsRejectsFractionalVariantID() {
	res, err := s.callTool("catalog/pricelists/records/upsert", map[string]any{
		"price_list_id": float64(9),
		"records": []any{
			map[string]any{
				"variant_id": float64(12.7),
				"currency":   "usd",
				"price":      float64(19.99),
			},
		},
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "variant_id must be an integer")
}

func (s *PriceListsToolsSuite) TestRecordsListRejectsPageAndCursorMix() {
	res, err := s.callTool("catalog/pricelists/records/list", map[string]any{
		"price_list_id": float64(5),
		"page":          float64(2),
		"after":         "cursor_2",
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "page cannot be combined")
}

func (s *PriceListsToolsSuite) TestDeleteRecordsRequiresSelector() {
	res, err := s.callTool("catalog/pricelists/records/delete", map[string]any{
		"price_list_id": float64(10),
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "at least one of variant_ids or skus is required")
}

func (s *PriceListsToolsSuite) TestUpsertRecordsCap() {
	records := make([]any, 0, 101)
	for i := 1; i <= 101; i++ {
		records = append(records, map[string]any{
			"variant_id": float64(i),
			"currency":   "usd",
			"price":      float64(9.99),
		})
	}

	res, err := s.callTool("catalog/pricelists/records/upsert", map[string]any{
		"price_list_id": float64(9),
		"records":       records,
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "maximum 100 rows")
}

func (s *PriceListsToolsSuite) TestCreateAssignmentsBatchCap() {
	rows := make([]any, 0, 26)
	for i := 1; i <= 26; i++ {
		rows = append(rows, map[string]any{
			"price_list_id":     float64(3),
			"customer_group_id": float64(i),
		})
	}

	res, err := s.callTool("catalog/pricelists/assignments/create_batch", map[string]any{
		"assignments": rows,
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "maximum 25 rows")
}

func (s *PriceListsToolsSuite) TestAssignmentsListRejectsPageAndCursorMix() {
	res, err := s.callTool("catalog/pricelists/assignments/list", map[string]any{
		"page":  float64(1),
		"after": "cursor_abc",
	})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "page cannot be combined")
}

func (s *PriceListsToolsSuite) TestDeleteAssignmentsRequiresFilter() {
	res, err := s.callTool("catalog/pricelists/assignments/delete", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
	text := res.Content[0].(mcp.TextContent).Text
	s.Contains(text, "at least one filter is required")
}

func (s *PriceListsToolsSuite) TestUpsertAssignmentConfirmed() {
	s.mockBC.EXPECT().
		UpsertPriceListAssignment(gomock.Any(), 5, bigcommerce.PriceListAssignmentUpsert{
			CustomerGroupID: 7,
			ChannelID:       2,
		}).
		Return(&bigcommerce.PriceListAssignment{
			ID:              99,
			PriceListID:     5,
			CustomerGroupID: 7,
			ChannelID:       2,
		}, nil)

	res, err := s.callTool("catalog/pricelists/assignments/upsert", map[string]any{
		"price_list_id":     float64(5),
		"customer_group_id": float64(7),
		"channel_id":        float64(2),
		"confirmed":         true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("completed", data["status"])
}
