package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// SearchBrands fetches brands with arbitrary query parameters (e.g. name,
// name:like, keyword, page_title) using GET /v3/catalog/brands.
func (c *Client) SearchBrands(ctx context.Context, params map[string]string) ([]Brand, error) {
	path := "catalog/brands"
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	if encoded := vals.Encode(); encoded != "" {
		path += "?" + encoded
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("search brands: %w", err)
	}
	out := make([]Brand, 0, len(raw))
	for _, r := range raw {
		var b Brand
		if err := json.Unmarshal(r, &b); err != nil {
			return nil, fmt.Errorf("unmarshal brand: %w", err)
		}
		out = append(out, b)
	}
	return out, nil
}

// GetBrand fetches a single brand by ID via GET /v3/catalog/brands/{id}.
func (c *Client) GetBrand(ctx context.Context, brandID int) (*Brand, error) {
	body, err := c.Get(ctx, fmt.Sprintf("catalog/brands/%d", brandID))
	if err != nil {
		return nil, fmt.Errorf("get brand %d: %w", brandID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse brand %d: %w", brandID, err)
	}
	var b Brand
	if err := json.Unmarshal(resp.Data, &b); err != nil {
		return nil, fmt.Errorf("unmarshal brand %d: %w", brandID, err)
	}
	return &b, nil
}

// CreateBrand creates a brand via POST /v3/catalog/brands.
func (c *Client) CreateBrand(ctx context.Context, payload BrandCreate) (*Brand, error) {
	respBody, err := c.Post(ctx, "catalog/brands", payload)
	if err != nil {
		return nil, fmt.Errorf("create brand: %w", err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse create brand response: %w", err)
	}
	var created Brand
	if err := json.Unmarshal(resp.Data, &created); err != nil {
		return nil, fmt.Errorf("unmarshal created brand: %w", err)
	}
	return &created, nil
}

// DeleteBrand deletes a single brand via DELETE /v3/catalog/brands/{id}.
// Products previously assigned to the brand remain; their brand_id is cleared.
func (c *Client) DeleteBrand(ctx context.Context, brandID int) error {
	if brandID <= 0 {
		return fmt.Errorf("brand id must be positive")
	}
	if _, err := c.Delete(ctx, fmt.Sprintf("catalog/brands/%d", brandID)); err != nil {
		return fmt.Errorf("delete brand %d: %w", brandID, err)
	}
	return nil
}

// DeleteBrandImage removes a brand's image via
// DELETE /v3/catalog/brands/{id}/image.
func (c *Client) DeleteBrandImage(ctx context.Context, brandID int) error {
	if brandID <= 0 {
		return fmt.Errorf("brand id must be positive")
	}
	if _, err := c.Delete(ctx, fmt.Sprintf("catalog/brands/%d/image", brandID)); err != nil {
		return fmt.Errorf("delete brand %d image: %w", brandID, err)
	}
	return nil
}

// UpdateBrand updates a brand via PUT /v3/catalog/brands/{id}.
func (c *Client) UpdateBrand(ctx context.Context, brandID int, payload BrandUpdate) (*Brand, error) {
	path := fmt.Sprintf("catalog/brands/%d", brandID)
	respBody, err := c.Put(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("update brand %d: %w", brandID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse update brand response: %w", err)
	}
	var updated Brand
	if err := json.Unmarshal(resp.Data, &updated); err != nil {
		return nil, fmt.Errorf("unmarshal updated brand: %w", err)
	}
	return &updated, nil
}
