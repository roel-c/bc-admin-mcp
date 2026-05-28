package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

const (
	cacheKeyBulkSKUUpdate = "bulk_sku_update"
	maxBulkSKUPairs       = 100
)

// bulkSKUEntry is a single product_id → new SKU mapping.
type bulkSKUEntry struct {
	ProductID int
	NewSKU    string
}

// parseBulkSKUParams validates and zips the parallel product_ids / skus arrays.
func parseBulkSKUParams(args map[string]any) ([]bulkSKUEntry, bool, error) {
	// --- product_ids ---
	rawIDs, ok := args["product_ids"]
	if !ok {
		return nil, false, fmt.Errorf("product_ids is required")
	}
	ids, err := parseFloat64SliceToPositiveInts(rawIDs, "product_ids")
	if err != nil {
		return nil, false, err
	}
	if len(ids) == 0 {
		return nil, false, fmt.Errorf("product_ids must not be empty")
	}

	// --- skus ---
	rawSKUs, ok := args["skus"]
	if !ok {
		return nil, false, fmt.Errorf("skus is required")
	}
	skus, err := parseStringSlice(rawSKUs, "skus")
	if err != nil {
		return nil, false, err
	}
	if len(skus) == 0 {
		return nil, false, fmt.Errorf("skus must not be empty")
	}

	// --- length match ---
	if len(ids) != len(skus) {
		return nil, false, fmt.Errorf(
			"product_ids (%d) and skus (%d) must have the same length",
			len(ids), len(skus),
		)
	}

	// --- max batch size ---
	if len(ids) > maxBulkSKUPairs {
		return nil, false, fmt.Errorf(
			"batch too large: %d pairs exceed the maximum of %d; split into smaller calls",
			len(ids), maxBulkSKUPairs,
		)
	}

	// --- validate individual SKUs ---
	for i, s := range skus {
		if strings.TrimSpace(s) == "" {
			return nil, false, fmt.Errorf("skus[%d] must not be empty or whitespace", i)
		}
	}

	// --- no duplicate product_ids ---
	seenIDs := make(map[int]bool, len(ids))
	for i, id := range ids {
		if seenIDs[id] {
			return nil, false, fmt.Errorf("product_ids contains duplicate value %d at index %d", id, i)
		}
		seenIDs[id] = true
	}

	// --- no duplicate SKUs ---
	seenSKUs := make(map[string]bool, len(skus))
	for i, s := range skus {
		if seenSKUs[s] {
			return nil, false, fmt.Errorf("skus contains duplicate value %q at index %d", s, i)
		}
		seenSKUs[s] = true
	}

	entries := make([]bulkSKUEntry, len(ids))
	for i := range ids {
		entries[i] = bulkSKUEntry{ProductID: ids[i], NewSKU: skus[i]}
	}

	confirmed := middleware.IsConfirmedFromArgs(args)
	return entries, confirmed, nil
}

// bulkSKUCacheKey produces a stable SHA256-derived key over the (product_id, sku) pairs.
func bulkSKUCacheKey(entries []bulkSKUEntry) string {
	// Sort by product ID so the key is order-independent.
	sorted := make([]bulkSKUEntry, len(entries))
	copy(sorted, entries)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ProductID < sorted[j].ProductID })

	var b strings.Builder
	for _, e := range sorted {
		b.WriteString(strconv.Itoa(e.ProductID))
		b.WriteByte('=')
		b.WriteString(e.NewSKU)
		b.WriteByte(',')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return cacheKeyBulkSKUUpdate + ":" + hex.EncodeToString(sum[:8])
}

// handleBulkSKUUpdate is the MCP handler for catalog/products/bulk_sku_update.
func (p *Products) handleBulkSKUUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	entries, confirmed, err := parseBulkSKUParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if confirmed {
		return p.executeBulkSKUUpdate(ctx, entries)
	}
	return p.previewBulkSKUUpdate(ctx, entries)
}

// previewBulkSKUUpdate fetches the current SKUs and returns a diff for user review.
func (p *Products) previewBulkSKUUpdate(ctx context.Context, entries []bulkSKUEntry) (*mcp.CallToolResult, error) {
	ids := make([]int, len(entries))
	for i, e := range entries {
		ids[i] = e.ProductID
	}

	products, err := fetchProductsByIDs(ctx, p.bc, ids)
	if err != nil {
		return toolError("failed to fetch products: %v", err), nil
	}

	// Build a lookup from product ID to current product.
	byID := make(map[int]bigcommerce.Product, len(products))
	for _, prod := range products {
		byID[prod.ID] = prod
	}

	// Build the preview diff list and the data we'll cache for confirm.
	type cachedEntry struct {
		ProductID int
		OldSKU    string
		NewSKU    string
	}
	cached := make([]cachedEntry, 0, len(entries))
	diffs := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		prod, ok := byID[e.ProductID]
		if !ok {
			// fetchProductsByIDs already errors if any ID is missing, but guard anyway.
			return toolError("product %d not found", e.ProductID), nil
		}
		row := map[string]any{
			"product_id":   prod.ID,
			"product_name": prod.Name,
			"old_sku":      prod.SKU,
			"new_sku":      e.NewSKU,
		}
		if prod.SKU == e.NewSKU {
			row["note"] = "no change — old and new SKU are identical"
		}
		diffs = append(diffs, row)
		cached = append(cached, cachedEntry{ProductID: prod.ID, OldSKU: prod.SKU, NewSKU: e.NewSKU})
	}

	// Store in session cache so confirm can skip the re-fetch.
	sessionCache := p.cache.ForSession(cacheSessionID(ctx))
	sessionCache.Set(bulkSKUCacheKey(entries), cached)

	return toolJSON(map[string]any{
		"status":         "pending_confirmation",
		"total_products": len(entries),
		"changes":        diffs,
		"message": fmt.Sprintf(
			"%d product SKU(s) will be updated. Pass confirmed=true to execute.",
			len(entries),
		),
	})
}

// executeBulkSKUUpdate applies the SKU changes, using the session cache when available.
func (p *Products) executeBulkSKUUpdate(ctx context.Context, entries []bulkSKUEntry) (*mcp.CallToolResult, error) {
	sessionCache := p.cache.ForSession(cacheSessionID(ctx))
	key := bulkSKUCacheKey(entries)

	// Build the update slice — either from cache (fast path) or a fresh fetch.
	type skuPair struct {
		ProductID int
		NewSKU    string
	}

	var updates []bigcommerce.ProductUpdate

	type cachedEntry struct {
		ProductID int
		OldSKU    string
		NewSKU    string
	}
	if cached, ok := sessionCache.Get(key); ok {
		if rows, valid := cached.([]cachedEntry); valid && len(rows) == len(entries) {
			updates = make([]bigcommerce.ProductUpdate, len(rows))
			for i, r := range rows {
				sku := r.NewSKU
				updates[i] = bigcommerce.ProductUpdate{ID: r.ProductID, SKU: &sku}
			}
		}
	}

	if len(updates) == 0 {
		// Cache miss — re-resolve product IDs and build updates directly.
		ids := make([]int, len(entries))
		for i, e := range entries {
			ids[i] = e.ProductID
		}
		if _, err := fetchProductsByIDs(ctx, p.bc, ids); err != nil {
			return toolError("failed to validate product IDs: %v", err), nil
		}
		updates = make([]bigcommerce.ProductUpdate, len(entries))
		for i, e := range entries {
			sku := e.NewSKU
			updates[i] = bigcommerce.ProductUpdate{ID: e.ProductID, SKU: &sku}
		}
	}

	result, err := p.bc.BatchUpdateProducts(ctx, updates)
	if err != nil {
		return toolError("batch SKU update failed: %v", err), nil
	}
	sessionCache.Delete(key)

	resp := map[string]any{
		"status":           "completed",
		"products_updated": result.Succeeded,
		"products_failed":  result.Failed,
	}
	if len(result.Errors) > 0 {
		resp["errors"] = result.Errors
	}
	return toolJSON(resp)
}
