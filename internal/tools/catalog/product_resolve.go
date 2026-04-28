package catalog

import (
	"context"
	"fmt"
	"strconv"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// FetchProductsForWrite loads products by explicit IDs, or resolves a single product by exact SKU or exact name.
// Exactly one mode must be used: non-empty ids, or non-empty sku, or non-empty productName.
func FetchProductsForWrite(
	ctx context.Context,
	bc BigCommerceAPI,
	ids []int,
	sku string,
	productName string,
) ([]bigcommerce.Product, error) {
	modes := 0
	if len(ids) > 0 {
		modes++
	}
	if sku != "" {
		modes++
	}
	if productName != "" {
		modes++
	}
	if modes == 0 {
		return nil, fmt.Errorf("provide product_ids (non-empty), sku, or product_name")
	}
	if modes > 1 {
		return nil, fmt.Errorf("use only one of: product_ids, sku, or product_name")
	}
	if len(ids) > 0 {
		return fetchProductsByIDs(ctx, bc, ids)
	}
	if sku != "" {
		p, err := resolveProductBySKU(ctx, bc, sku)
		if err != nil {
			return nil, err
		}
		return []bigcommerce.Product{*p}, nil
	}
	p, err := resolveProductByName(ctx, bc, productName)
	if err != nil {
		return nil, err
	}
	return []bigcommerce.Product{*p}, nil
}

func fetchProductsByIDs(ctx context.Context, bc BigCommerceAPI, ids []int) ([]bigcommerce.Product, error) {
	for _, pid := range ids {
		if pid <= 0 {
			return nil, fmt.Errorf("invalid product_id %d — must be positive", pid)
		}
	}
	products, err := bc.GetProductsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(products) != len(ids) {
		fetched := make(map[int]bool, len(products))
		for _, p := range products {
			fetched[p.ID] = true
		}
		for _, id := range ids {
			if !fetched[id] {
				return nil, fmt.Errorf("failed to get product %d: not found", id)
			}
		}
	}
	return products, nil
}

func resolveProductBySKU(ctx context.Context, bc BigCommerceAPI, sku string) (*bigcommerce.Product, error) {
	prods, err := bc.SearchProducts(ctx, map[string]string{"sku": sku})
	if err != nil {
		return nil, fmt.Errorf("search product by sku: %w", err)
	}
	if len(prods) == 0 {
		return nil, fmt.Errorf("no product found with sku %q", sku)
	}
	if len(prods) > 1 {
		ids := make([]string, 0, len(prods))
		for _, p := range prods {
			ids = append(ids, strconv.Itoa(p.ID))
		}
		return nil, fmt.Errorf("multiple products match sku %q (IDs: %v) — use product_ids", sku, ids)
	}
	return &prods[0], nil
}

func resolveProductByName(ctx context.Context, bc BigCommerceAPI, name string) (*bigcommerce.Product, error) {
	prods, err := bc.SearchProducts(ctx, map[string]string{"name": name})
	if err != nil {
		return nil, fmt.Errorf("search product by name: %w", err)
	}
	if len(prods) == 0 {
		return nil, fmt.Errorf("no product found with name %q", name)
	}
	if len(prods) > 1 {
		ids := make([]string, 0, len(prods))
		for _, p := range prods {
			ids = append(ids, strconv.Itoa(p.ID))
		}
		return nil, fmt.Errorf("multiple products match name %q (IDs: %v) — use product_ids", name, ids)
	}
	return &prods[0], nil
}

func resolveCategoryByExactName(ctx context.Context, bc BigCommerceAPI, name string) (int, error) {
	cats, err := bc.SearchCategories(ctx, map[string]string{"name": name})
	if err != nil {
		return 0, fmt.Errorf("search category %q: %w", name, err)
	}
	if len(cats) == 0 {
		return 0, fmt.Errorf("no category found with name %q — check spelling or use category_ids", name)
	}
	if len(cats) > 1 {
		ids := make([]int, len(cats))
		for i, c := range cats {
			ids[i] = c.ID
		}
		return 0, fmt.Errorf("multiple categories match name %q (IDs: %v) — use category_ids", name, ids)
	}
	return cats[0].ID, nil
}

