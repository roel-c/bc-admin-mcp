package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListProductOptions fetches all variant-generating options for a product.
func (c *Client) ListProductOptions(ctx context.Context, productID int) ([]ProductOption, error) {
	path := fmt.Sprintf("catalog/products/%d/options", productID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list options for product %d: %w", productID, err)
	}
	opts := make([]ProductOption, 0, len(raw))
	for _, r := range raw {
		var o ProductOption
		if err := json.Unmarshal(r, &o); err != nil {
			return nil, fmt.Errorf("unmarshal product option: %w", err)
		}
		opts = append(opts, o)
	}
	return opts, nil
}

// CreateProductOption adds a variant-generating option to a product.
func (c *Client) CreateProductOption(ctx context.Context, productID int, payload ProductOptionCreate) (*ProductOption, error) {
	path := fmt.Sprintf("catalog/products/%d/options", productID)
	respBody, err := c.Post(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("create option on product %d: %w", productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse option response: %w", err)
	}
	var opt ProductOption
	if err := json.Unmarshal(resp.Data, &opt); err != nil {
		return nil, fmt.Errorf("unmarshal created option: %w", err)
	}
	return &opt, nil
}

// UpdateProductOption updates an existing product option.
func (c *Client) UpdateProductOption(ctx context.Context, productID, optionID int, payload ProductOptionUpdate) (*ProductOption, error) {
	path := fmt.Sprintf("catalog/products/%d/options/%d", productID, optionID)
	respBody, err := c.Put(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("update option %d on product %d: %w", optionID, productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse option response: %w", err)
	}
	var opt ProductOption
	if err := json.Unmarshal(resp.Data, &opt); err != nil {
		return nil, fmt.Errorf("unmarshal updated option: %w", err)
	}
	return &opt, nil
}

// DeleteProductOption removes a variant-generating option from a product.
func (c *Client) DeleteProductOption(ctx context.Context, productID, optionID int) error {
	path := fmt.Sprintf("catalog/products/%d/options/%d", productID, optionID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete option %d on product %d: %w", optionID, productID, err)
	}
	return nil
}
