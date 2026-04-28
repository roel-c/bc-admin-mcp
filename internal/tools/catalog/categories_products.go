package catalog

import (
	"context"
	"fmt"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/mark3labs/mcp-go/mcp"
)

// ParseCategoryProductsParams validates arguments for the category products listing tool.
func ParseCategoryProductsParams(args map[string]any) (*CategoryProductsParams, error) {
	p := &CategoryProductsParams{}

	_, hasID := args["category_id"]
	_, hasName := args["category_name"]
	if hasID && hasName {
		return nil, fmt.Errorf("category_id and category_name are mutually exclusive")
	}
	if !hasID && !hasName {
		return nil, fmt.Errorf("provide either category_id or category_name")
	}

	if v, ok := args["category_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("category_id must be a number")
		}
		id := int(f)
		if id <= 0 {
			return nil, fmt.Errorf("category_id must be positive")
		}
		p.CategoryID = id
	}

	if v, ok := args["category_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("category_name must be a non-empty string")
		}
		p.CategoryName = s
	}

	if v, ok := args["limit"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("limit must be a number")
		}
		p.Limit = int(f)
		if p.Limit < 1 {
			return nil, fmt.Errorf("limit must be at least 1")
		}
	}

	if v, ok := args["sort"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("sort must be a string")
		}
		if !validSortFields[s] {
			return nil, fmt.Errorf("sort must be one of: id, name, sku, price, date_modified, date_last_imported, inventory_level, is_visible, total_sold")
		}
		p.Sort = s
	}

	if v, ok := args["sort_direction"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("sort_direction must be a string")
		}
		if s != "asc" && s != "desc" {
			return nil, fmt.Errorf("sort_direction must be 'asc' or 'desc'")
		}
		p.SortDirection = s
	}

	return p, nil
}

// CategoryProductsParams holds parsed arguments for the category products listing tool.
type CategoryProductsParams struct {
	CategoryID    int
	CategoryName  string
	Limit         int
	Sort          string
	SortDirection string
}

func (c *Categories) handleCategoryProducts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseCategoryProductsParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	categoryID := params.CategoryID
	categoryName := params.CategoryName

	if params.CategoryName != "" {
		cid, resolveErr := resolveCategoryByExactName(ctx, c.bc, params.CategoryName)
		if resolveErr != nil {
			return toolError("%s", resolveErr.Error()), nil
		}
		categoryID = cid
	}

	if categoryName == "" {
		cat, catErr := c.bc.GetCategory(ctx, categoryID)
		if catErr != nil {
			return toolError("failed to get category %d: %v", categoryID, catErr), nil
		}
		categoryName = cat.Name
	}

	opts := bigcommerce.ProductListOptions{
		IncludeFields: []string{"id", "name", "sku", "price", "calculated_price", "sale_price", "categories"},
		Sort:          params.Sort,
		Direction:     params.SortDirection,
	}
	products, err := c.bc.ListProductsByCategory(ctx, categoryID, opts)
	if err != nil {
		return toolError("failed to list products: %v", err), nil
	}

	if params.Limit > 0 && len(products) > params.Limit {
		products = products[:params.Limit]
	}

	type productSummary struct {
		ID              int     `json:"id"`
		Name            string  `json:"name"`
		SKU             string  `json:"sku,omitempty"`
		Price           float64 `json:"price"`
		CalculatedPrice float64 `json:"calculated_price,omitempty"`
		SalePrice       float64 `json:"sale_price,omitempty"`
		Categories      []int   `json:"categories,omitempty"`
	}

	summaries := make([]productSummary, len(products))
	for i, p := range products {
		summaries[i] = productSummary{
			ID:              p.ID,
			Name:            p.Name,
			SKU:             p.SKU,
			Price:           p.Price,
			CalculatedPrice: p.CalculatedPrice,
			SalePrice:       p.SalePrice,
			Categories:      p.Categories,
		}
	}

	result := map[string]any{
		"category_id":    categoryID,
		"category_name":  categoryName,
		"total_products": len(products),
		"products":       summaries,
	}

	return toolJSON(result)
}
