package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ProductListOptions controls optional query parameters for product listing.
type ProductListOptions struct {
	IncludeFields []string
	Sort          string // e.g. "name", "price", "date_modified", "id"
	Direction     string // "asc" or "desc"
}

// ListProductsByCategory fetches all products in a given category, handling
// pagination server-side so the caller (and LLM) never needs to paginate.
func (c *Client) ListProductsByCategory(ctx context.Context, categoryID int, opts ProductListOptions) ([]Product, error) {
	path := fmt.Sprintf("catalog/products?categories:in=%d", categoryID)
	if len(opts.IncludeFields) > 0 {
		fields := ""
		for i, f := range opts.IncludeFields {
			if i > 0 {
				fields += ","
			}
			fields += f
		}
		path += "&include_fields=" + url.QueryEscape(fields)
	}
	if opts.Sort != "" {
		path += "&sort=" + url.QueryEscape(opts.Sort)
	}
	if opts.Direction != "" {
		path += "&direction=" + url.QueryEscape(opts.Direction)
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list products by category %d: %w", categoryID, err)
	}

	products := make([]Product, 0, len(raw))
	for _, r := range raw {
		var p Product
		if err := json.Unmarshal(r, &p); err != nil {
			return nil, fmt.Errorf("unmarshal product: %w", err)
		}
		products = append(products, p)
	}
	return products, nil
}

// GetProduct fetches a single product by ID.
func (c *Client) GetProduct(ctx context.Context, productID int) (*Product, error) {
	body, err := c.Get(ctx, fmt.Sprintf("catalog/products/%d", productID))
	if err != nil {
		return nil, fmt.Errorf("get product %d: %w", productID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse product %d: %w", productID, err)
	}
	var p Product
	if err := json.Unmarshal(resp.Data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal product %d: %w", productID, err)
	}
	return &p, nil
}

// SearchProducts searches products with arbitrary query parameters.
func (c *Client) SearchProducts(ctx context.Context, params map[string]string) ([]Product, error) {
	path := "catalog/products"
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	if encoded := vals.Encode(); encoded != "" {
		path += "?" + encoded
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("search products: %w", err)
	}

	products := make([]Product, 0, len(raw))
	for _, r := range raw {
		var p Product
		if err := json.Unmarshal(r, &p); err != nil {
			return nil, fmt.Errorf("unmarshal product: %w", err)
		}
		products = append(products, p)
	}
	return products, nil
}

// BatchUpdateProducts updates products in batches of the configured size.
func (c *Client) BatchUpdateProducts(ctx context.Context, updates []ProductUpdate) (*BatchResult, error) {
	items := make([]any, len(updates))
	for i, u := range updates {
		items[i] = u
	}
	return c.BatchPut(ctx, "catalog/products", items, c.cfg.ProductBatchSize)
}

// CreateProduct creates a new product via POST /v3/catalog/products.
func (c *Client) CreateProduct(ctx context.Context, payload ProductCreate) (*Product, error) {
	respBody, err := c.Post(ctx, "catalog/products", payload)
	if err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse create product response: %w", err)
	}
	var p Product
	if err := json.Unmarshal(resp.Data, &p); err != nil {
		return nil, fmt.Errorf("unmarshal created product: %w", err)
	}
	return &p, nil
}

// ListVariantsForProduct fetches all variants for a product.
func (c *Client) ListVariantsForProduct(ctx context.Context, productID int) ([]Variant, error) {
	path := fmt.Sprintf("catalog/products/%d/variants", productID)
	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list variants for product %d: %w", productID, err)
	}
	variants := make([]Variant, 0, len(raw))
	for _, r := range raw {
		var v Variant
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, fmt.Errorf("unmarshal variant: %w", err)
		}
		variants = append(variants, v)
	}
	return variants, nil
}

// DeleteProduct deletes a single product by ID via DELETE /v3/catalog/products/{id}.
func (c *Client) DeleteProduct(ctx context.Context, productID int) error {
	_, err := c.Delete(ctx, fmt.Sprintf("catalog/products/%d", productID))
	if err != nil {
		return fmt.Errorf("delete product %d: %w", productID, err)
	}
	return nil
}

// DeleteProducts deletes multiple products sequentially. BigCommerce does not
// offer a batch delete endpoint for products, so each is deleted individually.
func (c *Client) DeleteProducts(ctx context.Context, productIDs []int) (deleted []int, errors []error) {
	for _, id := range productIDs {
		if err := c.DeleteProduct(ctx, id); err != nil {
			errors = append(errors, err)
		} else {
			deleted = append(deleted, id)
		}
	}
	return deleted, errors
}

// SearchCategories fetches categories with arbitrary query parameters, using
// the same map[string]string pattern as SearchProducts.
func (c *Client) SearchCategories(ctx context.Context, params map[string]string) ([]Category, error) {
	path := "catalog/trees/categories"
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	if encoded := vals.Encode(); encoded != "" {
		path += "?" + encoded
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("search categories: %w", err)
	}
	cats := make([]Category, 0, len(raw))
	for _, r := range raw {
		var cat Category
		if err := json.Unmarshal(r, &cat); err != nil {
			return nil, fmt.Errorf("unmarshal category: %w", err)
		}
		cats = append(cats, cat)
	}
	return cats, nil
}

const categoryBatchSize = 50

// CreateCategory creates a new category via POST /v3/catalog/trees/categories.
// POST is for creates; PUT is exclusively for updates on this endpoint.
func (c *Client) CreateCategory(ctx context.Context, create CategoryCreate) ([]Category, error) {
	payload := []CategoryCreate{create}
	respBody, err := c.Post(ctx, "catalog/trees/categories", payload)
	if err != nil {
		return nil, fmt.Errorf("create category: %w", err)
	}
	var resp PaginatedResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse create response: %w", err)
	}
	cats := make([]Category, 0, len(resp.Data))
	for _, raw := range resp.Data {
		var cat Category
		if err := json.Unmarshal(raw, &cat); err != nil {
			return nil, fmt.Errorf("unmarshal created category: %w", err)
		}
		cats = append(cats, cat)
	}
	return cats, nil
}

// BatchUpdateCategories updates categories via PUT /v3/catalog/trees/categories.
// BigCommerce limits this endpoint to 50 categories per request.
func (c *Client) BatchUpdateCategories(ctx context.Context, updates []CategoryUpdate) (*BatchResult, error) {
	items := make([]any, len(updates))
	for i, u := range updates {
		items[i] = u
	}
	return c.BatchPut(ctx, "catalog/trees/categories", items, categoryBatchSize)
}

// DeleteCategories deletes categories via DELETE /v3/catalog/trees/categories.
// BigCommerce cascades the delete to all subcategories. Products remain in the
// store but lose the deleted category assignment.
func (c *Client) DeleteCategories(ctx context.Context, categoryIDs []int) error {
	if len(categoryIDs) == 0 {
		return fmt.Errorf("no category IDs provided")
	}
	strs := make([]string, len(categoryIDs))
	for i, id := range categoryIDs {
		strs[i] = strconv.Itoa(id)
	}
	path := "catalog/trees/categories?category_id:in=" + strings.Join(strs, ",")
	_, err := c.Delete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete categories: %w", err)
	}
	return nil
}

// GetDefaultTreeID fetches the first category tree's ID. For single-storefront
// stores there is typically one tree.
func (c *Client) GetDefaultTreeID(ctx context.Context) (int, error) {
	raw, err := c.GetAll(ctx, "catalog/trees")
	if err != nil {
		return 0, fmt.Errorf("list category trees: %w", err)
	}
	if len(raw) == 0 {
		return 0, fmt.Errorf("no category trees found")
	}
	var tree struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw[0], &tree); err != nil {
		return 0, fmt.Errorf("unmarshal tree: %w", err)
	}
	return tree.ID, nil
}

// GetCategory fetches a single category by ID.
func (c *Client) GetCategory(ctx context.Context, categoryID int) (*Category, error) {
	body, err := c.Get(ctx, "catalog/trees/categories?category_id:in="+strconv.Itoa(categoryID))
	if err != nil {
		return nil, fmt.Errorf("get category %d: %w", categoryID, err)
	}
	var resp PaginatedResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse category %d: %w", categoryID, err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("category %d not found", categoryID)
	}
	var cat Category
	if err := json.Unmarshal(resp.Data[0], &cat); err != nil {
		return nil, fmt.Errorf("unmarshal category %d: %w", categoryID, err)
	}
	return &cat, nil
}

// GetCategoriesByIDs fetches multiple categories in a single paginated request
// using GET /v3/catalog/trees/categories?category_id:in=1,2,3.
// IDs are chunked to keep URL lengths safe (~100 per request).
func (c *Client) GetCategoriesByIDs(ctx context.Context, categoryIDs []int) ([]Category, error) {
	if len(categoryIDs) == 0 {
		return nil, nil
	}

	const chunkSize = 100
	var allCats []Category

	for i := 0; i < len(categoryIDs); i += chunkSize {
		end := i + chunkSize
		if end > len(categoryIDs) {
			end = len(categoryIDs)
		}
		chunk := categoryIDs[i:end]

		strs := make([]string, len(chunk))
		for j, id := range chunk {
			strs[j] = strconv.Itoa(id)
		}
		path := "catalog/trees/categories?category_id:in=" + strings.Join(strs, ",")

		raw, err := c.GetAll(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("get categories (offset %d): %w", i, err)
		}
		for _, r := range raw {
			var cat Category
			if err := json.Unmarshal(r, &cat); err != nil {
				return nil, fmt.Errorf("unmarshal category: %w", err)
			}
			allCats = append(allCats, cat)
		}
	}

	return allCats, nil
}

// GetProductsByIDs fetches multiple products in a single paginated request
// using GET /v3/catalog/products?id:in=1,2,3.
// IDs are chunked to keep URL lengths safe (~100 per request).
func (c *Client) GetProductsByIDs(ctx context.Context, productIDs []int) ([]Product, error) {
	if len(productIDs) == 0 {
		return nil, nil
	}

	const chunkSize = 100
	var allProducts []Product

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
		path := "catalog/products?id:in=" + strings.Join(strs, ",")

		raw, err := c.GetAll(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("get products (offset %d): %w", i, err)
		}
		for _, r := range raw {
			var p Product
			if err := json.Unmarshal(r, &p); err != nil {
				return nil, fmt.Errorf("unmarshal product: %w", err)
			}
			allProducts = append(allProducts, p)
		}
	}

	return allProducts, nil
}
