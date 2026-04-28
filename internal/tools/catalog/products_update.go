package catalog

import (
	"context"
	"fmt"

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
		if fOk && f > 0 {
			p.Limit = int(f)
		}
	}
	if v, ok := args["sort"]; ok {
		s, _ := v.(string)
		p.Sort = s
	}
	if v, ok := args["sort_direction"]; ok {
		s, _ := v.(string)
		p.SortDir = s
	}

	// --- Updatable fields ---
	extractString(args, "name", &p.Name)
	extractString(args, "type", &p.Type)
	extractString(args, "sku_field", &p.SKUField)
	extractString(args, "description", &p.Description)

	extractFloat(args, "weight", &p.Weight)
	extractFloat(args, "width", &p.Width)
	extractFloat(args, "height", &p.Height)
	extractFloat(args, "depth", &p.Depth)

	extractFloat(args, "price", &p.Price)
	extractFloat(args, "cost_price", &p.CostPrice)
	extractFloat(args, "retail_price", &p.RetailPrice)
	extractFloat(args, "sale_price", &p.SalePrice)
	extractFloat(args, "map_price", &p.MapPrice)
	extractInt(args, "tax_class_id", &p.TaxClassID)

	if v, ok := args["categories"]; ok {
		ids, err := parseFloat64SliceToNonNegativeInts(v, "categories")
		if err != nil {
			return nil, err
		}
		p.Categories = ids
	}
	extractInt(args, "brand_id", &p.BrandID)

	extractString(args, "inventory_tracking", &p.InventoryTracking)
	extractInt(args, "inventory_level", &p.InventoryLevel)
	extractInt(args, "inventory_warning_level", &p.InventoryWarningLevel)

	extractBool(args, "is_visible", &p.IsVisible)
	extractBool(args, "is_featured", &p.IsFeatured)
	extractInt(args, "sort_order", &p.SortOrder)
	extractString(args, "condition", &p.Condition)
	extractBool(args, "is_condition_shown", &p.IsConditionShown)

	extractString(args, "page_title", &p.PageTitle)
	extractString(args, "meta_description", &p.MetaDescription)
	extractString(args, "search_keywords", &p.SearchKeywords)
	extractString(args, "custom_url", &p.CustomURL)

	extractString(args, "availability", &p.Availability)
	extractString(args, "availability_description", &p.AvailabilityDescription)
	extractBool(args, "is_preorder_only", &p.IsPreorderOnly)
	extractString(args, "preorder_message", &p.PreorderMessage)
	extractString(args, "preorder_release_date", &p.PreorderReleaseDate)

	extractBool(args, "is_free_shipping", &p.IsFreeShipping)
	extractFloat(args, "fixed_cost_shipping_price", &p.FixedCostShippingPrice)

	extractString(args, "upc", &p.UPC)
	extractString(args, "gtin", &p.GTIN)
	extractString(args, "mpn", &p.MPN)
	extractString(args, "bin_picking_number", &p.BinPickingNumber)

	extractString(args, "warranty", &p.Warranty)
	extractInt(args, "order_quantity_minimum", &p.OrderQuantityMinimum)
	extractInt(args, "order_quantity_maximum", &p.OrderQuantityMaximum)

	extractString(args, "gift_wrapping_options_type", &p.GiftWrappingOptionsType)
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

	extractString(args, "open_graph_type", &p.OpenGraphType)
	extractString(args, "open_graph_title", &p.OpenGraphTitle)
	extractString(args, "open_graph_description", &p.OpenGraphDescription)
	extractBool(args, "open_graph_use_meta_description", &p.OpenGraphUseMetaDesc)
	extractBool(args, "open_graph_use_product_name", &p.OpenGraphUseProductName)
	extractBool(args, "open_graph_use_image", &p.OpenGraphUseImage)

	extractString(args, "layout_file", &p.LayoutFile)

	if !p.hasFields() {
		return nil, fmt.Errorf("provide at least one field to update")
	}

	p.Confirmed = middleware.IsConfirmedFromArgs(args)
	return p, nil
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

	sessionCache := p.cache.ForSession(sessionKeyDefault)
	sessionCache.Set(cacheKeyProductUpdate, products)

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

	return toolJSON(map[string]any{
		"status":         "pending_confirmation",
		"total_products": len(products),
		"fields_updated": fields,
		"sample_changes": samples,
		"message": fmt.Sprintf(
			"%d product(s) will be updated (%d field(s)). Pass confirmed=true to execute.",
			len(products), len(fields),
		),
	})
}

func (p *Products) executeUpdate(ctx context.Context, params *UpdateParams) (*mcp.CallToolResult, error) {
	sessionCache := p.cache.ForSession(sessionKeyDefault)
	cached, ok := sessionCache.Get(cacheKeyProductUpdate)
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

	result, err := p.bc.BatchUpdateProducts(ctx, updates)
	if err != nil {
		return toolError("batch update failed: %v", err), nil
	}

	sessionCache.Delete(cacheKeyProductUpdate)

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

func extractString(args map[string]any, key string, dst **string) {
	if v, ok := args[key]; ok {
		s, sOk := v.(string)
		if sOk {
			*dst = &s
		}
	}
}

func extractFloat(args map[string]any, key string, dst **float64) {
	if v, ok := args[key]; ok {
		f, fOk := v.(float64)
		if fOk {
			*dst = &f
		}
	}
}

func extractInt(args map[string]any, key string, dst **int) {
	if v, ok := args[key]; ok {
		f, fOk := v.(float64)
		if fOk {
			i := int(f)
			*dst = &i
		}
	}
}

func extractBool(args map[string]any, key string, dst **bool) {
	if v, ok := args[key]; ok {
		b, bOk := v.(bool)
		if bOk {
			*dst = &b
		}
	}
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
