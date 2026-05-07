package catalog

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

type deleteParams struct {
	ProductIDs  []int
	SKU         string
	ProductName string
	Confirmed   bool
}

func parseDeleteParams(args map[string]any) (*deleteParams, error) {
	p := &deleteParams{}

	modes := 0
	if v, ok := args["product_ids"]; ok {
		ids, err := parseFloat64SliceToPositiveInts(v, "product_ids")
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			p.ProductIDs = ids
			modes++
		}
	}
	if v, ok := args["sku"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("sku must be a non-empty string")
		}
		p.SKU = s
		modes++
	}
	if v, ok := args["product_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("product_name must be a non-empty string")
		}
		p.ProductName = s
		modes++
	}
	if modes == 0 {
		return nil, fmt.Errorf("provide one of: product_ids, sku, or product_name")
	}
	if modes > 1 {
		return nil, fmt.Errorf("use only one of: product_ids, sku, or product_name")
	}

	p.Confirmed = middleware.IsConfirmedFromArgs(args)
	return p, nil
}

func (p *Products) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := parseDeleteParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Confirmed {
		return p.executeDelete(ctx, params)
	}
	return p.previewDelete(ctx, params)
}

func (p *Products) previewDelete(ctx context.Context, params *deleteParams) (*mcp.CallToolResult, error) {
	products, err := FetchProductsForWrite(ctx, p.bc, params.ProductIDs, params.SKU, params.ProductName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(products) == 0 {
		return toolError("no products found for the given criteria"), nil
	}

	sessionCache := p.cache.ForSession(cacheSessionID(ctx))
	sessionCache.Set(cacheKeyProductDelete, products)

	type deleteSummary struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		SKU  string `json:"sku,omitempty"`
	}
	summaries := make([]deleteSummary, len(products))
	for i, prod := range products {
		summaries[i] = deleteSummary{ID: prod.ID, Name: prod.Name, SKU: prod.SKU}
	}

	return toolJSON(map[string]any{
		"status":         "pending_confirmation",
		"total_products": len(products),
		"products":       summaries,
		"message": fmt.Sprintf(
			"WARNING: %d product(s) will be PERMANENTLY DELETED. This cannot be undone. "+
				"Pass confirmed=true to execute.",
			len(products),
		),
	})
}

func (p *Products) executeDelete(ctx context.Context, params *deleteParams) (*mcp.CallToolResult, error) {
	sessionCache := p.cache.ForSession(cacheSessionID(ctx))
	cached, ok := sessionCache.Get(cacheKeyProductDelete)
	if !ok {
		return toolError("no preview found — call without confirmed=true first to generate a preview"), nil
	}
	products, ok := cached.([]bigcommerce.Product)
	if !ok || len(products) == 0 {
		return toolError("cached product data is invalid — re-run the preview"), nil
	}

	ids := make([]int, len(products))
	for i, prod := range products {
		ids[i] = prod.ID
	}

	deleted, errs := p.bc.DeleteProducts(ctx, ids)
	sessionCache.Delete(cacheKeyProductDelete)

	resp := map[string]any{
		"status":           "completed",
		"products_deleted": len(deleted),
		"deleted_ids":      deleted,
	}
	if len(errs) > 0 {
		errMsgs := make([]string, len(errs))
		for i, e := range errs {
			errMsgs[i] = e.Error()
		}
		resp["errors"] = errMsgs
	}
	return toolJSON(resp)
}
