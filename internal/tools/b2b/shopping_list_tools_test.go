package b2b_test

import (
	"go.uber.org/mock/gomock"
)

// --- b2b/shopping_lists ---

func (s *B2BCompanyToolsSuite) TestShoppingListListReturnsLists() {
	s.mockBC.EXPECT().ListB2BShoppingLists(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"id": float64(1), "name": "My List"},
	}, nil)

	res, err := s.callTool("b2b/shopping_lists/list", map[string]any{"user_id": float64(41)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestShoppingListListRejectsNoOwner() {
	res, err := s.callTool("b2b/shopping_lists/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestShoppingListListRejectsBothOwners() {
	res, err := s.callTool("b2b/shopping_lists/list", map[string]any{"user_id": float64(41), "customer_id": float64(34)})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestShoppingListGetReturnsDetail() {
	s.mockBC.EXPECT().GetB2BShoppingList(gomock.Any(), 1, gomock.Any()).Return(map[string]any{"id": float64(1)}, nil)

	res, err := s.callTool("b2b/shopping_lists/get", map[string]any{"shopping_list_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["shopping_list"])
}

func (s *B2BCompanyToolsSuite) TestShoppingListCreatePreviewThenConfirm() {
	prev, err := s.callTool("b2b/shopping_lists/create", map[string]any{
		"name": "Reorder List", "user_id": float64(41),
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().CreateB2BShoppingList(gomock.Any(), gomock.Any()).Return(map[string]any{"id": float64(5)}, nil)
	res, err := s.callTool("b2b/shopping_lists/create", map[string]any{
		"name": "Reorder List", "user_id": float64(41), "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("created", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestShoppingListCreateRejectsNoName() {
	res, err := s.callTool("b2b/shopping_lists/create", map[string]any{"user_id": float64(41), "confirmed": true})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestShoppingListUpdateConfirmed() {
	s.mockBC.EXPECT().UpdateB2BShoppingList(gomock.Any(), 5, gomock.Any()).Return(map[string]any{"id": float64(5)}, nil)
	res, err := s.callTool("b2b/shopping_lists/update", map[string]any{
		"shopping_list_id": float64(5), "name": "Renamed List", "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestShoppingListUpdateRejectsNoFields() {
	res, err := s.callTool("b2b/shopping_lists/update", map[string]any{"shopping_list_id": float64(5), "confirmed": true})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestShoppingListDeletePreviewThenConfirm() {
	prev, err := s.callTool("b2b/shopping_lists/delete", map[string]any{"shopping_list_id": float64(5)})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().DeleteB2BShoppingList(gomock.Any(), 5, 0).Return(nil)
	res, err := s.callTool("b2b/shopping_lists/delete", map[string]any{"shopping_list_id": float64(5), "confirmed": true})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("deleted", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestShoppingListItemRemoveConfirmed() {
	s.mockBC.EXPECT().DeleteB2BShoppingListItem(gomock.Any(), 5, 10).Return(nil)
	res, err := s.callTool("b2b/shopping_lists/items/remove", map[string]any{
		"shopping_list_id": float64(5), "item_id": float64(10), "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("removed", s.parseJSON(res)["status"])
}
