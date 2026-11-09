package catalog

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// LocalizationCustomFieldOverridesTools manages MSF IE custom-field overlays.
type LocalizationCustomFieldOverridesTools struct {
	bc BigCommerceAPI
}

// NewLocalizationCustomFieldOverridesTools constructs custom-field override tools.
func NewLocalizationCustomFieldOverridesTools(bc BigCommerceAPI) *LocalizationCustomFieldOverridesTools {
	return &LocalizationCustomFieldOverridesTools{bc: bc}
}

// RegisterTools registers catalog/localization/overrides/custom_fields/* tools.
func (t *LocalizationCustomFieldOverridesTools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/custom_fields/get",
		Tier:    middleware.TierR0,
		Summary: "Get product custom-field locale overlays",
		Description: "Reads global custom fields plus channel-locale overrides via GraphQL Admin " +
			"(MSF International Enhancements).",
		Tool: mcp.NewTool("catalog_localization_overrides_custom_fields_get",
			mcp.WithDescription(
				"Get custom field name/value/visibility overlays for a product channel+locale.",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel."), mcp.Required()),
		),
		Handler: t.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/custom_fields/set",
		Tier:    middleware.TierR1,
		Summary: "Set custom-field locale overlays (preview → confirm)",
		Description: "Sets channel-locale overlays for existing custom fields (does not change global values). " +
			"fields: [{custom_field_id|custom_field_numeric_id, name?, value?, is_visible?}]. " +
			"Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_overrides_custom_fields_set",
			mcp.WithDescription(
				"Set custom field overlays for a channel locale. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel."), mcp.Required()),
			mcp.WithArray("fields", mcp.Description(
				"Array of {custom_field_id or custom_field_numeric_id, name?, value?, is_visible?}.",
			)),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/custom_fields/remove",
		Tier:    middleware.TierR1,
		Summary: "Remove custom-field locale overlays (preview → confirm)",
		Description: "Clears custom-field overlays for a channel locale. " +
			"Optional custom_field_ids / custom_field_numeric_ids; omit to clear fields that have overlays. " +
			"Optional attributes: NAME, VALUE, IS_VISIBLE (default all). Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_overrides_custom_fields_remove",
			mcp.WithDescription(
				"Remove custom field overlays. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel."), mcp.Required()),
			mcp.WithArray("custom_field_ids", mcp.Description("Optional global custom field IDs."),
				mcp.WithStringItems()),
			mcp.WithArray("custom_field_numeric_ids", mcp.Description("Optional numeric custom field IDs."),
				mcp.WithNumberItems()),
			mcp.WithArray("attributes", mcp.Description("Optional: NAME, VALUE, IS_VISIBLE. Default all."),
				mcp.WithStringItems()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleRemove,
	})
}

func (t *LocalizationCustomFieldOverridesTools) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	productID, channelID, locale, err := parseOverrideContext(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	snap, err := t.bc.GetProductCustomFieldLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to get custom field overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"result":     snap,
		"api":        "GraphQL Admin product customFields overrides",
	})
}

func (t *LocalizationCustomFieldOverridesTools) handleSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, channelID, locale, err := parseOverrideContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	fields, err := parseCustomFieldOverrideEntries(args["fields"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(fields) == 0 {
		return toolError("fields is required and must not be empty"), nil
	}

	in := bigcommerce.SetProductCustomFieldOverridesInput{
		ProductID: productID,
		Context:   bigcommerce.LocaleContext{ChannelID: channelID, Locale: locale},
		Fields:    fields,
	}

	current, err := t.bc.GetProductCustomFieldLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to fetch current custom field overrides for preview: %v", err), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":      "pending_confirmation",
			"product_id":  productID,
			"channel_id":  channelID,
			"locale":      locale,
			"current":     current,
			"would_apply": fields,
			"message": fmt.Sprintf(
				"Will set custom field overlays on product %d for channel %d / %s. Pass confirmed=true to apply.",
				productID, channelID, locale,
			),
			"api": "GraphQL Admin product updateProductCustomFields",
		})
	}

	updated, err := t.bc.SetProductCustomFieldLocaleOverrides(ctx, in)
	if err != nil {
		return toolError("failed to set custom field overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"status":     "updated",
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"result":     updated,
		"api":        "GraphQL Admin product updateProductCustomFields",
	})
}

func (t *LocalizationCustomFieldOverridesTools) handleRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, channelID, locale, err := parseOverrideContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	in := bigcommerce.RemoveProductCustomFieldOverridesInput{
		ProductID: productID,
		Context:   bigcommerce.LocaleContext{ChannelID: channelID, Locale: locale},
	}
	if v, ok := args["custom_field_ids"]; ok && v != nil {
		ids, err := parseStringSlice(v, "custom_field_ids")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		in.CustomFieldIDs = ids
	}
	if v, ok := args["custom_field_numeric_ids"]; ok && v != nil {
		nums, err := parsePositiveIntSlice(v, "custom_field_numeric_ids")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		in.CustomFieldNumericIDs = nums
	}
	if v, ok := args["attributes"]; ok && v != nil {
		attrs, err := parseStringSlice(v, "attributes")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		in.Attributes = attrs
	}

	current, err := t.bc.GetProductCustomFieldLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to fetch current custom field overrides for preview: %v", err), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":                   "pending_confirmation",
			"product_id":               productID,
			"channel_id":               channelID,
			"locale":                   locale,
			"custom_field_ids":         in.CustomFieldIDs,
			"custom_field_numeric_ids": in.CustomFieldNumericIDs,
			"attributes":               in.Attributes,
			"current":                  current,
			"message": fmt.Sprintf(
				"Will remove custom field overlays on product %d for channel %d / %s. Pass confirmed=true to apply.",
				productID, channelID, locale,
			),
			"api": "GraphQL Admin product removeProductCustomFieldsOverrides",
		})
	}

	updated, err := t.bc.RemoveProductCustomFieldLocaleOverrides(ctx, in)
	if err != nil {
		return toolError("failed to remove custom field overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"status":     "removed",
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"result":     updated,
		"api":        "GraphQL Admin product removeProductCustomFieldsOverrides",
	})
}

func parseCustomFieldOverrideEntries(raw any) ([]bigcommerce.ProductCustomFieldOverrideEntry, error) {
	if raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("fields must be an array")
	}
	out := make([]bigcommerce.ProductCustomFieldOverrideEntry, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("fields[%d] must be an object", i)
		}
		entry := bigcommerce.ProductCustomFieldOverrideEntry{}
		if v, ok := m["custom_field_id"]; ok && v != nil {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("fields[%d].custom_field_id must be a string", i)
			}
			entry.CustomFieldID = s
		}
		if v, ok := m["custom_field_numeric_id"]; ok && v != nil {
			n, err := argPositiveInt("custom_field_numeric_id", v)
			if err != nil {
				return nil, fmt.Errorf("fields[%d]: %w", i, err)
			}
			entry.CustomFieldNumericID = n
		}
		if s, ok, err := optionalStringPtr(m, "name"); err != nil {
			return nil, fmt.Errorf("fields[%d]: %w", i, err)
		} else if ok {
			entry.Name = s
		}
		if s, ok, err := optionalStringPtr(m, "value"); err != nil {
			return nil, fmt.Errorf("fields[%d]: %w", i, err)
		} else if ok {
			entry.Value = s
		}
		if v, ok := m["is_visible"]; ok && v != nil {
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("fields[%d].is_visible must be a boolean", i)
			}
			entry.IsVisible = &b
		}
		out = append(out, entry)
	}
	return out, nil
}
