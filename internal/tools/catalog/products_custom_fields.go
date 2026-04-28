package catalog

import (
	"context"
	"fmt"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

// RegisterCustomFieldTools registers the product custom field tools.
func (p *Products) RegisterCustomFieldTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/custom_fields/list",
		Tier:        middleware.TierR0,
		Summary:     "List all custom fields for a product",
		Description: "Returns custom key-value pairs stored on a product.",
		Tool: mcp.NewTool("catalog_products_custom_fields_list",
			mcp.WithDescription("List all custom fields for a product."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
		),
		Handler: p.handleCustomFieldList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/custom_fields/set",
		Tier:    middleware.TierR1,
		Summary: "Create or update a custom field on a product (upsert)",
		Description: "Sets a custom field by name. If a field with that name exists, it is updated; " +
			"otherwise a new field is created.",
		Tool: mcp.NewTool("catalog_products_custom_fields_set",
			mcp.WithDescription(
				"Set a custom field on a product (upsert). "+
					"Provide product_id, name, and value. "+
					"Preview shows create or update action; pass confirmed=true to execute.",
			),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithString("name", mcp.Description("Custom field name"), mcp.Required()),
			mcp.WithString("value", mcp.Description("Custom field value"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleCustomFieldSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/custom_fields/delete",
		Tier:    middleware.TierR2,
		Summary: "Delete a custom field from a product",
		Description: "Removes a custom field by ID or by name. Cannot be undone.",
		Tool: mcp.NewTool("catalog_products_custom_fields_delete",
			mcp.WithDescription("Delete a custom field. Identify by custom_field_id or name. Preview first."),
			mcp.WithNumber("product_id", mcp.Description("Product ID"), mcp.Required()),
			mcp.WithNumber("custom_field_id", mcp.Description("Custom field ID to delete")),
			mcp.WithString("name", mcp.Description("Custom field name to delete (resolved by listing)")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true after reviewing preview")),
		),
		Handler: p.handleCustomFieldDelete,
	})
}

func (p *Products) handleCustomFieldList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	productID, err := requiredPositiveInt(request.GetArguments(), "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	fields, err := p.bc.ListProductCustomFields(ctx, productID)
	if err != nil {
		return toolError("failed to list custom fields: %v", err), nil
	}

	return toolJSON(map[string]any{
		"product_id":          productID,
		"total_custom_fields": len(fields),
		"custom_fields":       fields,
	})
}

func (p *Products) handleCustomFieldSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	name, _ := args["name"].(string)
	if name == "" {
		return toolError("name is required"), nil
	}
	value, _ := args["value"].(string)
	if value == "" {
		return toolError("value is required"), nil
	}

	existing, err := p.bc.ListProductCustomFields(ctx, productID)
	if err != nil {
		return toolError("failed to list existing custom fields: %v", err), nil
	}

	var found *bigcommerce.ProductCustomField
	for i := range existing {
		if existing[i].Name == name {
			found = &existing[i]
			break
		}
	}

	action := "create"
	if found != nil {
		action = "update"
	}

	if !middleware.IsConfirmedFromArgs(args) {
		preview := map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"action":     action,
			"name":       name,
			"new_value":  value,
		}
		if found != nil {
			preview["old_value"] = found.Value
			preview["custom_field_id"] = found.ID
		}
		preview["message"] = fmt.Sprintf("Custom field '%s' will be %sd. Pass confirmed=true to execute.", name, action)
		return toolJSON(preview)
	}

	payload := bigcommerce.ProductCustomFieldCreate{Name: name, Value: value}
	if found != nil {
		updated, uErr := p.bc.UpdateProductCustomField(ctx, productID, found.ID, payload)
		if uErr != nil {
			return toolError("failed to update custom field: %v", uErr), nil
		}
		return toolJSON(map[string]any{
			"status":       "completed",
			"action":       "updated",
			"product_id":   productID,
			"custom_field": updated,
		})
	}

	created, cErr := p.bc.CreateProductCustomField(ctx, productID, payload)
	if cErr != nil {
		return toolError("failed to create custom field: %v", cErr), nil
	}
	return toolJSON(map[string]any{
		"status":       "completed",
		"action":       "created",
		"product_id":   productID,
		"custom_field": created,
	})
}

func (p *Products) handleCustomFieldDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, err := requiredPositiveInt(args, "product_id")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	var fieldID int
	if v, ok := args["custom_field_id"].(float64); ok && v > 0 {
		fieldID = int(v)
	}

	name, _ := args["name"].(string)

	if fieldID == 0 && name == "" {
		return toolError("provide custom_field_id or name to identify the field"), nil
	}

	if fieldID == 0 {
		existing, lErr := p.bc.ListProductCustomFields(ctx, productID)
		if lErr != nil {
			return toolError("failed to list custom fields: %v", lErr), nil
		}
		for _, cf := range existing {
			if cf.Name == name {
				fieldID = cf.ID
				break
			}
		}
		if fieldID == 0 {
			return toolError("custom field with name %q not found on product %d", name, productID), nil
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		existing, lErr := p.bc.ListProductCustomFields(ctx, productID)
		if lErr != nil {
			return toolError("failed to list custom fields for preview: %v", lErr), nil
		}
		var target *bigcommerce.ProductCustomField
		for i := range existing {
			if existing[i].ID == fieldID {
				target = &existing[i]
				break
			}
		}
		if target == nil {
			return toolError("custom field %d not found on product %d", fieldID, productID), nil
		}
		return toolJSON(map[string]any{
			"status":          "pending_confirmation",
			"product_id":      productID,
			"custom_field_id": fieldID,
			"name":            target.Name,
			"value":           target.Value,
			"message":         "This custom field will be permanently deleted. Pass confirmed=true to execute.",
		})
	}

	if err := p.bc.DeleteProductCustomField(ctx, productID, fieldID); err != nil {
		return toolError("failed to delete custom field: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":          "completed",
		"product_id":      productID,
		"custom_field_id": fieldID,
		"message":         "Custom field deleted successfully.",
	})
}
