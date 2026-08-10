package b2b_test

import (
	"context"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"go.uber.org/mock/gomock"
)

// --- b2b/payment_records reads ---

func (s *B2BCompanyToolsSuite) TestPaymentRecordListReturnsRecords() {
	s.mockBC.EXPECT().ListB2BPaymentRecords(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"id": float64(1)},
	}, nil)

	res, err := s.callTool("b2b/payment_records/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordGetReturnsRecord() {
	s.mockBC.EXPECT().GetB2BPaymentRecord(gomock.Any(), 1).Return(map[string]any{"id": float64(1)}, nil)

	res, err := s.callTool("b2b/payment_records/get", map[string]any{"payment_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["payment_record"])
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordTransactionsReturnsList() {
	s.mockBC.EXPECT().ListB2BPaymentTransactions(gomock.Any(), 1).Return([]map[string]any{
		{"type": "OfflineTransaction"},
	}, nil)

	res, err := s.callTool("b2b/payment_records/transactions", map[string]any{"payment_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordOperationsReturnsOps() {
	s.mockBC.EXPECT().GetB2BPaymentOperations(gomock.Any(), 1).Return(map[string]any{"allowed": []any{"0", "1"}}, nil)

	res, err := s.callTool("b2b/payment_records/operations", map[string]any{"payment_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["operations"])
}

// --- b2b/payment_records writes ---

func (s *B2BCompanyToolsSuite) TestPaymentRecordCreateOfflinePreviewThenConfirm() {
	prev, err := s.callTool("b2b/payment_records/create_offline", map[string]any{
		"line_items_json": `[{"invoiceId":141,"amount":"25.00"},{"invoiceId":142,"amount":"10.00"}]`,
	})
	s.NoError(err)
	prevData := s.parseJSON(prev)
	s.Equal("preview", prevData["status"])
	s.Equal(float64(2), prevData["invoice_count"])

	s.mockBC.EXPECT().CreateB2BOfflinePayment(gomock.Any(), gomock.Any()).Return(map[string]any{"id": float64(200)}, nil)
	res, err := s.callTool("b2b/payment_records/create_offline", map[string]any{
		"line_items_json": `[{"invoiceId":141,"amount":"25.00"},{"invoiceId":142,"amount":"10.00"}]`, "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("created", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordCreateOfflinePayPercentResolvesAmounts() {
	s.mockBC.EXPECT().GetB2BInvoice(gomock.Any(), "141").Return(map[string]any{
		"id": 141, "originalBalance": map[string]any{"code": "USD", "value": "79.9500"},
	}, nil)
	s.mockBC.EXPECT().GetB2BInvoice(gomock.Any(), "142").Return(map[string]any{
		"id": 142, "originalBalance": map[string]any{"code": "USD", "value": "89.9500"},
	}, nil)

	prev, err := s.callTool("b2b/payment_records/create_offline", map[string]any{
		"line_items_json": `[{"invoiceId":141},{"invoiceId":142}]`,
		"pay_percent":     float64(50),
	})
	s.NoError(err)
	prevData := s.parseJSON(prev)
	s.Equal("preview", prevData["status"])
	s.Equal(float64(50), prevData["pay_percent"])
	payload := prevData["payload"].(map[string]any)
	items := payload["lineItems"].([]any)
	s.Equal("39.98", items[0].(map[string]any)["amount"])
	s.Equal("44.98", items[1].(map[string]any)["amount"])

	s.mockBC.EXPECT().GetB2BInvoice(gomock.Any(), "141").Return(map[string]any{
		"id": 141, "originalBalance": map[string]any{"code": "USD", "value": "79.9500"},
	}, nil)
	s.mockBC.EXPECT().GetB2BInvoice(gomock.Any(), "142").Return(map[string]any{
		"id": 142, "originalBalance": map[string]any{"code": "USD", "value": "89.9500"},
	}, nil)
	s.mockBC.EXPECT().CreateB2BOfflinePayment(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, payload bigcommerce.B2BOfflinePaymentCreate) (map[string]any, error) {
			s.Len(payload.LineItems, 2)
			s.Equal("39.98", payload.LineItems[0].Amount)
			s.Equal("44.98", payload.LineItems[1].Amount)
			return map[string]any{"paymentId": float64(201)}, nil
		},
	)

	res, err := s.callTool("b2b/payment_records/create_offline", map[string]any{
		"line_items_json": `[{"invoiceId":141},{"invoiceId":142}]`,
		"pay_percent":     float64(50),
		"confirmed":       true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("created", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordCreateOfflineRejectsNoLineItems() {
	res, err := s.callTool("b2b/payment_records/create_offline", map[string]any{"confirmed": true})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordUpdateOfflineConfirmed() {
	s.mockBC.EXPECT().UpdateB2BOfflinePayment(gomock.Any(), 200, gomock.Any()).Return(map[string]any{"id": float64(200)}, nil)
	res, err := s.callTool("b2b/payment_records/update_offline", map[string]any{
		"payment_id": float64(200), "memo": "corrected memo", "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordPerformOperationPreviewThenConfirm() {
	prev, err := s.callTool("b2b/payment_records/perform_operation", map[string]any{
		"payment_id": float64(200), "operation_code": "1",
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().PerformB2BPaymentOperation(gomock.Any(), 200, "1").Return(map[string]any{}, nil)
	res, err := s.callTool("b2b/payment_records/perform_operation", map[string]any{
		"payment_id": float64(200), "operation_code": "1", "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("performed", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordUpdateProcessingStatusConfirmed() {
	s.mockBC.EXPECT().UpdateB2BPaymentProcessingStatus(gomock.Any(), 200, 3).Return(map[string]any{}, nil)
	res, err := s.callTool("b2b/payment_records/update_processing_status", map[string]any{
		"payment_id": float64(200), "processing_status": float64(3), "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordUpdateProcessingStatusRejectsOutOfRange() {
	res, err := s.callTool("b2b/payment_records/update_processing_status", map[string]any{
		"payment_id": float64(200), "processing_status": float64(9), "confirmed": true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestPaymentRecordDeletePreviewThenConfirm() {
	prev, err := s.callTool("b2b/payment_records/delete", map[string]any{"payment_id": float64(200)})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().DeleteB2BPayment(gomock.Any(), 200).Return(nil)
	res, err := s.callTool("b2b/payment_records/delete", map[string]any{"payment_id": float64(200), "confirmed": true})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("deleted", s.parseJSON(res)["status"])
}
