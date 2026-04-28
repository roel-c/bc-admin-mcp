package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

// RegisterVariantTools registers the product variant CRUD tools.
func (p *Products) RegisterVariantTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/variants/list",
		Tier:        middleware.TierR0,
		Summary:     "List all variants for a product with full field details",
		Description: "Returns variants with SKU, pricing, inventory, dimensions, option values, and more.",
		Tool: mcp.NewTool("catalog_products_variants_list",
			mcp.WithDescription("List all variants for a product."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
		),
		Handler: p.handleVariantList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/create",
		Tier:    middleware.TierR1,
		Summary: "Create a new variant on a product",
		Description: "Creates a variant with option values. Options must already exist on the product. " +
			"Provide option_values as [{option_display_name, label}] to specify the combination.",
		Tool: mcp.NewTool("catalog_products_variants_create",
			mcp.WithDescription(
				"Create a variant on a product. Options must exist first. "+
					"Provide option_values mapping option names to value labels. "+
					"Preview shows proposed variant; pass confirmed=true to create.",
			),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithArray("option_values", mcp.Description("Array of {option_display_name, label} objects mapping options to values")),
			mcp.WithString("sku", mcp.Description("Variant SKU")),
			mcp.WithNumber("price", mcp.Description("Variant price (0 = inherit from product)")),
			mcp.WithNumber("cost_price", mcp.Description("Cost price")),
			mcp.WithNumber("sale_price", mcp.Description("Sale price")),
			mcp.WithNumber("retail_price", mcp.Description("Retail / compare-at price")),
			mcp.WithNumber("map_price", mcp.Description("Minimum advertised price")),
			mcp.WithNumber("weight", mcp.Description("Variant weight")),
			mcp.WithNumber("width", mcp.Description("Width")),
			mcp.WithNumber("height", mcp.Description("Height")),
			mcp.WithNumber("depth", mcp.Description("Depth")),
			mcp.WithNumber("inventory_level", mcp.Description("Inventory count")),
			mcp.WithNumber("inventory_warning_level", mcp.Description("Low-stock threshold")),
			mcp.WithString("bin_picking_number", mcp.Description("Bin picking number")),
			mcp.WithString("upc", mcp.Description("UPC code")),
			mcp.WithString("gtin", mcp.Description("GTIN")),
			mcp.WithString("mpn", mcp.Description("MPN")),
			mcp.WithString("image_url", mcp.Description("Variant image URL")),
			mcp.WithBoolean("purchasing_disabled", mcp.Description("Disable purchasing for this variant")),
			mcp.WithString("purchasing_disabled_message", mcp.Description("Message shown when purchasing is disabled")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleVariantCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/update",
		Tier:    middleware.TierR1,
		Summary: "Update a single variant's fields",
		Description: "Update any writable variant field: pricing, inventory, SKU, dimensions, etc. " +
			"Uses single PUT for precision.",
		Tool: mcp.NewTool("catalog_products_variants_update",
			mcp.WithDescription("Update a variant. Pass only fields to change. Preview shows diff."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithNumber("variant_id", mcp.Description("Variant ID to update"), mcp.Required()),
			mcp.WithString("sku", mcp.Description("Variant SKU")),
			mcp.WithNumber("price", mcp.Description("Price (0 = inherit)")),
			mcp.WithNumber("cost_price", mcp.Description("Cost price")),
			mcp.WithNumber("sale_price", mcp.Description("Sale price")),
			mcp.WithNumber("retail_price", mcp.Description("Retail price")),
			mcp.WithNumber("map_price", mcp.Description("MAP price")),
			mcp.WithNumber("weight", mcp.Description("Weight")),
			mcp.WithNumber("width", mcp.Description("Width")),
			mcp.WithNumber("height", mcp.Description("Height")),
			mcp.WithNumber("depth", mcp.Description("Depth")),
			mcp.WithNumber("inventory_level", mcp.Description("Inventory count")),
			mcp.WithNumber("inventory_warning_level", mcp.Description("Low-stock threshold")),
			mcp.WithString("bin_picking_number", mcp.Description("Bin picking number")),
			mcp.WithString("upc", mcp.Description("UPC")),
			mcp.WithString("gtin", mcp.Description("GTIN")),
			mcp.WithString("mpn", mcp.Description("MPN")),
			mcp.WithString("image_url", mcp.Description("Image URL")),
			mcp.WithBoolean("purchasing_disabled", mcp.Description("Disable purchasing")),
			mcp.WithString("purchasing_disabled_message", mcp.Description("Disabled message")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleVariantUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/delete",
		Tier:    middleware.TierR2,
		Summary: "Delete a product variant",
		Description: "Removes a variant from a product. Cannot be undone.",
		Tool: mcp.NewTool("catalog_products_variants_delete",
			mcp.WithDescription("Delete a variant. Preview shows variant details; pass confirmed=true to execute."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithNumber("variant_id", mcp.Description("Variant ID to delete"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleVariantDelete,
	})
}

func (p *Products) handleVariantList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	productID, err := requiredPositiveInt(request.GetArguments(), "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	variants, err := p.bc.ListVariantsForProduct(ctx, productID)
	if err != nil {
		return toolError("failed to list variants: %v", err), nil
	}

	return toolJSON(map[string]any{
		"product_id":     productID,
		"total_variants": len(variants),
		"variants":       variants,
	})
}

func parseVariantOptionValues(raw any) ([]bigcommerce.VariantOptionVal, error) {
	if raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("option_values must be an array")
	}
	vals := make([]bigcommerce.VariantOptionVal, 0, len(arr))
	for _, item := range arr {
		m, mOk := item.(map[string]any)
		if !mOk {
			b, bOk := item.(json.RawMessage)
			if bOk {
				var parsed map[string]any
				if json.Unmarshal(b, &parsed) == nil {
					m = parsed
					mOk = true
				}
			}
			if !mOk {
				return nil, fmt.Errorf("each option_value must be an object with option_display_name and label")
			}
		}
		optName, _ := m["option_display_name"].(string)
		label, _ := m["label"].(string)
		if label == "" {
			return nil, fmt.Errorf("each option_value must have a non-empty 'label'")
		}
		val := bigcommerce.VariantOptionVal{
			OptionDisplayName: optName,
			Label:             label,
		}
		if id, ok := m["id"].(float64); ok {
			val.ID = int(id)
		}
		if oid, ok := m["option_id"].(float64); ok {
			val.OptionID = int(oid)
		}
		vals = append(vals, val)
	}
	return vals, nil
}

func (p *Products) handleVariantCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	optionValues, err := parseVariantOptionValues(args["option_values"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	payload := bigcommerce.ProductVariantCreate{OptionValues: optionValues}
	if v, ok := args["sku"].(string); ok {
		payload.SKU = v
	}
	extractFloatPtr(args, "price", &payload.Price)
	extractFloatPtr(args, "cost_price", &payload.CostPrice)
	extractFloatPtr(args, "sale_price", &payload.SalePrice)
	extractFloatPtr(args, "retail_price", &payload.RetailPrice)
	extractFloatPtr(args, "map_price", &payload.MapPrice)
	extractFloatPtr(args, "weight", &payload.Weight)
	extractFloatPtr(args, "width", &payload.Width)
	extractFloatPtr(args, "height", &payload.Height)
	extractFloatPtr(args, "depth", &payload.Depth)
	extractIntPtr(args, "inventory_level", &payload.InventoryLevel)
	extractIntPtr(args, "inventory_warning_level", &payload.InventoryWarningLevel)
	if v, ok := args["bin_picking_number"].(string); ok {
		payload.BinPickingNumber = v
	}
	if v, ok := args["upc"].(string); ok {
		payload.UPC = v
	}
	if v, ok := args["gtin"].(string); ok {
		payload.GTIN = v
	}
	if v, ok := args["mpn"].(string); ok {
		payload.MPN = v
	}
	if v, ok := args["image_url"].(string); ok {
		payload.ImageURL = v
	}
	extractBoolPtr(args, "purchasing_disabled", &payload.PurchasingDisabled)
	if v, ok := args["purchasing_disabled_message"].(string); ok {
		payload.PurchasingDisabledMsg = v
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"payload":    payload,
			"message":    "Variant will be created. Pass confirmed=true to execute.",
		})
	}

	variant, err := p.bc.CreateVariant(ctx, productID, payload)
	if err != nil {
		return toolError("failed to create variant: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"variant":    variant,
	})
}

func (p *Products) handleVariantUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	variantID, err := requiredPositiveInt(args, "variant_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	payload := bigcommerce.ProductVariantUpdate{}
	hasChange := false

	if v, ok := args["sku"].(string); ok {
		payload.SKU = &v
		hasChange = true
	}
	setFloatChange(args, "price", &payload.Price, &hasChange)
	setFloatChange(args, "cost_price", &payload.CostPrice, &hasChange)
	setFloatChange(args, "sale_price", &payload.SalePrice, &hasChange)
	setFloatChange(args, "retail_price", &payload.RetailPrice, &hasChange)
	setFloatChange(args, "map_price", &payload.MapPrice, &hasChange)
	setFloatChange(args, "weight", &payload.Weight, &hasChange)
	setFloatChange(args, "width", &payload.Width, &hasChange)
	setFloatChange(args, "height", &payload.Height, &hasChange)
	setFloatChange(args, "depth", &payload.Depth, &hasChange)
	setIntChange(args, "inventory_level", &payload.InventoryLevel, &hasChange)
	setIntChange(args, "inventory_warning_level", &payload.InventoryWarningLevel, &hasChange)
	if v, ok := args["bin_picking_number"].(string); ok {
		payload.BinPickingNumber = &v
		hasChange = true
	}
	if v, ok := args["upc"].(string); ok {
		payload.UPC = &v
		hasChange = true
	}
	if v, ok := args["gtin"].(string); ok {
		payload.GTIN = &v
		hasChange = true
	}
	if v, ok := args["mpn"].(string); ok {
		payload.MPN = &v
		hasChange = true
	}
	if v, ok := args["image_url"].(string); ok {
		payload.ImageURL = &v
		hasChange = true
	}
	if v, ok := args["purchasing_disabled"].(bool); ok {
		payload.PurchasingDisabled = &v
		hasChange = true
	}
	if v, ok := args["purchasing_disabled_message"].(string); ok {
		payload.PurchasingDisabledMsg = &v
		hasChange = true
	}

	if !hasChange {
		return toolError("provide at least one field to update"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		current, gErr := p.bc.GetVariant(ctx, productID, variantID)
		if gErr != nil {
			return toolError("failed to get variant for preview: %v", gErr), nil
		}
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"variant_id": variantID,
			"current":    current,
			"proposed":   payload,
			"message":    "Variant will be updated. Pass confirmed=true to execute.",
		})
	}

	variant, err := p.bc.UpdateVariant(ctx, productID, variantID, payload)
	if err != nil {
		return toolError("failed to update variant: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"variant":    variant,
	})
}

func (p *Products) handleVariantDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	variantID, err := requiredPositiveInt(args, "variant_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		v, gErr := p.bc.GetVariant(ctx, productID, variantID)
		if gErr != nil {
			return toolError("failed to get variant for preview: %v", gErr), nil
		}
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"variant_id": variantID,
			"sku":        v.SKU,
			"price":      v.Price,
			"message":    "This variant will be permanently deleted. Pass confirmed=true to execute.",
		})
	}

	if err := p.bc.DeleteVariant(ctx, productID, variantID); err != nil {
		return toolError("failed to delete variant: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"variant_id": variantID,
		"message":    "Variant deleted successfully.",
	})
}

// Helpers for constructing variant payloads from args.
func extractFloatPtr(args map[string]any, key string, dst **float64) {
	if v, ok := args[key].(float64); ok {
		*dst = &v
	}
}

func extractIntPtr(args map[string]any, key string, dst **int) {
	if v, ok := args[key].(float64); ok {
		i := int(v)
		*dst = &i
	}
}

func extractBoolPtr(args map[string]any, key string, dst **bool) {
	if v, ok := args[key].(bool); ok {
		*dst = &v
	}
}

func setFloatChange(args map[string]any, key string, dst **float64, changed *bool) {
	if v, ok := args[key].(float64); ok {
		*dst = &v
		*changed = true
	}
}

func setIntChange(args map[string]any, key string, dst **int, changed *bool) {
	if v, ok := args[key].(float64); ok {
		i := int(v)
		*dst = &i
		*changed = true
	}
}
