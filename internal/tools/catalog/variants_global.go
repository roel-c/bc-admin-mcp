package catalog

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
)

const (
	maxVariantGlobalProductIDs = 100
	maxVariantGlobalVariantIDs = 100
	maxVariantGlobalBulkRows   = 200
)

// VariantGlobalSearchFilters maps tool params to GET /v3/catalog/variants query keys.
var VariantGlobalSearchFilters = []SearchFilter{
	{ToolKey: "sku", BCKey: "sku", Kind: "string"},
	{ToolKey: "sku_like", BCKey: "sku:like", Kind: "string"},
	{ToolKey: "product_id", BCKey: "product_id", Kind: "number"},
	{ToolKey: "variant_id", BCKey: "id", Kind: "number"},
	{ToolKey: "sort", BCKey: "sort", Kind: "string"},
	{ToolKey: "sort_direction", BCKey: "direction", Kind: "string"},
}

var variantGlobalNonSortKeys = map[string]bool{
	"sort": true, "sort_direction": true,
}

var validVariantGlobalSortFields = map[string]bool{
	"id": true, "sku": true, "product_id": true, "price": true,
}

// GlobalVariants exposes MCP tools for GET/PUT /v3/catalog/variants (global catalog variants).
type GlobalVariants struct {
	bc    BigCommerceAPI
	cache *session.Store
}

func NewGlobalVariants(bc BigCommerceAPI, cache *session.Store) *GlobalVariants {
	return &GlobalVariants{bc: bc, cache: cache}
}

// RegisterTools registers catalog/variants/list and catalog/variants/bulk_update.
func (g *GlobalVariants) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/variants/list",
		Tier:    middleware.TierR0,
		Summary: "List or search variants via global catalog API (GET /v3/catalog/variants)",
		Description: "Uses the global variants endpoint for efficient multi-product reads (e.g. product_id:in, id:in) " +
			"or SKU filters. For single-product variant CRUD and option-aware creates, use catalog/products/variants/*.",
		Tool: mcp.NewTool("catalog_variants_list",
			mcp.WithDescription(
				"Search variants globally. Provide at least one filter (product_id, product_ids, variant_id, variant_ids, sku, sku_like) "+
					"or list_all=true. product_ids and variant_ids are capped at 100 IDs per request.",
			),
			mcp.WithBoolean("list_all",
				mcp.Description("Set to true to paginate through all variants (subject to server max total records cap)."),
			),
			mcp.WithNumber("product_id",
				mcp.Description("Single product ID. Mutually exclusive with product_ids."),
			),
			mcp.WithArray("product_ids",
				mcp.Description("Many product IDs; uses product_id:in on the global API. Mutually exclusive with product_id."),
				mcp.WithNumberItems(),
			),
			mcp.WithNumber("variant_id",
				mcp.Description("Single variant ID filter (maps to id=). Mutually exclusive with variant_ids."),
			),
			mcp.WithArray("variant_ids",
				mcp.Description("Many variant IDs; uses id:in. Mutually exclusive with variant_id."),
				mcp.WithNumberItems(),
			),
			mcp.WithString("sku", mcp.Description("Exact SKU match.")),
			mcp.WithString("sku_like", mcp.Description("Partial SKU match (sku:like).")),
			mcp.WithString("sort", mcp.Description("Sort field: id, sku, product_id, or price.")),
			mcp.WithString("sort_direction", mcp.Description("asc or desc.")),
		),
		Handler: g.handleGlobalList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/variants/bulk_update",
		Tier:    middleware.TierR2,
		Summary: "Batch update variants via PUT /v3/catalog/variants (IMS-style bulk)",
		Description: "Updates up to 200 variants per call; server chunks into batches of 10 per BigCommerce limits. " +
			"Each row must include variant_id and at least one writable field (price, inventory_level, SKU, etc.). " +
			"Preview first; pass confirmed=true. Prefer catalog/products/variants/update for single-product precision.",
		Tool: mcp.NewTool("catalog_variants_bulk_update",
			mcp.WithDescription(
				"Batch update many variants by ID. Pass updates as an array of objects with variant_id plus fields to change. "+
					"Maximum "+strconv.Itoa(maxVariantGlobalBulkRows)+" rows per call. Preview then confirmed=true.",
			),
			mcp.WithArray("updates",
				mcp.Description("Array of objects: { variant_id, sku?, price?, cost_price?, sale_price?, retail_price?, map_price?, weight?, width?, height?, depth?, inventory_level?, inventory_warning_level?, bin_picking_number?, upc?, gtin?, mpn?, image_url?, purchasing_disabled?, purchasing_disabled_message? }"),
			),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing the preview.")),
		),
		Handler: g.handleGlobalBulkUpdate,
	})
}

func (g *GlobalVariants) handleGlobalList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	listAll := ReadListAllBoolean(args, "list_all")

	_, hasPID := args["product_id"]
	_, hasPIDs := args["product_ids"]
	if hasPID && hasPIDs {
		return toolError("product_id and product_ids are mutually exclusive"), nil
	}
	_, hasVID := args["variant_id"]
	_, hasVIDs := args["variant_ids"]
	if hasVID && hasVIDs {
		return toolError("variant_id and variant_ids are mutually exclusive"), nil
	}

	params, err := ExtractFilters(args, VariantGlobalSearchFilters)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if v, ok := args["product_ids"]; ok {
		ids, joinErr := parseIntArrayParam(v, "product_ids", maxVariantGlobalProductIDs)
		if joinErr != nil {
			return toolError("%s", joinErr.Error()), nil
		}
		if len(ids) > 0 {
			params["product_id:in"] = joinInts(ids)
		}
	}

	if v, ok := args["variant_ids"]; ok {
		ids, joinErr := parseIntArrayParam(v, "variant_ids", maxVariantGlobalVariantIDs)
		if joinErr != nil {
			return toolError("%s", joinErr.Error()), nil
		}
		if len(ids) > 0 {
			params["id:in"] = joinInts(ids)
		}
	}

	hasData := HasDataFilterBCParams(params, VariantGlobalSearchFilters, variantGlobalNonSortKeys)

	if !hasData && !listAll {
		return toolError(
			"provide at least one filter (product_id, product_ids, variant_id, variant_ids, sku, sku_like) or list_all=true",
		), nil
	}

	if err := ErrInvalidBCSort(params, validVariantGlobalSortFields, "valid: id, sku, product_id, price"); err != nil {
		return toolError("%s", err.Error()), nil
	}

	variants, err := g.bc.SearchVariants(ctx, params)
	if err != nil {
		return toolError("failed to list variants: %v", err), nil
	}

	return toolJSON(map[string]any{
		"total":    len(variants),
		"variants": variants,
	})
}

func parseIntArrayParam(raw any, name string, maxLen int) ([]int, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of numbers", name)
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("%s must not be empty", name)
	}
	if len(arr) > maxLen {
		return nil, fmt.Errorf("%s must have at most %d entries", name, maxLen)
	}
	out := make([]int, 0, len(arr))
	for _, item := range arr {
		f, ok := item.(float64)
		if !ok {
			return nil, fmt.Errorf("each %s entry must be a number", name)
		}
		id := int(f)
		if id <= 0 {
			return nil, fmt.Errorf("each %s entry must be positive", name)
		}
		out = append(out, id)
	}
	return out, nil
}

func joinInts(ids []int) string {
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = strconv.Itoa(id)
	}
	return strings.Join(strs, ",")
}

func (g *GlobalVariants) handleGlobalBulkUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	raw, ok := args["updates"]
	if !ok {
		return toolError("updates is required (array of objects with variant_id and fields to change)"), nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return toolError("updates must be an array"), nil
	}
	if len(arr) == 0 {
		return toolError("updates must not be empty"), nil
	}
	if len(arr) > maxVariantGlobalBulkRows {
		return toolError("at most %d updates per call", maxVariantGlobalBulkRows), nil
	}

	updates := make([]bigcommerce.CatalogVariantUpdate, 0, len(arr))
	for i, item := range arr {
		row, ok := item.(map[string]any)
		if !ok {
			return toolError("updates[%d] must be an object", i), nil
		}
		u, err := parseCatalogVariantBulkRow(row, i)
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		if !HasProductVariantUpdateChanges(&u.ProductVariantUpdate) {
			return toolError("updates[%d] must include at least one field to change besides variant_id", i), nil
		}
		updates = append(updates, u)
	}

	if !middleware.IsConfirmedFromArgs(args) {
		previewRows := updates
		if len(previewRows) > 5 {
			previewRows = previewRows[:5]
		}
		return toolJSON(map[string]any{
			"status":          "pending_confirmation",
			"total_updates":   len(updates),
			"batch_size":      10,
			"estimated_calls": (len(updates) + 9) / 10,
			"sample":          previewRows,
			"message":         "Review sample (first 5 rows). Pass confirmed=true with the same updates array to execute.",
		})
	}

	result, err := g.bc.BatchUpdateVariants(ctx, updates)
	if err != nil {
		return toolError("batch update failed: %v", err), nil
	}

	status := "completed"
	if result.Failed > 0 {
		status = "partial_success"
	}
	return toolJSON(map[string]any{
		"status":    status,
		"succeeded": result.Succeeded,
		"failed":    result.Failed,
		"errors":    result.Errors,
	})
}

func parseCatalogVariantBulkRow(row map[string]any, index int) (bigcommerce.CatalogVariantUpdate, error) {
	vidRaw, ok := row["variant_id"]
	if !ok {
		return bigcommerce.CatalogVariantUpdate{}, fmt.Errorf("updates[%d].variant_id is required", index)
	}
	vf, ok := vidRaw.(float64)
	if !ok {
		return bigcommerce.CatalogVariantUpdate{}, fmt.Errorf("updates[%d].variant_id must be a number", index)
	}
	vid := int(vf)
	if vid <= 0 {
		return bigcommerce.CatalogVariantUpdate{}, fmt.Errorf("updates[%d].variant_id must be positive", index)
	}

	u := bigcommerce.CatalogVariantUpdate{ID: vid}
	ApplyProductVariantUpdateFromMap(row, &u.ProductVariantUpdate)

	return u, nil
}
