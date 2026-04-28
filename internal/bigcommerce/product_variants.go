package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// GetVariant fetches a single variant by product and variant ID.
func (c *Client) GetVariant(ctx context.Context, productID, variantID int) (*ProductVariantFull, error) {
	path := fmt.Sprintf("catalog/products/%d/variants/%d", productID, variantID)
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get variant %d on product %d: %w", variantID, productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse variant response: %w", err)
	}
	var v ProductVariantFull
	if err := json.Unmarshal(resp.Data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal variant: %w", err)
	}
	return &v, nil
}

// CreateVariant adds a new variant to a product.
func (c *Client) CreateVariant(ctx context.Context, productID int, payload ProductVariantCreate) (*ProductVariantFull, error) {
	path := fmt.Sprintf("catalog/products/%d/variants", productID)
	respBody, err := c.Post(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("create variant on product %d: %w", productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse variant response: %w", err)
	}
	var v ProductVariantFull
	if err := json.Unmarshal(resp.Data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal created variant: %w", err)
	}
	return &v, nil
}

// UpdateVariant updates a single variant via PUT /v3/catalog/products/{id}/variants/{vid}.
func (c *Client) UpdateVariant(ctx context.Context, productID, variantID int, payload ProductVariantUpdate) (*ProductVariantFull, error) {
	path := fmt.Sprintf("catalog/products/%d/variants/%d", productID, variantID)
	respBody, err := c.Put(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("update variant %d on product %d: %w", variantID, productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse variant response: %w", err)
	}
	var v ProductVariantFull
	if err := json.Unmarshal(resp.Data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal updated variant: %w", err)
	}
	return &v, nil
}

// DeleteVariant removes a variant from a product.
func (c *Client) DeleteVariant(ctx context.Context, productID, variantID int) error {
	path := fmt.Sprintf("catalog/products/%d/variants/%d", productID, variantID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete variant %d on product %d: %w", variantID, productID, err)
	}
	return nil
}
