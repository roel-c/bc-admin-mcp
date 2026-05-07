package customers_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/tools/customers"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type CustomerSegmentsSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	mockBC   *MockBigCommerceCustomersAPI
	registry *discovery.Registry
}

func TestCustomerSegmentsSuite(t *testing.T) {
	suite.Run(t, new(CustomerSegmentsSuite))
}

func (s *CustomerSegmentsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockBigCommerceCustomersAPI(s.ctrl)
	s.registry = discovery.NewRegistry()
	s.registry.RegisterCategory("customers", "Customers")
	s.registry.RegisterCategory("customers/segments", "Segments")
	s.registry.RegisterCategory("customers/segments/shoppers", "Shoppers in segment")
	s.registry.RegisterCategory("customers/shopper_profiles", "Shopper profiles")

	customers.NewCustomerSegments(s.mockBC).RegisterTools(s.registry)
	customers.NewShopperProfiles(s.mockBC).RegisterTools(s.registry)
}

func (s *CustomerSegmentsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CustomerSegmentsSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.registry.GetTool(path)
	s.Require().NotNil(def, "tool %s not registered", path)
	req := mcp.CallToolRequest{Params: mcp.CallToolParams{Name: path, Arguments: args}}
	return def.Handler(context.Background(), req)
}

func (s *CustomerSegmentsSuite) parseJSON(res *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(res)
	s.Require().False(res.IsError, "unexpected tool error: %s", textOf(res))
	var m map[string]any
	s.Require().NoError(json.Unmarshal([]byte(textOf(res)), &m))
	return m
}

func textOf(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if t, ok := res.Content[0].(mcp.TextContent); ok {
		return t.Text
	}
	return ""
}

// ---------------------------------------------------------------------------
// segments/list and get
// ---------------------------------------------------------------------------

func (s *CustomerSegmentsSuite) TestListSegmentsPassesIDsAsIDIn() {
	s.mockBC.EXPECT().
		SearchSegments(gomock.Any(), map[string]string{"id:in": "uuid-a,uuid-b"}).
		Return([]bigcommerce.Segment{{ID: "uuid-a", Name: "VIP"}}, nil)

	res, err := s.callTool("customers/segments/list", map[string]any{
		"segment_ids": []any{"uuid-a", "uuid-b"},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *CustomerSegmentsSuite) TestGetSegmentDelegatesToIDIn() {
	s.mockBC.EXPECT().
		GetSegmentsByIDs(gomock.Any(), []string{"uuid-a"}).
		Return([]bigcommerce.Segment{{ID: "uuid-a", Name: "VIP"}}, nil)

	res, err := s.callTool("customers/segments/get", map[string]any{"segment_id": "uuid-a"})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("uuid-a", data["id"])
	s.Equal("VIP", data["name"])
}

func (s *CustomerSegmentsSuite) TestGetSegmentRejectsEmptyID() {
	res, err := s.callTool("customers/segments/get", map[string]any{"segment_id": "  "})
	s.NoError(err)
	s.True(res.IsError)
}

// ---------------------------------------------------------------------------
// segments/create
// ---------------------------------------------------------------------------

func (s *CustomerSegmentsSuite) TestCreateSegmentPreviewBlocksWithoutConfirm() {
	res, err := s.callTool("customers/segments/create", map[string]any{
		"name":        "VIP",
		"description": "high value",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	payload := data["payload"].([]any)
	s.Len(payload, 1)
}

func (s *CustomerSegmentsSuite) TestCreateSegmentRejectsMissingName() {
	res, err := s.callTool("customers/segments/create", map[string]any{
		"segments_batch": []any{map[string]any{"description": "no name"}},
		"confirmed":      true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerSegmentsSuite) TestCreateSegmentEnforcesBatchCap() {
	rows := make([]any, 11)
	for i := range rows {
		rows[i] = map[string]any{"name": "x"}
	}
	res, err := s.callTool("customers/segments/create", map[string]any{
		"segments_batch": rows,
		"confirmed":      true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerSegmentsSuite) TestCreateSegmentExecutesOnConfirm() {
	s.mockBC.EXPECT().
		CreateSegments(gomock.Any(), []bigcommerce.SegmentCreate{{Name: "VIP"}}).
		Return([]bigcommerce.Segment{{ID: "uuid-a", Name: "VIP"}}, nil)

	res, err := s.callTool("customers/segments/create", map[string]any{
		"name":      "VIP",
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

// ---------------------------------------------------------------------------
// segments/update
// ---------------------------------------------------------------------------

func (s *CustomerSegmentsSuite) TestUpdateSegmentRequiresIDPerRow() {
	res, err := s.callTool("customers/segments/update", map[string]any{
		"segments_batch": []any{map[string]any{"name": "no id"}},
		"confirmed":      true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerSegmentsSuite) TestUpdateSegmentRequiresAtLeastOneField() {
	res, err := s.callTool("customers/segments/update", map[string]any{
		"segments_batch": []any{map[string]any{"id": "uuid-a"}},
		"confirmed":      true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerSegmentsSuite) TestUpdateSegmentPreviewFetchesCurrent() {
	s.mockBC.EXPECT().
		GetSegmentsByIDs(gomock.Any(), []string{"uuid-a"}).
		Return([]bigcommerce.Segment{{ID: "uuid-a", Name: "Old"}}, nil)

	res, err := s.callTool("customers/segments/update", map[string]any{
		"segments_batch": []any{map[string]any{"id": "uuid-a", "name": "New"}},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

// ---------------------------------------------------------------------------
// segments/delete
// ---------------------------------------------------------------------------

func (s *CustomerSegmentsSuite) TestDeleteSegmentPreviewLooksUpExisting() {
	s.mockBC.EXPECT().
		GetSegmentsByIDs(gomock.Any(), []string{"uuid-a"}).
		Return([]bigcommerce.Segment{{ID: "uuid-a", Name: "VIP"}}, nil)

	res, err := s.callTool("customers/segments/delete", map[string]any{
		"segment_ids": []any{"uuid-a"},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal(float64(1), data["would_delete"])
}

func (s *CustomerSegmentsSuite) TestDeleteSegmentExecutesOnConfirm() {
	s.mockBC.EXPECT().
		DeleteSegments(gomock.Any(), []string{"uuid-a"}).
		Return(nil)

	res, err := s.callTool("customers/segments/delete", map[string]any{
		"segment_ids": []any{"uuid-a"},
		"confirmed":   true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
}

// ---------------------------------------------------------------------------
// segments/shoppers/* (membership)
// ---------------------------------------------------------------------------

func (s *CustomerSegmentsSuite) TestListShoppersInSegment() {
	s.mockBC.EXPECT().
		ListShopperProfilesInSegment(gomock.Any(), "uuid-a").
		Return([]bigcommerce.ShopperProfile{{ID: "p1", CustomerID: 7}}, nil)

	res, err := s.callTool("customers/segments/shoppers/list", map[string]any{"segment_id": "uuid-a"})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *CustomerSegmentsSuite) TestAddShoppersResolvesCustomerIDs() {
	s.mockBC.EXPECT().
		SearchCustomers(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params map[string]string) ([]bigcommerce.Customer, error) {
			s.Equal("shopper_profile_id", params["include"])
			s.Equal("7,8", params["id:in"])
			return []bigcommerce.Customer{
				{ID: 7, ShopperProfileID: "p7"},
				{ID: 8, ShopperProfileID: ""}, // no profile yet
			}, nil
		})

	res, err := s.callTool("customers/segments/shoppers/add", map[string]any{
		"segment_id":   "uuid-a",
		"customer_ids": []any{float64(7), float64(8)},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal(float64(1), data["count"])
	missing := data["missing_shopper_profiles"].([]any)
	s.Equal([]any{float64(8)}, missing)
}

func (s *CustomerSegmentsSuite) TestAddShoppersErrorsWhenNoneResolved() {
	s.mockBC.EXPECT().
		SearchCustomers(gomock.Any(), gomock.Any()).
		Return([]bigcommerce.Customer{{ID: 8, ShopperProfileID: ""}}, nil)

	res, err := s.callTool("customers/segments/shoppers/add", map[string]any{
		"segment_id":   "uuid-a",
		"customer_ids": []any{float64(8)},
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerSegmentsSuite) TestAddShoppersExecutesOnConfirm() {
	s.mockBC.EXPECT().
		AddShopperProfilesToSegment(gomock.Any(), "uuid-a", []string{"p1", "p2"}).
		Return([]bigcommerce.ShopperProfile{{ID: "p1"}, {ID: "p2"}}, nil)

	res, err := s.callTool("customers/segments/shoppers/add", map[string]any{
		"segment_id":          "uuid-a",
		"shopper_profile_ids": []any{"p1", "p2"},
		"confirmed":           true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("added", data["status"])
}

func (s *CustomerSegmentsSuite) TestAddShoppersDeduplicatesProfileIDs() {
	s.mockBC.EXPECT().
		AddShopperProfilesToSegment(gomock.Any(), "uuid-a", []string{"p1"}).
		Return([]bigcommerce.ShopperProfile{{ID: "p1"}}, nil)

	res, err := s.callTool("customers/segments/shoppers/add", map[string]any{
		"segment_id":          "uuid-a",
		"shopper_profile_ids": []any{"p1", "p1", " p1 "},
		"confirmed":           true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("added", data["status"])
}

func (s *CustomerSegmentsSuite) TestAddShoppersEnforcesBatchCap() {
	ids := make([]any, 51)
	for i := range ids {
		ids[i] = "p"
	}
	for i := range ids {
		ids[i] = "p" + string(rune('a'+(i%26))) + "-" + string(rune('A'+(i/26)))
	}
	res, err := s.callTool("customers/segments/shoppers/add", map[string]any{
		"segment_id":          "uuid-a",
		"shopper_profile_ids": ids,
		"confirmed":           true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CustomerSegmentsSuite) TestRemoveShoppersExecutesOnConfirm() {
	s.mockBC.EXPECT().
		RemoveShopperProfilesFromSegment(gomock.Any(), "uuid-a", []string{"p1", "p2"}).
		Return(nil)

	res, err := s.callTool("customers/segments/shoppers/remove", map[string]any{
		"segment_id":          "uuid-a",
		"shopper_profile_ids": []any{"p1", "p2"},
		"confirmed":           true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("removed", data["status"])
}

// ---------------------------------------------------------------------------
// shopper_profiles/*
// ---------------------------------------------------------------------------

func (s *CustomerSegmentsSuite) TestListShopperProfilesAllowsEmptyParams() {
	s.mockBC.EXPECT().
		ListShopperProfiles(gomock.Any(), map[string]string{}).
		Return([]bigcommerce.ShopperProfile{{ID: "p1", CustomerID: 7}}, nil)

	res, err := s.callTool("customers/shopper_profiles/list", map[string]any{})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *CustomerSegmentsSuite) TestCreateShopperProfilesPreviewDedupes() {
	res, err := s.callTool("customers/shopper_profiles/create", map[string]any{
		"customer_ids": []any{float64(1), float64(1), float64(2)},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal(float64(2), data["count"])
}

func (s *CustomerSegmentsSuite) TestCreateShopperProfilesExecutesOnConfirm() {
	s.mockBC.EXPECT().
		CreateShopperProfiles(gomock.Any(), []bigcommerce.ShopperProfileCreate{{CustomerID: 1}}).
		Return([]bigcommerce.ShopperProfile{{ID: "p1", CustomerID: 1}}, nil)

	res, err := s.callTool("customers/shopper_profiles/create", map[string]any{
		"customer_ids": []any{float64(1)},
		"confirmed":    true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

func (s *CustomerSegmentsSuite) TestDeleteShopperProfilesPreview() {
	res, err := s.callTool("customers/shopper_profiles/delete", map[string]any{
		"shopper_profile_ids": []any{"p1", "p2"},
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CustomerSegmentsSuite) TestDeleteShopperProfilesExecutes() {
	s.mockBC.EXPECT().
		DeleteShopperProfiles(gomock.Any(), []string{"p1"}).
		Return(nil)

	res, err := s.callTool("customers/shopper_profiles/delete", map[string]any{
		"shopper_profile_ids": []any{"p1"},
		"confirmed":           true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
}

func (s *CustomerSegmentsSuite) TestListSegmentsForShopperProfile() {
	s.mockBC.EXPECT().
		ListSegmentsForShopperProfile(gomock.Any(), "p1").
		Return([]bigcommerce.Segment{{ID: "uuid-a", Name: "VIP"}}, nil)

	res, err := s.callTool("customers/shopper_profiles/list_segments", map[string]any{
		"shopper_profile_id": "p1",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}
