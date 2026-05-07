package bigcommerce

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListPriceListsSinglePageWhenPageProvided(t *testing.T) {
	callCount := 0
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		require.Equal(t, "/stores/hash/v3/pricelists", req.URL.Path)
		q := req.URL.Query()
		require.Equal(t, "2", q.Get("page"))
		require.Equal(t, "20", q.Get("limit"))
		require.Equal(t, "Wholesale", q.Get("name"))

		body := `{"data":[{"id":3,"name":"Wholesale","active":true}],"meta":{"pagination":{"current_page":2,"total_pages":4}}}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListPriceLists(context.Background(), PriceListListParams{
		Name:  "Wholesale",
		Page:  2,
		Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 3, rows[0].ID)
	require.Equal(t, 1, callCount, "single-page list should perform exactly one GET")
}

func TestListPriceListAssignmentsAutoPaginatesWithoutExplicitPagination(t *testing.T) {
	callCount := 0
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		require.Equal(t, "/stores/hash/v3/pricelists/assignments", req.URL.Path)
		q := req.URL.Query()
		require.Equal(t, "7", q.Get("price_list_id"))
		require.NotEmpty(t, q.Get("page"))
		require.NotEmpty(t, q.Get("limit"))

		switch q.Get("page") {
		case "1":
			body := `{"data":[{"id":11,"price_list_id":7,"customer_group_id":3,"channel_id":1}],"meta":{"pagination":{"current_page":1,"total_pages":2}}}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{},
			}, nil
		case "2":
			body := `{"data":[{"id":12,"price_list_id":7,"customer_group_id":4,"channel_id":1}],"meta":{"pagination":{"current_page":2,"total_pages":2}}}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{},
			}, nil
		default:
			t.Fatalf("unexpected page query: %q", q.Get("page"))
			return nil, nil
		}
	}))

	rows, err := c.ListPriceListAssignments(context.Background(), PriceListAssignmentListParams{
		PriceListID: 7,
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, 11, rows[0].ID)
	require.Equal(t, 12, rows[1].ID)
	require.Equal(t, 2, callCount, "auto-pagination should fetch all pages")
}

func TestListPriceListRecordsSinglePageWhenCursorProvided(t *testing.T) {
	callCount := 0
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		require.Equal(t, "/stores/hash/v3/pricelists/5/records", req.URL.Path)
		q := req.URL.Query()
		require.Equal(t, "cursor_abc", q.Get("after"))
		require.Equal(t, "25", q.Get("limit"))
		require.Equal(t, "usd", q.Get("currency"))

		body := `{"data":[{"price_list_id":5,"variant_id":1001,"currency":"usd","price":9.99}],"meta":{"cursor_pagination":{"count":1}}}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListPriceListRecords(context.Background(), 5, PriceListRecordListParams{
		After:    "cursor_abc",
		Limit:    25,
		Currency: "usd",
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, 1001, rows[0].VariantID)
	require.Equal(t, 1, callCount, "cursor pagination should perform exactly one GET")
}

func TestDeletePriceListAssignmentsRequiresFilter(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected HTTP call: %s %s", req.Method, req.URL.String())
		return nil, nil
	}))

	err := c.DeletePriceListAssignments(context.Background(), PriceListAssignmentDeleteParams{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "at least one assignment filter is required")
}
