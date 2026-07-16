package b2b_test

import (
	"go.uber.org/mock/gomock"
)

// --- b2b/sales_staff ---

func (s *B2BCompanyToolsSuite) TestSalesStaffListReturnsStaff() {
	s.mockBC.EXPECT().ListB2BSalesStaff(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"id": float64(1), "email": "rep@acme.com"},
	}, nil)

	res, err := s.callTool("b2b/sales_staff/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestSalesStaffGetReturnsDetail() {
	s.mockBC.EXPECT().GetB2BSalesStaff(gomock.Any(), 1).Return(map[string]any{"id": float64(1)}, nil)

	res, err := s.callTool("b2b/sales_staff/get", map[string]any{"sales_staff_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["sales_staff"])
}

func (s *B2BCompanyToolsSuite) TestSalesStaffUpdateAssignmentsPreviewThenConfirm() {
	prev, err := s.callTool("b2b/sales_staff/update_assignments", map[string]any{
		"sales_staff_id":   float64(1),
		"assignments_json": `[{"companyId":42,"assignStatus":true}]`,
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().UpdateB2BSalesStaffAssignments(gomock.Any(), 1, gomock.Any()).Return(map[string]any{"status": "ok"}, nil)
	res, err := s.callTool("b2b/sales_staff/update_assignments", map[string]any{
		"sales_staff_id":   float64(1),
		"assignments_json": `[{"companyId":42,"assignStatus":true}]`,
		"confirmed":        true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestSalesStaffUpdateAssignmentsRejectsEmpty() {
	res, err := s.callTool("b2b/sales_staff/update_assignments", map[string]any{
		"sales_staff_id":   float64(1),
		"assignments_json": `[]`,
		"confirmed":        true,
	})
	s.NoError(err)
	s.True(res.IsError)
}
