package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

// ListBrandMetafields fetches all metafields for a brand.
func (c *Client) ListBrandMetafields(ctx context.Context, brandID int) ([]Metafield, error) {
	path := fmt.Sprintf("catalog/brands/%d/metafields", brandID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list metafields for brand %d: %w", brandID, err)
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

// CreateBrandMetafield creates a new metafield on a brand.
func (c *Client) CreateBrandMetafield(ctx context.Context, brandID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("catalog/brands/%d/metafields", brandID)
	respBody, err := c.Post(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("create metafield on brand %d: %w", brandID, err)
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

// UpdateBrandMetafield updates an existing metafield on a brand.
func (c *Client) UpdateBrandMetafield(ctx context.Context, brandID, mfID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("catalog/brands/%d/metafields/%d", brandID, mfID)
	respBody, err := c.Put(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("update metafield %d on brand %d: %w", mfID, brandID, err)
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

// DeleteBrandMetafield deletes a metafield from a brand.
func (c *Client) DeleteBrandMetafield(ctx context.Context, brandID, mfID int) error {
	path := fmt.Sprintf("catalog/brands/%d/metafields/%d", brandID, mfID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete metafield %d on brand %d: %w", mfID, brandID, err)
	}
	return nil
}

// ListProductMetafields fetches all metafields for a product.
func (c *Client) ListProductMetafields(ctx context.Context, productID int) ([]Metafield, error) {
	path := fmt.Sprintf("catalog/products/%d/metafields", productID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list metafields for product %d: %w", productID, err)
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

// CreateProductMetafield creates a new metafield on a product.
func (c *Client) CreateProductMetafield(ctx context.Context, productID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("catalog/products/%d/metafields", productID)
	respBody, err := c.Post(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("create metafield on product %d: %w", productID, err)
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

// UpdateProductMetafield updates an existing metafield on a product.
func (c *Client) UpdateProductMetafield(ctx context.Context, productID, mfID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("catalog/products/%d/metafields/%d", productID, mfID)
	respBody, err := c.Put(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("update metafield %d on product %d: %w", mfID, productID, err)
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

// DeleteProductMetafield deletes a metafield from a product.
func (c *Client) DeleteProductMetafield(ctx context.Context, productID, mfID int) error {
	path := fmt.Sprintf("catalog/products/%d/metafields/%d", productID, mfID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete metafield %d on product %d: %w", mfID, productID, err)
	}
	return nil
}

// ListVariantMetafields fetches all metafields for a product variant.
func (c *Client) ListVariantMetafields(ctx context.Context, productID, variantID int) ([]Metafield, error) {
	path := fmt.Sprintf("catalog/products/%d/variants/%d/metafields", productID, variantID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list metafields for variant %d on product %d: %w", variantID, productID, err)
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

// CreateVariantMetafield creates a new metafield on a product variant.
func (c *Client) CreateVariantMetafield(ctx context.Context, productID, variantID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("catalog/products/%d/variants/%d/metafields", productID, variantID)
	respBody, err := c.Post(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("create metafield on variant %d (product %d): %w", variantID, productID, err)
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

// UpdateVariantMetafield updates an existing metafield on a product variant.
func (c *Client) UpdateVariantMetafield(ctx context.Context, productID, variantID, mfID int, mf Metafield) (*Metafield, error) {
	path := fmt.Sprintf("catalog/products/%d/variants/%d/metafields/%d", productID, variantID, mfID)
	respBody, err := c.Put(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("update metafield %d on variant %d (product %d): %w", mfID, variantID, productID, err)
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

// DeleteVariantMetafield deletes a metafield from a product variant.
func (c *Client) DeleteVariantMetafield(ctx context.Context, productID, variantID, mfID int) error {
	path := fmt.Sprintf("catalog/products/%d/variants/%d/metafields/%d", productID, variantID, mfID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete metafield %d from variant %d (product %d): %w", mfID, variantID, productID, err)
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

// DeleteCategoryAssignmentsByFilter removes assignments matching the given
// product_id:in and category_id:in filters via DELETE /v3/catalog/products/category-assignments.
// At least one of the slices must be non-empty (BigCommerce returns 422 otherwise).
func (c *Client) DeleteCategoryAssignmentsByFilter(ctx context.Context, productIDs, categoryIDs []int) error {
	if len(productIDs) == 0 && len(categoryIDs) == 0 {
		return fmt.Errorf("at least one of product IDs or category IDs is required for delete")
	}
	parts := []string{}
	if len(productIDs) > 0 {
		parts = append(parts, "product_id:in="+joinIntsURL(productIDs))
	}
	if len(categoryIDs) > 0 {
		parts = append(parts, "category_id:in="+joinIntsURL(categoryIDs))
	}
	path := "catalog/products/category-assignments?" + strings.Join(parts, "&")
	if _, err := c.Delete(ctx, path); err != nil {
		return fmt.Errorf("delete category assignments by filter: %w", err)
	}
	return nil
}

func joinIntsURL(ids []int) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.Itoa(id)
	}
	return strings.Join(parts, ",")
}
