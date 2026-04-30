package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// SearchVariants lists variants via GET /v3/catalog/variants with arbitrary
// query parameters (e.g. product_id, product_id:in, id, id:in, sku, sort).
func (c *Client) SearchVariants(ctx context.Context, params map[string]string) ([]Variant, error) {
	path := "catalog/variants"
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	if encoded := vals.Encode(); encoded != "" {
		path += "?" + encoded
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("search variants: %w", err)
	}
	out := make([]Variant, 0, len(raw))
	for _, r := range raw {
		var v Variant
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, fmt.Errorf("unmarshal variant: %w", err)
		}
		out = append(out, v)
	}
	return out, nil
}

// ListVariantsByProductIDs batch-fetches variants for multiple products using
// GET /v3/catalog/variants?product_id:in=… (chunked for URL length).
func (c *Client) ListVariantsByProductIDs(ctx context.Context, productIDs []int) ([]Variant, error) {
	if len(productIDs) == 0 {
		return nil, nil
	}

	const chunkSize = 100
	var all []Variant
	for i := 0; i < len(productIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(productIDs) {
			end = len(productIDs)
		}
		chunk := productIDs[i:end]
		strs := make([]string, len(chunk))
		for j, id := range chunk {
			strs[j] = strconv.Itoa(id)
		}
		params := map[string]string{
			"product_id:in": strings.Join(strs, ","),
		}
		part, err := c.SearchVariants(ctx, params)
		if err != nil {
			return nil, fmt.Errorf("list variants for products (offset %d): %w", i, err)
		}
		all = append(all, part...)
	}
	return all, nil
}

// BatchUpdateVariants updates many variants via PUT /v3/catalog/variants in
// chunks of cfg.VariantBatchSize (default 10 per BC-Tool-Boundaries.md).
func (c *Client) BatchUpdateVariants(ctx context.Context, updates []CatalogVariantUpdate) (*BatchResult, error) {
	if len(updates) == 0 {
		return &BatchResult{}, nil
	}
	items := make([]any, len(updates))
	for i := range updates {
		items[i] = updates[i]
	}
	return c.BatchPut(ctx, "catalog/variants", items, c.cfg.VariantBatchSize)
}
