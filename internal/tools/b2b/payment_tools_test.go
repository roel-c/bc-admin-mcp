package b2b_test

import (
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"go.uber.org/mock/gomock"
)

// --- b2b/payments ---

func (s *B2BCompanyToolsSuite) TestPaymentsListReturnsMethods() {
	s.mockBC.EXPECT().ListB2BPaymentMethods(gomock.Any()).Return([]bigcommerce.B2BPaymentMethod{
		{ID: 1, PaymentCode: "cheque", PaymentTitle: "Check"},
	}, nil)

	res, err := s.callTool("b2b/payments/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestPaymentsActiveMethodsList() {
	s.mockBC.EXPECT().ListB2BActivePaymentMethods(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"companyId": float64(42), "paymentId": float64(1)},
	}, nil)

	res, err := s.callTool("b2b/payments/active_methods", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

// --- b2b/companies/payments ---

func (s *B2BCompanyToolsSuite) TestCompanyPaymentsListReturnsMethods() {
	s.mockBC.EXPECT().ListB2BCompanyPaymentMethods(gomock.Any(), 42).Return([]bigcommerce.B2BCompanyPaymentMethod{
		{PaymentID: 1, Code: "cheque", PaymentTitle: "Check", IsEnabled: true},
	}, nil)

	res, err := s.callTool("b2b/companies/payments/list", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

// --- b2b/companies/credit ---

func (s *B2BCompanyToolsSuite) TestCompanyCreditGetReturnsStatus() {
	s.mockBC.EXPECT().GetB2BCompanyCredit(gomock.Any(), 42).Return(&bigcommerce.B2BCompanyCredit{CreditEnabled: false}, nil)

	res, err := s.callTool("b2b/companies/credit/get", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["credit"])
}

// --- b2b/companies/payment_terms ---

func (s *B2BCompanyToolsSuite) TestCompanyPaymentTermsGetReturnsTerms() {
	s.mockBC.EXPECT().GetB2BCompanyPaymentTerms(gomock.Any(), 42).Return(&bigcommerce.B2BPaymentTerms{IsEnabled: true}, nil)

	res, err := s.callTool("b2b/companies/payment_terms/get", map[string]any{"company_id": float64(42)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["payment_terms"])
}

func (s *B2BCompanyToolsSuite) TestCompanyCreditGetRejectsMissingID() {
	res, err := s.callTool("b2b/companies/credit/get", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}
