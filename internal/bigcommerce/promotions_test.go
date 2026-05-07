package bigcommerce

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestPromotionsClient(t *testing.T, rt http.RoundTripper) *Client {
	t.Helper()
	cfg := config.BigCommerceConfig{
		StoreHash:           "hash",
		AuthToken:           "token",
		RequestsPerSecond:   1000,
		QuotaSafetyBuffer:   1,
		MaxRetries:          1,
		DefaultPageLimit:    250,
		MaxTotalRecords:     10000,
		DelayBetweenChunks:  0,
		MaxWriteConcurrency: 1,
	}
	c := NewClient(cfg, nil)
	t.Cleanup(c.Close)
	c.httpClient = &http.Client{Transport: rt}
	return c
}

func TestSearchPromotionsSinglePageWhenPageOrLimitProvided(t *testing.T) {
	callCount := 0
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		q := req.URL.Query()
		require.Equal(t, []string{"2"}, q["page"])
		require.Equal(t, []string{"25"}, q["limit"])
		require.Len(t, q["page"], 1)
		require.Len(t, q["limit"], 1)
		body := `{"data":[{"id":101,"name":"Paged Promo","redemption_type":"AUTOMATIC"}],"meta":{"pagination":{"current_page":2,"total_pages":9}}}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{},
		}, nil
	}))

	promos, err := c.SearchPromotions(context.Background(), PromotionListParams{
		RedemptionType: "automatic",
		Page:           2,
		Limit:          25,
	})
	require.NoError(t, err)
	require.Len(t, promos, 1)
	require.Equal(t, 101, promos[0].ID)
	require.Equal(t, 1, callCount, "single-page query should perform exactly one GET")
}

func TestSearchPromotionsAutoPaginatesWithoutPageOrLimit(t *testing.T) {
	callCount := 0
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		q := req.URL.Query()
		require.Len(t, q["page"], 1)
		require.Len(t, q["limit"], 1)
		switch q.Get("page") {
		case "1":
			body := `{"data":[{"id":201,"name":"Promo 1","redemption_type":"COUPON"}],"meta":{"pagination":{"current_page":1,"total_pages":2}}}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     http.Header{},
			}, nil
		case "2":
			body := `{"data":[{"id":202,"name":"Promo 2","redemption_type":"COUPON"}],"meta":{"pagination":{"current_page":2,"total_pages":2}}}`
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

	promos, err := c.SearchPromotions(context.Background(), PromotionListParams{
		RedemptionType: "coupon",
	})
	require.NoError(t, err)
	require.Len(t, promos, 2)
	require.Equal(t, 201, promos[0].ID)
	require.Equal(t, 202, promos[1].ID)
	require.Equal(t, 2, callCount, "auto-pagination should fetch all pages")
}

