package catalog

import (
	"context"
	"fmt"
	"strconv"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/mark3labs/mcp-go/mcp"
)

// CategorySearchFilters maps tool parameters to BigCommerce Category Tree
// query parameters, using the same declarative pattern as ProductSearchFilters.
var CategorySearchFilters = []SearchFilter{
	{"name", "name", "string"},
	{"name_like", "name:like", "string"},
	{"parent_id", "parent_id", "number"},
	{"tree_id", "tree_id", "number"},
	{"is_visible", "is_visible", "bool"},
	{"keyword", "keyword", "string"},
}

// Categories provides MCP tool handlers for category operations.
type Categories struct {
	bc    BigCommerceAPI
	cache *session.Store
}

func NewCategories(bc BigCommerceAPI, cache *session.Store) *Categories {
	return &Categories{bc: bc, cache: cache}
}

func (c *Categories) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/list",
		Tier:    middleware.TierR0,
		Summary: "List or search categories by name, parent, visibility, or other filters",
		Description: "Fetches categories from the store's category tree. Supports filtering " +
			"or listing all categories. Use list_all=true when the user wants every category.",
		Tool: mcp.NewTool("catalog_categories_list",
			mcp.WithDescription(
				"List or search categories. Pass list_all=true to return every category, "+
					"or provide one or more filters. Returns category summaries with SEO fields.",
			),
			mcp.WithBoolean("list_all",
				mcp.Description("Set to true to return every category in the store. Use when the user says 'list all categories' or 'show me all categories'."),
			),
			mcp.WithString("name",
				mcp.Description("Exact category name match. Use when the user knows the full category name."),
			),
			mcp.WithString("name_like",
				mcp.Description("Partial name match (SQL LIKE). Use when the user gives a partial name or wants categories containing a word."),
			),
			mcp.WithNumber("parent_id",
				mcp.Description("Filter by parent category ID. Use to find sub-categories of a specific parent."),
			),
			mcp.WithNumber("tree_id",
				mcp.Description("Filter by category tree ID (for multi-storefront stores)."),
			),
			mcp.WithBoolean("is_visible",
				mcp.Description("Filter by storefront visibility. true = visible only, false = hidden only."),
			),
			mcp.WithString("keyword",
				mcp.Description("Full-text keyword search across category fields."),
			),
		),
		Handler: c.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/categories/get",
		Tier:        middleware.TierR0,
		Summary:     "Get a single category by ID with full details",
		Description: "Fetches full details for a specific category including SEO metadata.",
		Tool: mcp.NewTool("catalog_categories_get",
			mcp.WithDescription("Get category details by ID, including SEO fields and visibility."),
			mcp.WithNumber("category_id", mcp.Description("Category ID"), mcp.Required()),
		),
		Handler: c.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/create",
		Tier:    middleware.TierR1,
		Summary: "Create a new category in the store's catalog",
		Description: "Creates a single category. Requires a name; optionally accepts parent_name or " +
			"parent_id to create a subcategory, plus visibility, description, and SEO fields. " +
			"Uses the store's default category tree. Returns a preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_categories_create",
			mcp.WithDescription(
				"Create a new category. Provide at least a name. "+
					"To create a subcategory, provide parent_name (preferred) or parent_id. "+
					"Returns a preview first; pass confirmed=true to create.",
			),
			mcp.WithString("name",
				mcp.Description("Category name (required)."),
				mcp.Required(),
			),
			mcp.WithString("parent_name",
				mcp.Description("Parent category name. Use this when the user refers to a parent by name (e.g. 'under Electronics'). Resolved server-side to an ID. Mutually exclusive with parent_id."),
			),
			mcp.WithNumber("parent_id",
				mcp.Description("Parent category ID. Use only if the numeric ID is known. 0 = root-level category (default)."),
			),
			mcp.WithString("description",
				mcp.Description("Category description (can contain HTML)."),
			),
			mcp.WithBoolean("is_visible",
				mcp.Description("Storefront visibility. Defaults to true."),
			),
			mcp.WithString("page_title",
				mcp.Description("SEO page title for the category."),
			),
			mcp.WithString("meta_description",
				mcp.Description("SEO meta description for the category."),
			),
			mcp.WithString("search_keywords",
				mcp.Description("Comma-separated search keywords."),
			),
			mcp.WithNumber("sort_order",
				mcp.Description("Display sort order among sibling categories."),
			),
			mcp.WithString("default_product_sort",
				mcp.Description("Product sort on category page. Valid: best_selling, price_desc, price_asc, avg_customer_review, alpha_asc, alpha_desc, featured, newest, use_store_settings."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: c.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/bulk_update",
		Tier:    middleware.TierR1,
		Summary: "Bulk update category properties: visibility, SEO, name, sort order",
		Description: "Update multiple categories at once. Supports modifying name, visibility, " +
			"page_title, meta_description, search_keywords, description, sort_order, and " +
			"default_product_sort. Returns a preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_categories_bulk_update",
			mcp.WithDescription(
				"Bulk update categories. Provide category_ids and the fields to change. "+
					"Returns a preview first; pass confirmed=true to execute the changes.",
			),
			mcp.WithArray("category_ids",
				mcp.Description("Array of category IDs to update"),
				mcp.WithNumberItems(),
			),
			mcp.WithString("set_name",
				mcp.Description("New name for all targeted categories. Use with caution on multiple categories."),
			),
			mcp.WithBoolean("set_is_visible",
				mcp.Description("Set visibility for all targeted categories. true = show on storefront, false = hide."),
			),
			mcp.WithString("set_page_title",
				mcp.Description("New SEO page title for all targeted categories."),
			),
			mcp.WithString("set_meta_description",
				mcp.Description("New SEO meta description for all targeted categories."),
			),
			mcp.WithString("set_search_keywords",
				mcp.Description("New search keywords (comma-separated) for all targeted categories."),
			),
			mcp.WithString("set_description",
				mcp.Description("New category description (can include HTML) for all targeted categories."),
			),
			mcp.WithNumber("set_sort_order",
				mcp.Description("New sort order for all targeted categories."),
			),
			mcp.WithString("set_default_product_sort",
				mcp.Description("How products are sorted on category page. Valid: best_selling, price_desc, price_asc, avg_customer_review, alpha_asc, alpha_desc, featured, newest, use_store_settings."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: c.handleBulkUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete a single category from the store",
		Description: "Deletes one category by name or ID. Subcategories are automatically deleted " +
			"by BigCommerce. Products remain in the store but lose the category assignment. " +
			"If the category has children, include_children=true is required as an explicit acknowledgment.",
		Tool: mcp.NewTool("catalog_categories_delete",
			mcp.WithDescription(
				"Delete a category. Provide category_name (preferred) or category_id. "+
					"Products assigned to the category are NOT deleted — they remain in the store. "+
					"If the category has subcategories, you must set include_children=true to acknowledge "+
					"the subtree will also be deleted. Returns a preview first; pass confirmed=true to execute.",
			),
			mcp.WithString("category_name",
				mcp.Description("Category name to delete. Resolved to an ID server-side. Use when the user refers to a category by name."),
			),
			mcp.WithNumber("category_id",
				mcp.Description("Category ID to delete. Use only if the numeric ID is known."),
			),
			mcp.WithBoolean("include_children",
				mcp.Description("Required acknowledgment when the category has subcategories. Set to true to confirm you want the entire subtree deleted."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: c.handleDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/products",
		Tier:    middleware.TierR0,
		Summary: "List products belonging to a category",
		Description: "Lists products in a category with summaries including price, SKU, and category memberships. " +
			"Accepts category_id or category_name for resolution.",
		Tool: mcp.NewTool("catalog_categories_products",
			mcp.WithDescription(
				"List products in a category. Provide category_id or category_name (exact match). "+
					"Returns product summaries with category memberships.",
			),
			mcp.WithNumber("category_id",
				mcp.Description("Category ID. Mutually exclusive with category_name."),
			),
			mcp.WithString("category_name",
				mcp.Description("Exact category name. Resolved server-side. Mutually exclusive with category_id."),
			),
			mcp.WithNumber("limit",
				mcp.Description("Max products to return (default: all)."),
			),
			mcp.WithString("sort",
				mcp.Description("Sort field: id, name, sku, price, date_modified, total_sold (default: id)."),
			),
			mcp.WithString("sort_direction",
				mcp.Description("Sort direction: asc or desc (default: asc)."),
			),
		),
		Handler: c.handleCategoryProducts,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/seo_audit",
		Tier:    middleware.TierR0,
		Summary: "Audit categories for missing SEO fields (page_title, meta_description, search_keywords)",
		Description: "Scans all or filtered categories and reports which ones have empty SEO fields. " +
			"Use the results with catalog/categories/bulk_update to fill in missing fields.",
		Tool: mcp.NewTool("catalog_categories_seo_audit",
			mcp.WithDescription(
				"Audit categories for missing SEO fields. Returns categories grouped by missing fields. "+
					"Optionally filter by parent_id or tree_id.",
			),
			mcp.WithNumber("parent_id",
				mcp.Description("Only audit categories under this parent ID."),
			),
			mcp.WithNumber("tree_id",
				mcp.Description("Only audit categories in this tree (multi-storefront)."),
			),
		),
		Handler: c.handleSEOAudit,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/move",
		Tier:    middleware.TierR2,
		Summary: "Move a category to a new parent (reparent)",
		Description: "Changes a category's parent, moving it and its entire subtree to a new location " +
			"in the category tree. Validates against cycles. R2 tier — always requires confirmation.",
		Tool: mcp.NewTool("catalog_categories_move",
			mcp.WithDescription(
				"Move a category to a new parent. Accepts source by category_id or category_name, "+
					"destination by new_parent_id or new_parent_name (use new_parent_id=0 for root). "+
					"Prevents cycles. Preview first; pass confirmed=true to execute.",
			),
			mcp.WithNumber("category_id",
				mcp.Description("ID of the category to move. Mutually exclusive with category_name."),
			),
			mcp.WithString("category_name",
				mcp.Description("Name of the category to move. Mutually exclusive with category_id."),
			),
			mcp.WithNumber("new_parent_id",
				mcp.Description("Destination parent ID (0 = move to root). Mutually exclusive with new_parent_name."),
			),
			mcp.WithString("new_parent_name",
				mcp.Description("Destination parent name. Mutually exclusive with new_parent_id."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: c.handleMove,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/reorder",
		Tier:    middleware.TierR1,
		Summary: "Reorder sibling categories by providing them in the desired order",
		Description: "Assigns sequential sort_order values to categories in the order provided. " +
			"All categories must share the same parent. Uses configurable start and increment " +
			"(default: 0, 10) to leave gaps for future insertions.",
		Tool: mcp.NewTool("catalog_categories_reorder",
			mcp.WithDescription(
				"Reorder sibling categories. Provide an ordered list of category_ids or category_names. "+
					"All must share the same parent. Preview first; pass confirmed=true to execute.",
			),
			mcp.WithArray("category_ids",
				mcp.Description("Category IDs in desired display order."),
				mcp.WithNumberItems(),
			),
			mcp.WithArray("category_names",
				mcp.Description("Category names in desired display order (resolved server-side). Mutually exclusive with category_ids."),
				mcp.WithStringItems(),
			),
			mcp.WithNumber("start_sort_order",
				mcp.Description("Starting sort_order value (default: 0)."),
			),
			mcp.WithNumber("increment",
				mcp.Description("Gap between sort_order values (default: 10)."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: c.handleReorder,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/metafields/list",
		Tier:    middleware.TierR0,
		Summary: "List metafields on a category",
		Description: "Returns all custom key-value metafields attached to a category. " +
			"Accepts category_id or category_name.",
		Tool: mcp.NewTool("catalog_categories_metafields_list",
			mcp.WithDescription("List all metafields on a category. Provide category_id or category_name."),
			mcp.WithNumber("category_id", mcp.Description("Category ID. Mutually exclusive with category_name.")),
			mcp.WithString("category_name", mcp.Description("Exact category name. Mutually exclusive with category_id.")),
		),
		Handler: c.handleMetafieldsList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/metafields/set",
		Tier:    middleware.TierR1,
		Summary: "Create or update a metafield on a category",
		Description: "Sets a metafield by namespace+key. If a metafield with the same namespace and key " +
			"exists, it is updated; otherwise a new one is created. Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_categories_metafields_set",
			mcp.WithDescription(
				"Create or update a category metafield. Provide category target, namespace, key, and value. "+
					"Preview first; pass confirmed=true to execute.",
			),
			mcp.WithNumber("category_id", mcp.Description("Category ID. Mutually exclusive with category_name.")),
			mcp.WithString("category_name", mcp.Description("Exact category name. Mutually exclusive with category_id.")),
			mcp.WithString("namespace", mcp.Description("Metafield namespace (e.g. 'my_app')."), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key within the namespace."), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional human-readable description of this metafield.")),
			mcp.WithString("permission_set", mcp.Description("Access level: app_only, read, write, read_and_sf_access, write_and_sf_access. Default: write.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute after reviewing the preview.")),
		),
		Handler: c.handleMetafieldsSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/metafields/delete",
		Tier:    middleware.TierR1,
		Summary: "Delete a metafield from a category",
		Description: "Deletes a metafield by metafield_id, or by namespace+key lookup. " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_categories_metafields_delete",
			mcp.WithDescription(
				"Delete a category metafield. Provide category target and either metafield_id or namespace+key. "+
					"Preview first; pass confirmed=true to execute.",
			),
			mcp.WithNumber("category_id", mcp.Description("Category ID. Mutually exclusive with category_name.")),
			mcp.WithString("category_name", mcp.Description("Exact category name. Mutually exclusive with category_id.")),
			mcp.WithNumber("metafield_id", mcp.Description("Metafield ID to delete. Mutually exclusive with namespace+key.")),
			mcp.WithString("namespace", mcp.Description("Metafield namespace (use with key). Mutually exclusive with metafield_id.")),
			mcp.WithString("key", mcp.Description("Metafield key (use with namespace). Mutually exclusive with metafield_id.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute after reviewing the preview.")),
		),
		Handler: c.handleMetafieldsDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/categories/bulk_delete",
		Tier:    middleware.TierR3,
		Summary: "Bulk delete multiple categories from the store",
		Description: "Deletes multiple categories by ID. Subcategories are automatically deleted " +
			"by BigCommerce. Products remain in the store but lose the category assignment. " +
			"If any targeted category has children, include_children=true is required.",
		Tool: mcp.NewTool("catalog_categories_bulk_delete",
			mcp.WithDescription(
				"Bulk delete categories. Provide an array of category IDs. "+
					"Products are NOT deleted — they remain in the store. "+
					"If any category has subcategories, set include_children=true to acknowledge "+
					"the subtrees will also be deleted. Returns a preview first; pass confirmed=true to execute.",
			),
			mcp.WithArray("category_ids",
				mcp.Description("Array of category IDs to delete."),
				mcp.WithNumberItems(),
			),
			mcp.WithBoolean("include_children",
				mcp.Description("Required acknowledgment when any targeted category has subcategories. Set to true to confirm subtree deletion."),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: c.handleBulkDelete,
	})
}

func (c *Categories) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	listAll := false
	if v, ok := args["list_all"]; ok {
		if b, bOk := v.(bool); bOk {
			listAll = b
		}
	}

	params, err := ExtractFilters(args, CategorySearchFilters)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if len(params) == 0 && !listAll {
		return toolError(
			"provide at least one filter (e.g. name, name_like, parent_id, is_visible) "+
				"or set list_all=true to return every category.",
		), nil
	}

	cats, err := c.bc.SearchCategories(ctx, params)
	if err != nil {
		return toolError("failed to search categories: %v", err), nil
	}

	type categorySummary struct {
		ID              int    `json:"id"`
		ParentID        int    `json:"parent_id,omitempty"`
		Name            string `json:"name"`
		IsVisible       bool   `json:"is_visible"`
		PageTitle       string `json:"page_title,omitempty"`
		MetaDescription string `json:"meta_description,omitempty"`
	}

	summaries := make([]categorySummary, len(cats))
	for i, cat := range cats {
		summaries[i] = categorySummary{
			ID:              cat.ID,
			ParentID:        cat.ParentID,
			Name:            cat.Name,
			IsVisible:       cat.IsVisible,
			PageTitle:       cat.PageTitle,
			MetaDescription: cat.MetaDescription,
		}
	}

	result := map[string]any{
		"total":      len(cats),
		"categories": summaries,
	}

	return toolJSON(result)
}

func (c *Categories) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cidRaw, ok := args["category_id"]
	if !ok {
		return toolError("category_id is required"), nil
	}
	cidFloat, fOk := cidRaw.(float64)
	if !fOk {
		return toolError("category_id must be a number"), nil
	}
	cid := int(cidFloat)

	cat, err := c.bc.GetCategory(ctx, cid)
	if err != nil {
		return toolError("failed to get category %d: %v", cid, err), nil
	}

	return toolJSON(cat)
}

type bulkCategoryParams struct {
	categoryIDs        []int
	setName            *string
	setIsVisible       *bool
	setPageTitle       *string
	setMetaDescription *string
	setSearchKeywords  *string
	setDescription     *string
	setSortOrder       *int
	setDefaultSort     *string
	confirmed          bool
}

func parseBulkCategoryParams(args map[string]any) (*bulkCategoryParams, error) {
	p := &bulkCategoryParams{}

	idsRaw, ok := args["category_ids"]
	if !ok {
		return nil, fmt.Errorf("category_ids is required")
	}
	idsSlice, sOk := idsRaw.([]any)
	if !sOk {
		return nil, fmt.Errorf("category_ids must be an array of numbers")
	}
	if len(idsSlice) == 0 {
		return nil, fmt.Errorf("category_ids must not be empty")
	}
	for _, raw := range idsSlice {
		f, fOk := raw.(float64)
		if !fOk {
			return nil, fmt.Errorf("each category_id must be a number")
		}
		p.categoryIDs = append(p.categoryIDs, int(f))
	}

	if v, ok := args["set_name"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("set_name must be a string")
		}
		p.setName = &s
	}
	if v, ok := args["set_is_visible"]; ok {
		b, bOk := v.(bool)
		if !bOk {
			return nil, fmt.Errorf("set_is_visible must be a boolean")
		}
		p.setIsVisible = &b
	}
	if v, ok := args["set_page_title"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("set_page_title must be a string")
		}
		p.setPageTitle = &s
	}
	if v, ok := args["set_meta_description"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("set_meta_description must be a string")
		}
		p.setMetaDescription = &s
	}
	if v, ok := args["set_search_keywords"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("set_search_keywords must be a string")
		}
		p.setSearchKeywords = &s
	}
	if v, ok := args["set_description"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("set_description must be a string")
		}
		p.setDescription = &s
	}
	if v, ok := args["set_sort_order"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("set_sort_order must be a number")
		}
		i := int(f)
		p.setSortOrder = &i
	}
	if v, ok := args["set_default_product_sort"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("set_default_product_sort must be a string")
		}
		validSorts := map[string]bool{
			"best_selling": true, "price_desc": true, "price_asc": true,
			"avg_customer_review": true, "alpha_asc": true, "alpha_desc": true,
			"featured": true, "newest": true, "use_store_settings": true,
		}
		if !validSorts[s] {
			return nil, fmt.Errorf("invalid default_product_sort %q", s)
		}
		p.setDefaultSort = &s
	}

	if v, ok := args["confirmed"]; ok {
		b, bOk := v.(bool)
		if bOk {
			p.confirmed = b
		}
	}

	hasUpdate := p.setName != nil || p.setIsVisible != nil || p.setPageTitle != nil ||
		p.setMetaDescription != nil || p.setSearchKeywords != nil || p.setDescription != nil ||
		p.setSortOrder != nil || p.setDefaultSort != nil
	if !hasUpdate {
		return nil, fmt.Errorf("at least one set_* parameter is required to specify what to change")
	}

	return p, nil
}

func (c *Categories) handleBulkUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := parseBulkCategoryParams(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if params.confirmed {
		return c.executeBulkUpdate(ctx, params)
	}
	return c.previewBulkUpdate(ctx, params)
}

func (c *Categories) previewBulkUpdate(ctx context.Context, params *bulkCategoryParams) (*mcp.CallToolResult, error) {
	cats, err := c.fetchCategoriesByIDs(ctx, params.categoryIDs)
	if err != nil {
		return toolError("failed to fetch categories: %v", err), nil
	}

	type change struct {
		CategoryID int    `json:"category_id"`
		Name       string `json:"name"`
		Field      string `json:"field"`
		Current    any    `json:"current"`
		Proposed   any    `json:"proposed"`
	}

	var changes []change

	for _, cat := range cats {
		if params.setName != nil {
			changes = append(changes, change{cat.ID, cat.Name, "name", cat.Name, *params.setName})
		}
		if params.setIsVisible != nil {
			changes = append(changes, change{cat.ID, cat.Name, "is_visible", cat.IsVisible, *params.setIsVisible})
		}
		if params.setPageTitle != nil {
			changes = append(changes, change{cat.ID, cat.Name, "page_title", cat.PageTitle, *params.setPageTitle})
		}
		if params.setMetaDescription != nil {
			changes = append(changes, change{cat.ID, cat.Name, "meta_description", cat.MetaDescription, *params.setMetaDescription})
		}
		if params.setSearchKeywords != nil {
			changes = append(changes, change{cat.ID, cat.Name, "search_keywords", cat.SearchKeywords, *params.setSearchKeywords})
		}
		if params.setDescription != nil {
			current := cat.Description
			if len(current) > 100 {
				current = current[:100] + "..."
			}
			proposed := *params.setDescription
			if len(proposed) > 100 {
				proposed = proposed[:100] + "..."
			}
			changes = append(changes, change{cat.ID, cat.Name, "description", current, proposed})
		}
		if params.setSortOrder != nil {
			changes = append(changes, change{cat.ID, cat.Name, "sort_order", cat.SortOrder, *params.setSortOrder})
		}
		if params.setDefaultSort != nil {
			changes = append(changes, change{cat.ID, cat.Name, "default_product_sort", cat.DefaultProductSort, *params.setDefaultSort})
		}
	}

	result := map[string]any{
		"status":             "preview",
		"categories_count":   len(cats),
		"changes":            changes,
		"total_field_updates": len(changes),
		"message":            "Review the changes above. Pass confirmed=true with the same parameters to execute.",
	}

	return toolJSON(result)
}

func (c *Categories) executeBulkUpdate(ctx context.Context, params *bulkCategoryParams) (*mcp.CallToolResult, error) {
	cats, err := c.fetchCategoriesByIDs(ctx, params.categoryIDs)
	if err != nil {
		return toolError("failed to fetch categories: %v", err), nil
	}

	updates := make([]bigcommerce.CategoryUpdate, len(cats))
	for i, cat := range cats {
		u := bigcommerce.CategoryUpdate{CategoryID: cat.ID}
		if params.setName != nil {
			u.Name = params.setName
		}
		if params.setIsVisible != nil {
			u.IsVisible = params.setIsVisible
		}
		if params.setPageTitle != nil {
			u.PageTitle = params.setPageTitle
		}
		if params.setMetaDescription != nil {
			u.MetaDescription = params.setMetaDescription
		}
		if params.setSearchKeywords != nil {
			u.SearchKeywords = params.setSearchKeywords
		}
		if params.setDescription != nil {
			u.Description = params.setDescription
		}
		if params.setSortOrder != nil {
			u.SortOrder = params.setSortOrder
		}
		if params.setDefaultSort != nil {
			u.DefaultProductSort = params.setDefaultSort
		}
		updates[i] = u
	}

	batchResult, err := c.bc.BatchUpdateCategories(ctx, updates)
	if err != nil {
		return toolError("batch update failed: %v", err), nil
	}

	result := map[string]any{
		"status":    "executed",
		"succeeded": batchResult.Succeeded,
		"failed":    batchResult.Failed,
	}
	if len(batchResult.Errors) > 0 {
		result["errors"] = batchResult.Errors
	}

	fields := []string{}
	if params.setName != nil {
		fields = append(fields, "name")
	}
	if params.setIsVisible != nil {
		fields = append(fields, "is_visible")
	}
	if params.setPageTitle != nil {
		fields = append(fields, "page_title")
	}
	if params.setMetaDescription != nil {
		fields = append(fields, "meta_description")
	}
	if params.setSearchKeywords != nil {
		fields = append(fields, "search_keywords")
	}
	if params.setDescription != nil {
		fields = append(fields, "description")
	}
	if params.setSortOrder != nil {
		fields = append(fields, "sort_order")
	}
	if params.setDefaultSort != nil {
		fields = append(fields, "default_product_sort")
	}
	result["fields_updated"] = fields

	return toolJSON(result)
}

func (c *Categories) fetchCategoriesByIDs(ctx context.Context, ids []int) ([]bigcommerce.Category, error) {
	cats, err := c.bc.GetCategoriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(cats) != len(ids) {
		fetched := make(map[int]bool, len(cats))
		for _, cat := range cats {
			fetched[cat.ID] = true
		}
		for _, id := range ids {
			if !fetched[id] {
				return nil, fmt.Errorf("category %d not found", id)
			}
		}
	}
	return cats, nil
}

// --- Create category handler ---

func (c *Categories) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	params, err := ParseCategoryCreateParams(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if params.ParentName != "" {
		parentID, resolveErr := c.resolveParentName(ctx, params.ParentName)
		if resolveErr != nil {
			return toolError("%s", resolveErr.Error()), nil
		}
		params.Payload.ParentID = parentID
	}

	if params.Payload.ParentID > 0 {
		// Child categories inherit the tree from their parent; no need to set tree_id.
		params.Payload.TreeID = 0
	} else {
		treeID, err := c.bc.GetDefaultTreeID(ctx)
		if err != nil {
			return toolError("failed to determine category tree: %v", err), nil
		}
		params.Payload.TreeID = treeID
	}

	if params.Confirmed {
		return c.executeCreate(ctx, params)
	}
	return c.previewCreate(params)
}

// resolveParentName looks up a category by exact name. Delegates to the shared
// resolveCategoryByExactName function in product_resolve.go.
func (c *Categories) resolveParentName(ctx context.Context, name string) (int, error) {
	return resolveCategoryByExactName(ctx, c.bc, name)
}

// CategoryCreateParams holds parsed create-category arguments. Exported for testing.
type CategoryCreateParams struct {
	Payload    bigcommerce.CategoryCreate
	ParentName string
	Confirmed  bool
}

// ParseCategoryCreateParams is exported for unit testing.
func ParseCategoryCreateParams(args map[string]any) (*CategoryCreateParams, error) {
	p := &CategoryCreateParams{}

	nameRaw, ok := args["name"]
	if !ok {
		return nil, fmt.Errorf("name is required")
	}
	name, sOk := nameRaw.(string)
	if !sOk || name == "" {
		return nil, fmt.Errorf("name must be a non-empty string")
	}
	p.Payload.Name = name

	_, hasParentID := args["parent_id"]
	_, hasParentName := args["parent_name"]
	if hasParentID && hasParentName {
		return nil, fmt.Errorf("parent_id and parent_name are mutually exclusive; provide one or neither")
	}

	if v, ok := args["parent_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("parent_id must be a number")
		}
		p.Payload.ParentID = int(f)
	}

	if v, ok := args["parent_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("parent_name must be a non-empty string")
		}
		p.ParentName = s
	}

	if v, ok := args["description"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("description must be a string")
		}
		p.Payload.Description = s
	}

	if v, ok := args["is_visible"]; ok {
		b, bOk := v.(bool)
		if !bOk {
			return nil, fmt.Errorf("is_visible must be a boolean")
		}
		p.Payload.IsVisible = &b
	}

	if v, ok := args["page_title"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("page_title must be a string")
		}
		p.Payload.PageTitle = s
	}

	if v, ok := args["meta_description"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("meta_description must be a string")
		}
		p.Payload.MetaDescription = s
	}

	if v, ok := args["search_keywords"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("search_keywords must be a string")
		}
		p.Payload.SearchKeywords = s
	}

	if v, ok := args["sort_order"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("sort_order must be a number")
		}
		p.Payload.SortOrder = int(f)
	}

	if v, ok := args["default_product_sort"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("default_product_sort must be a string")
		}
		validSorts := map[string]bool{
			"best_selling": true, "price_desc": true, "price_asc": true,
			"avg_customer_review": true, "alpha_asc": true, "alpha_desc": true,
			"featured": true, "newest": true, "use_store_settings": true,
		}
		if !validSorts[s] {
			return nil, fmt.Errorf("invalid default_product_sort %q", s)
		}
		p.Payload.DefaultProductSort = s
	}

	if v, ok := args["confirmed"]; ok {
		b, bOk := v.(bool)
		if bOk {
			p.Confirmed = b
		}
	}

	return p, nil
}

func (c *Categories) previewCreate(params *CategoryCreateParams) (*mcp.CallToolResult, error) {
	catMap := map[string]any{
		"name": params.Payload.Name,
	}
	if params.Payload.TreeID > 0 {
		catMap["tree_id"] = params.Payload.TreeID
	}
	if params.Payload.ParentID > 0 {
		catMap["parent_id"] = params.Payload.ParentID
		if params.ParentName != "" {
			catMap["parent_name"] = params.ParentName
		}
	} else {
		catMap["parent_id"] = 0
	}

	preview := map[string]any{
		"status":   "preview",
		"message":  "Review the category below. Pass confirmed=true with the same parameters to create it.",
		"category": catMap,
	}
	if params.Payload.Description != "" {
		catMap["description"] = params.Payload.Description
	}
	if params.Payload.IsVisible != nil {
		catMap["is_visible"] = *params.Payload.IsVisible
	}
	if params.Payload.PageTitle != "" {
		catMap["page_title"] = params.Payload.PageTitle
	}
	if params.Payload.MetaDescription != "" {
		catMap["meta_description"] = params.Payload.MetaDescription
	}
	if params.Payload.SearchKeywords != "" {
		catMap["search_keywords"] = params.Payload.SearchKeywords
	}
	if params.Payload.SortOrder != 0 {
		catMap["sort_order"] = params.Payload.SortOrder
	}
	if params.Payload.DefaultProductSort != "" {
		catMap["default_product_sort"] = params.Payload.DefaultProductSort
	}

	return toolJSON(preview)
}

func (c *Categories) executeCreate(ctx context.Context, params *CategoryCreateParams) (*mcp.CallToolResult, error) {
	cats, err := c.bc.CreateCategory(ctx, params.Payload)
	if err != nil {
		return toolError("failed to create category: %v", err), nil
	}

	if len(cats) == 0 {
		return toolError("category creation returned no results"), nil
	}

	created := cats[0]
	result := map[string]any{
		"status":  "created",
		"message": fmt.Sprintf("Category %q created successfully with ID %d.", created.Name, created.ID),
		"category": map[string]any{
			"id":         created.ID,
			"name":       created.Name,
			"parent_id":  created.ParentID,
			"tree_id":    created.TreeID,
			"is_visible": created.IsVisible,
		},
	}
	if created.PageTitle != "" {
		result["category"].(map[string]any)["page_title"] = created.PageTitle
	}

	return toolJSON(result)
}

// --- Delete category handlers ---

// DeleteParams holds parsed arguments for single and bulk delete. Exported for testing.
type DeleteParams struct {
	CategoryIDs     []int
	CategoryName    string
	IncludeChildren bool
	Confirmed       bool
}

// ParseSingleDeleteParams is exported for unit testing.
func ParseSingleDeleteParams(args map[string]any) (*DeleteParams, error) {
	p := &DeleteParams{}

	_, hasID := args["category_id"]
	_, hasName := args["category_name"]
	if !hasID && !hasName {
		return nil, fmt.Errorf("provide either category_name or category_id")
	}
	if hasID && hasName {
		return nil, fmt.Errorf("category_id and category_name are mutually exclusive; provide one")
	}

	if v, ok := args["category_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("category_id must be a number")
		}
		p.CategoryIDs = []int{int(f)}
	}

	if v, ok := args["category_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("category_name must be a non-empty string")
		}
		p.CategoryName = s
	}

	if v, ok := args["include_children"]; ok {
		b, bOk := v.(bool)
		if !bOk {
			return nil, fmt.Errorf("include_children must be a boolean")
		}
		p.IncludeChildren = b
	}

	if v, ok := args["confirmed"]; ok {
		b, bOk := v.(bool)
		if bOk {
			p.Confirmed = b
		}
	}

	return p, nil
}

// ParseBulkDeleteParams is exported for unit testing.
func ParseBulkDeleteParams(args map[string]any) (*DeleteParams, error) {
	p := &DeleteParams{}

	idsRaw, ok := args["category_ids"]
	if !ok {
		return nil, fmt.Errorf("category_ids is required")
	}
	idsSlice, sOk := idsRaw.([]any)
	if !sOk {
		return nil, fmt.Errorf("category_ids must be an array of numbers")
	}
	if len(idsSlice) == 0 {
		return nil, fmt.Errorf("category_ids must not be empty")
	}
	for _, raw := range idsSlice {
		f, fOk := raw.(float64)
		if !fOk {
			return nil, fmt.Errorf("each category_id must be a number")
		}
		p.CategoryIDs = append(p.CategoryIDs, int(f))
	}

	if v, ok := args["include_children"]; ok {
		b, bOk := v.(bool)
		if !bOk {
			return nil, fmt.Errorf("include_children must be a boolean")
		}
		p.IncludeChildren = b
	}

	if v, ok := args["confirmed"]; ok {
		b, bOk := v.(bool)
		if bOk {
			p.Confirmed = b
		}
	}

	return p, nil
}

func (c *Categories) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := ParseSingleDeleteParams(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if params.CategoryName != "" {
		id, resolveErr := c.resolveParentName(ctx, params.CategoryName)
		if resolveErr != nil {
			return toolError("%s", resolveErr.Error()), nil
		}
		params.CategoryIDs = []int{id}
	}

	return c.processDelete(ctx, params)
}

func (c *Categories) handleBulkDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := ParseBulkDeleteParams(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	return c.processDelete(ctx, params)
}

func (c *Categories) processDelete(ctx context.Context, params *DeleteParams) (*mcp.CallToolResult, error) {
	cats, err := c.fetchCategoriesByIDs(ctx, params.CategoryIDs)
	if err != nil {
		return toolError("failed to fetch categories: %v", err), nil
	}

	type categoryDetail struct {
		ID          int      `json:"id"`
		Name        string   `json:"name"`
		ParentID    int      `json:"parent_id,omitempty"`
		ChildCount  int      `json:"child_count"`
		ChildNames  []string `json:"child_names,omitempty"`
	}

	details := make([]categoryDetail, 0, len(cats))
	totalChildren := 0
	for _, cat := range cats {
		children, childErr := c.getDirectChildren(ctx, cat.ID)
		if childErr != nil {
			return toolError("failed to check children for category %d: %v", cat.ID, childErr), nil
		}
		childNames := make([]string, len(children))
		for i, ch := range children {
			childNames[i] = fmt.Sprintf("%s (ID %d)", ch.Name, ch.ID)
		}
		details = append(details, categoryDetail{
			ID:         cat.ID,
			Name:       cat.Name,
			ParentID:   cat.ParentID,
			ChildCount: len(children),
			ChildNames: childNames,
		})
		totalChildren += len(children)
	}

	hasChildren := totalChildren > 0
	if hasChildren && !params.IncludeChildren {
		result := map[string]any{
			"status": "blocked",
			"message": "One or more categories have subcategories that will also be deleted. " +
				"Set include_children=true to acknowledge this before proceeding.",
			"categories":     details,
			"total_children":  totalChildren,
			"products_impact": "Products are NOT deleted. They remain in your store but lose the deleted category assignment. You can reassign them to other categories afterward.",
		}
		return toolJSON(result)
	}

	if !params.Confirmed {
		result := map[string]any{
			"status":  "preview",
			"message": "Review the categories below. Pass confirmed=true with the same parameters to delete.",
			"categories":       details,
			"categories_count": len(cats),
			"products_impact":  "Products are NOT deleted. They remain in your store but lose the deleted category assignment. You can reassign them to other categories afterward.",
		}
		if hasChildren {
			result["total_children"] = totalChildren
			result["children_warning"] = "Subcategories listed above will also be permanently deleted."
		}
		return toolJSON(result)
	}

	err = c.bc.DeleteCategories(ctx, params.CategoryIDs)
	if err != nil {
		return toolError("delete failed: %v", err), nil
	}

	deletedNames := make([]string, len(cats))
	for i, cat := range cats {
		deletedNames[i] = fmt.Sprintf("%s (ID %d)", cat.Name, cat.ID)
	}

	result := map[string]any{
		"status":          "deleted",
		"message":         fmt.Sprintf("Successfully deleted %d category(ies).", len(cats)),
		"deleted":         deletedNames,
		"products_impact": "Products that were assigned to these categories remain in your store. They can be reassigned to other categories.",
	}
	if hasChildren {
		result["children_deleted"] = fmt.Sprintf("%d subcategory(ies) were also deleted.", totalChildren)
	}

	return toolJSON(result)
}

// getDirectChildren returns the immediate children of a category.
func (c *Categories) getDirectChildren(ctx context.Context, categoryID int) ([]bigcommerce.Category, error) {
	return c.bc.SearchCategories(ctx, map[string]string{
		"parent_id": strconv.Itoa(categoryID),
	})
}
