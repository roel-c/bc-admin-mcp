package bigcommerce

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListInventoryLocationsUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/locations", req.URL.Path)
		require.Equal(t, "2", req.URL.Query().Get("page"))
		require.Equal(t, "25", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":1,"name":"Main"}]}`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListInventoryLocations(context.Background(), InventoryLocationListParams{Page: 2, Limit: 25})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"Main"`)
}

func TestListInventoryItemsUsesFilters(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/items", req.URL.Path)
		q := req.URL.Query()
		require.Equal(t, "5", q.Get("location_id:in"))
		require.Equal(t, "11", q.Get("product_id:in"))
		require.Equal(t, "44,45", q.Get("variant_id:in"))
		require.Equal(t, "SKU-1,SKU-2", q.Get("sku:in"))
		require.Equal(t, "1", q.Get("page"))
		require.Equal(t, "50", q.Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"variant_id":44}]}`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListInventoryItems(context.Background(), InventoryItemListParams{
		LocationIDs: []int{5},
		ProductIDs:  []int{11},
		VariantIDs:  []int{44, 45},
		SKUs:        []string{"SKU-1", "SKU-2"},
		Page:        1,
		Limit:       50,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Contains(t, string(rows[0]), `"variant_id":44`)
}

func TestCreateInventoryLocationUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/locations", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"name":"Warehouse East"`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"id":7,"name":"Warehouse East"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	resp, err := c.CreateInventoryLocation(context.Background(), json.RawMessage(`{"name":"Warehouse East","code":"WH-EAST"}`))
	require.NoError(t, err)
	require.Contains(t, string(resp), `"id":7`)
}

func TestUpdateInventoryLocationUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPut, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/locations/7", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"name":"Warehouse East 2"`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"id":7,"name":"Warehouse East 2"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	resp, err := c.UpdateInventoryLocation(context.Background(), 7, json.RawMessage(`{"name":"Warehouse East 2"}`))
	require.NoError(t, err)
	require.Contains(t, string(resp), `"Warehouse East 2"`)
}

func TestDeleteInventoryLocationUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodDelete, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/locations/7", req.URL.Path)
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{},
		}, nil
	}))

	err := c.DeleteInventoryLocation(context.Background(), 7)
	require.NoError(t, err)
}

func TestListInventoryLocationMetafieldsUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/locations/7/metafields", req.URL.Path)
		require.Equal(t, "1", req.URL.Query().Get("page"))
		require.Equal(t, "25", req.URL.Query().Get("limit"))
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"id":88,"namespace":"ops","key":"zone","value":"east"}]}`)),
			Header:     http.Header{},
		}, nil
	}))

	rows, err := c.ListInventoryLocationMetafields(context.Background(), 7, InventoryLocationMetafieldListParams{
		Page:  1,
		Limit: 25,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "ops", rows[0].Namespace)
	require.Equal(t, "zone", rows[0].Key)
}

func TestCreateInventoryLocationMetafieldUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/locations/7/metafields", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"namespace":"ops"`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"id":89,"namespace":"ops","key":"zone","value":"east"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	created, err := c.CreateInventoryLocationMetafield(context.Background(), 7, Metafield{
		Namespace:     "ops",
		Key:           "zone",
		Value:         "east",
		PermissionSet: "app_only",
	})
	require.NoError(t, err)
	require.Equal(t, 89, created.ID)
}

func TestUpdateInventoryLocationMetafieldUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPut, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/locations/7/metafields/89", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"value":"west"`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"id":89,"namespace":"ops","key":"zone","value":"west"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	updated, err := c.UpdateInventoryLocationMetafield(context.Background(), 7, 89, Metafield{
		Namespace:     "ops",
		Key:           "zone",
		Value:         "west",
		PermissionSet: "app_only",
	})
	require.NoError(t, err)
	require.Equal(t, "west", updated.Value)
}

func TestDeleteInventoryLocationMetafieldUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodDelete, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/locations/7/metafields/89", req.URL.Path)
		return &http.Response{
			StatusCode: 204,
			Body:       io.NopCloser(strings.NewReader("")),
			Header:     http.Header{},
		}, nil
	}))

	err := c.DeleteInventoryLocationMetafield(context.Background(), 7, 89)
	require.NoError(t, err)
}

func TestGetInventoryItemUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodGet, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/items/44", req.URL.Path)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"variant_id":44,"available_to_sell":7}}`)),
			Header:     http.Header{},
		}, nil
	}))

	row, err := c.GetInventoryItem(context.Background(), 44)
	require.NoError(t, err)
	require.Contains(t, string(row), `"available_to_sell":7`)
}

func TestCreateInventoryAbsoluteAdjustmentUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPut, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/adjustments/absolute", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"reason":"cycle_count"`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"transaction_id":"txn_abs_1"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	resp, err := c.CreateInventoryAbsoluteAdjustment(context.Background(), json.RawMessage(`{"reason":"cycle_count","items":[{"location_id":1,"variant_id":44,"quantity":10}]}`))
	require.NoError(t, err)
	require.Contains(t, string(resp), `"transaction_id":"txn_abs_1"`)
}

func TestCreateInventoryRelativeAdjustmentUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/adjustments/relative", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"reason":"order_adjustment"`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"transaction_id":"txn_rel_1"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	resp, err := c.CreateInventoryRelativeAdjustment(context.Background(), json.RawMessage(`{"reason":"order_adjustment","items":[{"location_id":1,"variant_id":44,"quantity":-1}]}`))
	require.NoError(t, err)
	require.Contains(t, string(resp), `"transaction_id":"txn_rel_1"`)
}

func TestUpdateInventoryItemsUsesV3Endpoint(t *testing.T) {
	c := newTestPromotionsClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPut, req.Method)
		require.Equal(t, "/stores/hash/v3/inventory/items", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), `"items"`)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":{"transaction_id":"txn_items_1"}}`)),
			Header:     http.Header{},
		}, nil
	}))

	resp, err := c.UpdateInventoryItems(context.Background(), json.RawMessage(`{"items":[{"location_id":1,"variant_id":44,"safety_stock":2}]}`))
	require.NoError(t, err)
	require.Contains(t, string(resp), `"transaction_id":"txn_items_1"`)
}
