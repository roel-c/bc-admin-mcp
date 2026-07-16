package b2b_test

import (
	"go.uber.org/mock/gomock"
)

// --- b2b/super_admins ---

func (s *B2BCompanyToolsSuite) TestSuperAdminListReturnsAdmins() {
	s.mockBC.EXPECT().ListB2BSuperAdmins(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"id": float64(1), "email": "sa@acme.com"},
	}, nil)

	res, err := s.callTool("b2b/super_admins/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestSuperAdminCompaniesOverviewList() {
	s.mockBC.EXPECT().ListB2BSuperAdminCompaniesOverview(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"companyId": float64(42)},
	}, nil)

	res, err := s.callTool("b2b/super_admins/companies_overview", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestSuperAdminGetReturnsDetail() {
	s.mockBC.EXPECT().GetB2BSuperAdmin(gomock.Any(), 1).Return(map[string]any{"id": float64(1)}, nil)

	res, err := s.callTool("b2b/super_admins/get", map[string]any{"super_admin_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["super_admin"])
}

func (s *B2BCompanyToolsSuite) TestSuperAdminCompaniesReturnsList() {
	s.mockBC.EXPECT().GetB2BSuperAdminCompanies(gomock.Any(), 1, gomock.Any()).Return([]map[string]any{
		{"companyId": float64(42)},
	}, nil)

	res, err := s.callTool("b2b/super_admins/companies", map[string]any{"super_admin_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestSuperAdminCreatePreviewThenConfirm() {
	prev, err := s.callTool("b2b/super_admins/create", map[string]any{
		"first_name": "New", "last_name": "Admin", "email": "new-admin@acme.com",
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().CreateB2BSuperAdmin(gomock.Any(), gomock.Any()).Return(map[string]any{"id": float64(99)}, nil)
	res, err := s.callTool("b2b/super_admins/create", map[string]any{
		"first_name": "New", "last_name": "Admin", "email": "new-admin@acme.com", "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("created", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestSuperAdminCreateRejectsMissingFields() {
	res, err := s.callTool("b2b/super_admins/create", map[string]any{
		"first_name": "New",
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestSuperAdminBulkCreateConfirmed() {
	s.mockBC.EXPECT().BulkCreateB2BSuperAdmins(gomock.Any(), gomock.Any()).Return(map[string]any{"status": "ok"}, nil)
	res, err := s.callTool("b2b/super_admins/bulk_create", map[string]any{
		"super_admins_json": `[{"first_name":"A","last_name":"One","email":"a@acme.com"},{"first_name":"B","last_name":"Two","email":"b@acme.com"}]`,
		"confirmed":         true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
	s.Equal(float64(2), data["count"])
}

func (s *B2BCompanyToolsSuite) TestSuperAdminBulkCreateRejectsOverTen() {
	rows := make([]string, 11)
	for i := range rows {
		rows[i] = `{"first_name":"X","last_name":"Y","email":"x@acme.com"}`
	}
	res, err := s.callTool("b2b/super_admins/bulk_create", map[string]any{
		"super_admins_json": "[" + joinComma(rows) + "]",
		"confirmed":         true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func joinComma(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

func (s *B2BCompanyToolsSuite) TestSuperAdminUpdateConfirmed() {
	s.mockBC.EXPECT().UpdateB2BSuperAdmin(gomock.Any(), 1, gomock.Any()).Return(map[string]any{"id": float64(1)}, nil)
	res, err := s.callTool("b2b/super_admins/update", map[string]any{
		"super_admin_id": float64(1), "phone": "555-0100", "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestSuperAdminUpdateRejectsNoFields() {
	res, err := s.callTool("b2b/super_admins/update", map[string]any{"super_admin_id": float64(1), "confirmed": true})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestSuperAdminUpdateAssignmentsPreviewThenConfirm() {
	prev, err := s.callTool("b2b/super_admins/update_assignments", map[string]any{
		"super_admin_id": float64(1), "assignments_json": `[{"companyId":42,"isAssigned":true}]`,
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().UpdateB2BSuperAdminCompanyAssignments(gomock.Any(), 1, gomock.Any()).Return(map[string]any{}, nil)
	res, err := s.callTool("b2b/super_admins/update_assignments", map[string]any{
		"super_admin_id": float64(1), "assignments_json": `[{"companyId":42,"isAssigned":true}]`, "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}

// --- b2b/companies/super_admins ---

func (s *B2BCompanyToolsSuite) TestCompanySuperAdminsListReturnsAdmins() {
	s.mockBC.EXPECT().ListB2BCompanySuperAdmins(gomock.Any(), 42, gomock.Any()).Return([]map[string]any{
		{"id": float64(1)},
	}, nil)

	res, err := s.callTool("b2b/companies/super_admins/list", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestCompanySuperAdminsUpdateAssignmentsConfirmed() {
	s.mockBC.EXPECT().UpdateB2BCompanySuperAdminAssignments(gomock.Any(), 42, gomock.Any()).Return(map[string]any{}, nil)
	res, err := s.callTool("b2b/companies/super_admins/update_assignments", map[string]any{
		"company_id": float64(42), "assignments_json": `[{"superAdminId":1,"isAssigned":true}]`, "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}
