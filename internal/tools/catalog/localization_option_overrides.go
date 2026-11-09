package catalog

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// LocalizationOptionOverridesTools manages MSF IE variant-option label overlays.
type LocalizationOptionOverridesTools struct {
	bc BigCommerceAPI
}

// NewLocalizationOptionOverridesTools constructs option override tools.
func NewLocalizationOptionOverridesTools(bc BigCommerceAPI) *LocalizationOptionOverridesTools {
	return &LocalizationOptionOverridesTools{bc: bc}
}

// RegisterTools registers catalog/localization/overrides/options/* tools.
func (t *LocalizationOptionOverridesTools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/options/get",
		Tier:    middleware.TierR0,
		Summary: "Get product variant-option locale overlays",
		Description: "Reads global variant option display names/values plus overridesForLocale " +
			"via GraphQL Admin (MSF International Enhancements).",
		Tool: mcp.NewTool("catalog_localization_overrides_options_get",
			mcp.WithDescription(
				"Get variant option display name/value overlays for a product channel+locale.",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel."), mcp.Required()),
		),
		Handler: t.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/options/set",
		Tier:    middleware.TierR1,
		Summary: "Set variant-option locale overlays (preview → confirm)",
		Description: "Sets dropdown option display_name and/or value labels for a channel locale. " +
			"options: [{option_id|option_numeric_id, display_name?, values:[{value_id|value_numeric_id, label}]}]. " +
			"Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_overrides_options_set",
			mcp.WithDescription(
				"Set variant option label overlays. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel."), mcp.Required()),
			mcp.WithArray("options", mcp.Description(
				"Array of {option_id or option_numeric_id, display_name?, values:[{value_id or value_numeric_id, label}]}.",
			)),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/options/remove",
		Tier:    middleware.TierR1,
		Summary: "Remove variant-option locale overlays (preview → confirm)",
		Description: "Clears dropdown option display name and value label overlays for a channel locale. " +
			"Optional option_ids / option_numeric_ids; omit to clear all product options with overlays. " +
			"Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_overrides_options_remove",
			mcp.WithDescription(
				"Remove variant option label overlays. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel."), mcp.Required()),
			mcp.WithArray("option_ids", mcp.Description("Optional global option IDs (bc/store/productOption/{id})."),
				mcp.WithStringItems()),
			mcp.WithArray("option_numeric_ids", mcp.Description("Optional numeric option IDs."),
				mcp.WithNumberItems()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleRemove,
	})
}

func (t *LocalizationOptionOverridesTools) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	productID, channelID, locale, err := parseOverrideContext(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	snap, err := t.bc.GetProductOptionLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to get option overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"result":     snap,
		"api":        "GraphQL Admin product options overridesForLocale",
	})
}

func (t *LocalizationOptionOverridesTools) handleSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, channelID, locale, err := parseOverrideContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	entries, err := parseOptionOverrideEntries(args["options"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(entries) == 0 {
		return toolError("options is required and must not be empty"), nil
	}

	in := bigcommerce.SetProductOptionOverridesInput{
		ProductID: productID,
		Context:   bigcommerce.LocaleContext{ChannelID: channelID, Locale: locale},
		Options:   entries,
	}

	current, err := t.bc.GetProductOptionLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to fetch current option overrides for preview: %v", err), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":      "pending_confirmation",
			"product_id":  productID,
			"channel_id":  channelID,
			"locale":      locale,
			"current":     current,
			"would_apply": entries,
			"message": fmt.Sprintf(
				"Will set option overlays on product %d for channel %d / %s. Pass confirmed=true to apply.",
				productID, channelID, locale,
			),
			"api": "GraphQL Admin product setProductOptionsInformation",
		})
	}

	updated, err := t.bc.SetProductOptionLocaleOverrides(ctx, in)
	if err != nil {
		return toolError("failed to set option overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"status":     "updated",
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"result":     updated,
		"api":        "GraphQL Admin product setProductOptionsInformation",
	})
}

func (t *LocalizationOptionOverridesTools) handleRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, channelID, locale, err := parseOverrideContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	in := bigcommerce.RemoveProductOptionOverridesInput{
		ProductID: productID,
		Context:   bigcommerce.LocaleContext{ChannelID: channelID, Locale: locale},
	}
	if v, ok := args["option_ids"]; ok && v != nil {
		ids, err := parseStringSlice(v, "option_ids")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		in.OptionIDs = ids
	}
	if v, ok := args["option_numeric_ids"]; ok && v != nil {
		nums, err := parsePositiveIntSlice(v, "option_numeric_ids")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		in.OptionNumericIDs = nums
	}

	current, err := t.bc.GetProductOptionLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to fetch current option overrides for preview: %v", err), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":             "pending_confirmation",
			"product_id":         productID,
			"channel_id":         channelID,
			"locale":             locale,
			"option_ids":         in.OptionIDs,
			"option_numeric_ids": in.OptionNumericIDs,
			"current":            current,
			"message": fmt.Sprintf(
				"Will remove option overlays on product %d for channel %d / %s. Pass confirmed=true to apply.",
				productID, channelID, locale,
			),
			"api": "GraphQL Admin product removeProductOptionsOverrides",
		})
	}

	updated, err := t.bc.RemoveProductOptionLocaleOverrides(ctx, in)
	if err != nil {
		return toolError("failed to remove option overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"status":     "removed",
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"result":     updated,
		"api":        "GraphQL Admin product removeProductOptionsOverrides",
	})
}

func parseOptionOverrideEntries(raw any) ([]bigcommerce.ProductOptionOverrideEntry, error) {
	if raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("options must be an array")
	}
	out := make([]bigcommerce.ProductOptionOverrideEntry, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("options[%d] must be an object", i)
		}
		entry := bigcommerce.ProductOptionOverrideEntry{}
		if v, ok := m["option_id"]; ok && v != nil {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("options[%d].option_id must be a string", i)
			}
			entry.OptionID = s
		}
		if v, ok := m["option_numeric_id"]; ok && v != nil {
			n, err := argPositiveInt("option_numeric_id", v)
			if err != nil {
				return nil, fmt.Errorf("options[%d]: %w", i, err)
			}
			entry.OptionNumericID = n
		}
		if s, ok, err := optionalStringPtr(m, "display_name"); err != nil {
			return nil, fmt.Errorf("options[%d]: %w", i, err)
		} else if ok {
			entry.DisplayName = s
		}
		if v, ok := m["values"]; ok && v != nil {
			vals, err := parseOptionOverrideValues(v, i)
			if err != nil {
				return nil, err
			}
			entry.Values = vals
		}
		out = append(out, entry)
	}
	return out, nil
}

func parseOptionOverrideValues(raw any, optionIdx int) ([]bigcommerce.ProductOptionOverrideValue, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("options[%d].values must be an array", optionIdx)
	}
	out := make([]bigcommerce.ProductOptionOverrideValue, 0, len(arr))
	for j, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("options[%d].values[%d] must be an object", optionIdx, j)
		}
		val := bigcommerce.ProductOptionOverrideValue{}
		if v, ok := m["value_id"]; ok && v != nil {
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("options[%d].values[%d].value_id must be a string", optionIdx, j)
			}
			val.ValueID = s
		}
		if v, ok := m["value_numeric_id"]; ok && v != nil {
			n, err := argPositiveInt("value_numeric_id", v)
			if err != nil {
				return nil, fmt.Errorf("options[%d].values[%d]: %w", optionIdx, j, err)
			}
			val.ValueNumericID = n
		}
		s, ok, err := optionalStringPtr(m, "label")
		if err != nil {
			return nil, fmt.Errorf("options[%d].values[%d]: %w", optionIdx, j, err)
		}
		if !ok || s == nil {
			return nil, fmt.Errorf("options[%d].values[%d].label is required", optionIdx, j)
		}
		val.Label = *s
		out = append(out, val)
	}
	return out, nil
}

func parsePositiveIntSlice(v any, field string) ([]int, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", field)
	}
	out := make([]int, 0, len(arr))
	for i, item := range arr {
		n, err := argPositiveInt(fmt.Sprintf("%s[%d]", field, i), item)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}
