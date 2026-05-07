package catalog

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

const (
	maxPriceListRecordUpsertRows = 100
	maxPriceListAssignmentBatch  = 25
	priceListPreviewSampleRows   = 5
)

// PriceLists exposes catalog/pricelists* tools.
type PriceLists struct {
	bc BigCommerceAPI
}

// NewPriceLists constructs price list tool handlers.
func NewPriceLists(bc BigCommerceAPI) *PriceLists {
	return &PriceLists{bc: bc}
}

// RegisterTools registers catalog/pricelists/* tools.
func (p *PriceLists) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/pricelists/list",
		Tier:    middleware.TierR0,
		Summary: "List price lists",
		Description: "GET /v3/pricelists with optional filters and pagination. " +
			"Price lists are catalog pricing overlays used by channel/customer-group assignment.",
		Tool: mcp.NewTool("catalog_pricelists_list",
			mcp.WithDescription("List price lists with optional ID/name/date filters."),
			mcp.WithNumber("id", mcp.Description("Exact price list ID filter.")),
			mcp.WithArray("ids", mcp.Description("Filter by a list of price list IDs."), mcp.WithNumberItems()),
			mcp.WithString("name", mcp.Description("Exact name filter.")),
			mcp.WithString("name_like", mcp.Description("Partial name filter (name:like).")),
			mcp.WithString("date_created", mcp.Description("Exact date_created filter.")),
			mcp.WithString("date_modified", mcp.Description("Exact date_modified filter.")),
			mcp.WithString("date_created_min", mcp.Description("date_created:min filter.")),
			mcp.WithString("date_created_max", mcp.Description("date_created:max filter.")),
			mcp.WithString("date_modified_min", mcp.Description("date_modified:min filter.")),
			mcp.WithString("date_modified_max", mcp.Description("date_modified:max filter.")),
			mcp.WithNumber("page", mcp.Description("Offset pagination page number.")),
			mcp.WithNumber("limit", mcp.Description("Offset pagination page size.")),
			mcp.WithString("before", mcp.Description("Cursor pagination: fetch rows before cursor.")),
			mcp.WithString("after", mcp.Description("Cursor pagination: fetch rows after cursor.")),
		),
		Handler: p.handleListPriceLists,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/get",
		Tier:        middleware.TierR0,
		Summary:     "Get one price list",
		Description: "GET /v3/pricelists/{price_list_id}.",
		Tool: mcp.NewTool("catalog_pricelists_get",
			mcp.WithDescription("Fetch one price list by ID."),
			mcp.WithNumber("price_list_id", mcp.Description("Price list ID."), mcp.Required()),
		),
		Handler: p.handleGetPriceList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/create",
		Tier:        middleware.TierR1,
		Summary:     "Create a price list",
		Description: "POST /v3/pricelists. Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_pricelists_create",
			mcp.WithDescription("Create a price list. Preview first, then pass confirmed=true."),
			mcp.WithString("name", mcp.Description("Unique price list name."), mcp.Required()),
			mcp.WithBoolean("active", mcp.Description("Whether the list is active; defaults to true if omitted.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: p.handleCreatePriceList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/update",
		Tier:        middleware.TierR1,
		Summary:     "Update a price list",
		Description: "PUT /v3/pricelists/{price_list_id}. Fetches current state for preview; pass confirmed=true to apply.",
		Tool: mcp.NewTool("catalog_pricelists_update",
			mcp.WithDescription("Update a price list. Provide one or more patch fields; preview then confirmed=true."),
			mcp.WithNumber("price_list_id", mcp.Description("Price list ID."), mcp.Required()),
			mcp.WithString("name", mcp.Description("New price list name.")),
			mcp.WithBoolean("active", mcp.Description("Set active state.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: p.handleUpdatePriceList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/delete",
		Tier:        middleware.TierR3,
		Summary:     "Delete a price list",
		Description: "DELETE /v3/pricelists/{price_list_id}. Destructive; preview first then confirmed=true.",
		Tool: mcp.NewTool("catalog_pricelists_delete",
			mcp.WithDescription("Delete a price list by ID. Preview first; confirmed=true to execute."),
			mcp.WithNumber("price_list_id", mcp.Description("Price list ID."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: p.handleDeletePriceList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/records/list",
		Tier:        middleware.TierR0,
		Summary:     "List records in one price list",
		Description: "GET /v3/pricelists/{price_list_id}/records with optional filters and pagination.",
		Tool: mcp.NewTool("catalog_pricelists_records_list",
			mcp.WithDescription("List price records for one price list."),
			mcp.WithNumber("price_list_id", mcp.Description("Price list ID."), mcp.Required()),
			mcp.WithArray("variant_ids", mcp.Description("Filter variant_id:in"), mcp.WithNumberItems()),
			mcp.WithArray("product_ids", mcp.Description("Filter product_id:in"), mcp.WithNumberItems()),
			mcp.WithString("sku", mcp.Description("Filter exact SKU.")),
			mcp.WithArray("skus", mcp.Description("Filter sku:in"), mcp.WithStringItems()),
			mcp.WithString("currency", mcp.Description("Filter exact currency code.")),
			mcp.WithArray("currencies", mcp.Description("Filter currency:in"), mcp.WithStringItems()),
			mcp.WithArray("include", mcp.Description("Include expansions: bulk_pricing_tiers, sku."), mcp.WithStringItems()),
			mcp.WithNumber("page", mcp.Description("Offset pagination page number.")),
			mcp.WithNumber("limit", mcp.Description("Offset pagination page size.")),
			mcp.WithString("before", mcp.Description("Cursor pagination: fetch rows before cursor.")),
			mcp.WithString("after", mcp.Description("Cursor pagination: fetch rows after cursor.")),
		),
		Handler: p.handleListPriceListRecords,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/records/upsert",
		Tier:        middleware.TierR2,
		Summary:     "Upsert records in one price list",
		Description: "PUT /v3/pricelists/{price_list_id}/records. High-risk pricing write; preview first then confirmed=true.",
		Tool: mcp.NewTool("catalog_pricelists_records_upsert",
			mcp.WithDescription("Upsert price list records for one list. Max 100 rows per call. Preview then confirmed=true."),
			mcp.WithNumber("price_list_id", mcp.Description("Price list ID."), mcp.Required()),
			mcp.WithArray("records",
				mcp.Description("Record rows [{variant_id|sku, currency, price|sale_price|retail_price|map_price, bulk_pricing_tiers?}]."),
				mcp.Items(map[string]any{"type": "object"}),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: p.handleUpsertPriceListRecords,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/records/delete",
		Tier:        middleware.TierR2,
		Summary:     "Delete records in one price list",
		Description: "DELETE /v3/pricelists/{price_list_id}/records with selectors. High-risk pricing write; preview first then confirmed=true.",
		Tool: mcp.NewTool("catalog_pricelists_records_delete",
			mcp.WithDescription("Delete selected price records from one list. Requires variant_ids or skus."),
			mcp.WithNumber("price_list_id", mcp.Description("Price list ID."), mcp.Required()),
			mcp.WithArray("variant_ids", mcp.Description("Delete selector variant_id:in."), mcp.WithNumberItems()),
			mcp.WithArray("skus", mcp.Description("Delete selector sku:in."), mcp.WithStringItems()),
			mcp.WithString("currency", mcp.Description("Optional currency selector.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: p.handleDeletePriceListRecords,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/assignments/list",
		Tier:        middleware.TierR0,
		Summary:     "List price list assignments",
		Description: "GET /v3/pricelists/assignments with optional filters and pagination.",
		Tool: mcp.NewTool("catalog_pricelists_assignments_list",
			mcp.WithDescription("List price list assignments by ID, price_list_id, customer_group_id, and channel filters."),
			mcp.WithNumber("id", mcp.Description("Exact assignment id.")),
			mcp.WithNumber("price_list_id", mcp.Description("Exact price list id.")),
			mcp.WithNumber("customer_group_id", mcp.Description("Exact customer group id.")),
			mcp.WithNumber("channel_id", mcp.Description("Exact channel id.")),
			mcp.WithArray("ids", mcp.Description("Filter id:in."), mcp.WithNumberItems()),
			mcp.WithArray("price_list_ids", mcp.Description("Filter price_list_id:in."), mcp.WithNumberItems()),
			mcp.WithArray("customer_group_ids", mcp.Description("Filter customer_group_id:in."), mcp.WithNumberItems()),
			mcp.WithArray("channel_ids", mcp.Description("Filter channel_id:in."), mcp.WithNumberItems()),
			mcp.WithNumber("page", mcp.Description("Offset pagination page number.")),
			mcp.WithNumber("limit", mcp.Description("Offset pagination page size.")),
			mcp.WithString("before", mcp.Description("Cursor pagination: fetch rows before cursor.")),
			mcp.WithString("after", mcp.Description("Cursor pagination: fetch rows after cursor.")),
		),
		Handler: p.handleListPriceListAssignments,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/assignments/create_batch",
		Tier:        middleware.TierR2,
		Summary:     "Create assignment batch",
		Description: "POST /v3/pricelists/assignments. High-risk pricing write; preview first then confirmed=true.",
		Tool: mcp.NewTool("catalog_pricelists_assignments_create_batch",
			mcp.WithDescription("Create assignment rows in batch (max 25). Preview then confirmed=true."),
			mcp.WithArray("assignments",
				mcp.Description("Rows [{price_list_id, customer_group_id?, channel_id?}] with at least one target field."),
				mcp.Items(map[string]any{"type": "object"}),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: p.handleCreatePriceListAssignments,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/assignments/upsert",
		Tier:        middleware.TierR2,
		Summary:     "Upsert one assignment",
		Description: "PUT /v3/pricelists/{price_list_id}/assignments. High-risk pricing write; preview first then confirmed=true.",
		Tool: mcp.NewTool("catalog_pricelists_assignments_upsert",
			mcp.WithDescription("Upsert one assignment for a price list + customer group + channel tuple."),
			mcp.WithNumber("price_list_id", mcp.Description("Price list id."), mcp.Required()),
			mcp.WithNumber("customer_group_id", mcp.Description("Customer group id."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("Channel id."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: p.handleUpsertPriceListAssignment,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/pricelists/assignments/delete",
		Tier:        middleware.TierR2,
		Summary:     "Delete assignments by filter",
		Description: "DELETE /v3/pricelists/assignments with one or more filter params. High-risk pricing write; preview first then confirmed=true.",
		Tool: mcp.NewTool("catalog_pricelists_assignments_delete",
			mcp.WithDescription("Delete assignment rows by id/price_list_id/customer_group_id/channel selectors."),
			mcp.WithNumber("id", mcp.Description("Exact assignment id.")),
			mcp.WithNumber("price_list_id", mcp.Description("Exact price list id.")),
			mcp.WithNumber("customer_group_id", mcp.Description("Exact customer group id.")),
			mcp.WithNumber("channel_id", mcp.Description("Exact channel id.")),
			mcp.WithArray("channel_ids", mcp.Description("Filter channel_id:in"), mcp.WithNumberItems()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: p.handleDeletePriceListAssignments,
	})
}

func (p *PriceLists) handleListPriceLists(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := bigcommerce.PriceListListParams{}
	var err error

	if params.ID, _, err = readOptionalPositiveInt(args, "id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.IDs, err = readOptionalPositiveIntArray(args, "ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Name, _, err = readOptionalTrimmedString(args, "name"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.NameLike, _, err = readOptionalTrimmedString(args, "name_like"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.DateCreated, _, err = readOptionalTrimmedString(args, "date_created"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.DateModified, _, err = readOptionalTrimmedString(args, "date_modified"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.DateCreatedMin, _, err = readOptionalTrimmedString(args, "date_created_min"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.DateCreatedMax, _, err = readOptionalTrimmedString(args, "date_created_max"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.DateModifiedMin, _, err = readOptionalTrimmedString(args, "date_modified_min"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.DateModifiedMax, _, err = readOptionalTrimmedString(args, "date_modified_max"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Page, _, err = readOptionalPositiveInt(args, "page"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Limit, _, err = readOptionalPositiveInt(args, "limit"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Before, _, err = readOptionalTrimmedString(args, "before"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.After, _, err = readOptionalTrimmedString(args, "after"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Page > 0 && (params.Before != "" || params.After != "") {
		return toolError("page cannot be combined with before/after cursor pagination"), nil
	}

	rows, err := p.bc.ListPriceLists(ctx, params)
	if err != nil {
		return toolError("list price lists: %v", err), nil
	}
	return toolJSON(map[string]any{
		"total":       len(rows),
		"price_lists": rows,
		"filters": map[string]any{
			"id":                params.ID,
			"ids":               params.IDs,
			"name":              params.Name,
			"name_like":         params.NameLike,
			"date_created":      params.DateCreated,
			"date_modified":     params.DateModified,
			"date_created_min":  params.DateCreatedMin,
			"date_created_max":  params.DateCreatedMax,
			"date_modified_min": params.DateModifiedMin,
			"date_modified_max": params.DateModifiedMax,
			"page":              params.Page,
			"limit":             params.Limit,
			"before":            params.Before,
			"after":             params.After,
		},
	})
}

func (p *PriceLists) handleGetPriceList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	priceListID, err := shared.ReadPositiveInt(request.GetArguments(), "price_list_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	row, err := p.bc.GetPriceList(ctx, priceListID)
	if err != nil {
		return toolError("get price list: %v", err), nil
	}
	return toolJSON(map[string]any{"price_list": row})
}

func (p *PriceLists) handleCreatePriceList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	nameRaw, ok := args["name"]
	if !ok {
		return toolError("name is required"), nil
	}
	name, ok := nameRaw.(string)
	if !ok || strings.TrimSpace(name) == "" {
		return toolError("name must be a non-empty string"), nil
	}
	name = strings.TrimSpace(name)
	active, hasActive, err := readOptionalBool(args, "active")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	preview := map[string]any{
		"status":       "pending_confirmation",
		"would_create": map[string]any{"name": name, "active": activeOrDefault(hasActive, active)},
		"message":      "Review the new price list payload. Pass confirmed=true to execute.",
	}
	if !middleware.IsConfirmed(request) {
		return toolJSON(preview)
	}

	payload := bigcommerce.PriceListCreate{Name: name}
	if hasActive {
		payload.Active = &active
	}
	created, err := p.bc.CreatePriceList(ctx, payload)
	if err != nil {
		return toolError("create price list: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":     "completed",
		"price_list": created,
	})
}

func (p *PriceLists) handleUpdatePriceList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	priceListID, err := shared.ReadPositiveInt(args, "price_list_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	current, err := p.bc.GetPriceList(ctx, priceListID)
	if err != nil {
		return toolError("get current price list: %v", err), nil
	}

	var payload bigcommerce.PriceListUpdate
	var wouldApply = map[string]any{
		"id":     current.ID,
		"name":   current.Name,
		"active": current.Active,
	}
	patchFields := 0

	if name, ok := args["name"]; ok {
		s, ok := name.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return toolError("name must be a non-empty string when provided"), nil
		}
		s = strings.TrimSpace(s)
		payload.Name = &s
		wouldApply["name"] = s
		patchFields++
	}
	if active, ok := args["active"]; ok {
		b, ok := active.(bool)
		if !ok {
			return toolError("active must be a boolean when provided"), nil
		}
		payload.Active = &b
		wouldApply["active"] = b
		patchFields++
	}
	if patchFields == 0 {
		return toolError("at least one of name or active is required for update"), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":      "pending_confirmation",
			"price_list":  current,
			"would_apply": wouldApply,
			"message":     "Review the update diff. Pass confirmed=true to execute.",
		})
	}

	updated, err := p.bc.UpdatePriceList(ctx, priceListID, payload)
	if err != nil {
		return toolError("update price list: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":     "completed",
		"price_list": updated,
	})
}

func (p *PriceLists) handleDeletePriceList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	priceListID, err := shared.ReadPositiveInt(args, "price_list_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	current, err := p.bc.GetPriceList(ctx, priceListID)
	if err != nil {
		return toolError("get current price list: %v", err), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"price_list": current,
			"message":    fmt.Sprintf("Will delete price list %d (%q). Pass confirmed=true to execute.", current.ID, current.Name),
		})
	}

	if err := p.bc.DeletePriceList(ctx, priceListID); err != nil {
		return toolError("delete price list: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":  "completed",
		"message": fmt.Sprintf("Deleted price list %d.", priceListID),
	})
}

func (p *PriceLists) handleListPriceListRecords(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	priceListID, err := shared.ReadPositiveInt(args, "price_list_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	params := bigcommerce.PriceListRecordListParams{}
	if params.VariantIDs, err = readOptionalPositiveIntArray(args, "variant_ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.ProductIDs, err = readOptionalPositiveIntArray(args, "product_ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.SKU, _, err = readOptionalTrimmedString(args, "sku"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.SKUs, err = readOptionalStringArray(args, "skus"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Currency, _, err = readOptionalTrimmedString(args, "currency"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Currencies, err = readOptionalStringArray(args, "currencies"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Include, err = readOptionalStringArray(args, "include"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Page, _, err = readOptionalPositiveInt(args, "page"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Limit, _, err = readOptionalPositiveInt(args, "limit"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Before, _, err = readOptionalTrimmedString(args, "before"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.After, _, err = readOptionalTrimmedString(args, "after"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Page > 0 && (params.Before != "" || params.After != "") {
		return toolError("page cannot be combined with before/after cursor pagination"), nil
	}

	rows, err := p.bc.ListPriceListRecords(ctx, priceListID, params)
	if err != nil {
		return toolError("list price list records: %v", err), nil
	}
	return toolJSON(map[string]any{
		"price_list_id": priceListID,
		"total":         len(rows),
		"records":       rows,
	})
}

func (p *PriceLists) handleUpsertPriceListRecords(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	priceListID, err := shared.ReadPositiveInt(args, "price_list_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	records, err := parsePriceListRecordRows(args["records"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(records) > maxPriceListRecordUpsertRows {
		return toolError("records: maximum %d rows per call", maxPriceListRecordUpsertRows), nil
	}

	if !middleware.IsConfirmed(request) {
		sample := make([]bigcommerce.PriceListRecordUpsert, 0, len(records))
		for i := 0; i < len(records) && i < priceListPreviewSampleRows; i++ {
			sample = append(sample, records[i])
		}
		return toolJSON(map[string]any{
			"status":        "pending_confirmation",
			"price_list_id": priceListID,
			"record_count":  len(records),
			"sample":        sample,
			"message":       "Will upsert price list records. Pass confirmed=true to execute.",
		})
	}

	if err := p.bc.UpsertPriceListRecords(ctx, priceListID, records); err != nil {
		return toolError("upsert price list records: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":        "completed",
		"price_list_id": priceListID,
		"record_count":  len(records),
		"message":       "Price list records upsert completed.",
	})
}

func (p *PriceLists) handleDeletePriceListRecords(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	priceListID, err := shared.ReadPositiveInt(args, "price_list_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	var params bigcommerce.PriceListRecordDeleteParams
	if params.VariantIDs, err = readOptionalPositiveIntArray(args, "variant_ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.SKUs, err = readOptionalStringArray(args, "skus"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Currency, _, err = readOptionalTrimmedString(args, "currency"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(params.VariantIDs) == 0 && len(params.SKUs) == 0 {
		return toolError("at least one of variant_ids or skus is required"), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":        "pending_confirmation",
			"price_list_id": priceListID,
			"selectors": map[string]any{
				"variant_ids": params.VariantIDs,
				"skus":        params.SKUs,
				"currency":    params.Currency,
			},
			"message": "Will delete selected price list records. Pass confirmed=true to execute.",
		})
	}

	if err := p.bc.DeletePriceListRecords(ctx, priceListID, params); err != nil {
		return toolError("delete price list records: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":        "completed",
		"price_list_id": priceListID,
		"message":       "Price list records delete completed.",
	})
}

func (p *PriceLists) handleListPriceListAssignments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := bigcommerce.PriceListAssignmentListParams{}
	var err error
	if params.ID, _, err = readOptionalPositiveInt(args, "id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.PriceListID, _, err = readOptionalPositiveInt(args, "price_list_id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.CustomerGroupID, _, err = readOptionalPositiveInt(args, "customer_group_id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.ChannelID, _, err = readOptionalPositiveInt(args, "channel_id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.IDs, err = readOptionalPositiveIntArray(args, "ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.PriceListIDs, err = readOptionalPositiveIntArray(args, "price_list_ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.CustomerGroupIDs, err = readOptionalPositiveIntArray(args, "customer_group_ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.ChannelIDs, err = readOptionalPositiveIntArray(args, "channel_ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Page, _, err = readOptionalPositiveInt(args, "page"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Limit, _, err = readOptionalPositiveInt(args, "limit"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Before, _, err = readOptionalTrimmedString(args, "before"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.After, _, err = readOptionalTrimmedString(args, "after"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.Page > 0 && (params.Before != "" || params.After != "") {
		return toolError("page cannot be combined with before/after cursor pagination"), nil
	}

	rows, err := p.bc.ListPriceListAssignments(ctx, params)
	if err != nil {
		return toolError("list price list assignments: %v", err), nil
	}
	return toolJSON(map[string]any{
		"total":       len(rows),
		"assignments": rows,
	})
}

func (p *PriceLists) handleCreatePriceListAssignments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	assignments, err := parsePriceListAssignmentCreateRows(args["assignments"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(assignments) > maxPriceListAssignmentBatch {
		return toolError("assignments: maximum %d rows per call", maxPriceListAssignmentBatch), nil
	}
	if !middleware.IsConfirmed(request) {
		sample := make([]bigcommerce.PriceListAssignmentCreate, 0, len(assignments))
		for i := 0; i < len(assignments) && i < priceListPreviewSampleRows; i++ {
			sample = append(sample, assignments[i])
		}
		return toolJSON(map[string]any{
			"status":           "pending_confirmation",
			"assignment_count": len(assignments),
			"sample":           sample,
			"message":          "Will create price list assignments. Pass confirmed=true to execute.",
		})
	}
	if err := p.bc.CreatePriceListAssignments(ctx, assignments); err != nil {
		return toolError("create price list assignments: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":           "completed",
		"assignment_count": len(assignments),
		"message":          "Price list assignment create batch completed.",
	})
}

func (p *PriceLists) handleUpsertPriceListAssignment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	priceListID, err := shared.ReadPositiveInt(args, "price_list_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	customerGroupID, err := shared.ReadPositiveInt(args, "customer_group_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	channelID, err := shared.ReadPositiveInt(args, "channel_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	payload := bigcommerce.PriceListAssignmentUpsert{
		CustomerGroupID: customerGroupID,
		ChannelID:       channelID,
	}
	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":        "pending_confirmation",
			"price_list_id": priceListID,
			"would_apply":   payload,
			"message":       "Will upsert the price list assignment. Pass confirmed=true to execute.",
		})
	}

	row, err := p.bc.UpsertPriceListAssignment(ctx, priceListID, payload)
	if err != nil {
		return toolError("upsert price list assignment: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":     "completed",
		"assignment": row,
	})
}

func (p *PriceLists) handleDeletePriceListAssignments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := bigcommerce.PriceListAssignmentDeleteParams{}
	var err error
	if params.ID, _, err = readOptionalPositiveInt(args, "id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.PriceListID, _, err = readOptionalPositiveInt(args, "price_list_id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.CustomerGroupID, _, err = readOptionalPositiveInt(args, "customer_group_id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.ChannelID, _, err = readOptionalPositiveInt(args, "channel_id"); err != nil {
		return toolError("%s", err.Error()), nil
	}
	if params.ChannelIDs, err = readOptionalPositiveIntArray(args, "channel_ids"); err != nil {
		return toolError("%s", err.Error()), nil
	}

	if params.ID == 0 && params.PriceListID == 0 && params.CustomerGroupID == 0 &&
		params.ChannelID == 0 && len(params.ChannelIDs) == 0 {
		return toolError("at least one filter is required: id, price_list_id, customer_group_id, channel_id, or channel_ids"), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status": "pending_confirmation",
			"filters": map[string]any{
				"id":                params.ID,
				"price_list_id":     params.PriceListID,
				"customer_group_id": params.CustomerGroupID,
				"channel_id":        params.ChannelID,
				"channel_ids":       params.ChannelIDs,
			},
			"message": "Will delete matching price list assignments. Pass confirmed=true to execute.",
		})
	}
	if err := p.bc.DeletePriceListAssignments(ctx, params); err != nil {
		return toolError("delete price list assignments: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":  "completed",
		"message": "Price list assignment delete request completed.",
	})
}

func parsePriceListRecordRows(v any) ([]bigcommerce.PriceListRecordUpsert, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("records must be an array of objects")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("records must contain at least one row")
	}
	out := make([]bigcommerce.PriceListRecordUpsert, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("records[%d] must be an object", i)
		}
		row, err := parsePriceListRecordRow(obj, i)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func parsePriceListRecordRow(obj map[string]any, idx int) (bigcommerce.PriceListRecordUpsert, error) {
	var row bigcommerce.PriceListRecordUpsert
	var hasVariant bool

	if raw, ok := obj["variant_id"]; ok {
		id, err := numericPositiveInt(raw, fmt.Sprintf("records[%d].variant_id", idx))
		if err != nil {
			return row, err
		}
		row.VariantID = id
		hasVariant = true
	}
	if raw, ok := obj["sku"]; ok {
		s, ok := raw.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return row, fmt.Errorf("records[%d].sku must be a non-empty string", idx)
		}
		row.SKU = strings.TrimSpace(s)
	}
	if !hasVariant && row.SKU == "" {
		return row, fmt.Errorf("records[%d] must include variant_id or sku", idx)
	}

	curRaw, ok := obj["currency"]
	if !ok {
		return row, fmt.Errorf("records[%d].currency is required", idx)
	}
	cur, ok := curRaw.(string)
	if !ok || strings.TrimSpace(cur) == "" {
		return row, fmt.Errorf("records[%d].currency must be a non-empty string", idx)
	}
	row.Currency = strings.ToLower(strings.TrimSpace(cur))

	if n, ok, err := optionalNumber(obj, "price"); err != nil {
		return row, fmt.Errorf("records[%d].price %v", idx, err)
	} else if ok {
		row.Price = &n
	}
	if n, ok, err := optionalNumber(obj, "sale_price"); err != nil {
		return row, fmt.Errorf("records[%d].sale_price %v", idx, err)
	} else if ok {
		row.SalePrice = &n
	}
	if n, ok, err := optionalNumber(obj, "retail_price"); err != nil {
		return row, fmt.Errorf("records[%d].retail_price %v", idx, err)
	} else if ok {
		row.RetailPrice = &n
	}
	if n, ok, err := optionalNumber(obj, "map_price"); err != nil {
		return row, fmt.Errorf("records[%d].map_price %v", idx, err)
	} else if ok {
		row.MapPrice = &n
	}
	if tiers, ok, err := optionalBulkPricingTiers(obj, fmt.Sprintf("records[%d].bulk_pricing_tiers", idx)); err != nil {
		return row, err
	} else if ok {
		row.BulkPricingTiers = tiers
	}

	if row.Price == nil && row.SalePrice == nil && row.RetailPrice == nil &&
		row.MapPrice == nil && len(row.BulkPricingTiers) == 0 {
		return row, fmt.Errorf("records[%d] must include at least one pricing field", idx)
	}
	return row, nil
}

func optionalBulkPricingTiers(obj map[string]any, path string) ([]bigcommerce.PriceListBulkPricingTier, bool, error) {
	raw, ok := obj["bulk_pricing_tiers"]
	if !ok || raw == nil {
		return nil, false, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s must be an array", path)
	}
	out := make([]bigcommerce.PriceListBulkPricingTier, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("%s[%d] must be an object", path, i)
		}
		minRaw, ok := m["quantity_min"]
		if !ok {
			return nil, false, fmt.Errorf("%s[%d].quantity_min is required", path, i)
		}
		min, err := numericPositiveInt(minRaw, fmt.Sprintf("%s[%d].quantity_min", path, i))
		if err != nil {
			return nil, false, err
		}

		var maxPtr *int
		if maxRaw, ok := m["quantity_max"]; ok && maxRaw != nil {
			max, err := numericPositiveInt(maxRaw, fmt.Sprintf("%s[%d].quantity_max", path, i))
			if err != nil {
				return nil, false, err
			}
			maxPtr = &max
		}

		typRaw, ok := m["type"]
		if !ok {
			return nil, false, fmt.Errorf("%s[%d].type is required", path, i)
		}
		typ, ok := typRaw.(string)
		if !ok {
			return nil, false, fmt.Errorf("%s[%d].type must be a string", path, i)
		}
		typ = strings.ToLower(strings.TrimSpace(typ))
		switch typ {
		case "fixed", "price", "percent":
		default:
			return nil, false, fmt.Errorf("%s[%d].type must be one of fixed, price, percent", path, i)
		}
		amountRaw, ok := m["amount"]
		if !ok {
			return nil, false, fmt.Errorf("%s[%d].amount is required", path, i)
		}
		amount, ok := amountRaw.(float64)
		if !ok {
			return nil, false, fmt.Errorf("%s[%d].amount must be a number", path, i)
		}
		out = append(out, bigcommerce.PriceListBulkPricingTier{
			QuantityMin: min,
			QuantityMax: maxPtr,
			Type:        typ,
			Amount:      amount,
		})
	}
	return out, true, nil
}

func parsePriceListAssignmentCreateRows(v any) ([]bigcommerce.PriceListAssignmentCreate, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("assignments must be an array of objects")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("assignments must contain at least one row")
	}
	out := make([]bigcommerce.PriceListAssignmentCreate, 0, len(raw))
	for i, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("assignments[%d] must be an object", i)
		}
		priceListIDRaw, ok := obj["price_list_id"]
		if !ok {
			return nil, fmt.Errorf("assignments[%d].price_list_id is required", i)
		}
		priceListID, err := numericPositiveInt(priceListIDRaw, fmt.Sprintf("assignments[%d].price_list_id", i))
		if err != nil {
			return nil, err
		}

		row := bigcommerce.PriceListAssignmentCreate{PriceListID: priceListID}
		if rawCG, ok := obj["customer_group_id"]; ok && rawCG != nil {
			v, err := numericPositiveInt(rawCG, fmt.Sprintf("assignments[%d].customer_group_id", i))
			if err != nil {
				return nil, err
			}
			row.CustomerGroupID = v
		}
		if rawCh, ok := obj["channel_id"]; ok && rawCh != nil {
			v, err := numericPositiveInt(rawCh, fmt.Sprintf("assignments[%d].channel_id", i))
			if err != nil {
				return nil, err
			}
			row.ChannelID = v
		}
		if row.CustomerGroupID == 0 && row.ChannelID == 0 {
			return nil, fmt.Errorf("assignments[%d] must include customer_group_id or channel_id", i)
		}
		out = append(out, row)
	}
	return out, nil
}

func readOptionalPositiveInt(args map[string]any, key string) (int, bool, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	n, err := numericPositiveInt(raw, key)
	if err != nil {
		return 0, false, err
	}
	return n, true, nil
}

func readOptionalPositiveIntArray(args map[string]any, key string) ([]int, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of numbers", key)
	}
	out := make([]int, 0, len(arr))
	for i, item := range arr {
		n, err := numericPositiveInt(item, fmt.Sprintf("%s[%d]", key, i))
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func readOptionalStringArray(args map[string]any, key string) ([]string, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
	out := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string", key, i)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, fmt.Errorf("%s[%d] must be a non-empty string", key, i)
		}
		out = append(out, s)
	}
	return out, nil
}

func readOptionalTrimmedString(args map[string]any, key string) (string, bool, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return "", false, nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be a string", key)
	}
	return strings.TrimSpace(s), true, nil
}

func readOptionalBool(args map[string]any, key string) (bool, bool, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return false, false, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return false, false, fmt.Errorf("%s must be a boolean", key)
	}
	return b, true, nil
}

func numericPositiveInt(raw any, path string) (int, error) {
	f, ok := raw.(float64)
	if !ok {
		return 0, fmt.Errorf("%s must be a number", path)
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("%s must be an integer", path)
	}
	if f <= 0 {
		return 0, fmt.Errorf("%s must be positive", path)
	}
	return int(f), nil
}

func optionalNumber(obj map[string]any, key string) (float64, bool, error) {
	raw, ok := obj[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	n, ok := raw.(float64)
	if !ok {
		return 0, false, fmt.Errorf("must be a number")
	}
	return n, true, nil
}

func activeOrDefault(hasActive bool, active bool) bool {
	if hasActive {
		return active
	}
	return true
}
