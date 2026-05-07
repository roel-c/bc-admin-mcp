package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListOrdersSinglePageWhenPageProvided(t *testing.T) {
	callCount := 0
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		require.Equal(t, "/stores/hash/v2/orders", req.URL.Path)
		require.Equal(t, "2", req.URL.Query().Get("page"))
		require.Equal(t, "25", req.URL.Query().Get("limit"))
		require.Equal(t, "7", req.URL.Query().Get("status_id"))
		body := `[{"id":101,"status_id":7}]`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListOrders(context.Background(), OrderListParams{
		StatusID: 7,
		Page:     2,
		Limit:    25,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 101, rows[0].ID)
	require.Equal(t, 1, callCount)
}

func TestListOrdersAutoPaginatesWithoutPageOrLimit(t *testing.T) {
	callCount := 0
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		require.Equal(t, "/stores/hash/v2/orders", req.URL.Path)
		require.NotEmpty(t, req.URL.Query().Get("page"))
		require.NotEmpty(t, req.URL.Query().Get("limit"))
		switch req.URL.Query().Get("page") {
		case "1":
			rows := make([]string, 0, 50)
			for i := 0; i < 50; i++ {
				rows = append(rows, fmt.Sprintf(`{"id":%d}`, 100+i))
			}
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("[" + strings.Join(rows, ",") + "]")),
				Header:     http.Header{},
			}, nil
		case "2":
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(`[{"id":999}]`)),
				Header:     http.Header{},
			}, nil
		default:
			t.Fatalf("unexpected page query: %q", req.URL.Query().Get("page"))
			return nil, nil
		}
	}))

	rows, err := c.ListOrders(context.Background(), OrderListParams{CustomerID: 90})
	require.NoError(t, err)
	require.Len(t, rows, 51)
	require.Equal(t, 100, rows[0].ID)
	require.Equal(t, 999, rows[50].ID)
	require.Equal(t, 2, callCount)
}

func TestCountOrdersUsesCountEndpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "/stores/hash/v2/orders/count", req.URL.Path)
		require.Equal(t, "pending", req.URL.Query().Get("payment_method"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"count":37}`)),
			Header:     http.Header{},
		}, nil
	}))

	count, err := c.CountOrders(context.Background(), OrderCountParams{PaymentMethod: "pending"})
	require.NoError(t, err)
	require.Equal(t, 37, count)
}

func TestUpdateOrderStatusUsesV2Put(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPut, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/88", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"status_id":10`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"id":88,"status_id":10}`)),
			Header:     http.Header{},
		}, nil
	}))

	updated, err := c.UpdateOrderStatus(context.Background(), 88, 10)
	require.NoError(t, err)
	require.Equal(t, 88, updated.ID)
	require.Equal(t, 10, updated.StatusID)
}

func TestUpdateOrderUsesV2Put(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPut, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/89", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"staff_notes":"packed"`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"id":89,"status_id":2}`)),
			Header:     http.Header{},
		}, nil
	}))

	updated, err := c.UpdateOrder(context.Background(), 89, json.RawMessage(`{"staff_notes":"packed"}`))
	require.NoError(t, err)
	require.Equal(t, 89, updated.ID)
}

func TestCreateOrderUsesV2Post(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/stores/hash/v2/orders", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"billing_address"`)
		return &http.Response{
			StatusCode: 201,
			Body:       io.NopCloser(strings.NewReader(`{"id":1201,"status_id":0}`)),
			Header:     http.Header{},
		}, nil
	}))

	created, err := c.CreateOrder(context.Background(), json.RawMessage(`{"billing_address":{"first_name":"Ada"}}`))
	require.NoError(t, err)
	require.Equal(t, 1201, created.ID)
}

func TestDeleteOrderUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodDelete, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/1201", req.URL.Path)
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{},
		}, nil
	}))

	err := c.DeleteOrder(context.Background(), 1201)
	require.NoError(t, err)
}

func TestCreateOrderShipmentRequiresItems(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	_, err := c.CreateOrderShipment(context.Background(), 99, OrderShipmentCreate{
		OrderAddressID: 1,
		Items:          nil,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "shipment items are required")
}

func TestListOrderPaymentActionsUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v3/orders/45/payment_actions", req.URL.Path)
		require.Equal(t, "3", req.URL.Query().Get("page"))
		require.Equal(t, "20", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"pa_1","type":"capture"}]}`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListOrderPaymentActions(context.Background(), 45, OrderPaymentActionListParams{
		Page:  3,
		Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"pa_1"`)
}

func TestListOrderTransactionsUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v3/orders/45/transactions", req.URL.Path)
		require.Equal(t, "2", req.URL.Query().Get("page"))
		require.Equal(t, "10", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":"txn_1","event":"sale"}]}`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListOrderTransactions(context.Background(), 45, OrderTransactionListParams{
		Page:  2,
		Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"txn_1"`)
}

func TestCreateOrderPaymentCaptureUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/stores/hash/v3/orders/22/payment_actions/capture", req.URL.Path)
		return &http.Response{
			StatusCode: 201,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"id":"cap_1","status":"capture pending"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	data, err := c.CreateOrderPaymentCapture(context.Background(), 22)
	require.NoError(t, err)
	require.Contains(t, string(data), `"cap_1"`)
}

func TestCreateOrderRefundQuoteRequiresPayload(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	_, err := c.CreateOrderRefundQuote(context.Background(), 99, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refund quote payload is required")
}

func TestCreateOrderRefundIncludesTransactionQueryWhenProvided(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/stores/hash/v3/orders/77/payment_actions/refunds", req.URL.Path)
		require.Equal(t, "txn_abc", req.URL.Query().Get("transaction_id"))
		return &http.Response{
			StatusCode: 201,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"id":"ref_9","status":"pending"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	data, err := c.CreateOrderRefund(context.Background(), 77, json.RawMessage(`{"items":[{"item_type":"PRODUCT","item_id":1,"quantity":1}]}`), "txn_abc")
	require.NoError(t, err)
	require.Contains(t, string(data), `"ref_9"`)
}

func TestListOrderCouponsUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/18/coupons", req.URL.Path)
		require.Equal(t, "2", req.URL.Query().Get("page"))
		require.Equal(t, "10", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`[{"id":901}]`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListOrderCoupons(context.Background(), 18, OrderCouponListParams{Page: 2, Limit: 10})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"id":901`)
}

func TestListOrderShippingAddressesUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/18/shipping_addresses", req.URL.Path)
		require.Equal(t, "3", req.URL.Query().Get("page"))
		require.Equal(t, "5", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`[{"id":333}]`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListOrderShippingAddresses(context.Background(), 18, OrderShippingAddressListParams{Page: 3, Limit: 5})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"id":333`)
}

func TestListOrderMessagesUsesFilters(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/18/messages", req.URL.Path)
		q := req.URL.Query()
		require.Equal(t, "read", q.Get("status"))
		require.Equal(t, "true", q.Get("is_flagged"))
		require.Equal(t, "2", q.Get("page"))
		require.Equal(t, "15", q.Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`[{"id":777}]`)),
			Header:     http.Header{},
		}, nil
	}))

	flagged := true
	rows, err := c.ListOrderMessages(context.Background(), 18, OrderMessageListParams{
		Status:    "read",
		IsFlagged: &flagged,
		Page:      2,
		Limit:     15,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"id":777`)
}

func TestListOrderTaxesUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/18/taxes", req.URL.Path)
		require.Equal(t, "1", req.URL.Query().Get("page"))
		require.Equal(t, "50", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`[{"name":"State Tax"}]`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListOrderTaxes(context.Background(), 18, OrderTaxListParams{Page: 1, Limit: 50})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"State Tax"`)
}

func TestListOrderLegacyRefundsUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/18/refunds", req.URL.Path)
		require.Equal(t, "1", req.URL.Query().Get("page"))
		require.Equal(t, "20", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`[{"id":444}]`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListOrderLegacyRefunds(context.Background(), 18, OrderLegacyRefundListParams{Page: 1, Limit: 20})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"id":444`)
}

func TestListOrderMetafieldsUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v3/orders/18/metafields", req.URL.Path)
		require.Equal(t, "1", req.URL.Query().Get("page"))
		require.Equal(t, "25", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":9,"namespace":"ops","key":"flag","value":"yes"}]}`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListOrderMetafields(context.Background(), 18, OrderMetafieldListParams{Page: 1, Limit: 25})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 9, rows[0].ID)
}

func TestCreateOrderMetafieldUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/stores/hash/v3/orders/18/metafields", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"namespace":"ops"`)
		return &http.Response{
			StatusCode: 201,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"id":10,"namespace":"ops","key":"flag","value":"yes"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	row, err := c.CreateOrderMetafield(context.Background(), 18, Metafield{
		Namespace: "ops",
		Key:       "flag",
		Value:     "yes",
	})
	require.NoError(t, err)
	require.Equal(t, 10, row.ID)
}

func TestDeleteOrderMetafieldUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodDelete, req.Method)
		require.Equal(t, "/stores/hash/v3/orders/18/metafields/10", req.URL.Path)
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{},
		}, nil
	}))

	err := c.DeleteOrderMetafield(context.Background(), 18, 10)
	require.NoError(t, err)
}

func TestGetOrderProductUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/5/products/10", req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"id":10,"order_id":5}`)),
			Header:     http.Header{},
		}, nil
	}))

	row, err := c.GetOrderProduct(context.Background(), 5, 10)
	require.NoError(t, err)
	require.Contains(t, string(row), `"order_id":5`)
}

func TestGetOrderShippingAddressUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/5/shipping_addresses/12", req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"id":12,"order_id":5}`)),
			Header:     http.Header{},
		}, nil
	}))

	row, err := c.GetOrderShippingAddress(context.Background(), 5, 12)
	require.NoError(t, err)
	require.Contains(t, string(row), `"id":12`)
}

func TestUpdateOrderShippingAddressRequiresPayload(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP request: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	_, err := c.UpdateOrderShippingAddress(context.Background(), 5, 12, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "shipping address update payload is required")
}

func TestGetOrderShipmentUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/5/shipments/7", req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"id":7,"order_id":5}`)),
			Header:     http.Header{},
		}, nil
	}))

	row, err := c.GetOrderShipment(context.Background(), 5, 7)
	require.NoError(t, err)
	require.Equal(t, 7, row.ID)
}

func TestDeleteOrderShipmentUsesV2Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodDelete, req.Method)
		require.Equal(t, "/stores/hash/v2/orders/5/shipments/7", req.URL.Path)
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{},
		}, nil
	}))

	err := c.DeleteOrderShipment(context.Background(), 5, 7)
	require.NoError(t, err)
}
