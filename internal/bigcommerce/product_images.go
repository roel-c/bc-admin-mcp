package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// ListProductImages fetches all images for a product.
func (c *Client) ListProductImages(ctx context.Context, productID int) ([]ProductImage, error) {
	path := fmt.Sprintf("catalog/products/%d/images", productID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list images for product %d: %w", productID, err)
	}
	imgs := make([]ProductImage, 0, len(raw))
	for _, r := range raw {
		var img ProductImage
		if err := json.Unmarshal(r, &img); err != nil {
			return nil, fmt.Errorf("unmarshal product image: %w", err)
		}
		imgs = append(imgs, img)
	}
	return imgs, nil
}

// CreateProductImage adds a URL-based image to a product via
// POST /v3/catalog/products/{id}/images with application/json body.
func (c *Client) CreateProductImage(ctx context.Context, productID int, payload ProductImageCreate) (*ProductImage, error) {
	path := fmt.Sprintf("catalog/products/%d/images", productID)
	respBody, err := c.Post(ctx, path, payload)
	if err != nil {
		return nil, fmt.Errorf("create image on product %d: %w", productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse image response: %w", err)
	}
	var img ProductImage
	if err := json.Unmarshal(resp.Data, &img); err != nil {
		return nil, fmt.Errorf("unmarshal created image: %w", err)
	}
	return &img, nil
}

// DeleteProductImage removes an image from a product.
func (c *Client) DeleteProductImage(ctx context.Context, productID, imageID int) error {
	path := fmt.Sprintf("catalog/products/%d/images/%d", productID, imageID)
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete image %d on product %d: %w", imageID, productID, err)
	}
	return nil
}
