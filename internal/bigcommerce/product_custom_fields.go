package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListProductCustomFields fetches all custom fields for a product.
func (c *Client) ListProductCustomFields(ctx context.Context, productID int) ([]ProductCustomField, error) {
	path := fmt.Sprintf("catalog/products/%d/custom-fields", productID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list custom fields for product %d: %w", productID, err)
	}
	fields := make([]ProductCustomField, 0, len(raw))
	for _, r := range raw {
		var cf ProductCustomField
		if err := json.Unmarshal(r, &cf); err != nil {
			return nil, fmt.Errorf("unmarshal custom field: %w", err)
		}
		fields = append(fields, cf)
	}
	return fields, nil
}

// CreateProductCustomField adds a custom field to a product.
func (c *Client) CreateProductCustomField(ctx context.Context, productID int, payload ProductCustomFieldCreate) (*ProductCustomField, error) {
	path := fmt.Sprintf("catalog/products/%d/custom-fields", productID)
	respBody, err := c.Post(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("create custom field on product %d: %w", productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse custom field response: %w", err)
	}
	var cf ProductCustomField
	if err := json.Unmarshal(resp.Data, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal created custom field: %w", err)
	}
	return &cf, nil
}

// UpdateProductCustomField updates an existing custom field.
func (c *Client) UpdateProductCustomField(ctx context.Context, productID, fieldID int, payload ProductCustomFieldCreate) (*ProductCustomField, error) {
	path := fmt.Sprintf("catalog/products/%d/custom-fields/%d", productID, fieldID)
	respBody, err := c.Put(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("update custom field %d on product %d: %w", fieldID, productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse custom field response: %w", err)
	}
	var cf ProductCustomField
	if err := json.Unmarshal(resp.Data, &cf); err != nil {
		return nil, fmt.Errorf("unmarshal updated custom field: %w", err)
	}
	return &cf, nil
}

// DeleteProductCustomField removes a custom field from a product.
func (c *Client) DeleteProductCustomField(ctx context.Context, productID, fieldID int) error {
	path := fmt.Sprintf("catalog/products/%d/custom-fields/%d", productID, fieldID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete custom field %d on product %d: %w", fieldID, productID, err)
	}
	return nil
}
