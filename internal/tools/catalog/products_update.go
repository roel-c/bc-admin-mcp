package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

// UpdateParams holds parsed arguments for the unified product update tool.
type UpdateParams struct {
	// Targeting (mutually exclusive)
	CategoryID  int
	ProductIDs  []int
	SKU         string
	ProductName string
	Limit       int
	Sort        string
	SortDir     string

	// Fields to update (nil = not specified by user)
	Name        *string
	Type        *string
	SKUField    *string
	Description *string

	Weight *float64
	Width  *float64
	Height *float64
	Depth  *float64

	Price       *float64
	CostPrice   *float64
	RetailPrice *float64
	SalePrice   *float64
	MapPrice    *float64
	TaxClassID  *int

	Categories []int
	BrandID    *int

	InventoryTracking     *string
	InventoryLevel        *int
	InventoryWarningLevel *int

	IsVisible        *bool
	IsFeatured       *bool
	SortOrder        *int
	Condition        *string
	IsConditionShown *bool

	PageTitle       *string
	MetaDescription *string
	SearchKeywords  *string
	CustomURL       *string

	Availability            *string
	AvailabilityDescription *string
	IsPreorderOnly          *bool
	PreorderMessage         *string
	PreorderReleaseDate     *string

	IsFreeShipping         *bool
	FixedCostShippingPrice *float64

	UPC              *string
	GTIN             *string
	MPN              *string
	BinPickingNumber *string

	Warranty             *string
	OrderQuantityMinimum *int
	OrderQuantityMaximum *int

	GiftWrappingOptionsType *string
	GiftWrappingOptionsList []int
	RelatedProducts         []int

	OpenGraphType           *string
	OpenGraphTitle          *string
	OpenGraphDescription    *string
	OpenGraphUseMetaDesc    *bool
	OpenGraphUseProductName *bool
	OpenGraphUseImage       *bool

	LayoutFile *string

	// MSF additive side-effect: after the catalog update succeeds for all
	// targeted products, additively assign each one to these channels.
	// Existing assignments are preserved.
	ChannelIDs []int

	Confirmed bool
}

func (p *UpdateParams) hasFields() bool {
	return p.Name != nil || p.Type != nil || p.SKUField != nil || p.Description != nil ||
		p.Weight != nil || p.Width != nil || p.Height != nil || p.Depth != nil ||
		p.Price != nil || p.CostPrice != nil || p.RetailPrice != nil ||
		p.SalePrice != nil || p.MapPrice != nil || p.TaxClassID != nil ||
		p.Categories != nil || p.BrandID != nil ||
		p.InventoryTracking != nil || p.InventoryLevel != nil || p.InventoryWarningLevel != nil ||
		p.IsVisible != nil || p.IsFeatured != nil || p.SortOrder != nil ||
		p.Condition != nil || p.IsConditionShown != nil ||
		p.PageTitle != nil || p.MetaDescription != nil || p.SearchKeywords != nil || p.CustomURL != nil ||
		p.Availability != nil || p.AvailabilityDescription != nil ||
		p.IsPreorderOnly != nil || p.PreorderMessage != nil || p.PreorderReleaseDate != nil ||
		p.IsFreeShipping != nil || p.FixedCostShippingPrice != nil ||
		p.UPC != nil || p.GTIN != nil || p.MPN != nil || p.BinPickingNumber != nil ||
		p.Warranty != nil || p.OrderQuantityMinimum != nil || p.OrderQuantityMaximum != nil ||
		p.GiftWrappingOptionsType != nil || p.GiftWrappingOptionsList != nil ||
		p.RelatedProducts != nil ||
		p.OpenGraphType != nil || p.OpenGraphTitle != nil || p.OpenGraphDescription != nil ||
		p.OpenGraphUseMetaDesc != nil || p.OpenGraphUseProductName != nil || p.OpenGraphUseImage != nil ||
		p.LayoutFile != nil
}

func parseUpdateParams(args map[string]any) (*UpdateParams, error) {
	p := &UpdateParams{}

	// --- Targeting ---
	modes := 0
	if v, ok := args["category_id"]; ok {
		f, fOk := v.(float64)
		if !fOk || f <= 0 {
			return nil, fmt.Errorf("category_id must be a positive number")
		}
		p.CategoryID = int(f)
		modes++
	}
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
		return nil, fmt.Errorf("provide one of: category_id, product_ids, sku, or product_name")
	}
	if modes > 1 {
		return nil, fmt.Errorf("use only one of: category_id, product_ids, sku, or product_name")
	}

	if v, ok := args["limit"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("limit must be a number")
		}
		if f > 0 {
			p.Limit = int(f)
		}
	}
	if v, ok := args["sort"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("sort must be a string")
		}
		p.Sort = s
	}
	if v, ok := args["sort_direction"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("sort_direction must be a string")
		}
		p.SortDir = s
	}

	// --- Updatable fields ---
	// All extractors below reject wrong types (rather than silently dropping
	// them), so an LLM that passes e.g. price="24.99" gets a structured error
	// instead of a silently no-op update.
	e := fieldExtractor{args: args}

	e.String("name", &p.Name)
	e.String("type", &p.Type)
	e.String("sku_field", &p.SKUField)
	e.String("description", &p.Description)

	e.Float("weight", &p.Weight)
	e.Float("width", &p.Width)
	e.Float("height", &p.Height)
	e.Float("depth", &p.Depth)

	e.Float("price", &p.Price)
	e.Float("cost_price", &p.CostPrice)
	e.Float("retail_price", &p.RetailPrice)
	e.Float("sale_price", &p.SalePrice)
	e.Float("map_price", &p.MapPrice)
	e.Int("tax_class_id", &p.TaxClassID)

	if v, ok := args["categories"]; ok {
		ids, err := parseFloat64SliceToNonNegativeInts(v, "categories")
		if err != nil {
			return nil, err
		}
		p.Categories = ids
	}
	e.Int("brand_id", &p.BrandID)

	e.String("inventory_tracking", &p.InventoryTracking)
	e.Int("inventory_level", &p.InventoryLevel)
	e.Int("inventory_warning_level", &p.InventoryWarningLevel)

	e.Bool("is_visible", &p.IsVisible)
	e.Bool("is_featured", &p.IsFeatured)
	e.Int("sort_order", &p.SortOrder)
	e.String("condition", &p.Condition)
	e.Bool("is_condition_shown", &p.IsConditionShown)

	e.String("page_title", &p.PageTitle)
	e.String("meta_description", &p.MetaDescription)
	e.String("search_keywords", &p.SearchKeywords)
	e.String("custom_url", &p.CustomURL)

	e.String("availability", &p.Availability)
	e.String("availability_description", &p.AvailabilityDescription)
	e.Bool("is_preorder_only", &p.IsPreorderOnly)
	e.String("preorder_message", &p.PreorderMessage)
	e.String("preorder_release_date", &p.PreorderReleaseDate)

	e.Bool("is_free_shipping", &p.IsFreeShipping)
	e.Float("fixed_cost_shipping_price", &p.FixedCostShippingPrice)

	e.String("upc", &p.UPC)
	e.String("gtin", &p.GTIN)
	e.String("mpn", &p.MPN)
	e.String("bin_picking_number", &p.BinPickingNumber)

	e.String("warranty", &p.Warranty)
	e.Int("order_quantity_minimum", &p.OrderQuantityMinimum)
	e.Int("order_quantity_maximum", &p.OrderQuantityMaximum)

	e.String("gift_wrapping_options_type", &p.GiftWrappingOptionsType)
	if v, ok := args["gift_wrapping_options_list"]; ok {
		ids, err := parseFloat64SliceToNonNegativeInts(v, "gift_wrapping_options_list")
		if err != nil {
			return nil, err
		}
		p.GiftWrappingOptionsList = ids
	}
	if v, ok := args["related_products"]; ok {
		ids, err := parseFloat64SliceToNonNegativeInts(v, "related_products")
		if err != nil {
			return nil, err
		}
		p.RelatedProducts = ids
	}

	e.String("open_graph_type", &p.OpenGraphType)
	e.String("open_graph_title", &p.OpenGraphTitle)
	e.String("open_graph_description", &p.OpenGraphDescription)
	e.Bool("open_graph_use_meta_description", &p.OpenGraphUseMetaDesc)
	e.Bool("open_graph_use_product_name", &p.OpenGraphUseProductName)
	e.Bool("open_graph_use_image", &p.OpenGraphUseImage)

	e.String("layout_file", &p.LayoutFile)

	if e.err != nil {
		return nil, e.err
	}

	if v, ok := args["channel_ids"]; ok && v != nil {
		ids, err := parseFloat64SliceToPositiveInts(v, "channel_ids")
		if err != nil {
			return nil, err
		}
		if len(ids) > maxChannelAssignListChannels {
			return nil, fmt.Errorf("channel_ids: maximum %d channels per call", maxChannelAssignListChannels)
		}
		p.ChannelIDs = ids
	}

	if !p.hasFields() && len(p.ChannelIDs) == 0 {
		return nil, fmt.Errorf("provide at least one field to update or a channel_ids list to assign")
	}

	p.Confirmed = middleware.IsConfirmedFromArgs(args)
	return p, nil
}

// cacheKey returns the session-cache key used to bind a preview to its
// matching confirm call. The key combines a stable fingerprint of the
// targeting fields plus the field map being applied. Two consequences:
//
//  1. A confirm call with the SAME targeting+fields as a preview reuses the
//     cached snapshot (no redundant BC fetch).
//  2. A confirm call with DIFFERENT targeting+fields than its preview misses
//     the cache and falls back to a fresh fetch — applying the current
//     call's fields to products resolved from the current call's targeting.
//     This eliminates the failure mode where a cached snapshot from preview
//     A is updated using preview B's params.
func (p *UpdateParams) cacheKey() string {
	var b strings.Builder
	b.WriteString("c:")
	b.WriteString(strconv.Itoa(p.CategoryID))
	b.WriteString("|ids:")
	if len(p.ProductIDs) > 0 {
		sorted := append([]int(nil), p.ProductIDs...)
		sort.Ints(sorted)
		for _, id := range sorted {
			b.WriteString(strconv.Itoa(id))
			b.WriteByte(',')
		}
	}
	b.WriteString("|sku:")
	b.WriteString(p.SKU)
	b.WriteString("|name:")
	b.WriteString(p.ProductName)
	b.WriteString("|limit:")
	b.WriteString(strconv.Itoa(p.Limit))
	b.WriteString("|sort:")
	b.WriteString(p.Sort)
	b.WriteString("|dir:")
	b.WriteString(p.SortDir)
	b.WriteString("|fields:")
	for _, f := range listChangedFields(p) {
		b.WriteString(f)
		b.WriteByte(',')
	}
	b.WriteString("|channels:")
	if len(p.ChannelIDs) > 0 {
		sorted := append([]int(nil), p.ChannelIDs...)
		sort.Ints(sorted)
		for _, id := range sorted {
			b.WriteString(strconv.Itoa(id))
			b.WriteByte(',')
		}
	}

	sum := sha256.Sum256([]byte(b.String()))
	return cacheKeyProductUpdate + ":" + hex.EncodeToString(sum[:8])
}

// buildProductUpdate converts parsed params into a ProductUpdate for a given product ID.
func buildProductUpdate(id int, p *UpdateParams) bigcommerce.ProductUpdate {
	u := bigcommerce.ProductUpdate{ID: id}

	u.Name = p.Name
	u.Type = p.Type
	u.SKU = p.SKUField
	u.Description = p.Description

	u.Weight = p.Weight
	u.Width = p.Width
	u.Height = p.Height
	u.Depth = p.Depth

	u.Price = p.Price
	u.CostPrice = p.CostPrice
	u.RetailPrice = p.RetailPrice
	u.SalePrice = p.SalePrice
	u.MapPrice = p.MapPrice
	u.TaxClassID = p.TaxClassID

	u.Categories = p.Categories
	u.BrandID = p.BrandID

	u.InventoryTracking = p.InventoryTracking
	u.InventoryLevel = p.InventoryLevel
	u.InventoryWarningLevel = p.InventoryWarningLevel

	u.IsVisible = p.IsVisible
	u.IsFeatured = p.IsFeatured
	u.SortOrder = p.SortOrder
	u.Condition = p.Condition
	u.IsConditionShown = p.IsConditionShown

	u.PageTitle = p.PageTitle
	u.MetaDescription = p.MetaDescription
	u.SearchKeywords = p.SearchKeywords
	if p.CustomURL != nil {
		u.CustomURL = &bigcommerce.CustomURL{URL: *p.CustomURL, IsCustomized: true}
	}

	u.Availability = p.Availability
	u.AvailabilityDescription = p.AvailabilityDescription
	u.IsPreorderOnly = p.IsPreorderOnly
	u.PreorderMessage = p.PreorderMessage
	u.PreorderReleaseDate = p.PreorderReleaseDate

	u.IsFreeShipping = p.IsFreeShipping
	u.FixedCostShippingPrice = p.FixedCostShippingPrice

	u.UPC = p.UPC
	u.GTIN = p.GTIN
	u.MPN = p.MPN
	u.BinPickingNumber = p.BinPickingNumber

	u.Warranty = p.Warranty
	u.OrderQuantityMinimum = p.OrderQuantityMinimum
	u.OrderQuantityMaximum = p.OrderQuantityMaximum

	u.GiftWrappingOptionsType = p.GiftWrappingOptionsType
	u.GiftWrappingOptionsList = p.GiftWrappingOptionsList
	u.RelatedProducts = p.RelatedProducts

	u.OpenGraphType = p.OpenGraphType
	u.OpenGraphTitle = p.OpenGraphTitle
	u.OpenGraphDescription = p.OpenGraphDescription
	u.OpenGraphUseMetaDesc = p.OpenGraphUseMetaDesc
	u.OpenGraphUseProductName = p.OpenGraphUseProductName
	u.OpenGraphUseImage = p.OpenGraphUseImage

	u.LayoutFile = p.LayoutFile

	return u
}

func (p *Products) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := parseUpdateParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Confirmed {
		return p.executeUpdate(ctx, params)
	}
	return p.previewUpdate(ctx, params)
}

func (p *Products) previewUpdate(ctx context.Context, params *UpdateParams) (*mcp.CallToolResult, error) {
	products, err := p.fetchUpdateTargets(ctx, params)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(products) == 0 {
		return toolError("no products found for the given criteria"), nil
	}

	if len(params.ChannelIDs) > 0 {
		pairs := len(products) * len(params.ChannelIDs)
		if pairs > maxChannelAssignPairsPerCall {
			return toolError(
				"channel_ids × products would create %d (product, channel) pairs; "+
					"maximum %d per call. Split the update into smaller batches or call "+
					"catalog/products/channel_assignments/assign separately.",
				pairs, maxChannelAssignPairsPerCall,
			), nil
		}
	}

	sessionCache := p.cache.ForSession(sessionKeyDefault)
	sessionCache.Set(params.cacheKey(), products)

	sampleSize := min(5, len(products))
	samples := make([]map[string]any, sampleSize)
	for i := 0; i < sampleSize; i++ {
		prod := products[i]
		sample := map[string]any{"id": prod.ID, "name": prod.Name}
		addDiff(sample, "price", prod.Price, params.Price)
		addDiffStr(sample, "name", prod.Name, params.Name)
		addDiffStr(sample, "sku", prod.SKU, params.SKUField)
		addDiffStr(sample, "type", prod.Type, params.Type)
		addDiff(sample, "sale_price", prod.SalePrice, params.SalePrice)
		addDiff(sample, "cost_price", prod.CostPrice, params.CostPrice)
		addDiff(sample, "retail_price", prod.RetailPrice, params.RetailPrice)
		addDiff(sample, "map_price", prod.MapPrice, params.MapPrice)
		addDiffBool(sample, "is_visible", prod.IsVisible, params.IsVisible)
		addDiffBool(sample, "is_featured", prod.IsFeatured, params.IsFeatured)
		addDiffStr(sample, "page_title", prod.PageTitle, params.PageTitle)
		addDiffStr(sample, "meta_description", prod.MetaDescription, params.MetaDescription)
		addDiffStr(sample, "search_keywords", prod.SearchKeywords, params.SearchKeywords)
		addDiffStr(sample, "condition", prod.Condition, params.Condition)
		addDiffStr(sample, "availability", prod.Availability, params.Availability)
		addDiff(sample, "weight", prod.Weight, params.Weight)
		addDiffStr(sample, "inventory_tracking", prod.InventoryTracking, params.InventoryTracking)
		addDiffInt(sample, "inventory_level", prod.InventoryLevel, params.InventoryLevel)
		addDiffBool(sample, "is_free_shipping", prod.IsFreeShipping, params.IsFreeShipping)
		addDiffStr(sample, "upc", prod.UPC, params.UPC)
		addDiffStr(sample, "gtin", prod.GTIN, params.GTIN)
		addDiffStr(sample, "mpn", prod.MPN, params.MPN)
		if params.Categories != nil {
			sample["old_categories"] = prod.Categories
			sample["new_categories"] = params.Categories
		}
		samples[i] = sample
	}

	fields := listChangedFields(params)

	resp := map[string]any{
		"status":         "pending_confirmation",
		"total_products": len(products),
		"fields_updated": fields,
		"sample_changes": samples,
		"message": fmt.Sprintf(
			"%d product(s) will be updated (%d field(s)). Pass confirmed=true to execute.",
			len(products), len(fields),
		),
	}

	if len(params.ChannelIDs) > 0 {
		resp["channel_assignments_preview"] = map[string]any{
			"channel_ids":         params.ChannelIDs,
			"target_product_count": len(products),
			"total_pairs":          len(products) * len(params.ChannelIDs),
			"effect": "After successful catalog update of every targeted product, " +
				"PUT /v3/catalog/products/channel-assignments will additively assign each product to each listed channel " +
				"(existing assignments preserved). Skipped if any catalog update fails.",
		}
	}

	return toolJSON(resp)
}

func (p *Products) executeUpdate(ctx context.Context, params *UpdateParams) (*mcp.CallToolResult, error) {
	sessionCache := p.cache.ForSession(sessionKeyDefault)
	key := params.cacheKey()
	cached, ok := sessionCache.Get(key)
	var products []bigcommerce.Product
	if ok {
		products, _ = cached.([]bigcommerce.Product)
	}
	if len(products) == 0 {
		var err error
		products, err = p.fetchUpdateTargets(ctx, params)
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
	}
	if len(products) == 0 {
		return toolError("no products found for the given criteria"), nil
	}

	updates := make([]bigcommerce.ProductUpdate, len(products))
	for i, prod := range products {
		updates[i] = buildProductUpdate(prod.ID, params)
	}

	var batchResult *bigcommerce.BatchResult
	if params.hasFields() {
		var err error
		batchResult, err = p.bc.BatchUpdateProducts(ctx, updates)
		if err != nil {
			return toolError("batch update failed: %v", err), nil
		}
	} else {
		// channel_ids only — no catalog write needed; treat all targets as succeeded.
		batchResult = &bigcommerce.BatchResult{Succeeded: len(products)}
	}

	sessionCache.Delete(key)

	resp := map[string]any{
		"status":           "completed",
		"products_updated": batchResult.Succeeded,
		"products_failed":  batchResult.Failed,
	}
	if len(batchResult.Errors) > 0 {
		resp["errors"] = batchResult.Errors
	}

	if len(params.ChannelIDs) > 0 {
		if batchResult.Failed > 0 {
			resp["status"] = "partial_success"
			resp["channel_assignments"] = map[string]any{
				"status": "skipped",
				"reason": "one or more catalog updates failed; channel assignment not attempted to avoid mixed state",
			}
			return toolJSON(resp)
		}
		assignments := make([]bigcommerce.ProductChannelAssignment, 0, len(products)*len(params.ChannelIDs))
		for _, prod := range products {
			for _, cid := range params.ChannelIDs {
				assignments = append(assignments, bigcommerce.ProductChannelAssignment{
					ProductID: prod.ID,
					ChannelID: cid,
				})
			}
		}
		if err := p.bc.UpsertProductChannelAssignments(ctx, assignments); err != nil {
			resp["status"] = "partial_success"
			resp["channel_assignments"] = map[string]any{
				"status":      "failed",
				"channel_ids": params.ChannelIDs,
				"product_ids": collectProductIDs(products),
				"error":       err.Error(),
			}
			return toolJSON(resp)
		}
		resp["channel_assignments"] = map[string]any{
			"status":      "completed",
			"channel_ids": params.ChannelIDs,
			"pairs":       len(assignments),
		}
	}

	return toolJSON(resp)
}

func collectProductIDs(products []bigcommerce.Product) []int {
	out := make([]int, len(products))
	for i, prod := range products {
		out[i] = prod.ID
	}
	return out
}

func (p *Products) fetchUpdateTargets(ctx context.Context, params *UpdateParams) ([]bigcommerce.Product, error) {
	if params.CategoryID > 0 {
		opts := bigcommerce.ProductListOptions{
			Sort:      params.Sort,
			Direction: params.SortDir,
		}
		products, err := p.bc.ListProductsByCategory(ctx, params.CategoryID, opts)
		if err != nil {
			return nil, err
		}
		if params.Limit > 0 && len(products) > params.Limit {
			products = products[:params.Limit]
		}
		return products, nil
	}
	return FetchProductsForWrite(ctx, p.bc, params.ProductIDs, params.SKU, params.ProductName)
}

// ---------------------------------------------------------------------------
// Helper functions for argument extraction and diff generation
// ---------------------------------------------------------------------------

// fieldExtractor reads typed fields from an MCP args map and accumulates the
// first type-mismatch error so callers can validate many fields with a single
// trailing check. Wrong-typed inputs produce a structured error instead of
// being silently dropped — silent drops let an LLM believe a field was set
// when the resulting BC payload omitted it.
type fieldExtractor struct {
	args map[string]any
	err  error
}

func (e *fieldExtractor) String(key string, dst **string) {
	if e.err != nil {
		return
	}
	v, ok := e.args[key]
	if !ok {
		return
	}
	s, sOk := v.(string)
	if !sOk {
		e.err = fmt.Errorf("%s must be a string", key)
		return
	}
	*dst = &s
}

func (e *fieldExtractor) Float(key string, dst **float64) {
	if e.err != nil {
		return
	}
	v, ok := e.args[key]
	if !ok {
		return
	}
	f, fOk := v.(float64)
	if !fOk {
		e.err = fmt.Errorf("%s must be a number", key)
		return
	}
	*dst = &f
}

func (e *fieldExtractor) Int(key string, dst **int) {
	if e.err != nil {
		return
	}
	v, ok := e.args[key]
	if !ok {
		return
	}
	f, fOk := v.(float64)
	if !fOk {
		e.err = fmt.Errorf("%s must be a number", key)
		return
	}
	i := int(f)
	*dst = &i
}

func (e *fieldExtractor) Bool(key string, dst **bool) {
	if e.err != nil {
		return
	}
	v, ok := e.args[key]
	if !ok {
		return
	}
	b, bOk := v.(bool)
	if !bOk {
		e.err = fmt.Errorf("%s must be a boolean", key)
		return
	}
	*dst = &b
}

func addDiff(m map[string]any, field string, old float64, new *float64) {
	if new != nil {
		m["old_"+field] = old
		m["new_"+field] = *new
	}
}

func addDiffStr(m map[string]any, field string, old string, new *string) {
	if new != nil {
		m["old_"+field] = old
		m["new_"+field] = *new
	}
}

func addDiffBool(m map[string]any, field string, old bool, new *bool) {
	if new != nil {
		m["old_"+field] = old
		m["new_"+field] = *new
	}
}

func addDiffInt(m map[string]any, field string, old int, new *int) {
	if new != nil {
		m["old_"+field] = old
		m["new_"+field] = *new
	}
}

func listChangedFields(p *UpdateParams) []string {
	var out []string
	if p.Name != nil {
		out = append(out, "name")
	}
	if p.Type != nil {
		out = append(out, "type")
	}
	if p.SKUField != nil {
		out = append(out, "sku")
	}
	if p.Description != nil {
		out = append(out, "description")
	}
	if p.Weight != nil {
		out = append(out, "weight")
	}
	if p.Width != nil {
		out = append(out, "width")
	}
	if p.Height != nil {
		out = append(out, "height")
	}
	if p.Depth != nil {
		out = append(out, "depth")
	}
	if p.Price != nil {
		out = append(out, "price")
	}
	if p.CostPrice != nil {
		out = append(out, "cost_price")
	}
	if p.RetailPrice != nil {
		out = append(out, "retail_price")
	}
	if p.SalePrice != nil {
		out = append(out, "sale_price")
	}
	if p.MapPrice != nil {
		out = append(out, "map_price")
	}
	if p.TaxClassID != nil {
		out = append(out, "tax_class_id")
	}
	if p.Categories != nil {
		out = append(out, "categories")
	}
	if p.BrandID != nil {
		out = append(out, "brand_id")
	}
	if p.InventoryTracking != nil {
		out = append(out, "inventory_tracking")
	}
	if p.InventoryLevel != nil {
		out = append(out, "inventory_level")
	}
	if p.InventoryWarningLevel != nil {
		out = append(out, "inventory_warning_level")
	}
	if p.IsVisible != nil {
		out = append(out, "is_visible")
	}
	if p.IsFeatured != nil {
		out = append(out, "is_featured")
	}
	if p.SortOrder != nil {
		out = append(out, "sort_order")
	}
	if p.Condition != nil {
		out = append(out, "condition")
	}
	if p.IsConditionShown != nil {
		out = append(out, "is_condition_shown")
	}
	if p.PageTitle != nil {
		out = append(out, "page_title")
	}
	if p.MetaDescription != nil {
		out = append(out, "meta_description")
	}
	if p.SearchKeywords != nil {
		out = append(out, "search_keywords")
	}
	if p.CustomURL != nil {
		out = append(out, "custom_url")
	}
	if p.Availability != nil {
		out = append(out, "availability")
	}
	if p.AvailabilityDescription != nil {
		out = append(out, "availability_description")
	}
	if p.IsPreorderOnly != nil {
		out = append(out, "is_preorder_only")
	}
	if p.PreorderMessage != nil {
		out = append(out, "preorder_message")
	}
	if p.PreorderReleaseDate != nil {
		out = append(out, "preorder_release_date")
	}
	if p.IsFreeShipping != nil {
		out = append(out, "is_free_shipping")
	}
	if p.FixedCostShippingPrice != nil {
		out = append(out, "fixed_cost_shipping_price")
	}
	if p.UPC != nil {
		out = append(out, "upc")
	}
	if p.GTIN != nil {
		out = append(out, "gtin")
	}
	if p.MPN != nil {
		out = append(out, "mpn")
	}
	if p.BinPickingNumber != nil {
		out = append(out, "bin_picking_number")
	}
	if p.Warranty != nil {
		out = append(out, "warranty")
	}
	if p.OrderQuantityMinimum != nil {
		out = append(out, "order_quantity_minimum")
	}
	if p.OrderQuantityMaximum != nil {
		out = append(out, "order_quantity_maximum")
	}
	if p.GiftWrappingOptionsType != nil {
		out = append(out, "gift_wrapping_options_type")
	}
	if p.GiftWrappingOptionsList != nil {
		out = append(out, "gift_wrapping_options_list")
	}
	if p.RelatedProducts != nil {
		out = append(out, "related_products")
	}
	if p.OpenGraphType != nil {
		out = append(out, "open_graph_type")
	}
	if p.OpenGraphTitle != nil {
		out = append(out, "open_graph_title")
	}
	if p.OpenGraphDescription != nil {
		out = append(out, "open_graph_description")
	}
	if p.OpenGraphUseMetaDesc != nil {
		out = append(out, "open_graph_use_meta_description")
	}
	if p.OpenGraphUseProductName != nil {
		out = append(out, "open_graph_use_product_name")
	}
	if p.OpenGraphUseImage != nil {
		out = append(out, "open_graph_use_image")
	}
	if p.LayoutFile != nil {
		out = append(out, "layout_file")
	}
	return out
}
