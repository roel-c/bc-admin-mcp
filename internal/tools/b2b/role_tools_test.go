package b2b_test

import (
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"go.uber.org/mock/gomock"
)

// --- b2b/companies/roles ---

func (s *B2BCompanyToolsSuite) TestRoleListReturnsRoles() {
	s.mockBC.EXPECT().ListB2BRoles(gomock.Any(), gomock.Any()).Return([]bigcommerce.B2BRole{
		{ID: 1, Name: "Custom Buyer", RoleType: "2", Permissions: []bigcommerce.B2BRolePermission{{Code: "get_orders", PermissionLevel: "2"}}},
	}, nil)

	res, err := s.callTool("b2b/companies/roles/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestRoleGetReturnsRole() {
	s.mockBC.EXPECT().GetB2BRole(gomock.Any(), 5).Return(&bigcommerce.B2BRole{ID: 5, Name: "Buyer", RoleType: "1"}, nil)

	res, err := s.callTool("b2b/companies/roles/get", map[string]any{"role_id": float64(5)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	role := data["role"].(map[string]any)
	s.Equal("Buyer", role["name"])
}

func (s *B2BCompanyToolsSuite) TestRoleCreatePreviewThenConfirm() {
	// Preview (no confirm) — should not call the API.
	prev, err := s.callTool("b2b/companies/roles/create", map[string]any{
		"name":             "Custom Buyer",
		"permissions_json": `[{"code":"get_orders","permissionLevel":"2"}]`,
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	// Confirm.
	s.mockBC.EXPECT().CreateB2BRole(gomock.Any(), gomock.Any()).Return(&bigcommerce.B2BRole{ID: 9, Name: "Custom Buyer", RoleType: "2"}, nil)
	res, err := s.callTool("b2b/companies/roles/create", map[string]any{
		"name":             "Custom Buyer",
		"permissions_json": `[{"code":"get_orders","permissionLevel":"2"}]`,
		"confirmed":        true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("created", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestRoleCreateRejectsEmptyPermissions() {
	res, err := s.callTool("b2b/companies/roles/create", map[string]any{
		"name":             "Custom Buyer",
		"permissions_json": `[]`,
		"confirmed":        true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestRoleDeleteConfirmed() {
	s.mockBC.EXPECT().DeleteB2BRole(gomock.Any(), 9).Return(nil)
	res, err := s.callTool("b2b/companies/roles/delete", map[string]any{"role_id": float64(9), "confirmed": true})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("deleted", s.parseJSON(res)["status"])
}

// --- b2b/companies/permissions ---

func (s *B2BCompanyToolsSuite) TestPermissionListReturnsPermissions() {
	s.mockBC.EXPECT().ListB2BPermissions(gomock.Any(), gomock.Any()).Return([]bigcommerce.B2BPermission{
		{Name: "Get orders", Code: "get_orders"},
	}, nil)

	res, err := s.callTool("b2b/companies/permissions/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestPermissionCreateConfirmed() {
	s.mockBC.EXPECT().CreateB2BPermission(gomock.Any(), gomock.Any()).Return(&bigcommerce.B2BPermission{Name: "Custom", Code: "custom_x"}, nil)
	res, err := s.callTool("b2b/companies/permissions/create", map[string]any{
		"name":        "Custom",
		"description": "A custom permission",
		"code":        "custom_x",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("created", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestPermissionCreateRejectsMissingCode() {
	res, err := s.callTool("b2b/companies/permissions/create", map[string]any{
		"name":        "Custom",
		"description": "A custom permission",
		"confirmed":   true,
	})
	s.NoError(err)
	s.True(res.IsError)
}
