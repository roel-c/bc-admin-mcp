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

// RegisterOptionTools registers the product option CRUD tools.
func (p *Products) RegisterOptionTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/options/list",
		Tier:        middleware.TierR0,
		Summary:     "List all variant-generating options for a product",
		Description: "Returns options with their display names, types, and values.",
		Tool: mcp.NewTool("catalog_products_options_list",
			mcp.WithDescription("List all options for a product."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
		),
		Handler: p.handleOptionList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/options/create",
		Tier:    middleware.TierR1,
		Summary: "Create a variant-generating option on a product",
		Description: "Adds an option (e.g. Size, Color) with values, defining a variant axis. " +
			"NOTE: this does NOT auto-generate variants — create each variant explicitly via " +
			"catalog/products/variants/create (option_values need id + option_id + label). " +
			"To create a product and all its variants in ONE call, prefer catalog/products/create " +
			"with an inline variants array (BigCommerce V3 best practice).",
		Tool: mcp.NewTool("catalog_products_options_create",
			mcp.WithDescription(
				"Create an option on a product. Provide display_name, type, and option_values. "+
					"Preview shows proposed option; pass confirmed=true to create.",
			),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithString("display_name", mcp.Description("Option display name (e.g. Size, Color)"), mcp.Required()),
			mcp.WithString("type", mcp.Description("Option type: dropdown, radio_buttons, rectangles, swatch, product_list, product_list_with_images"), mcp.Required()),
			mcp.WithArray("option_values", mcp.Description("Array of {label, sort_order} objects for the option values")),
			mcp.WithNumber("sort_order", mcp.Description("Display sort order for this option")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleOptionCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/options/update",
		Tier:    middleware.TierR1,
		Summary: "Update an existing product option",
		Description: "Modify an option's display name, sort order, or add/update values. " +
			"Preview shows current vs. proposed.",
		Tool: mcp.NewTool("catalog_products_options_update",
			mcp.WithDescription("Update a product option. Provide product_id and option_id plus fields to change."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithNumber("option_id", mcp.Description("Option ID to update"), mcp.Required()),
			mcp.WithString("display_name", mcp.Description("New display name")),
			mcp.WithNumber("sort_order", mcp.Description("New sort order")),
			mcp.WithArray("option_values", mcp.Description("Updated option values array")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleOptionUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/options/delete",
		Tier:        middleware.TierR2,
		Summary:     "Delete a product option (removes associated variants)",
		Description: "Deletes an option and all variants that depend on it. Cannot be undone.",
		Tool: mcp.NewTool("catalog_products_options_delete",
			mcp.WithDescription("Delete a product option. WARNING: This removes all variants that use this option."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithNumber("option_id", mcp.Description("Option ID to delete"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleOptionDelete,
	})
}

func (p *Products) handleOptionList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	productID, err := requiredPositiveInt(request.GetArguments(), "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	opts, err := p.bc.ListProductOptions(ctx, productID)
	if err != nil {
		return toolError("failed to list options: %v", err), nil
	}

	return toolJSON(map[string]any{
		"product_id":    productID,
		"total_options": len(opts),
		"options":       opts,
	})
}

func parseOptionValues(raw any) ([]bigcommerce.ProductOptionValue, error) {
	if raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("option_values must be an array")
	}
	vals := make([]bigcommerce.ProductOptionValue, 0, len(arr))
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
		ov := bigcommerce.ProductOptionValue{Label: label}
		if so, ok := m["sort_order"].(float64); ok {
			ov.SortOrder = int(so)
		}
		if id, ok := m["id"].(float64); ok {
			ov.ID = int(id)
		}
		if def, ok := m["is_default"].(bool); ok {
			ov.IsDefault = def
		}
		vals = append(vals, ov)
	}
	return vals, nil
}

func (p *Products) handleOptionCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	displayName, _ := args["display_name"].(string)
	if displayName == "" {
		return toolError("display_name is required"), nil
	}
	optType, _ := args["type"].(string)
	if optType == "" {
		return toolError("type is required"), nil
	}

	values, err := parseOptionValues(args["option_values"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	payload := bigcommerce.ProductOptionCreate{
		DisplayName:  displayName,
		Type:         optType,
		OptionValues: values,
	}
	if v, ok := args["sort_order"].(float64); ok {
		payload.SortOrder = int(v)
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"payload":    payload,
			"message":    "Option will be created on the product. Pass confirmed=true to execute.",
		})
	}

	opt, err := p.bc.CreateProductOption(ctx, productID, payload)
	if err != nil {
		return toolError("failed to create option: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"option":     opt,
	})
}

func (p *Products) handleOptionUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	optionID, err := requiredPositiveInt(args, "option_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	payload := bigcommerce.ProductOptionUpdate{}
	hasChange := false
	if v, ok := args["display_name"].(string); ok && v != "" {
		payload.DisplayName = &v
		hasChange = true
	}
	if v, ok := args["sort_order"].(float64); ok {
		i := int(v)
		payload.SortOrder = &i
		hasChange = true
	}
	if v, ok := args["option_values"]; ok {
		values, vErr := parseOptionValues(v)
		if vErr != nil {
			return toolError("%s", vErr.Error()), nil
		}
		payload.OptionValues = values
		hasChange = true
	}

	if !hasChange {
		return toolError("provide at least one field to update: display_name, sort_order, or option_values"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"option_id":  optionID,
			"payload":    payload,
			"message":    "Option will be updated. Pass confirmed=true to execute.",
		})
	}

	opt, err := p.bc.UpdateProductOption(ctx, productID, optionID, payload)
	if err != nil {
		return toolError("failed to update option: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"option":     opt,
	})
}

func (p *Products) handleOptionDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	optionID, err := requiredPositiveInt(args, "option_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		opts, lErr := p.bc.ListProductOptions(ctx, productID)
		if lErr != nil {
			return toolError("failed to list options: %v", lErr), nil
		}
		var target *bigcommerce.ProductOption
		for i := range opts {
			if opts[i].ID == optionID {
				target = &opts[i]
				break
			}
		}
		if target == nil {
			return toolError("option %d not found on product %d", optionID, productID), nil
		}
		return toolJSON(map[string]any{
			"status":       "pending_confirmation",
			"product_id":   productID,
			"option_id":    optionID,
			"display_name": target.DisplayName,
			"type":         target.Type,
			"value_count":  len(target.OptionValues),
			"message":      "WARNING: Deleting this option will remove all associated variants. Pass confirmed=true to execute.",
		})
	}

	if err := p.bc.DeleteProductOption(ctx, productID, optionID); err != nil {
		return toolError("failed to delete option: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":     "completed",
		"product_id": productID,
		"option_id":  optionID,
		"message":    "Option deleted successfully.",
	})
}
