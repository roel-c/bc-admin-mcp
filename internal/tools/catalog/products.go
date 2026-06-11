package catalog

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ProductSearchFilters is the declarative mapping table for the product search
// tool. Adding a new filter is a single line here plus one mcp.With* call in
// the tool schema. The same pattern can be reused for orders, customers, etc.
var ProductSearchFilters = []shared.SearchFilter{
	{ToolKey: "keyword", BCKey: "keyword", Kind: "string"},
	{ToolKey: "name", BCKey: "name", Kind: "string"},
	{ToolKey: "name_like", BCKey: "name:like", Kind: "string"},
	{ToolKey: "sku", BCKey: "sku", Kind: "string"},
	{ToolKey: "sku_like", BCKey: "sku:like", Kind: "string"},
	{ToolKey: "category_id", BCKey: "categories:in", Kind: "number"},
	{ToolKey: "brand_id", BCKey: "brand_id", Kind: "number"},
	{ToolKey: "price_min", BCKey: "price:min", Kind: "number"},
	{ToolKey: "price_max", BCKey: "price:max", Kind: "number"},
	{ToolKey: "is_visible", BCKey: "is_visible", Kind: "bool"},
	{ToolKey: "sort", BCKey: "sort", Kind: "string"},
	{ToolKey: "sort_direction", BCKey: "direction", Kind: "string"},
	{ToolKey: "include_fields", BCKey: "include_fields", Kind: "string"},
}

// nonFilterKeys are tool parameters that control sorting/output, not data
// filtering. They are excluded from the "at least one filter" check.
var nonFilterKeys = map[string]bool{
	"sort": true, "sort_direction": true, "include_fields": true,
}

var validSortFields = map[string]bool{
	"id": true, "name": true, "sku": true, "price": true,
	"date_modified": true, "date_last_imported": true,
	"inventory_level": true, "is_visible": true, "total_sold": true,
}

// Products provides MCP tool handlers for catalog product operations.
type Products struct {
	bc    BigCommerceAPI
	cache *session.Store
}

func NewProducts(bc BigCommerceAPI, cache *session.Store) *Products {
	return &Products{bc: bc, cache: cache}
}

// RegisterTools registers all product-related tools into the discovery registry.
func (p *Products) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/search",
		Tier:    middleware.TierR0,
		Summary: "Search products by category, name, SKU, price, or other filters",
		Description: "Fetches products matching the given criteria. At least one filter " +
			"parameter is required to prevent returning the entire catalog.",
		Tool: mcp.NewTool("catalog_products_search",
			mcp.WithDescription(
				"Search products with BigCommerce filters. At least one filter is required. "+
					"Returns lightweight summaries with name, price, calculated_price, and sale_price.",
			),
			mcp.WithString("name",
				mcp.Description("Exact product name match. Use when the user knows the full product name."),
			),
			mcp.WithString("name_like",
				mcp.Description("Partial name match (SQL LIKE). Use when the user gives a partial name or wants all products containing a word, e.g. 'Testing Product'."),
			),
			mcp.WithString("keyword",
				mcp.Description("Full-text search across name, SKU, and description. Use for broad or fuzzy searches when the user's intent is general."),
			),
			mcp.WithString("sku",
				mcp.Description("Exact SKU match. Use when the user provides a full SKU."),
			),
			mcp.WithString("sku_like",
				mcp.Description("Partial SKU match (SQL LIKE). Use when the user provides a partial SKU prefix or pattern."),
			),
			mcp.WithNumber("category_id",
				mcp.Description("Filter by category ID. Products belonging to this category."),
			),
			mcp.WithNumber("brand_id",
				mcp.Description("Filter by brand ID."),
			),
			mcp.WithNumber("price_min",
				mcp.Description("Minimum base price filter (greater than or equal to)."),
			),
			mcp.WithNumber("price_max",
				mcp.Description("Maximum base price filter (less than or equal to)."),
			),
			mcp.WithBoolean("is_visible",
				mcp.Description("Filter by storefront visibility. true = visible products only, false = hidden products only."),
			),
			mcp.WithString("sort",
				mcp.Description("Sort field: 'id', 'name', 'sku', 'price', 'date_modified', 'total_sold' (default: id)."),
			),
			mcp.WithString("sort_direction",
				mcp.Description("Sort direction: 'asc' or 'desc' (default: asc)."),
			),
			mcp.WithString("include_fields",
				mcp.Description("Comma-separated list of product fields to return. Reduces response size and token usage."),
			),
			mcp.WithArray("channel_ids",
				mcp.Description("MSF: restrict to products available on these BigCommerce channel IDs (max 20). Sent as channel_id:in. Use catalog/channels/list to discover channel IDs."),
				mcp.WithNumberItems(),
			),
		),
		Handler: p.handleSearch,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/get",
		Tier:        middleware.TierR0,
		Summary:     "Get product details by ID; pass include_variants=true for full variant list",
		Description: "Fetches a single product with pricing details. By default returns variant_count and has_variant_pricing without the full variant array to keep responses compact. Pass include_variants=true when you need variant IDs, SKUs, or pricing.",
		Tool: mcp.NewTool("catalog_products_get",
			mcp.WithDescription("Get a product by ID. Returns product details plus variant pricing summary. Pass include_variants=true for the full variant list."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithBoolean("include_variants", mcp.Description("Return the full variant array. Defaults to false — only variant_count and has_variant_pricing are returned.")),
		),
		Handler: p.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/assign_categories",
		Tier:    middleware.TierR1,
		Summary: "Additively assign products to categories (no removal, no fetch needed)",
		Description: "Uses the dedicated PUT /v3/catalog/products/category-assignments endpoint to add " +
			"product-to-category mappings. Purely additive — existing memberships are preserved. " +
			"Does not need to fetch current category lists first. " +
			"Caps: product_ids (max 100), category_ids (max 50), product×category pairs (max 500). " +
			"Split larger fan-outs across multiple calls.",
		Tool: mcp.NewTool("catalog_products_assign_categories",
			mcp.WithDescription(
				"Assign products to categories additively. Provide product_ids and category_ids arrays. "+
					"Each product gets assigned to each category. Existing memberships are not affected. "+
					"Preview first; pass confirmed=true to execute.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Product IDs to assign (max 100). product_ids × category_ids ≤ 500."),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithArray("category_ids",
				mcp.Description("Category IDs to assign the products to (max 50). product_ids × category_ids ≤ 500."),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: p.handleAssignCategories,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/unassign_categories",
		Tier:    middleware.TierR2,
		Summary: "Remove products from specific categories (filter-based DELETE; non-destructive to other categories)",
		Description: "Uses DELETE /v3/catalog/products/category-assignments with product_id:in and category_id:in. " +
			"Removes only the listed (product, category) memberships; other category links are preserved. " +
			"Prefer this over catalog/products/update with a categories array (which fully replaces categories). " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_products_unassign_categories",
			mcp.WithDescription(
				"Remove product↔category links. Provide product_ids and category_ids; preview then confirmed=true.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Product IDs (max 100)."),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithArray("category_ids",
				mcp.Description("Category IDs to remove from each product (max 50)."),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set true to execute after reviewing the preview."),
			),
		),
		Handler: p.handleUnassignCategories,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/create",
		Tier:    middleware.TierR1,
		Summary: "Create a new product with all available fields, optional images, and category assignments",
		Description: "Creates a single product via POST /v3/catalog/products. Supports all writable " +
			"fields including pricing, dimensions, inventory, SEO, Open Graph, shipping, and inline images.",
		Tool: mcp.NewTool("catalog_products_create",
			mcp.WithDescription(
				"Create a new product. Only name is required; type defaults to 'physical'. "+
					"Preview shows proposed details; pass confirmed=true to create.\n\n"+
					"FIELD GROUPS: Basic (name, type, sku, description, weight), "+
					"Pricing (price, cost_price, retail_price, sale_price, map_price), "+
					"Dimensions (width, height, depth), Inventory, Shipping, Identifiers, "+
					"Storefront, SEO, Open Graph, Categories, Images (inline URL-based).",
			),
			mcp.WithString("name", mcp.Description("Product name"), mcp.Required()),
			mcp.WithString("type", mcp.Description("Product type: physical or digital (default: physical)")),
			mcp.WithNumber("weight", mcp.Description("Product weight")),
			mcp.WithNumber("price", mcp.Description("Base price")),
			mcp.WithString("sku", mcp.Description("SKU identifier")),
			mcp.WithString("description", mcp.Description("Product description (HTML)")),
			mcp.WithNumber("width", mcp.Description("Width")),
			mcp.WithNumber("height", mcp.Description("Height")),
			mcp.WithNumber("depth", mcp.Description("Depth")),
			mcp.WithNumber("cost_price", mcp.Description("Cost price")),
			mcp.WithNumber("retail_price", mcp.Description("Retail / compare-at price")),
			mcp.WithNumber("sale_price", mcp.Description("Sale price (0 = no sale)")),
			mcp.WithNumber("map_price", mcp.Description("Minimum advertised price")),
			mcp.WithNumber("tax_class_id", mcp.Description("Tax class ID")),
			mcp.WithArray("category_ids", mcp.Description("Category IDs"), mcp.WithNumberItems()),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID")),
			mcp.WithString("inventory_tracking", mcp.Description("Tracking: none, product, or variant")),
			mcp.WithNumber("inventory_level", mcp.Description("Inventory count")),
			mcp.WithNumber("inventory_warning_level", mcp.Description("Low-stock threshold")),
			mcp.WithBoolean("is_visible", mcp.Description("Visible on storefront (default: true)")),
			mcp.WithBoolean("is_featured", mcp.Description("Featured product")),
			mcp.WithNumber("sort_order", mcp.Description("Sort order")),
			mcp.WithString("condition", mcp.Description("Condition: New, Used, or Refurbished")),
			mcp.WithBoolean("is_condition_shown", mcp.Description("Show condition")),
			mcp.WithString("page_title", mcp.Description("SEO page title")),
			mcp.WithString("meta_description", mcp.Description("SEO meta description")),
			mcp.WithString("search_keywords", mcp.Description("SEO search keywords")),
			mcp.WithString("availability", mcp.Description("Availability: available, disabled, preorder")),
			mcp.WithString("availability_description", mcp.Description("Custom availability text")),
			mcp.WithBoolean("is_preorder_only", mcp.Description("Preorder-only flag")),
			mcp.WithString("preorder_message", mcp.Description("Preorder message")),
			mcp.WithString("preorder_release_date", mcp.Description("Preorder release date (ISO 8601)")),
			mcp.WithBoolean("is_free_shipping", mcp.Description("Free shipping")),
			mcp.WithNumber("fixed_cost_shipping_price", mcp.Description("Fixed shipping cost")),
			mcp.WithString("upc", mcp.Description("UPC code")),
			mcp.WithString("gtin", mcp.Description("GTIN")),
			mcp.WithString("mpn", mcp.Description("MPN")),
			mcp.WithString("bin_picking_number", mcp.Description("Bin picking number")),
			mcp.WithString("warranty", mcp.Description("Warranty text")),
			mcp.WithNumber("order_quantity_minimum", mcp.Description("Min order quantity")),
			mcp.WithNumber("order_quantity_maximum", mcp.Description("Max order quantity")),
			mcp.WithString("gift_wrapping_options_type", mcp.Description("Gift wrapping type: any, none, list")),
			mcp.WithArray("gift_wrapping_options_list", mcp.Description("Gift wrapping option IDs"), mcp.WithNumberItems()),
			mcp.WithArray("related_products", mcp.Description("Related product IDs"), mcp.WithNumberItems()),
			mcp.WithString("open_graph_type", mcp.Description("OG type")),
			mcp.WithString("open_graph_title", mcp.Description("OG title")),
			mcp.WithString("open_graph_description", mcp.Description("OG description")),
			mcp.WithBoolean("open_graph_use_meta_description", mcp.Description("Use meta_description as OG description")),
			mcp.WithBoolean("open_graph_use_product_name", mcp.Description("Use product name as OG title")),
			mcp.WithBoolean("open_graph_use_image", mcp.Description("Use product image for OG")),
			mcp.WithString("layout_file", mcp.Description("Layout template file")),
			mcp.WithArray("images", mcp.Description("Inline images: [{image_url, is_thumbnail, description, sort_order}]")),
			mcp.WithArray("channel_ids",
				mcp.Description(
					"MSF (optional): after the product is created, PUT /v3/catalog/products/channel-assignments "+
						"will additively assign the new product to these channel IDs. Max 20. Existing assignments preserved. "+
						"Use catalog/channels/list to discover channel IDs.",
				),
				mcp.WithNumberItems(),
			),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true to create the product after reviewing preview")),
		),
		Handler: p.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path: "catalog/products/update",
		Tier: middleware.TierR1,
		Summary: "Update any writable field on one or more products (pricing, SEO, " +
			"visibility, inventory, shipping, dimensions, identifiers, Open Graph, and more)",
		Description: "Unified product update tool. Target products by product_ids, sku, " +
			"product_name, or category_id. Pass only the fields you want to change — " +
			"all field parameters are optional. Optional MSF `channel_ids` triggers an additive " +
			"PUT to /v3/catalog/products/channel-assignments after the catalog update succeeds. " +
			"Preview shows a diff; pass confirmed=true to apply.",
		Tool: mcp.NewTool("catalog_products_update",
			mcp.WithDescription(
				"Update any writable field on one or more products. "+
					"Target products by product_ids, sku, product_name, or category_id (mutually exclusive). "+
					"Pass only the fields you want to change. Preview shows diff; pass confirmed=true to apply.\n\n"+
					"FIELD GROUPS:\n"+
					"- Basic: name, type, sku_field, description\n"+
					"- Pricing: price, cost_price, retail_price, sale_price, map_price, tax_class_id\n"+
					"- Dimensions: weight, width, height, depth\n"+
					"- Inventory: inventory_tracking, inventory_level, inventory_warning_level\n"+
					"- Shipping: is_free_shipping, fixed_cost_shipping_price\n"+
					"- Identifiers: upc, gtin, mpn, bin_picking_number\n"+
					"- Storefront: is_visible, is_featured, sort_order, availability, condition\n"+
					"- SEO: page_title, meta_description, search_keywords, custom_url\n"+
					"- Open Graph: open_graph_type, open_graph_title, open_graph_description\n"+
					"- Categories: categories (full replacement), brand_id\n"+
					"- Purchasability: order_quantity_minimum, order_quantity_maximum\n"+
					"- Related: related_products, gift_wrapping_options_type, warranty, layout_file",
			),
			// --- Targeting ---
			mcp.WithNumber("category_id", mcp.Description("Target all products in this category")),
			mcp.WithNumber("limit", mcp.Description("Max products to update when using category_id")),
			mcp.WithString("sort", mcp.Description("Sort field for category listing: name, price, date_modified, id")),
			mcp.WithString("sort_direction", mcp.Description("Sort direction: asc or desc")),
			mcp.WithArray("product_ids", mcp.Description("Specific product IDs to update"), mcp.WithNumberItems()),
			mcp.WithString("sku", mcp.Description("Target single product by exact SKU")),
			mcp.WithString("product_name", mcp.Description("Target single product by exact name")),
			// --- Basic ---
			mcp.WithString("name", mcp.Description("Product name")),
			mcp.WithString("type", mcp.Description("Product type: physical or digital")),
			mcp.WithString("sku_field", mcp.Description("New SKU value to set on the product. Use this to UPDATE a product's SKU — named sku_field to avoid conflict with the sku targeting selector. Example: to change a product's SKU to \"ABC-123\", pass sku_field=\"ABC-123\".")),
			mcp.WithString("description", mcp.Description("Product description (HTML)")),
			// --- Pricing ---
			mcp.WithNumber("price", mcp.Description("Base catalog price")),
			mcp.WithNumber("cost_price", mcp.Description("Cost basis")),
			mcp.WithNumber("retail_price", mcp.Description("MSRP / compare-at price")),
			mcp.WithNumber("sale_price", mcp.Description("Sale price override (0 = no sale)")),
			mcp.WithNumber("map_price", mcp.Description("Minimum advertised price")),
			mcp.WithNumber("tax_class_id", mcp.Description("Tax class ID")),
			// --- Dimensions ---
			mcp.WithNumber("weight", mcp.Description("Product weight")),
			mcp.WithNumber("width", mcp.Description("Product width")),
			mcp.WithNumber("height", mcp.Description("Product height")),
			mcp.WithNumber("depth", mcp.Description("Product depth")),
			// --- Inventory ---
			mcp.WithString("inventory_tracking", mcp.Description("Tracking mode: none, product, or variant")),
			mcp.WithNumber("inventory_level", mcp.Description("Current inventory count")),
			mcp.WithNumber("inventory_warning_level", mcp.Description("Low-stock warning threshold")),
			// --- Shipping ---
			mcp.WithBoolean("is_free_shipping", mcp.Description("Enable free shipping")),
			mcp.WithNumber("fixed_cost_shipping_price", mcp.Description("Fixed shipping cost")),
			// --- Identifiers ---
			mcp.WithString("upc", mcp.Description("UPC code")),
			mcp.WithString("gtin", mcp.Description("Global Trade Item Number")),
			mcp.WithString("mpn", mcp.Description("Manufacturer Part Number")),
			mcp.WithString("bin_picking_number", mcp.Description("Bin picking number for warehouse")),
			// --- Storefront ---
			mcp.WithBoolean("is_visible", mcp.Description("Whether product is visible on storefront")),
			mcp.WithBoolean("is_featured", mcp.Description("Whether product is featured")),
			mcp.WithNumber("sort_order", mcp.Description("Sort order for display")),
			mcp.WithString("condition", mcp.Description("Product condition: New, Used, or Refurbished")),
			mcp.WithBoolean("is_condition_shown", mcp.Description("Show condition on storefront")),
			mcp.WithString("availability", mcp.Description("Availability: available, disabled, or preorder")),
			mcp.WithString("availability_description", mcp.Description("Custom availability text")),
			// --- Preorder ---
			mcp.WithBoolean("is_preorder_only", mcp.Description("Preorder-only flag")),
			mcp.WithString("preorder_message", mcp.Description("Preorder message text")),
			mcp.WithString("preorder_release_date", mcp.Description("Preorder release date (ISO 8601)")),
			// --- SEO ---
			mcp.WithString("page_title", mcp.Description("SEO page title")),
			mcp.WithString("meta_description", mcp.Description("SEO meta description")),
			mcp.WithString("search_keywords", mcp.Description("Comma-separated SEO search keywords")),
			mcp.WithString("custom_url", mcp.Description("URL slug (e.g. /my-product/)")),
			// --- Open Graph ---
			mcp.WithString("open_graph_type", mcp.Description("OG type: product, album, book, drink, food, game, movie, song, tv_show")),
			mcp.WithString("open_graph_title", mcp.Description("OG title")),
			mcp.WithString("open_graph_description", mcp.Description("OG description")),
			mcp.WithBoolean("open_graph_use_meta_description", mcp.Description("Use meta_description as OG description")),
			mcp.WithBoolean("open_graph_use_product_name", mcp.Description("Use product name as OG title")),
			mcp.WithBoolean("open_graph_use_image", mcp.Description("Use product image for OG")),
			// --- Categories ---
			mcp.WithArray("categories", mcp.Description("Full replacement of category IDs"), mcp.WithNumberItems()),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID")),
			// --- MSF channel assignments (additive side-effect) ---
			mcp.WithArray("channel_ids",
				mcp.Description(
					"MSF (optional, additive): after every targeted product is updated, PUT "+
						"/v3/catalog/products/channel-assignments to add each product to these channel IDs. "+
						"Existing assignments are preserved; use catalog/products/channel_assignments/remove to drop. "+
						"Max 20 channel IDs; total (products × channels) ≤ 500 per call.",
				),
				mcp.WithNumberItems(),
			),
			// --- Purchasability ---
			mcp.WithNumber("order_quantity_minimum", mcp.Description("Minimum order quantity")),
			mcp.WithNumber("order_quantity_maximum", mcp.Description("Maximum order quantity (0 = unlimited)")),
			// --- Gift Wrapping ---
			mcp.WithString("gift_wrapping_options_type", mcp.Description("Gift wrapping type: any, none, or list")),
			mcp.WithArray("gift_wrapping_options_list", mcp.Description("Gift wrapping option IDs"), mcp.WithNumberItems()),
			// --- Related ---
			mcp.WithArray("related_products", mcp.Description("Related product IDs"), mcp.WithNumberItems()),
			mcp.WithString("warranty", mcp.Description("Warranty text")),
			mcp.WithString("layout_file", mcp.Description("Layout template file name")),
			// --- Confirm ---
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview to execute the update")),
		),
		Handler: p.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path: "catalog/products/bulk_sku_update",
		Tier: middleware.TierR1,
		Summary: "Batch-update the SKU of multiple specific products in one call " +
			"(one product_id → one new SKU per entry, up to 100 pairs)",
		Description: "Updates the SKU field of up to 100 products in a single operation. " +
			"Pass parallel arrays: product_ids (the products to change) and skus (the new SKU for each, " +
			"positionally matched). Preview shows old → new SKU diff; pass confirmed=true to apply. " +
			"Use this instead of catalog/products/update when you need a different SKU per product — " +
			"the update tool applies the same sku_field value to ALL matched products.",
		Tool: mcp.NewTool("catalog_products_bulk_sku_update",
			mcp.WithDescription(
				"Batch-update SKUs for multiple products in one call.\n\n"+
					"Pass two same-length arrays:\n"+
					"  • product_ids — IDs of the products to update\n"+
					"  • skus — new SKU for each product (same order as product_ids)\n\n"+
					"Example: product_ids=[101,102] skus=[\"PART-A\",\"PART-B\"] sets product 101's SKU "+
					"to PART-A and product 102's SKU to PART-B.\n\n"+
					"Preview shows old→new diff. Pass confirmed=true to execute. Max 100 pairs per call.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Product IDs whose SKUs will be updated (positionally matched with skus)"),
				mcp.WithNumberItems(),
			),
			mcp.WithArray("skus",
				mcp.Description("New SKU values, one per product_id entry in the same order"),
				mcp.WithStringItems(),
			),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview to execute the SKU updates")),
		),
		Handler: p.handleBulkSKUUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete one or more products (DESTRUCTIVE — cannot be undone)",
		Description: "Deletes products by product_ids, sku, or product_name. " +
			"No category-scoped batch delete for safety. Preview lists products to be deleted. " +
			"Requires confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_products_delete",
			mcp.WithDescription(
				"Permanently delete products. Target by product_ids, sku, or product_name. "+
					"WARNING: This action is irreversible. Preview shows products to be deleted; "+
					"pass confirmed=true to execute.",
			),
			mcp.WithArray("product_ids", mcp.Description("Product IDs to delete"), mcp.WithNumberItems()),
			mcp.WithString("sku", mcp.Description("Delete single product by exact SKU")),
			mcp.WithString("product_name", mcp.Description("Delete single product by exact name")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview to execute deletion")),
		),
		Handler: p.handleDelete,
	})
	p.registerChannelAssignmentTools(reg)
	p.registerChannelSummaryTool(reg)
}

func (p *Products) handleSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	params, err := shared.ExtractFilters(args, ProductSearchFilters)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if v, ok := args["channel_ids"]; ok && v != nil {
		ids, perr := parseFloat64SliceToPositiveInts(v, "channel_ids")
		if perr != nil {
			return toolError("%s", perr.Error()), nil
		}
		if len(ids) > 0 {
			if len(ids) > 20 {
				return toolError("channel_ids: maximum 20 ids per request"), nil
			}
			params["channel_id:in"] = joinIntSlice(ids)
		}
	}

	hasFilter := shared.HasDataFilterBCParams(params, ProductSearchFilters, nonFilterKeys) || params["channel_id:in"] != ""
	if !hasFilter {
		return toolError(
			"at least one filter parameter is required (e.g. name_like, keyword, " +
				"category_id, sku, price_min). Omitting all filters would return the entire catalog.",
		), nil
	}

	if err := ErrInvalidBCSort(params, validSortFields,
		"valid options: id, name, sku, price, date_modified, date_last_imported, inventory_level, is_visible, total_sold"); err != nil {
		return toolError("%s", err.Error()), nil
	}

	if _, ok := params["include_fields"]; !ok {
		params["include_fields"] = "name,sku,price,calculated_price,sale_price,is_visible"
	}

	products, err := p.bc.SearchProducts(ctx, params)
	if err != nil {
		return toolError("search failed: %v", err), nil
	}

	type productSummary struct {
		ID              int      `json:"id"`
		Name            string   `json:"name"`
		SKU             string   `json:"sku,omitempty"`
		Price           float64  `json:"price"`
		CalculatedPrice *float64 `json:"calculated_price,omitempty"`
		SalePrice       *float64 `json:"sale_price,omitempty"`
		IsVisible       *bool    `json:"is_visible,omitempty"`
	}

	summaries := make([]productSummary, len(products))
	for i, prod := range products {
		s := productSummary{
			ID:    prod.ID,
			Name:  prod.Name,
			SKU:   prod.SKU,
			Price: prod.Price,
		}
		if prod.CalculatedPrice != 0 && prod.CalculatedPrice != prod.Price {
			cp := prod.CalculatedPrice
			s.CalculatedPrice = &cp
		}
		if prod.SalePrice != 0 {
			sp := prod.SalePrice
			s.SalePrice = &sp
		}
		if prod.IsVisible {
			vis := prod.IsVisible
			s.IsVisible = &vis
		}
		summaries[i] = s
	}

	result := map[string]any{
		"total_products": len(products),
		"products":       summaries,
	}

	return toolJSON(result)
}

func (p *Products) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	pidRaw, ok := args["product_id"]
	if !ok {
		return toolError("product_id is required"), nil
	}
	pidFloat, fOk := pidRaw.(float64)
	if !fOk {
		return toolError("product_id must be a number"), nil
	}
	pid := int(pidFloat)

	includeVariants, _ := args["include_variants"].(bool)

	product, err := p.bc.GetProduct(ctx, pid)
	if err != nil {
		return toolError("failed to get product %d: %v", pid, err), nil
	}

	variants, err := p.bc.ListVariantsForProduct(ctx, pid)
	if err != nil {
		return toolError("failed to get variants for product %d: %v", pid, err), nil
	}

	hasVariantPricing := false
	for _, v := range variants {
		if v.Price != 0 {
			hasVariantPricing = true
			break
		}
	}

	result := map[string]any{
		"product":             product,
		"has_variant_pricing": hasVariantPricing,
		"variant_count":       len(variants),
	}
	if includeVariants {
		result["variants"] = variants
	}

	return toolJSON(result)
}
