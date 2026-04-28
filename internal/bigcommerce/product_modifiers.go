package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListProductModifiers fetches all modifiers for a product.
func (c *Client) ListProductModifiers(ctx context.Context, productID int) ([]ProductModifier, error) {
	path := fmt.Sprintf("catalog/products/%d/modifiers", productID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list modifiers for product %d: %w", productID, err)
	}
	mods := make([]ProductModifier, 0, len(raw))
	for _, r := range raw {
		var m ProductModifier
		if err := json.Unmarshal(r, &m); err != nil {
			return nil, fmt.Errorf("unmarshal product modifier: %w", err)
		}
		mods = append(mods, m)
	}
	return mods, nil
}

// CreateProductModifier adds a modifier to a product.
func (c *Client) CreateProductModifier(ctx context.Context, productID int, payload ProductModifierCreate) (*ProductModifier, error) {
	path := fmt.Sprintf("catalog/products/%d/modifiers", productID)
	respBody, err := c.Post(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("create modifier on product %d: %w", productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse modifier response: %w", err)
	}
	var mod ProductModifier
	if err := json.Unmarshal(resp.Data, &mod); err != nil {
		return nil, fmt.Errorf("unmarshal created modifier: %w", err)
	}
	return &mod, nil
}

// DeleteProductModifier removes a modifier from a product.
func (c *Client) DeleteProductModifier(ctx context.Context, productID, modifierID int) error {
	path := fmt.Sprintf("catalog/products/%d/modifiers/%d", productID, modifierID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete modifier %d on product %d: %w", modifierID, productID, err)
	}
	return nil
}
