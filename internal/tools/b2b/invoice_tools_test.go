package b2b_test

import (
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"go.uber.org/mock/gomock"
)

// --- b2b/invoices ---

func (s *B2BCompanyToolsSuite) TestInvoiceListReturnsInvoices() {
	s.mockBC.EXPECT().ListB2BInvoices(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"id": "inv-1", "invoiceNumber": "INV-001", "openBalance": 100.0},
	}, nil)

	res, err := s.callTool("b2b/invoices/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestInvoiceGetReturnsInvoice() {
	s.mockBC.EXPECT().GetB2BInvoice(gomock.Any(), "inv-1").Return(map[string]any{"id": "inv-1"}, nil)

	res, err := s.callTool("b2b/invoices/get", map[string]any{"invoice_id": "inv-1"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["invoice"])
}

func (s *B2BCompanyToolsSuite) TestInvoiceGetRejectsMissingID() {
	res, err := s.callTool("b2b/invoices/get", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestInvoiceDownloadPDFReturnsLink() {
	s.mockBC.EXPECT().DownloadB2BInvoicePDF(gomock.Any(), "inv-1").Return(map[string]any{"url": "https://cdn.example.com/inv-1.pdf"}, nil)

	res, err := s.callTool("b2b/invoices/download_pdf", map[string]any{"invoice_id": "inv-1"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["download"])
}

func (s *B2BCompanyToolsSuite) TestInvoiceExtraFieldsList() {
	s.mockBC.EXPECT().ListB2BInvoiceExtraFields(gomock.Any(), gomock.Any()).Return([]bigcommerce.B2BExtraFieldDef{
		{FieldName: "Cost Center", FieldType: "0"},
	}, nil)

	res, err := s.callTool("b2b/invoices/extra_fields", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

// --- b2b/receipts ---

func (s *B2BCompanyToolsSuite) TestReceiptListReturnsReceipts() {
	s.mockBC.EXPECT().ListB2BReceipts(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"id": "rcpt-1"},
	}, nil)

	res, err := s.callTool("b2b/receipts/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestReceiptGetReturnsReceipt() {
	s.mockBC.EXPECT().GetB2BReceipt(gomock.Any(), "rcpt-1").Return(map[string]any{"id": "rcpt-1"}, nil)

	res, err := s.callTool("b2b/receipts/get", map[string]any{"receipt_id": "rcpt-1"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["receipt"])
}

func (s *B2BCompanyToolsSuite) TestReceiptLinesListAllReturnsLines() {
	s.mockBC.EXPECT().ListB2BReceiptLines(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"id": "line-1"},
	}, nil)

	res, err := s.callTool("b2b/receipts/lines/list_all", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestReceiptLinesListForReceiptReturnsLines() {
	s.mockBC.EXPECT().ListB2BLinesOfReceipt(gomock.Any(), "rcpt-1", gomock.Any()).Return([]map[string]any{
		{"id": "line-1"},
	}, nil)

	res, err := s.callTool("b2b/receipts/lines/list_for_receipt", map[string]any{"receipt_id": "rcpt-1"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestReceiptLineGetReturnsLine() {
	s.mockBC.EXPECT().GetB2BReceiptLine(gomock.Any(), "rcpt-1", "line-1").Return(map[string]any{"id": "line-1"}, nil)

	res, err := s.callTool("b2b/receipts/lines/get", map[string]any{"receipt_id": "rcpt-1", "line_id": "line-1"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["line"])
}

func (s *B2BCompanyToolsSuite) TestReceiptLineGetRejectsMissingIDs() {
	res, err := s.callTool("b2b/receipts/lines/get", map[string]any{"receipt_id": "rcpt-1"})
	s.NoError(err)
	s.True(res.IsError)
}
