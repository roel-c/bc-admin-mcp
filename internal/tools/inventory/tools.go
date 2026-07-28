package inventory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

const maxInventoryListLimit = 250
const maxInventoryAdjustmentItems = 10
const defaultInventoryLocationMetafieldPermissionSet = "app_only"

// Tools holds handlers for inventory/* MCP tools.
type Tools struct {
	bc BigCommerceInventoryAPI
}

// New constructs inventory tool handlers.
func New(bc BigCommerceInventoryAPI) *Tools {
	return &Tools{bc: bc}
}

// RegisterTools wires inventory tools into discovery.
func (t *Tools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "inventory/locations/list",
		Tier:        middleware.TierR0,
		Summary:     "List inventory locations (V3)",
		Description: "GET /v3/inventory/locations with optional page/limit.",
		Tool: mcp.NewTool("inventory_locations_list",
			mcp.WithDescription("List inventory locations."),
			mcp.WithNumber("page", mcp.Description("Optional page number (single-page mode).")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (single-page mode, max 250).")),
		),
		Handler: t.handleListLocations,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/locations/create",
		Tier:    middleware.TierR2,
		Summary: "Create inventory location (V3)",
		Description: "POST /v3/inventory/locations with caller-supplied location payload. " +
			"High-risk operational write; preview required.",
		Tool: mcp.NewTool("inventory_locations_create",
			mcp.WithDescription("Create one inventory location. Preview first; confirmed=true to execute."),
			mcp.WithObject("location", mcp.Description("Inventory location payload object."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleCreateLocation,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/locations/update",
		Tier:    middleware.TierR2,
		Summary: "Update inventory location (V3)",
		Description: "PUT /v3/inventory/locations/{location_id} with caller-supplied patch payload. " +
			"High-risk operational write; preview required.",
		Tool: mcp.NewTool("inventory_locations_update",
			mcp.WithDescription("Update one inventory location by id. Preview first; confirmed=true to execute."),
			mcp.WithNumber("location_id", mcp.Description("Inventory location id."), mcp.Required()),
			mcp.WithObject("patch", mcp.Description("Inventory location patch payload object."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleUpdateLocation,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "inventory/locations/delete",
		Tier:        middleware.TierR3,
		Summary:     "Delete inventory location (V3)",
		Description: "DELETE /v3/inventory/locations/{location_id}. Destructive operation; preview required.",
		Tool: mcp.NewTool("inventory_locations_delete",
			mcp.WithDescription("Delete one inventory location by id. Preview first; confirmed=true to execute."),
			mcp.WithNumber("location_id", mcp.Description("Inventory location id."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleDeleteLocation,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "inventory/locations/metafields/list",
		Tier:        middleware.TierR0,
		Summary:     "List metafields on one inventory location (V3)",
		Description: "GET /v3/inventory/locations/{location_id}/metafields with optional page/limit.",
		Tool: mcp.NewTool("inventory_locations_metafields_list",
			mcp.WithDescription("List metafields on one inventory location."),
			mcp.WithNumber("location_id", mcp.Description("Inventory location id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: t.handleListLocationMetafields,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/locations/metafields/set",
		Tier:    middleware.TierR1,
		Summary: "Create or update one inventory location metafield (upsert by namespace+key)",
		Description: "POST or PUT /v3/inventory/locations/{location_id}/metafields. " +
			"Defaults new metafields to app_only when permission_set is omitted. " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("inventory_locations_metafields_set",
			mcp.WithDescription("Upsert one inventory location metafield by namespace+key."),
			mcp.WithNumber("location_id", mcp.Description("Inventory location id."), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("Metafield namespace."), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key."), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value (string)."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description.")),
			mcp.WithString("permission_set", mcp.Description("Optional permission_set; defaults to app_only for new metafields.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleSetLocationMetafield,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "inventory/locations/metafields/delete",
		Tier:        middleware.TierR1,
		Summary:     "Delete one inventory location metafield (V3)",
		Description: "DELETE /v3/inventory/locations/{location_id}/metafields/{metafield_id}. Delete by metafield_id or namespace+key. Preview then confirmed=true.",
		Tool: mcp.NewTool("inventory_locations_metafields_delete",
			mcp.WithDescription("Delete one inventory location metafield by metafield_id or namespace+key."),
			mcp.WithNumber("location_id", mcp.Description("Inventory location id."), mcp.Required()),
			mcp.WithNumber("metafield_id", mcp.Description("Metafield id (mutually exclusive with namespace+key).")),
			mcp.WithString("namespace", mcp.Description("Namespace (use with key).")),
			mcp.WithString("key", mcp.Description("Key (use with namespace).")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleDeleteLocationMetafield,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/locations/items/list",
		Tier:    middleware.TierR0,
		Summary: "List inventory items at one location (V3)",
		Description: "GET /v3/inventory/locations/{location_id}/items. " +
			"Returns per-item qty_backordered, available_to_sell, and settings.backorder_limit for that location.",
		Tool: mcp.NewTool("inventory_locations_items_list",
			mcp.WithDescription("List inventory + backorder fields for items at one location."),
			mcp.WithNumber("location_id", mcp.Description("Inventory location id."), mcp.Required()),
			mcp.WithArray("product_ids", mcp.Description("Filter product_id:in."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithArray("variant_ids", mcp.Description("Filter variant_id:in."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithArray("skus", mcp.Description("Filter sku:in."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithNumber("page", mcp.Description("Optional page number (single-page mode).")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (single-page mode, max 250).")),
		),
		Handler: t.handleListLocationItems,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/locations/items/update",
		Tier:    middleware.TierR2,
		Summary: "Update inventory settings at one location (V3)",
		Description: "PUT /v3/inventory/locations/{location_id}/items with a settings[] payload. " +
			"Use this to set backorder_limit (and other location settings such as safety_stock). " +
			"Not the same as inventory/items/update_batch (PUT /v3/inventory/items). " +
			"High-risk inventory write; preview required. Max 10 settings rows per call.",
		Tool: mcp.NewTool("inventory_locations_items_update",
			mcp.WithDescription("Update location inventory settings including backorder_limit. Preview first; confirmed=true to execute."),
			mcp.WithNumber("location_id", mcp.Description("Inventory location id."), mcp.Required()),
			mcp.WithArray("settings",
				mcp.Description("Array of { identity: {variant_id|product_id|sku exactly one}, backorder_limit?, safety_stock?, is_in_stock?, warning_level?, bin_picking_number? } (max 10)."),
				mcp.Items(map[string]any{"type": "object"}),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleUpdateLocationItems,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/items/list",
		Tier:    middleware.TierR0,
		Summary: "List inventory items with optional filters (V3)",
		Description: "GET /v3/inventory/items with optional filters. " +
			"Each location row includes qty_backordered, available_to_sell, and settings.backorder_limit. " +
			"Provide at least one filter or set list_all=true.",
		Tool: mcp.NewTool("inventory_items_list",
			mcp.WithDescription("List inventory items by variant/product/location/SKU filters (includes backorder fields)."),
			mcp.WithBoolean("list_all", mcp.Description("When true, allows listing without explicit filters.")),
			mcp.WithArray("location_ids", mcp.Description("Filter location_id:in."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithArray("product_ids", mcp.Description("Filter product_id:in."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithArray("variant_ids", mcp.Description("Filter variant_id:in."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithArray("skus", mcp.Description("Filter sku:in."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithNumber("page", mcp.Description("Optional page number (single-page mode).")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (single-page mode, max 250).")),
		),
		Handler: t.handleListItems,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/items/get",
		Tier:    middleware.TierR0,
		Summary: "Get one variant inventory record (V3)",
		Description: "GET /v3/inventory/items filtered by variant_id (no single-item path). " +
			"Response locations[] include qty_backordered, available_to_sell, and settings.backorder_limit. " +
			"Not the same as catalog availability=preorder.",
		Tool: mcp.NewTool("inventory_items_get",
			mcp.WithDescription("Fetch one inventory record by variant id (includes per-location backorder fields)."),
			mcp.WithNumber("variant_id", mcp.Description("Variant id."), mcp.Required()),
		),
		Handler: t.handleGetItem,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/items/update_batch",
		Tier:    middleware.TierR2,
		Summary: "Batch update inventory items settings (V3)",
		Description: "PUT /v3/inventory/items with a caller-supplied payload object. " +
			"High-risk inventory write; preview required. Supports either payload.items[] or payload.data[] with max 10 rows. " +
			"For backorder_limit use inventory/locations/items/update instead.",
		Tool: mcp.NewTool("inventory_items_update_batch",
			mcp.WithDescription("Submit a batch inventory items update payload. Preview first; confirmed=true to execute. Use locations/items/update for backorder_limit."),
			mcp.WithObject("update", mcp.Description("Inventory items update payload object."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleUpdateItemsBatch,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/adjustments/absolute",
		Tier:    middleware.TierR2,
		Summary: "Submit absolute inventory adjustment batch (V3)",
		Description: "PUT /v3/inventory/adjustments/absolute. High-risk inventory write; " +
			"preview required and max 10 items per call. Optional qty_backordered sets absolute backordered quantity. " +
			"Each row identifies the item with exactly one of variant_id, product_id, or sku.",
		Tool: mcp.NewTool("inventory_adjustments_absolute",
			mcp.WithDescription("Set absolute on-hand quantity and/or qty_backordered for up to 10 location+item rows."),
			mcp.WithString("reason", mcp.Description("Adjustment reason/audit note."), mcp.Required()),
			mcp.WithArray("items",
				mcp.Description("Array of { location_id, variant_id|product_id|sku (exactly one), quantity?, qty_backordered? } (max 10). At least one of quantity or qty_backordered required; both must be non-negative when present."),
				mcp.Items(map[string]any{"type": "object"}),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleAbsoluteAdjustment,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "inventory/adjustments/relative",
		Tier:    middleware.TierR2,
		Summary: "Submit relative inventory adjustment batch (V3)",
		Description: "POST /v3/inventory/adjustments/relative. High-risk inventory write; " +
			"preview required and max 10 items per call. Optional qty_backordered applies a delta to backordered quantity. " +
			"Each row identifies the item with exactly one of variant_id, product_id, or sku.",
		Tool: mcp.NewTool("inventory_adjustments_relative",
			mcp.WithDescription("Adjust on-hand quantity and/or qty_backordered by deltas for up to 10 location+item rows."),
			mcp.WithString("reason", mcp.Description("Adjustment reason/audit note."), mcp.Required()),
			mcp.WithArray("items",
				mcp.Description("Array of { location_id, variant_id|product_id|sku (exactly one), quantity?, qty_backordered? } (max 10). At least one of quantity or qty_backordered must be non-zero."),
				mcp.Items(map[string]any{"type": "object"}),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: t.handleRelativeAdjustment,
	})
}

func (t *Tools) handleListLocations(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := bigcommerce.InventoryLocationListParams{}
	if page, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.Page = page
	}
	if limit, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		if limit > maxInventoryListLimit {
			return shared.ToolError("limit must be <= %d", maxInventoryListLimit), nil
		}
		params.Limit = limit
	}
	rows, err := t.bc.ListInventoryLocations(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list inventory locations: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"total":     len(rows),
		"locations": rows,
	})
}

func (t *Tools) handleCreateLocation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	locationPayload, err := requiredObjectPayload(args, "location")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "inventory_locations_create",
			"location":    locationPayload,
			"message":     "High-risk operational write. Review payload and pass confirmed=true to execute.",
			"safety_note": "Creating locations changes inventory topology and may affect channel/location assignments.",
		})
	}
	// BigCommerce's POST /v3/inventory/locations is a BATCH endpoint — it
	// expects a JSON array of location objects, not a single object (a bare
	// object returns 422 "error.expected.jsarray"). Wrap the single location.
	raw, err := json.Marshal([]map[string]any{locationPayload})
	if err != nil {
		return shared.ToolError("failed to marshal location payload: %v", err), nil
	}
	resp, err := t.bc.CreateInventoryLocation(ctx, raw)
	if err != nil {
		return shared.ToolError("failed to create inventory location: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "created",
		"location": resp,
	})
}

func (t *Tools) handleUpdateLocation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	locationID, err := shared.ReadPositiveInt(args, "location_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	patch, err := requiredObjectPayload(args, "patch")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "inventory_locations_update",
			"location_id": locationID,
			"patch":       patch,
			"message":     "High-risk operational write. Review payload and pass confirmed=true to execute.",
		})
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return shared.ToolError("failed to marshal location patch payload: %v", err), nil
	}
	resp, err := t.bc.UpdateInventoryLocation(ctx, locationID, raw)
	if err != nil {
		return shared.ToolError("failed to update inventory location %d: %v", locationID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":      "updated",
		"location_id": locationID,
		"location":    resp,
	})
}

func (t *Tools) handleDeleteLocation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	locationID, err := shared.ReadPositiveInt(args, "location_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "inventory_locations_delete",
			"location_id": locationID,
			"destructive": true,
			"message":     "Deleting an inventory location is destructive. Pass confirmed=true to execute.",
		})
	}
	if err := t.bc.DeleteInventoryLocation(ctx, locationID); err != nil {
		return shared.ToolError("failed to delete inventory location %d: %v", locationID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":      "deleted",
		"location_id": locationID,
	})
}

func (t *Tools) handleListLocationMetafields(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	locationID, err := shared.ReadPositiveInt(args, "location_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.InventoryLocationMetafieldListParams{}
	if page, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.Page = page
	}
	if limit, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		if limit > maxInventoryListLimit {
			return shared.ToolError("limit must be <= %d", maxInventoryListLimit), nil
		}
		params.Limit = limit
	}
	rows, err := t.bc.ListInventoryLocationMetafields(ctx, locationID, params)
	if err != nil {
		return shared.ToolError("failed to list inventory location metafields: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"location_id": locationID,
		"total":       len(rows),
		"metafields":  rows,
	})
}

func (t *Tools) handleSetLocationMetafield(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	locationID, err := shared.ReadPositiveInt(args, "location_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	ns, key, value, desc, permSet, err := parseInventoryLocationMetafieldSetFields(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	existing, err := t.bc.ListInventoryLocationMetafields(ctx, locationID, bigcommerce.InventoryLocationMetafieldListParams{})
	if err != nil {
		return shared.ToolError("failed to list existing location metafields: %v", err), nil
	}
	var existingMF *bigcommerce.Metafield
	for i := range existing {
		if existing[i].Namespace == ns && existing[i].Key == key {
			existingMF = &existing[i]
			break
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		action := "create"
		preview := map[string]any{
			"status":      "pending_confirmation",
			"location_id": locationID,
			"namespace":   ns,
			"key":         key,
			"value":       value,
		}
		var effectivePerm string
		if existingMF != nil {
			action = "update"
			preview["metafield_id"] = existingMF.ID
			preview["existing_value"] = existingMF.Value
			preview["existing_permission_set"] = existingMF.PermissionSet
			if permSet != "" {
				effectivePerm = permSet
			} else {
				effectivePerm = existingMF.PermissionSet
			}
		} else if permSet != "" {
			effectivePerm = permSet
		} else {
			effectivePerm = defaultInventoryLocationMetafieldPermissionSet
		}
		preview["action"] = action
		preview["permission_set"] = effectivePerm
		preview["permission_note"] = shared.AppOnlyMetafieldPermissionNote
		if desc != "" {
			preview["description"] = desc
		}
		preview["message"] = fmt.Sprintf(
			"Will %s metafield %s.%s on inventory location %d. Pass confirmed=true to execute.",
			action, ns, key, locationID,
		)
		return shared.ToolJSON(preview)
	}

	payload := bigcommerce.Metafield{
		Namespace:     ns,
		Key:           key,
		Value:         value,
		Description:   desc,
		PermissionSet: permSet,
	}
	if existingMF != nil {
		if payload.PermissionSet == "" {
			payload.PermissionSet = existingMF.PermissionSet
		}
		updated, uerr := t.bc.UpdateInventoryLocationMetafield(ctx, locationID, existingMF.ID, payload)
		if uerr != nil {
			return shared.ToolError("update failed: %v", uerr), nil
		}
		return shared.ToolJSON(map[string]any{
			"status":    "updated",
			"metafield": updated,
			"message":   fmt.Sprintf("Metafield %s.%s updated on inventory location %d.", ns, key, locationID),
		})
	}

	if payload.PermissionSet == "" {
		payload.PermissionSet = defaultInventoryLocationMetafieldPermissionSet
	}
	created, cerr := t.bc.CreateInventoryLocationMetafield(ctx, locationID, payload)
	if cerr != nil {
		return shared.ToolError("create failed: %v", cerr), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":    "created",
		"metafield": created,
		"message":   fmt.Sprintf("Metafield %s.%s created on inventory location %d.", ns, key, locationID),
	})
}

func (t *Tools) handleDeleteLocationMetafield(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	locationID, err := shared.ReadPositiveInt(args, "location_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	mfID, ns, key, err := parseInventoryLocationMetafieldDeleteSelector(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if mfID == 0 {
		existing, lerr := t.bc.ListInventoryLocationMetafields(ctx, locationID, bigcommerce.InventoryLocationMetafieldListParams{})
		if lerr != nil {
			return shared.ToolError("failed to list metafields: %v", lerr), nil
		}
		for _, mf := range existing {
			if mf.Namespace == ns && mf.Key == key {
				mfID = mf.ID
				break
			}
		}
		if mfID == 0 {
			return shared.ToolError("no metafield found with namespace %q key %q on inventory location %d", ns, key, locationID), nil
		}
	}
	if !middleware.IsConfirmedFromArgs(args) {
		preview := map[string]any{
			"status":       "pending_confirmation",
			"location_id":  locationID,
			"metafield_id": mfID,
			"message":      fmt.Sprintf("Will delete metafield %d from inventory location %d. Pass confirmed=true to execute.", mfID, locationID),
		}
		if ns != "" {
			preview["namespace"] = ns
			preview["key"] = key
		}
		return shared.ToolJSON(preview)
	}
	if err := t.bc.DeleteInventoryLocationMetafield(ctx, locationID, mfID); err != nil {
		return shared.ToolError("delete failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":  "deleted",
		"message": fmt.Sprintf("Metafield %d deleted from inventory location %d.", mfID, locationID),
	})
}

func (t *Tools) handleListLocationItems(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	locationID, err := shared.ReadPositiveInt(args, "location_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params, err := parseInventoryLocationItemListParams(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	rows, err := t.bc.ListInventoryLocationItems(ctx, locationID, params)
	if err != nil {
		return shared.ToolError("failed to list inventory location items: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"location_id": locationID,
		"total":       len(rows),
		"items":       rows,
		"filters": map[string]any{
			"product_ids": params.ProductIDs,
			"variant_ids": params.VariantIDs,
			"skus":        params.SKUs,
			"page":        params.Page,
			"limit":       params.Limit,
		},
	})
}

func (t *Tools) handleUpdateLocationItems(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	locationID, err := shared.ReadPositiveInt(args, "location_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	settings, err := parseLocationItemSettings(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "inventory_locations_items_update",
			"location_id": locationID,
			"row_count":   len(settings),
			"settings":    settings,
			"message":     "High-risk inventory write. Review settings (e.g. backorder_limit) and pass confirmed=true to execute.",
			"max_rows":    maxInventoryAdjustmentItems,
		})
	}
	raw, err := json.Marshal(map[string]any{"settings": settings})
	if err != nil {
		return shared.ToolError("failed to marshal location items update payload: %v", err), nil
	}
	resp, err := t.bc.UpdateInventoryLocationItems(ctx, locationID, raw)
	if err != nil {
		return shared.ToolError("failed to update inventory location items: %v", err), nil
	}
	out := map[string]any{
		"status":      "submitted",
		"location_id": locationID,
		"row_count":   len(settings),
		"response":    resp,
	}
	if txID := extractTransactionID(resp); txID != "" {
		out["transaction_id"] = txID
	}
	return shared.ToolJSON(out)
}

func (t *Tools) handleListItems(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, hasFilter, err := parseInventoryItemListParams(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !shared.ReadBool(args, "list_all") && !hasFilter {
		return shared.ToolError("provide at least one filter or set list_all=true"), nil
	}
	rows, err := t.bc.ListInventoryItems(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list inventory items: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"total": len(rows),
		"items": rows,
		"filters": map[string]any{
			"location_ids": params.LocationIDs,
			"product_ids":  params.ProductIDs,
			"variant_ids":  params.VariantIDs,
			"skus":         params.SKUs,
			"page":         params.Page,
			"limit":        params.Limit,
		},
	})
}

func (t *Tools) handleGetItem(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	variantID, err := shared.ReadPositiveInt(args, "variant_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	row, err := t.bc.GetInventoryItem(ctx, variantID)
	if err != nil {
		return shared.ToolError("failed to get inventory item %d: %v", variantID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"variant_id": variantID,
		"item":       row,
	})
}

func (t *Tools) handleUpdateItemsBatch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	updatePayload, err := requiredObjectPayload(args, "update")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	rowCount, err := inventoryPayloadRowCount(updatePayload)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if rowCount > maxInventoryAdjustmentItems {
		return shared.ToolError("update payload exceeds maximum of %d rows", maxInventoryAdjustmentItems), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "inventory_items_update_batch",
			"row_count":   rowCount,
			"update":      updatePayload,
			"message":     "High-risk inventory write. Review payload and pass confirmed=true to execute.",
			"max_rows":    maxInventoryAdjustmentItems,
			"safety_note": "Prefer grouping by location for large batches and avoid parallel bulk writes with Catalog/Orders APIs.",
		})
	}
	raw, err := json.Marshal(updatePayload)
	if err != nil {
		return shared.ToolError("failed to marshal update payload: %v", err), nil
	}
	resp, err := t.bc.UpdateInventoryItems(ctx, raw)
	if err != nil {
		return shared.ToolError("failed to update inventory items: %v", err), nil
	}
	out := map[string]any{
		"status":    "submitted",
		"row_count": rowCount,
		"response":  resp,
	}
	if txID := extractTransactionID(resp); txID != "" {
		out["transaction_id"] = txID
	}
	return shared.ToolJSON(out)
}

func (t *Tools) handleAbsoluteAdjustment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	payload, err := parseAdjustmentPayload(args, false)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":          "preview",
			"action":          "inventory_absolute_adjustment",
			"adjustment_type": "absolute",
			"item_count":      len(payload.Items),
			"payload": map[string]any{
				"reason": payload.Reason,
				"items":  payload.Items,
			},
			"message": "High-risk inventory write. Review rows and pass confirmed=true to execute.",
		})
	}
	raw, err := json.Marshal(map[string]any{
		"reason": payload.Reason,
		"items":  payload.Items,
	})
	if err != nil {
		return shared.ToolError("failed to marshal absolute adjustment payload: %v", err), nil
	}
	resp, err := t.bc.CreateInventoryAbsoluteAdjustment(ctx, raw)
	if err != nil {
		return shared.ToolError("failed to submit absolute adjustment: %v", err), nil
	}
	out := map[string]any{
		"status":          "submitted",
		"adjustment_type": "absolute",
		"item_count":      len(payload.Items),
		"response":        resp,
	}
	if txID := extractTransactionID(resp); txID != "" {
		out["transaction_id"] = txID
	}
	return shared.ToolJSON(out)
}

func (t *Tools) handleRelativeAdjustment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	payload, err := parseAdjustmentPayload(args, true)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":          "preview",
			"action":          "inventory_relative_adjustment",
			"adjustment_type": "relative",
			"item_count":      len(payload.Items),
			"payload": map[string]any{
				"reason": payload.Reason,
				"items":  payload.Items,
			},
			"message": "High-risk inventory write. Review rows and pass confirmed=true to execute.",
		})
	}
	raw, err := json.Marshal(map[string]any{
		"reason": payload.Reason,
		"items":  payload.Items,
	})
	if err != nil {
		return shared.ToolError("failed to marshal relative adjustment payload: %v", err), nil
	}
	resp, err := t.bc.CreateInventoryRelativeAdjustment(ctx, raw)
	if err != nil {
		return shared.ToolError("failed to submit relative adjustment: %v", err), nil
	}
	out := map[string]any{
		"status":          "submitted",
		"adjustment_type": "relative",
		"item_count":      len(payload.Items),
		"response":        resp,
	}
	if txID := extractTransactionID(resp); txID != "" {
		out["transaction_id"] = txID
	}
	return shared.ToolJSON(out)
}

type adjustmentPayload struct {
	Reason string
	Items  []map[string]any
}

func parseAdjustmentPayload(args map[string]any, relative bool) (adjustmentPayload, error) {
	out := adjustmentPayload{}
	reasonRaw, ok := args["reason"]
	if !ok {
		return out, fmt.Errorf("reason is required")
	}
	reason, ok := reasonRaw.(string)
	if !ok || strings.TrimSpace(reason) == "" {
		return out, fmt.Errorf("reason must be a non-empty string")
	}
	out.Reason = strings.TrimSpace(reason)

	itemsRaw, ok := args["items"]
	if !ok || itemsRaw == nil {
		return out, fmt.Errorf("items is required")
	}
	itemsArr, ok := itemsRaw.([]any)
	if !ok {
		return out, fmt.Errorf("items must be an array of objects")
	}
	if len(itemsArr) == 0 {
		return out, fmt.Errorf("items must include at least one row")
	}
	if len(itemsArr) > maxInventoryAdjustmentItems {
		return out, fmt.Errorf("items exceeds maximum of %d", maxInventoryAdjustmentItems)
	}

	out.Items = make([]map[string]any, 0, len(itemsArr))
	for i, item := range itemsArr {
		row, ok := item.(map[string]any)
		if !ok {
			return out, fmt.Errorf("items[%d] must be an object", i)
		}
		locationID, err := readPositiveIntFromObject(row, "location_id")
		if err != nil {
			return out, fmt.Errorf("items[%d].%s", i, err.Error())
		}
		identity, err := parseAdjustmentItemIdentity(row, i)
		if err != nil {
			return out, err
		}
		quantity, hasQuantity, err := readOptionalIntFromObject(row, "quantity")
		if err != nil {
			return out, fmt.Errorf("items[%d].%s", i, err.Error())
		}
		qtyBackordered, hasQtyBackordered, err := readOptionalIntFromObject(row, "qty_backordered")
		if err != nil {
			return out, fmt.Errorf("items[%d].%s", i, err.Error())
		}
		if !hasQuantity && !hasQtyBackordered {
			return out, fmt.Errorf("items[%d]: at least one of quantity or qty_backordered is required", i)
		}
		if relative {
			if (!hasQuantity || quantity == 0) && (!hasQtyBackordered || qtyBackordered == 0) {
				return out, fmt.Errorf("items[%d]: at least one of quantity or qty_backordered must be non-zero for relative adjustments", i)
			}
			// BigCommerce rejects relative quantity=0 (422). Omit the field when
			// the caller only intends a qty_backordered delta.
			if hasQuantity && quantity == 0 {
				hasQuantity = false
			}
		} else {
			if hasQuantity && quantity < 0 {
				return out, fmt.Errorf("items[%d].quantity must be non-negative for absolute adjustments", i)
			}
			if hasQtyBackordered && qtyBackordered < 0 {
				return out, fmt.Errorf("items[%d].qty_backordered must be non-negative for absolute adjustments", i)
			}
		}
		itemPayload := map[string]any{
			"location_id": locationID,
		}
		for k, v := range identity {
			itemPayload[k] = v
		}
		if hasQuantity {
			itemPayload["quantity"] = quantity
		}
		if hasQtyBackordered {
			itemPayload["qty_backordered"] = qtyBackordered
		}
		out.Items = append(out.Items, itemPayload)
	}

	return out, nil
}

// parseAdjustmentItemIdentity requires exactly one of variant_id, sku, or
// product_id per BigCommerce inventory adjustment identity rules.
func parseAdjustmentItemIdentity(row map[string]any, index int) (map[string]any, error) {
	out := map[string]any{}
	count := 0
	if v, present, err := readOptionalIntFromObject(row, "variant_id"); err != nil {
		return nil, fmt.Errorf("items[%d].%s", index, err.Error())
	} else if present {
		if v <= 0 {
			return nil, fmt.Errorf("items[%d].variant_id must be positive", index)
		}
		out["variant_id"] = v
		count++
	}
	if v, present, err := readOptionalIntFromObject(row, "product_id"); err != nil {
		return nil, fmt.Errorf("items[%d].%s", index, err.Error())
	} else if present {
		if v <= 0 {
			return nil, fmt.Errorf("items[%d].product_id must be positive", index)
		}
		out["product_id"] = v
		count++
	}
	if v, ok := row["sku"]; ok && v != nil {
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("items[%d].sku must be a non-empty string", index)
		}
		out["sku"] = strings.TrimSpace(s)
		count++
	}
	if count == 0 {
		return nil, fmt.Errorf("items[%d]: exactly one of variant_id, product_id, or sku is required", index)
	}
	if count > 1 {
		return nil, fmt.Errorf("items[%d]: provide exactly one of variant_id, product_id, or sku (not multiple)", index)
	}
	return out, nil
}

func readPositiveIntFromObject(obj map[string]any, key string) (int, error) {
	v, ok := obj[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	n := int(f)
	if n <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return n, nil
}

func readOptionalIntFromObject(obj map[string]any, key string) (int, bool, error) {
	v, ok := obj[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
	return int(f), true, nil
}

func parseInventoryLocationItemListParams(args map[string]any) (bigcommerce.InventoryLocationItemListParams, error) {
	params := bigcommerce.InventoryLocationItemListParams{}
	if ids, ok, err := readOptionalPositiveIntArray(args, "product_ids"); err != nil {
		return params, err
	} else if ok {
		params.ProductIDs = ids
	}
	if ids, ok, err := readOptionalPositiveIntArray(args, "variant_ids"); err != nil {
		return params, err
	} else if ok {
		params.VariantIDs = ids
	}
	if skus, ok, err := readOptionalStringArray(args, "skus"); err != nil {
		return params, err
	} else if ok {
		params.SKUs = skus
	}
	if page, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return params, err
	} else if ok {
		params.Page = page
	}
	if limit, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return params, err
	} else if ok {
		if limit > maxInventoryListLimit {
			return params, fmt.Errorf("limit must be <= %d", maxInventoryListLimit)
		}
		params.Limit = limit
	}
	return params, nil
}

func parseLocationItemSettings(args map[string]any) ([]map[string]any, error) {
	settingsRaw, ok := args["settings"]
	if !ok || settingsRaw == nil {
		return nil, fmt.Errorf("settings is required")
	}
	settingsArr, ok := settingsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("settings must be an array of objects")
	}
	if len(settingsArr) == 0 {
		return nil, fmt.Errorf("settings must include at least one row")
	}
	if len(settingsArr) > maxInventoryAdjustmentItems {
		return nil, fmt.Errorf("settings exceeds maximum of %d rows", maxInventoryAdjustmentItems)
	}

	out := make([]map[string]any, 0, len(settingsArr))
	for i, entry := range settingsArr {
		row, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("settings[%d] must be an object", i)
		}
		identityRaw, ok := row["identity"]
		if !ok || identityRaw == nil {
			return nil, fmt.Errorf("settings[%d].identity is required", i)
		}
		identity, ok := identityRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("settings[%d].identity must be an object", i)
		}
		normalizedIdentity, err := normalizeLocationItemIdentity(identity, i)
		if err != nil {
			return nil, err
		}
		setting := map[string]any{"identity": normalizedIdentity}
		hasSettingField := false

		if v, present, err := readOptionalIntFromObject(row, "backorder_limit"); err != nil {
			return nil, fmt.Errorf("settings[%d].%s", i, err.Error())
		} else if present {
			if v < 0 {
				return nil, fmt.Errorf("settings[%d].backorder_limit must be non-negative", i)
			}
			setting["backorder_limit"] = v
			hasSettingField = true
		}
		if v, present, err := readOptionalIntFromObject(row, "safety_stock"); err != nil {
			return nil, fmt.Errorf("settings[%d].%s", i, err.Error())
		} else if present {
			if v < 0 {
				return nil, fmt.Errorf("settings[%d].safety_stock must be non-negative", i)
			}
			setting["safety_stock"] = v
			hasSettingField = true
		}
		if v, present, err := readOptionalIntFromObject(row, "warning_level"); err != nil {
			return nil, fmt.Errorf("settings[%d].%s", i, err.Error())
		} else if present {
			if v < 0 {
				return nil, fmt.Errorf("settings[%d].warning_level must be non-negative", i)
			}
			setting["warning_level"] = v
			hasSettingField = true
		}
		if v, ok := row["is_in_stock"]; ok && v != nil {
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("settings[%d].is_in_stock must be a boolean", i)
			}
			setting["is_in_stock"] = b
			hasSettingField = true
		}
		if v, ok := row["bin_picking_number"]; ok && v != nil {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("settings[%d].bin_picking_number must be a string", i)
			}
			setting["bin_picking_number"] = s
			hasSettingField = true
		}
		if !hasSettingField {
			return nil, fmt.Errorf("settings[%d] must include at least one setting field (e.g. backorder_limit)", i)
		}
		out = append(out, setting)
	}
	return out, nil
}

func normalizeLocationItemIdentity(identity map[string]any, index int) (map[string]any, error) {
	out := map[string]any{}
	count := 0
	if v, present, err := readOptionalIntFromObject(identity, "variant_id"); err != nil {
		return nil, fmt.Errorf("settings[%d].identity.%s", index, err.Error())
	} else if present {
		if v <= 0 {
			return nil, fmt.Errorf("settings[%d].identity.variant_id must be positive", index)
		}
		out["variant_id"] = v
		count++
	}
	if v, present, err := readOptionalIntFromObject(identity, "product_id"); err != nil {
		return nil, fmt.Errorf("settings[%d].identity.%s", index, err.Error())
	} else if present {
		if v <= 0 {
			return nil, fmt.Errorf("settings[%d].identity.product_id must be positive", index)
		}
		out["product_id"] = v
		count++
	}
	if v, ok := identity["sku"]; ok && v != nil {
		s, ok := v.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("settings[%d].identity.sku must be a non-empty string", index)
		}
		out["sku"] = strings.TrimSpace(s)
		count++
	}
	if count == 0 {
		return nil, fmt.Errorf("settings[%d].identity requires exactly one of variant_id, product_id, or sku", index)
	}
	if count > 1 {
		return nil, fmt.Errorf("settings[%d].identity: provide exactly one of variant_id, product_id, or sku (not multiple)", index)
	}
	return out, nil
}

func extractTransactionID(raw json.RawMessage) string {
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ""
	}
	return findTransactionID(parsed)
}

func findTransactionID(v any) string {
	switch typed := v.(type) {
	case map[string]any:
		if tx, ok := typed["transaction_id"].(string); ok && strings.TrimSpace(tx) != "" {
			return strings.TrimSpace(tx)
		}
		for _, child := range typed {
			if tx := findTransactionID(child); tx != "" {
				return tx
			}
		}
	case []any:
		for _, child := range typed {
			if tx := findTransactionID(child); tx != "" {
				return tx
			}
		}
	}
	return ""
}

func parseInventoryItemListParams(args map[string]any) (bigcommerce.InventoryItemListParams, bool, error) {
	params := bigcommerce.InventoryItemListParams{}
	hasFilter := false

	if ids, ok, err := readOptionalPositiveIntArray(args, "location_ids"); err != nil {
		return params, false, err
	} else if ok {
		params.LocationIDs = ids
		hasFilter = true
	}
	if ids, ok, err := readOptionalPositiveIntArray(args, "product_ids"); err != nil {
		return params, false, err
	} else if ok {
		params.ProductIDs = ids
		hasFilter = true
	}
	if ids, ok, err := readOptionalPositiveIntArray(args, "variant_ids"); err != nil {
		return params, false, err
	} else if ok {
		params.VariantIDs = ids
		hasFilter = true
	}
	if skus, ok, err := readOptionalStringArray(args, "skus"); err != nil {
		return params, false, err
	} else if ok {
		params.SKUs = skus
		hasFilter = true
	}
	if page, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return params, false, err
	} else if ok {
		params.Page = page
	}
	if limit, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return params, false, err
	} else if ok {
		if limit > maxInventoryListLimit {
			return params, false, fmt.Errorf("limit must be <= %d", maxInventoryListLimit)
		}
		params.Limit = limit
	}
	return params, hasFilter, nil
}

func requiredObjectPayload(args map[string]any, key string) (map[string]any, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	if len(obj) == 0 {
		return nil, fmt.Errorf("%s must not be empty", key)
	}
	return obj, nil
}

func inventoryPayloadRowCount(payload map[string]any) (int, error) {
	_, hasItems := payload["items"]
	_, hasData := payload["data"]
	// Reject ambiguous payloads: the whole map is marshaled and sent to BC, so
	// counting only one key while both are present would let the row cap be
	// bypassed (e.g. items[≤10] + data[many]).
	if hasItems && hasData {
		return 0, fmt.Errorf("provide either update.items or update.data, not both")
	}

	key := "items"
	if hasData {
		key = "data"
	} else if !hasItems {
		return 0, fmt.Errorf("update payload must include items[] or data[]")
	}

	arr, ok := payload[key].([]any)
	if !ok {
		return 0, fmt.Errorf("update.%s must be an array when provided", key)
	}
	if len(arr) == 0 {
		return 0, fmt.Errorf("update.%s must include at least one row", key)
	}
	return len(arr), nil
}

func parseInventoryLocationMetafieldSetFields(args map[string]any) (namespace, key, value, description, permissionSet string, err error) {
	nsRaw, ok := args["namespace"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("namespace is required")
	}
	ns, sOK := nsRaw.(string)
	if !sOK || strings.TrimSpace(ns) == "" {
		return "", "", "", "", "", fmt.Errorf("namespace must be a non-empty string")
	}
	ns = strings.TrimSpace(ns)

	keyRaw, ok := args["key"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("key is required")
	}
	k, sOK := keyRaw.(string)
	if !sOK || strings.TrimSpace(k) == "" {
		return "", "", "", "", "", fmt.Errorf("key must be a non-empty string")
	}
	k = strings.TrimSpace(k)

	valueRaw, ok := args["value"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("value is required")
	}
	val, sOK := valueRaw.(string)
	if !sOK {
		return "", "", "", "", "", fmt.Errorf("value must be a string")
	}

	desc := ""
	if v, ok := args["description"]; ok {
		s, ok := v.(string)
		if !ok {
			return "", "", "", "", "", fmt.Errorf("description must be a string")
		}
		desc = s
	}

	ps, perr := shared.ParseOptionalPermissionSet(args)
	if perr != nil {
		return "", "", "", "", "", perr
	}
	return ns, k, val, desc, ps, nil
}

func parseInventoryLocationMetafieldDeleteSelector(args map[string]any) (mfID int, namespace, key string, err error) {
	_, hasMFID := args["metafield_id"]
	_, hasNS := args["namespace"]
	_, hasKey := args["key"]
	if hasMFID && (hasNS || hasKey) {
		return 0, "", "", fmt.Errorf("use metafield_id alone, or namespace + key; do not combine")
	}
	if !hasMFID && (!hasNS || !hasKey) {
		return 0, "", "", fmt.Errorf("provide metafield_id, or both namespace and key")
	}
	if hasMFID {
		id, err := shared.ReadPositiveInt(args, "metafield_id")
		if err != nil {
			return 0, "", "", err
		}
		return id, "", "", nil
	}
	ns, _ := args["namespace"].(string)
	k, _ := args["key"].(string)
	ns = strings.TrimSpace(ns)
	k = strings.TrimSpace(k)
	if ns == "" || k == "" {
		return 0, "", "", fmt.Errorf("namespace and key must be non-empty")
	}
	return 0, ns, k, nil
}

func readOptionalPositiveInt(args map[string]any, key string) (int, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
	n := int(f)
	if n <= 0 {
		return 0, false, fmt.Errorf("%s must be positive", key)
	}
	return n, true, nil
}

func readOptionalPositiveIntArray(args map[string]any, key string) ([]int, bool, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, false, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s must be an array of numbers", key)
	}
	out := make([]int, 0, len(arr))
	for i, item := range arr {
		f, ok := item.(float64)
		if !ok {
			return nil, false, fmt.Errorf("%s[%d] must be a number", key, i)
		}
		if f != math.Trunc(f) {
			return nil, false, fmt.Errorf("%s[%d] must be an integer", key, i)
		}
		n := int(f)
		if n <= 0 {
			return nil, false, fmt.Errorf("%s[%d] must be positive", key, i)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, false, fmt.Errorf("%s must include at least one id", key)
	}
	return out, true, nil
}

func readOptionalStringArray(args map[string]any, key string) ([]string, bool, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, false, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, false, fmt.Errorf("%s must be an array of strings", key)
	}
	out := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, false, fmt.Errorf("%s[%d] must be a string", key, i)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false, fmt.Errorf("%s[%d] must be non-empty", key, i)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, false, fmt.Errorf("%s must include at least one value", key)
	}
	return out, true, nil
}
