package b2b_test

import (
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"go.uber.org/mock/gomock"
)

// --- b2b/quotes ---

func (s *B2BCompanyToolsSuite) TestQuoteListReturnsQuotes() {
	s.mockBC.EXPECT().ListB2BQuotes(gomock.Any(), gomock.Any()).Return([]map[string]any{
		{"quoteId": float64(1), "quoteNumber": "QN000001"},
	}, nil)

	res, err := s.callTool("b2b/quotes/list", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestQuoteGetReturnsQuote() {
	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 1).Return(map[string]any{"quoteId": float64(1)}, nil)

	res, err := s.callTool("b2b/quotes/get", map[string]any{"quote_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["quote"])
}

func (s *B2BCompanyToolsSuite) TestQuoteCreatePreviewThenConfirm() {
	prev, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{"quoteTitle":"Test Quote"}`,
	})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().CreateB2BQuote(gomock.Any(), gomock.Any()).Return(map[string]any{"quoteId": float64(99)}, nil)
	res, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{"quoteTitle":"Test Quote"}`,
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("created", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteCreateNormalizesMoneyToTwoDecimals() {
	prev, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{
			"quoteTitle":"Money Round",
			"subtotal":3059.099,
			"discount":0.1,
			"grandTotal":3059.091,
			"productList":[
				{"productId":1,"variantId":2,"basePrice":2499.999,"offeredPrice":2249.994,"discount":250.001},
				{"productId":3,"variantId":4,"basePrice":899,"offeredPrice":809.1,"discount":89.9}
			]
		}`,
	})
	s.NoError(err)
	s.False(prev.IsError)
	data := s.parseJSON(prev)
	payload := data["payload"].(map[string]any)
	// Line offered sum = 2249.99 + 809.10 = 3059.09; grandTotal = 3059.09 - 0.10 = 3058.99.
	s.Equal(3059.09, payload["subtotal"])
	s.Equal(0.10, payload["discount"])
	s.Equal(3058.99, payload["grandTotal"])
	items := payload["productList"].([]any)
	row0 := items[0].(map[string]any)
	s.Equal(2500.00, row0["basePrice"])
	s.Equal(2249.99, row0["offeredPrice"])
	s.Equal(250.00, row0["discount"])
	row1 := items[1].(map[string]any)
	s.Equal(899.00, row1["basePrice"])
	s.Equal(809.10, row1["offeredPrice"])
	s.Equal(89.90, row1["discount"])
	warns, ok := data["warnings"].([]any)
	s.Require().True(ok, "warnings expected when totals corrected")
	s.NotEmpty(warns)
}

func (s *B2BCompanyToolsSuite) TestQuoteCreateAlignsMismatchedTotalsFromProductList() {
	prev, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{
			"quoteTitle":"Total Fix",
			"subtotal":2849.98,
			"discount":0,
			"grandTotal":2849.98,
			"productList":[
				{"productId":563,"variantId":1959,"quantity":1,"offeredPrice":2599.99,"basePrice":2599.99,"discount":0},
				{"productId":566,"variantId":1964,"quantity":1,"offeredPrice":499.99,"basePrice":499.99,"discount":0}
			]
		}`,
	})
	s.NoError(err)
	s.False(prev.IsError)
	data := s.parseJSON(prev)
	payload := data["payload"].(map[string]any)
	s.Equal(3099.98, payload["subtotal"])
	s.Equal(3099.98, payload["grandTotal"])
	warns := data["warnings"].([]any)
	s.GreaterOrEqual(len(warns), 1)
}

func (s *B2BCompanyToolsSuite) TestQuoteCreateSetsMissingTotalsFromProductList() {
	prev, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{
			"quoteTitle":"Missing Totals",
			"discount":0,
			"productList":[
				{"productId":1,"variantId":2,"quantity":2,"offeredPrice":100.00,"basePrice":100.00,"discount":0}
			]
		}`,
	})
	s.NoError(err)
	data := s.parseJSON(prev)
	payload := data["payload"].(map[string]any)
	s.Equal(200.00, payload["subtotal"])
	s.Equal(200.00, payload["grandTotal"])
	s.NotEmpty(data["warnings"])
}

func (s *B2BCompanyToolsSuite) TestQuoteCreateLeavesMatchingTotalsUnchanged() {
	prev, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{
			"quoteTitle":"Totals OK",
			"subtotal":349.99,
			"discount":0,
			"grandTotal":349.99,
			"productList":[
				{"productId":566,"variantId":1963,"quantity":1,"offeredPrice":349.99,"basePrice":349.99,"discount":0}
			]
		}`,
	})
	s.NoError(err)
	data := s.parseJSON(prev)
	payload := data["payload"].(map[string]any)
	s.Equal(349.99, payload["subtotal"])
	s.Equal(349.99, payload["grandTotal"])
	_, hasWarn := data["warnings"]
	s.False(hasWarn)
}

func (s *B2BCompanyToolsSuite) TestQuoteCreateExpandsCurrencyString() {
	prev, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{"quoteTitle":"Currency Expand","currency":"usd"}`,
	})
	s.NoError(err)
	s.False(prev.IsError)
	payload := s.parseJSON(prev)["payload"].(map[string]any)
	cur, ok := payload["currency"].(map[string]any)
	s.Require().True(ok, "currency should be expanded to an object")
	s.Equal("USD", cur["currencyCode"])
	s.Equal("$", cur["token"])
	s.Equal("left", cur["location"])
	s.Equal(".", cur["decimalToken"])
	s.Equal(",", cur["thousandsToken"])
	s.Equal(float64(2), cur["decimalPlaces"])
	s.Equal("1.0000000000", cur["currencyExchangeRate"])
}

func (s *B2BCompanyToolsSuite) TestQuoteCreateConfirmSendsExpandedCurrencyObject() {
	s.mockBC.EXPECT().CreateB2BQuote(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, body map[string]any) (map[string]any, error) {
			cur, ok := body["currency"].(map[string]any)
			s.Require().True(ok, "confirm path must send currency as object, not string")
			s.Equal("USD", cur["currencyCode"])
			return map[string]any{"quoteId": float64(77)}, nil
		},
	)
	res, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{"quoteTitle":"Confirm Currency","currency":"USD"}`,
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("created", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteCreatePassesCurrencyObjectThrough() {
	prev, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{"quoteTitle":"Currency Object","currency":{"currencyCode":"EUR","token":"€","location":"left","currencyExchangeRate":"1.1"}}`,
	})
	s.NoError(err)
	s.False(prev.IsError)
	payload := s.parseJSON(prev)["payload"].(map[string]any)
	cur := payload["currency"].(map[string]any)
	s.Equal("EUR", cur["currencyCode"])
	s.Equal("€", cur["token"])
	s.Equal("left", cur["location"])
	s.Equal("1.1", cur["currencyExchangeRate"])
}

func (s *B2BCompanyToolsSuite) TestQuoteCreateRejectsInvalidCurrencyType() {
	res, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `{"quoteTitle":"Bad Currency","currency":123}`,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestQuoteCreateRejectsInvalidJSON() {
	res, err := s.callTool("b2b/quotes/create", map[string]any{
		"quote_json": `not-json`,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestQuoteUpdateConfirmed() {
	s.mockBC.EXPECT().UpdateB2BQuote(gomock.Any(), 1, gomock.Any()).Return(map[string]any{"quoteId": float64(1)}, nil)
	res, err := s.callTool("b2b/quotes/update", map[string]any{
		"quote_id":   float64(1),
		"quote_json": `{"notes":"Updated"}`,
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("updated", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteDeletePreviewThenConfirm() {
	prev, err := s.callTool("b2b/quotes/delete", map[string]any{"quote_id": float64(1)})
	s.NoError(err)
	s.Equal("preview", s.parseJSON(prev)["status"])

	s.mockBC.EXPECT().DeleteB2BQuote(gomock.Any(), 1).Return(nil)
	res, err := s.callTool("b2b/quotes/delete", map[string]any{"quote_id": float64(1), "confirmed": true})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("deleted", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteCheckoutConfirmed() {
	s.mockBC.EXPECT().GenerateB2BQuoteCheckout(gomock.Any(), 1).Return(map[string]any{"cartUrl": "https://example.com/cart"}, nil)
	res, err := s.callTool("b2b/quotes/checkout", map[string]any{"quote_id": float64(1), "confirmed": true})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("generated", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteAssignToOrderConfirmed() {
	s.mockBC.EXPECT().AssignB2BQuoteToOrder(gomock.Any(), 1, 500).Return(nil)
	res, err := s.callTool("b2b/quotes/assign_to_order", map[string]any{
		"quote_id": float64(1), "order_id": float64(500), "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("assigned", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuotePDFExportReturnsLink() {
	s.mockBC.EXPECT().ExportB2BQuotePDF(gomock.Any(), 1, gomock.Any()).Return(map[string]any{"url": "https://example.com/quote.pdf"}, nil)
	res, err := s.callTool("b2b/quotes/pdf_export", map[string]any{"quote_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.NotNil(data["download"])
}

func (s *B2BCompanyToolsSuite) TestQuoteExtraFieldsList() {
	s.mockBC.EXPECT().ListB2BQuoteExtraFields(gomock.Any(), gomock.Any()).Return([]bigcommerce.B2BExtraFieldDef{
		{FieldName: "Approval Needed"},
	}, nil)
	res, err := s.callTool("b2b/quotes/extra_fields", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

// --- b2b/quotes/shipping ---

func (s *B2BCompanyToolsSuite) TestQuoteShippingRatesList() {
	s.mockBC.EXPECT().ListB2BQuoteShippingRates(gomock.Any(), 1).Return([]map[string]any{
		{"id": "rate-1"},
	}, nil)
	res, err := s.callTool("b2b/quotes/shipping/rates", map[string]any{"quote_id": float64(1)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *B2BCompanyToolsSuite) TestQuoteShippingSelectWithMethodID() {
	s.mockBC.EXPECT().SelectB2BQuoteShippingRate(gomock.Any(), 1, "rate-1", "", float64(0), false).Return(map[string]any{"quoteId": float64(1)}, nil)
	res, err := s.callTool("b2b/quotes/shipping/select", map[string]any{
		"quote_id": float64(1), "shipping_method_id": "rate-1", "confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("selected", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteShippingSelectRejectsIncompleteCustom() {
	res, err := s.callTool("b2b/quotes/shipping/select", map[string]any{
		"quote_id": float64(1), "custom_name": "Freight Only", "confirmed": true,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestQuoteShippingRemoveConfirmed() {
	s.mockBC.EXPECT().RemoveB2BQuoteShippingRate(gomock.Any(), 1).Return(nil)
	res, err := s.callTool("b2b/quotes/shipping/remove", map[string]any{"quote_id": float64(1), "confirmed": true})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("removed", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteShippingCustomMethodsList() {
	s.mockBC.EXPECT().ListB2BQuoteCustomShippingMethods(gomock.Any()).Return([]map[string]any{
		{"name": "Freight"},
	}, nil)
	res, err := s.callTool("b2b/quotes/shipping/custom_methods", map[string]any{})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}
