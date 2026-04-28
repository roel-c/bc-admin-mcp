package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListCategoryMetafields fetches all metafields for a category.
// Uses the legacy path /v3/catalog/categories/{id}/metafields (not the tree path).
func (c *Client) ListCategoryMetafields(ctx context.Context, categoryID int) ([]Metafield, error) {
	path := fmt.Sprintf("catalog/categories/%d/metafields", categoryID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list metafields for category %d: %w", categoryID, err)
	}
	mfs := make([]Metafield, 0, len(raw))
	for _, r := range raw {
		var mf Metafield
		if err := json.Unmarshal(r, &mf); err != nil {
			return nil, fmt.Errorf("unmarshal metafield: %w", err)
		}
		mfs = append(mfs, mf)
	}
	return mfs, nil
}

// CreateCategoryMetafield creates a new metafield on a category.
func (c *Client) CreateCategoryMetafield(ctx context.Context, categoryID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("catalog/categories/%d/metafields", categoryID)
	respBody, err := c.Post(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("create metafield on category %d: %w", categoryID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse metafield response: %w", err)
	}
	var created Metafield
	if err := json.Unmarshal(resp.Data, &created); err != nil {
		return nil, fmt.Errorf("unmarshal created metafield: %w", err)
	}
	return &created, nil
}

// UpdateCategoryMetafield updates an existing metafield on a category.
func (c *Client) UpdateCategoryMetafield(ctx context.Context, categoryID, mfID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("catalog/categories/%d/metafields/%d", categoryID, mfID)
	respBody, err := c.Put(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("update metafield %d on category %d: %w", mfID, categoryID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse metafield response: %w", err)
	}
	var updated Metafield
	if err := json.Unmarshal(resp.Data, &updated); err != nil {
		return nil, fmt.Errorf("unmarshal updated metafield: %w", err)
	}
	return &updated, nil
}

// DeleteCategoryMetafield deletes a metafield from a category.
func (c *Client) DeleteCategoryMetafield(ctx context.Context, categoryID, mfID int) error {
	path := fmt.Sprintf("catalog/categories/%d/metafields/%d", categoryID, mfID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete metafield %d on category %d: %w", mfID, categoryID, err)
	}
	return nil
}

// UpsertCategoryAssignments creates or updates product-to-category assignments
// via PUT /v3/catalog/products/category-assignments.
func (c *Client) UpsertCategoryAssignments(ctx context.Context, assignments []CategoryAssignment) error {
	_, err := c.Put(ctx, "catalog/products/category-assignments", assignments)
	if err != nil {
		return fmt.Errorf("upsert category assignments: %w", err)
	}
	return nil
}

// DeleteCategoryAssignments removes product-to-category assignments
// via DELETE /v3/catalog/products/category-assignments with query params.
func (c *Client) DeleteCategoryAssignments(ctx context.Context, productID, categoryID int) error {
	path := fmt.Sprintf("catalog/products/category-assignments?product_id=%d&category_id=%d", productID, categoryID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete category assignment (product %d, category %d): %w", productID, categoryID, err)
	}
	return nil
}
