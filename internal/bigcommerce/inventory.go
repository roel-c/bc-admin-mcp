package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ListInventoryLocations returns rows from GET /v3/inventory/locations.
// If page/limit is omitted, it auto-paginates using GetAll.
func (c *Client) ListInventoryLocations(ctx context.Context, params InventoryLocationListParams) ([]json.RawMessage, error) {
	path := "inventory/locations"
	vals := url.Values{}
	if params.Page > 0 {
		vals.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		vals.Set("limit", strconv.Itoa(params.Limit))
	}
	if q := vals.Encode(); q != "" {
		path += "?" + q
		body, err := c.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list inventory locations: %w", err)
		}
		rows, err := decodeV3PageData(body)
		if err != nil {
			return nil, fmt.Errorf("parse inventory locations response: %w", err)
		}
		return rows, nil
	}
	rows, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list inventory locations: %w", err)
	}
	return rows, nil
}

// CreateInventoryLocation creates one inventory location via
// POST /v3/inventory/locations.
func (c *Client) CreateInventoryLocation(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("inventory location payload is required")
	}
	body, err := c.Post(ctx, "inventory/locations", json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("create inventory location: %w", err)
	}
	return decodeV3DataOrBody(body), nil
}

// UpdateInventoryLocation updates one inventory location. BigCommerce's update
// is a BATCH operation — PUT /v3/inventory/locations with an ARRAY body where
// each object carries the immutable id (there is no per-id PUT path; hitting
// /inventory/locations/{id} returns 403). We inject the id into the caller's
// patch object and send a single-element array.
func (c *Client) UpdateInventoryLocation(ctx context.Context, locationID int, payload json.RawMessage) (json.RawMessage, error) {
	if locationID <= 0 {
		return nil, fmt.Errorf("location id must be positive")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("inventory location update payload is required")
	}
	var patch map[string]any
	if err := json.Unmarshal(payload, &patch); err != nil {
		return nil, fmt.Errorf("invalid inventory location update payload: %w", err)
	}
	patch["id"] = locationID
	body, err := c.Put(ctx, "inventory/locations", []map[string]any{patch})
	if err != nil {
		return nil, fmt.Errorf("update inventory location %d: %w", locationID, err)
	}
	return decodeV3DataOrBody(body), nil
}

// DeleteInventoryLocation deletes one inventory location. BigCommerce's delete
// is a BATCH operation — DELETE /v3/inventory/locations?location_id:in=… (there
// is no per-id DELETE path; /inventory/locations/{id} returns 403, and the
// query param is location_id:in, NOT id:in).
func (c *Client) DeleteInventoryLocation(ctx context.Context, locationID int) error {
	if locationID <= 0 {
		return fmt.Errorf("location id must be positive")
	}
	if _, err := c.Delete(ctx, fmt.Sprintf("inventory/locations?location_id:in=%d", locationID)); err != nil {
		return fmt.Errorf("delete inventory location %d: %w", locationID, err)
	}
	return nil
}

// ListInventoryLocationMetafields returns metafields from
// GET /v3/inventory/locations/{id}/metafields.
// If page/limit is omitted, it auto-paginates all rows using GetAll.
func (c *Client) ListInventoryLocationMetafields(ctx context.Context, locationID int, params InventoryLocationMetafieldListParams) ([]Metafield, error) {
	if locationID <= 0 {
		return nil, fmt.Errorf("location id must be positive")
	}
	path := fmt.Sprintf("inventory/locations/%d/metafields", locationID)
	if params.Page > 0 || params.Limit > 0 {
		vals := url.Values{}
		if params.Page > 0 {
			vals.Set("page", strconv.Itoa(params.Page))
		}
		if params.Limit > 0 {
			vals.Set("limit", strconv.Itoa(params.Limit))
		}
		if q := vals.Encode(); q != "" {
			path += "?" + q
		}
		body, err := c.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list metafields for inventory location %d: %w", locationID, err)
		}
		rows, err := decodeV3PageData(body)
		if err != nil {
			return nil, fmt.Errorf("parse inventory location metafields response: %w", err)
		}
		out := make([]Metafield, 0, len(rows))
		for _, r := range rows {
			var mf Metafield
			if err := json.Unmarshal(r, &mf); err != nil {
				return nil, fmt.Errorf("unmarshal inventory location metafield: %w", err)
			}
			out = append(out, mf)
		}
		return out, nil
	}
	rows, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list metafields for inventory location %d: %w", locationID, err)
	}
	out := make([]Metafield, 0, len(rows))
	for _, r := range rows {
		var mf Metafield
		if err := json.Unmarshal(r, &mf); err != nil {
			return nil, fmt.Errorf("unmarshal inventory location metafield: %w", err)
		}
		out = append(out, mf)
	}
	return out, nil
}

// CreateInventoryLocationMetafield creates one location metafield via
// POST /v3/inventory/locations/{id}/metafields.
func (c *Client) CreateInventoryLocationMetafield(ctx context.Context, locationID int, mf Metafield) (*Metafield, error) {
	if locationID <= 0 {
		return nil, fmt.Errorf("location id must be positive")
	}
	path := fmt.Sprintf("inventory/locations/%d/metafields", locationID)
	body, err := c.Post(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("create metafield on inventory location %d: %w", locationID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse create inventory location metafield response: %w", err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("create inventory location metafield response missing data")
	}
	var created Metafield
	if err := json.Unmarshal(resp.Data, &created); err != nil {
		return nil, fmt.Errorf("unmarshal created inventory location metafield: %w", err)
	}
	return &created, nil
}

// UpdateInventoryLocationMetafield updates one location metafield via
// PUT /v3/inventory/locations/{id}/metafields/{metafield_id}.
func (c *Client) UpdateInventoryLocationMetafield(ctx context.Context, locationID, metafieldID int, mf Metafield) (*Metafield, error) {
	if locationID <= 0 {
		return nil, fmt.Errorf("location id must be positive")
	}
	if metafieldID <= 0 {
		return nil, fmt.Errorf("metafield id must be positive")
	}
	path := fmt.Sprintf("inventory/locations/%d/metafields/%d", locationID, metafieldID)
	body, err := c.Put(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("update metafield %d on inventory location %d: %w", metafieldID, locationID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse update inventory location metafield response: %w", err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("update inventory location metafield response missing data")
	}
	var updated Metafield
	if err := json.Unmarshal(resp.Data, &updated); err != nil {
		return nil, fmt.Errorf("unmarshal updated inventory location metafield: %w", err)
	}
	return &updated, nil
}

// DeleteInventoryLocationMetafield deletes one location metafield via
// DELETE /v3/inventory/locations/{id}/metafields/{metafield_id}.
func (c *Client) DeleteInventoryLocationMetafield(ctx context.Context, locationID, metafieldID int) error {
	if locationID <= 0 {
		return fmt.Errorf("location id must be positive")
	}
	if metafieldID <= 0 {
		return fmt.Errorf("metafield id must be positive")
	}
	path := fmt.Sprintf("inventory/locations/%d/metafields/%d", locationID, metafieldID)
	if _, err := c.Delete(ctx, path); err != nil {
		return fmt.Errorf("delete metafield %d on inventory location %d: %w", metafieldID, locationID, err)
	}
	return nil
}

// ListInventoryItems returns rows from GET /v3/inventory/items.
// If page/limit is omitted, it auto-paginates using GetAll.
func (c *Client) ListInventoryItems(ctx context.Context, params InventoryItemListParams) ([]json.RawMessage, error) {
	path := "inventory/items"
	vals := url.Values{}
	if len(params.LocationIDs) > 0 {
		vals.Set("location_id:in", joinInts(params.LocationIDs))
	}
	if len(params.ProductIDs) > 0 {
		vals.Set("product_id:in", joinInts(params.ProductIDs))
	}
	if len(params.VariantIDs) > 0 {
		vals.Set("variant_id:in", joinInts(params.VariantIDs))
	}
	if len(params.SKUs) > 0 {
		vals.Set("sku:in", strings.Join(params.SKUs, ","))
	}
	if params.Page > 0 {
		vals.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		vals.Set("limit", strconv.Itoa(params.Limit))
	}
	if q := vals.Encode(); q != "" {
		path += "?" + q
		if params.Page > 0 || params.Limit > 0 {
			body, err := c.Get(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("list inventory items: %w", err)
			}
			rows, err := decodeV3PageData(body)
			if err != nil {
				return nil, fmt.Errorf("parse inventory items response: %w", err)
			}
			return rows, nil
		}
	}
	rows, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list inventory items: %w", err)
	}
	return rows, nil
}

// GetInventoryItem fetches one variant's inventory record. BigCommerce has NO
// single-item GET (/v3/inventory/items/{id} does not exist and 403/404s) — the
// items API is list-only, so we query the list endpoint filtered by
// variant_id:in and return the single matching row.
func (c *Client) GetInventoryItem(ctx context.Context, variantID int) (json.RawMessage, error) {
	if variantID <= 0 {
		return nil, fmt.Errorf("variant id must be positive")
	}
	body, err := c.Get(ctx, fmt.Sprintf("inventory/items?variant_id:in=%d&limit=1", variantID))
	if err != nil {
		return nil, fmt.Errorf("get inventory item %d: %w", variantID, err)
	}
	rows, err := decodeV3PageData(body)
	if err != nil {
		return nil, fmt.Errorf("parse inventory item response: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no inventory item found for variant %d (it may not be inventory-tracked)", variantID)
	}
	return rows[0], nil
}

// CreateInventoryAbsoluteAdjustment submits one absolute adjustment batch via
// PUT /v3/inventory/adjustments/absolute.
func (c *Client) CreateInventoryAbsoluteAdjustment(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("absolute adjustment payload is required")
	}
	body, err := c.Put(ctx, "inventory/adjustments/absolute", json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("create absolute inventory adjustment: %w", err)
	}
	return decodeV3DataOrBody(body), nil
}

// CreateInventoryRelativeAdjustment submits one relative adjustment batch via
// POST /v3/inventory/adjustments/relative.
func (c *Client) CreateInventoryRelativeAdjustment(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("relative adjustment payload is required")
	}
	body, err := c.Post(ctx, "inventory/adjustments/relative", json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("create relative inventory adjustment: %w", err)
	}
	return decodeV3DataOrBody(body), nil
}

// UpdateInventoryItems submits one inventory item batch update via
// PUT /v3/inventory/items.
func (c *Client) UpdateInventoryItems(ctx context.Context, payload json.RawMessage) (json.RawMessage, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("inventory items update payload is required")
	}
	body, err := c.Put(ctx, "inventory/items", json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("update inventory items: %w", err)
	}
	return decodeV3DataOrBody(body), nil
}

// ListInventoryLocationItems returns rows from
// GET /v3/inventory/locations/{location_id}/items.
// If page/limit is omitted, it auto-paginates using GetAll.
func (c *Client) ListInventoryLocationItems(ctx context.Context, locationID int, params InventoryLocationItemListParams) ([]json.RawMessage, error) {
	if locationID <= 0 {
		return nil, fmt.Errorf("location id must be positive")
	}
	path := fmt.Sprintf("inventory/locations/%d/items", locationID)
	vals := url.Values{}
	if len(params.ProductIDs) > 0 {
		vals.Set("product_id:in", joinInts(params.ProductIDs))
	}
	if len(params.VariantIDs) > 0 {
		vals.Set("variant_id:in", joinInts(params.VariantIDs))
	}
	if len(params.SKUs) > 0 {
		vals.Set("sku:in", strings.Join(params.SKUs, ","))
	}
	if params.Page > 0 {
		vals.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		vals.Set("limit", strconv.Itoa(params.Limit))
	}
	if q := vals.Encode(); q != "" {
		path += "?" + q
		if params.Page > 0 || params.Limit > 0 {
			body, err := c.Get(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("list inventory location items: %w", err)
			}
			rows, err := decodeV3PageData(body)
			if err != nil {
				return nil, fmt.Errorf("parse inventory location items response: %w", err)
			}
			return rows, nil
		}
	}
	rows, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list inventory location items: %w", err)
	}
	return rows, nil
}

// UpdateInventoryLocationItems submits location-scoped inventory settings via
// PUT /v3/inventory/locations/{location_id}/items (e.g. backorder_limit).
func (c *Client) UpdateInventoryLocationItems(ctx context.Context, locationID int, payload json.RawMessage) (json.RawMessage, error) {
	if locationID <= 0 {
		return nil, fmt.Errorf("location id must be positive")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("inventory location items update payload is required")
	}
	path := fmt.Sprintf("inventory/locations/%d/items", locationID)
	body, err := c.Put(ctx, path, json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("update inventory location items: %w", err)
	}
	return decodeV3DataOrBody(body), nil
}

func decodeV3PageData(body []byte) ([]json.RawMessage, error) {
	var resp PaginatedResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func decodeV3DataOrBody(body []byte) json.RawMessage {
	var single SingleResponse
	if err := json.Unmarshal(body, &single); err == nil && len(single.Data) > 0 && string(single.Data) != "null" {
		return single.Data
	}
	return json.RawMessage(body)
}
