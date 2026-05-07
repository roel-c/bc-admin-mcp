package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// RegisterModifierTools registers the product modifier tools.
func (p *Products) RegisterModifierTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/modifiers/list",
		Tier:        middleware.TierR0,
		Summary:     "List all modifiers for a product",
		Description: "Returns modifiers with their type, config, and option values.",
		Tool: mcp.NewTool("catalog_products_modifiers_list",
			mcp.WithDescription("List all modifiers for a product."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
		),
		Handler: p.handleModifierList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/modifiers/create",
		Tier:    middleware.TierR1,
		Summary: "Create a modifier on a product",
		Description: "Adds a modifier (text, dropdown, checkbox, file upload, etc.). " +
			"Modifiers allow customization without creating separate variants.",
		Tool: mcp.NewTool("catalog_products_modifiers_create",
			mcp.WithDescription(
				"Create a modifier on a product. Types: text, multi_line_text, numbers_only_text, "+
					"date, checkbox, file, dropdown, radio_buttons, rectangles, swatch. "+
					"Preview shows proposed modifier; pass confirmed=true to create.",
			),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithString("display_name", mcp.Description("Modifier display name"), mcp.Required()),
			mcp.WithString("type", mcp.Description("Modifier type"), mcp.Required()),
			mcp.WithBoolean("required", mcp.Description("Whether this modifier is required")),
			mcp.WithNumber("sort_order", mcp.Description("Display sort order")),
			mcp.WithArray("option_values", mcp.Description("Array of {label, sort_order} objects for choice-type modifiers")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleModifierCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/modifiers/delete",
		Tier:        middleware.TierR2,
		Summary:     "Delete a modifier from a product",
		Description: "Removes a modifier. Cannot be undone.",
		Tool: mcp.NewTool("catalog_products_modifiers_delete",
			mcp.WithDescription("Delete a product modifier. Preview first; pass confirmed=true to execute."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithNumber("modifier_id", mcp.Description("Modifier ID to delete"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleModifierDelete,
	})
}

func (p *Products) handleModifierList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	productID, err := requiredPositiveInt(request.GetArguments(), "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	mods, err := p.bc.ListProductModifiers(ctx, productID)
	if err != nil {
		return toolError("failed to list modifiers: %v", err), nil
	}

	return toolJSON(map[string]any{
		"product_id":      productID,
		"total_modifiers": len(mods),
		"modifiers":       mods,
	})
}

func parseModifierValues(raw any) ([]bigcommerce.ProductModifierValue, error) {
	if raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("option_values must be an array")
	}
	vals := make([]bigcommerce.ProductModifierValue, 0, len(arr))
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
				return nil, fmt.Errorf("each option_value must be an object with 'label'")
			}
		}
		label, _ := m["label"].(string)
		if label == "" {
			return nil, fmt.Errorf("each option_value must have a non-empty 'label'")
		}
		mv := bigcommerce.ProductModifierValue{Label: label}
		if so, ok := m["sort_order"].(float64); ok {
			mv.SortOrder = int(so)
		}
		if id, ok := m["id"].(float64); ok {
			mv.ID = int(id)
		}
		if def, ok := m["is_default"].(bool); ok {
			mv.IsDefault = def
		}
		vals = append(vals, mv)
	}
	return vals, nil
}

func (p *Products) handleModifierCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	displayName, _ := args["display_name"].(string)
	if displayName == "" {
		return toolError("display_name is required"), nil
	}
	modType, _ := args["type"].(string)
	if modType == "" {
		return toolError("type is required"), nil
	}

	payload := bigcommerce.ProductModifierCreate{
		DisplayName: displayName,
		Type:        modType,
	}
	if v, ok := args["required"].(bool); ok {
		payload.Required = v
	}
	if v, ok := args["sort_order"].(float64); ok {
		payload.SortOrder = int(v)
	}
	if v, ok := args["option_values"]; ok {
		values, vErr := parseModifierValues(v)
		if vErr != nil {
			return toolError("%s", vErr.Error()), nil
		}
		payload.OptionValues = values
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"payload":    payload,
			"message":    "Modifier will be created on the product. Pass confirmed=true to execute.",
		})
	}

	mod, err := p.bc.CreateProductModifier(ctx, productID, payload)
	if err != nil {
		return toolError("failed to create modifier: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"modifier":   mod,
	})
}

func (p *Products) handleModifierDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	modifierID, err := requiredPositiveInt(args, "modifier_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		mods, lErr := p.bc.ListProductModifiers(ctx, productID)
		if lErr != nil {
			return toolError("failed to list modifiers: %v", lErr), nil
		}
		var target *bigcommerce.ProductModifier
		for i := range mods {
			if mods[i].ID == modifierID {
				target = &mods[i]
				break
			}
		}
		if target == nil {
			return toolError("modifier %d not found on product %d", modifierID, productID), nil
		}
		return toolJSON(map[string]any{
			"status":       "pending_confirmation",
			"product_id":   productID,
			"modifier_id":  modifierID,
			"display_name": target.DisplayName,
			"type":         target.Type,
			"message":      "This modifier will be permanently deleted. Pass confirmed=true to execute.",
		})
	}

	if err := p.bc.DeleteProductModifier(ctx, productID, modifierID); err != nil {
		return toolError("failed to delete modifier: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":      "completed",
		"product_id":  productID,
		"modifier_id": modifierID,
		"message":     "Modifier deleted successfully.",
	})
}
