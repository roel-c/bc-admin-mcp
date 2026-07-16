package b2b_test

import (
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"go.uber.org/mock/gomock"
)

// --- b2b/channels ---

func (s *B2BCompanyToolsSuite) TestChannelListReturnsChannels() {
	s.mockBC.EXPECT().ListB2BChannels(gomock.Any()).Return([]bigcommerce.B2BChannel{
		{ID: 1, ChannelID: 1741970, Name: "MSF-B2BE", Type: "storefront", Status: "active"},
	}, nil)

	res, err := s.callTool("b2b/channels/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestChannelGetReturnsChannel() {
	s.mockBC.EXPECT().GetB2BChannel(gomock.Any(), 1741970).Return(&bigcommerce.B2BChannel{ID: 1, ChannelID: 1741970, Name: "MSF-B2BE"}, nil)

	res, err := s.callTool("b2b/channels/get", map[string]any{"channel_id": float64(1741970)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	ch := data["channel"].(map[string]any)
	s.Equal("MSF-B2BE", ch["name"])
}

// --- b2b/orders ---

func (s *B2BCompanyToolsSuite) TestOrderGetReturnsOrder() {
	s.mockBC.EXPECT().GetB2BOrder(gomock.Any(), 105).Return(map[string]any{"bcOrderId": 105, "poNumber": "PO-1"}, nil)

	res, err := s.callTool("b2b/orders/get", map[string]any{"bc_order_id": float64(105)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["order"])
}

func (s *B2BCompanyToolsSuite) TestOrderUpdatePreviewThenConfirm() {
	prev, err := s.callTool("b2b/orders/update", map[string]any{
		"bc_order_id": float64(105),
		"po_number":   "PO-9",
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().UpdateB2BOrder(gomock.Any(), 105, gomock.Any()).Return(map[string]any{"bcOrderId": 105}, nil)
	res, err := s.callTool("b2b/orders/update", map[string]any{
		"bc_order_id": float64(105),
		"po_number":   "PO-9",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestOrderUpdateRejectsNoFields() {
	res, err := s.callTool("b2b/orders/update", map[string]any{
		"bc_order_id": float64(105),
		"confirmed":   true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestOrderAssignCustomerConfirmed() {
	s.mockBC.EXPECT().AssignCustomerOrdersToCompany(gomock.Any(), 34).Return(nil)
	res, err := s.callTool("b2b/orders/assign_customer_orders", map[string]any{
		"customer_id": float64(34),
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("assigned", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestOrderExtraFieldsList() {
	s.mockBC.EXPECT().ListB2BOrderExtraFields(gomock.Any(), gomock.Any()).Return([]bigcommerce.B2BExtraFieldDef{
		{FieldName: "Cost Center", FieldType: "0"},
	}, nil)

	res, err := s.callTool("b2b/orders/extra_fields", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}
