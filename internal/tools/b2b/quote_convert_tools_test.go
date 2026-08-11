package b2b_test

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"go.uber.org/mock/gomock"
)

func sampleConvertibleQuote(email string, companyID int, withShipping bool) map[string]any {
	q := map[string]any{
		"quoteId":   float64(10),
		"companyId": float64(companyID),
		"contactInfo": map[string]any{
			"email": email,
			"name":  "Ellen Ripley",
		},
		"companyInfo": map[string]any{
			"companyId":   float64(companyID),
			"companyName": "Nostromo LLC",
		},
		"shippingAddress": map[string]any{
			"firstName":   "Ellen",
			"lastName":    "Ripley",
			"address":     "1 LV-426",
			"city":        "Hadley's Hope",
			"state":       "California",
			"stateCode":   "CA",
			"countryCode": "US",
			"zipCode":     "90210",
			"phoneNumber": "555-0100",
		},
		"billingAddress": map[string]any{
			"firstName":   "Ellen",
			"lastName":    "Ripley",
			"address":     "1 LV-426",
			"city":        "Hadley's Hope",
			"state":       "California",
			"stateCode":   "CA",
			"countryCode": "US",
			"zipCode":     "90210",
			"phoneNumber": "555-0100",
		},
	}
	if withShipping {
		q["shippingMethod"] = map[string]any{
			"id":          "free-ship",
			"description": "Free Shipping",
		}
	}
	return q
}

func (s *B2BCompanyToolsSuite) expectSuccessfulQuoteConvert(quoteID, orderID, customerID int) {
	s.mockCheckout.EXPECT().UpdateCart(gomock.Any(), "cart-abc", bigcommerce.CartUpdate{CustomerID: customerID}).
		Return(&bigcommerce.Cart{ID: "cart-abc", CustomerID: customerID}, nil)
	s.mockCheckout.EXPECT().GetCart(gomock.Any(), "cart-abc", false).Return(&bigcommerce.Cart{
		ID: "cart-abc",
		LineItems: bigcommerce.CartLineItems{
			PhysicalItems: []bigcommerce.CartPhysicalItem{{ID: "li-1", Quantity: 2}},
		},
	}, nil)
	s.mockCheckout.EXPECT().SetBillingAddress(gomock.Any(), "cart-abc", gomock.Any()).
		Return(&bigcommerce.Checkout{ID: "cart-abc"}, nil)
	s.mockCheckout.EXPECT().AddConsignment(gomock.Any(), "cart-abc", gomock.Any()).
		Return(&bigcommerce.Checkout{
			ID: "cart-abc",
			Consignments: []bigcommerce.CheckoutConsignment{{
				ID: "consign-1",
				AvailableShippingOptions: []bigcommerce.CheckoutShippingOption{
					{ID: "opt-paid", Description: "Flat Rate", Cost: 10},
					{ID: "opt-free", Description: "Free Shipping", Cost: 0},
				},
			}},
		}, nil)
	s.mockCheckout.EXPECT().UpdateConsignment(gomock.Any(), "cart-abc", "consign-1", bigcommerce.CheckoutConsignmentUpdate{
		ShippingOptionID: "opt-free",
	}).Return(&bigcommerce.Checkout{ID: "cart-abc"}, nil)
	s.mockCheckout.EXPECT().ConvertCheckoutToOrder(gomock.Any(), "cart-abc").
		Return(&bigcommerce.CheckoutOrderResult{ID: orderID}, nil)
	s.mockCheckout.EXPECT().UpdateOrderStatus(gomock.Any(), orderID, 7).
		Return(&bigcommerce.Order{}, nil)
	s.mockBC.EXPECT().AssignB2BQuoteToOrder(gomock.Any(), quoteID, orderID).Return(nil)
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderPreview() {
	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{float64(10), float64(11)},
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal("convert_b2b_quotes_to_orders", data["action"])
	s.Equal(float64(2), data["count"])
	s.Equal(float64(7), data["status_id"])
	s.Contains(data["message"], "Prefer ≤3–5")
	s.Contains(data["message"], "already_converted")
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderRejectsEmptyIDs() {
	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{},
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderConfirmSuccess() {
	email := "ripley@nostromo.test"
	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 10).Return(sampleConvertibleQuote(email, 42, true), nil)
	s.mockBC.EXPECT().GenerateB2BQuoteCheckout(gomock.Any(), 10).Return(map[string]any{
		"cartId": "cart-abc",
	}, nil)
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"email:in": email}).
		Return([]bigcommerce.Customer{{ID: 9001, Email: email}}, nil)
	s.expectSuccessfulQuoteConvert(10, 501, 9001)

	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{float64(10)},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("converted", data["status"])
	s.Equal(float64(1), data["converted_count"])
	s.Equal(float64(0), data["failed_count"])
	converted := data["converted"].([]any)
	s.Len(converted, 1)
	row := converted[0].(map[string]any)
	s.Equal(float64(10), row["quote_id"])
	s.Equal(float64(501), row["order_id"])
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderResolvesCustomerViaCompanyUsers() {
	email := "bishop@nostromo.test"
	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 12).Return(sampleConvertibleQuote(email, 77, true), nil)
	s.mockBC.EXPECT().GenerateB2BQuoteCheckout(gomock.Any(), 12).Return(map[string]any{"cartId": "cart-abc"}, nil)
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"email:in": email}).
		Return([]bigcommerce.Customer{}, nil)
	s.mockBC.EXPECT().ListB2BUsers(gomock.Any(), "companyId=77").Return([]bigcommerce.B2BUser{
		{ID: 1, CompanyID: 77, Email: email, Role: 0, BCCustomerID: 0},
	}, nil)
	// enrichUsersWithBCCustomerIDs second lookup
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"email:in": email}).
		Return([]bigcommerce.Customer{{ID: 8800, Email: email}}, nil)
	s.expectSuccessfulQuoteConvert(12, 602, 8800)

	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{float64(12)},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("converted", data["status"])
	row := data["converted"].([]any)[0].(map[string]any)
	s.Equal(float64(602), row["order_id"])
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderPartialFailure() {
	emailOK := "hicks@nostromo.test"
	emailBad := "missing@nostromo.test"

	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 20).Return(sampleConvertibleQuote(emailOK, 42, true), nil)
	s.mockBC.EXPECT().GenerateB2BQuoteCheckout(gomock.Any(), 20).Return(map[string]any{"cartId": "cart-abc"}, nil)
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"email:in": emailOK}).
		Return([]bigcommerce.Customer{{ID: 7001, Email: emailOK}}, nil)
	s.expectSuccessfulQuoteConvert(20, 701, 7001)

	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 21).Return(sampleConvertibleQuote(emailBad, 42, true), nil)
	s.mockBC.EXPECT().GenerateB2BQuoteCheckout(gomock.Any(), 21).Return(map[string]any{"cartId": "cart-bad"}, nil)
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"email:in": emailBad}).
		Return([]bigcommerce.Customer{}, nil)
	s.mockBC.EXPECT().ListB2BUsers(gomock.Any(), "companyId=42").Return([]bigcommerce.B2BUser{
		{ID: 2, CompanyID: 42, Email: "other@nostromo.test", Role: 1, BCCustomerID: 0},
	}, nil)
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"email:in": "other@nostromo.test"}).
		Return([]bigcommerce.Customer{}, nil)

	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{float64(20), float64(21)},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("partial_success", data["status"])
	s.Equal(float64(1), data["converted_count"])
	s.Equal(float64(1), data["failed_count"])
	failures := data["failures"].([]any)
	s.Len(failures, 1)
	s.Equal(float64(21), failures[0].(map[string]any)["quote_id"])
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderSelectsShippingWhenMissing() {
	email := "hudson@nostromo.test"
	quote := sampleConvertibleQuote(email, 42, false)
	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 30).Return(quote, nil)
	s.mockBC.EXPECT().ListB2BQuoteShippingRates(gomock.Any(), 30).Return([]map[string]any{
		{"id": "rate-flat", "description": "Flat Rate"},
		{"id": "rate-free", "description": "Free Shipping"},
	}, nil)
	s.mockBC.EXPECT().SelectB2BQuoteShippingRate(gomock.Any(), 30, "rate-free", "", 0.0, false).
		Return(map[string]any{}, nil)
	s.mockBC.EXPECT().GenerateB2BQuoteCheckout(gomock.Any(), 30).Return(map[string]any{"cartId": "cart-abc"}, nil)
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"email:in": email}).
		Return([]bigcommerce.Customer{{ID: 3001, Email: email}}, nil)
	s.expectSuccessfulQuoteConvert(30, 801, 3001)

	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{float64(30)},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("converted", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderPaymentMethodStamp() {
	email := "vasquez@nostromo.test"
	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 40).Return(sampleConvertibleQuote(email, 42, true), nil)
	s.mockBC.EXPECT().GenerateB2BQuoteCheckout(gomock.Any(), 40).Return(map[string]any{"cartId": "cart-abc"}, nil)
	s.mockDeleter.EXPECT().SearchCustomers(gomock.Any(), map[string]string{"email:in": email}).
		Return([]bigcommerce.Customer{{ID: 4001, Email: email}}, nil)

	s.mockCheckout.EXPECT().UpdateCart(gomock.Any(), "cart-abc", bigcommerce.CartUpdate{CustomerID: 4001}).
		Return(&bigcommerce.Cart{ID: "cart-abc", CustomerID: 4001}, nil)
	s.mockCheckout.EXPECT().GetCart(gomock.Any(), "cart-abc", false).Return(&bigcommerce.Cart{
		ID: "cart-abc",
		LineItems: bigcommerce.CartLineItems{
			PhysicalItems: []bigcommerce.CartPhysicalItem{{ID: "li-1", Quantity: 1}},
		},
	}, nil)
	s.mockCheckout.EXPECT().SetBillingAddress(gomock.Any(), "cart-abc", gomock.Any()).
		Return(&bigcommerce.Checkout{ID: "cart-abc"}, nil)
	s.mockCheckout.EXPECT().AddConsignment(gomock.Any(), "cart-abc", gomock.Any()).
		Return(&bigcommerce.Checkout{
			ID: "cart-abc",
			Consignments: []bigcommerce.CheckoutConsignment{{
				ID: "consign-1",
				AvailableShippingOptions: []bigcommerce.CheckoutShippingOption{
					{ID: "opt-free", Description: "Free Shipping", Cost: 0},
				},
			}},
		}, nil)
	s.mockCheckout.EXPECT().UpdateConsignment(gomock.Any(), "cart-abc", "consign-1", gomock.Any()).
		Return(&bigcommerce.Checkout{ID: "cart-abc"}, nil)
	s.mockCheckout.EXPECT().ConvertCheckoutToOrder(gomock.Any(), "cart-abc").
		Return(&bigcommerce.CheckoutOrderResult{ID: 901}, nil)
	s.mockCheckout.EXPECT().UpdateOrderStatus(gomock.Any(), 901, 7).Return(&bigcommerce.Order{}, nil)
	s.mockCheckout.EXPECT().UpdateOrder(gomock.Any(), 901, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ int, payload json.RawMessage) (*bigcommerce.Order, error) {
			var body map[string]string
			s.Require().NoError(json.Unmarshal(payload, &body))
			s.Equal("Check", body["payment_method"])
			return &bigcommerce.Order{}, nil
		},
	)
	s.mockBC.EXPECT().AssignB2BQuoteToOrder(gomock.Any(), 40, 901).Return(nil)

	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids":      []any{float64(40)},
		"payment_method": "Check",
		"confirmed":      true,
	})
	s.NoError(err)
	s.False(res.IsError)
	s.Equal("converted", s.parseJSON(res)["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderAllFailed() {
	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 50).Return(nil, errors.New("not found"))

	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{float64(50)},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("failed", data["status"])
	s.Equal(float64(0), data["converted_count"])
	s.Equal(float64(1), data["failed_count"])
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderAlreadyConverted() {
	email := "apone@nostromo.test"
	quote := sampleConvertibleQuote(email, 42, true)
	quote["status"] = float64(4)
	quote["bcOrderId"] = float64(196)

	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 60).Return(quote, nil)
	// No checkout / assign calls — already converted.

	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{float64(60)},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("converted", data["status"])
	s.Equal(float64(1), data["converted_count"])
	s.Equal(float64(0), data["failed_count"])
	row := data["converted"].([]any)[0].(map[string]any)
	s.Equal(float64(60), row["quote_id"])
	s.Equal(float64(196), row["order_id"])
	s.Equal("already_converted", row["status"])
}

func (s *B2BCompanyToolsSuite) TestQuoteConvertToOrderAlreadyConvertedStringBcOrderID() {
	email := "frost@nostromo.test"
	quote := sampleConvertibleQuote(email, 42, true)
	quote["bcOrderId"] = "205"

	s.mockBC.EXPECT().GetB2BQuote(gomock.Any(), 61).Return(quote, nil)

	res, err := s.callTool("b2b/quotes/convert_to_order", map[string]any{
		"quote_ids": []any{float64(61)},
		"confirmed": true,
	})
	s.NoError(err)
	data := s.parseJSON(res)
	row := data["converted"].([]any)[0].(map[string]any)
	s.Equal(float64(205), row["order_id"])
	s.Equal("already_converted", row["status"])
}
