package b2b_test

import (
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"go.uber.org/mock/gomock"
)

// --- b2b/companies/hierarchy ---

func (s *B2BCompanyToolsSuite) TestHierarchyGetReturnsTree() {
	child := 43
	s.mockBC.EXPECT().ListB2BCompanyHierarchy(gomock.Any(), 42, gomock.Any()).Return([]bigcommerce.B2BHierarchyNode{
		{CompanyID: 42, CompanyName: "Parent", CompanyStatus: 1, Subsidiaries: []bigcommerce.B2BHierarchyNode{
			{CompanyID: child, CompanyName: "Child", CompanyStatus: 1},
		}},
	}, nil)

	res, err := s.callTool("b2b/companies/hierarchy/get", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestHierarchySubsidiariesList() {
	s.mockBC.EXPECT().ListB2BCompanySubsidiaries(gomock.Any(), 42, gomock.Any()).Return([]bigcommerce.B2BHierarchyNode{
		{CompanyID: 43, CompanyName: "Child", CompanyStatus: 1},
	}, nil)

	res, err := s.callTool("b2b/companies/hierarchy/subsidiaries", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestHierarchyAttachParentPreviewThenConfirm() {
	prev, err := s.callTool("b2b/companies/hierarchy/attach_parent", map[string]any{
		"company_id":        float64(43),
		"parent_company_id": float64(42),
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().AttachB2BCompanyParent(gomock.Any(), 43, 42).Return(nil)
	res, err := s.callTool("b2b/companies/hierarchy/attach_parent", map[string]any{
		"company_id":        float64(43),
		"parent_company_id": float64(42),
		"confirmed":         true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("attached", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestHierarchyAttachParentRejectsSelf() {
	res, err := s.callTool("b2b/companies/hierarchy/attach_parent", map[string]any{
		"company_id":        float64(42),
		"parent_company_id": float64(42),
		"confirmed":         true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestHierarchyDetachSubsidiaryConfirmed() {
	s.mockBC.EXPECT().DeleteB2BCompanySubsidiary(gomock.Any(), 42, 43).Return(nil)
	res, err := s.callTool("b2b/companies/hierarchy/detach_subsidiary", map[string]any{
		"company_id":       float64(42),
		"child_company_id": float64(43),
		"confirmed":        true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("detached", s.parseJSON(res)["status"])
}
