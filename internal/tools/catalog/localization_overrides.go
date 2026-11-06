package catalog

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// LocalizationOverridesTools manages MSF International Enhancements product overlays.
type LocalizationOverridesTools struct {
	bc BigCommerceAPI
}

// NewLocalizationOverridesTools constructs override tools.
func NewLocalizationOverridesTools(bc BigCommerceAPI) *LocalizationOverridesTools {
	return &LocalizationOverridesTools{bc: bc}
}

// RegisterTools registers catalog/localization/overrides/* tools.
func (t *LocalizationOverridesTools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/get",
		Tier:    middleware.TierR0,
		Summary: "Get product locale/channel overrides for a PDP context",
		Description: "Reads global product basic info plus overridesForLocale / overridesForChannel " +
			"via GraphQL Admin (MSF International Enhancements).",
		Tool: mcp.NewTool("catalog_localization_overrides_get",
			mcp.WithDescription(
				"Get product locale overrides for a channel+locale (name, description, SEO, URL, storefront details, featured).",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel (e.g. en, es, en-CA)."), mcp.Required()),
		),
		Handler: t.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/set",
		Tier:    middleware.TierR1,
		Summary: "Set product locale/channel PDP overrides (preview → confirm)",
		Description: "Sets MSF International Enhancement overlays for one product/channel/locale. " +
			"Optional fields: name, description, page_title, meta_description, url_path, warranty, " +
			"availability, search_keywords, preorder_message, is_featured. Server runs the needed mutations. " +
			"Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_overrides_set",
			mcp.WithDescription(
				"Set product locale overrides. Provide any subset of PDP overlay fields. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel."), mcp.Required()),
			mcp.WithString("name", mcp.Description("Override product name.")),
			mcp.WithString("description", mcp.Description("Override product description HTML.")),
			mcp.WithString("page_title", mcp.Description("Override SEO page title.")),
			mcp.WithString("meta_description", mcp.Description("Override SEO meta description.")),
			mcp.WithString("url_path", mcp.Description("Override product URL path (e.g. /my-product).")),
			mcp.WithString("warranty", mcp.Description("Override warranty text.")),
			mcp.WithString("availability", mcp.Description("Override availability text.")),
			mcp.WithString("search_keywords", mcp.Description("Override search keywords.")),
			mcp.WithString("preorder_message", mcp.Description("Override pre-order message.")),
			mcp.WithBoolean("is_featured", mcp.Description("Override featured flag for the channel (all locales).")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/overrides/remove",
		Tier:    middleware.TierR1,
		Summary: "Remove product locale/channel override groups (preview → confirm)",
		Description: "Clears override groups: basic, seo, url, storefront, preorder, featured. " +
			"Omit groups to clear all supported groups. Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_overrides_remove",
			mcp.WithDescription(
				"Remove product locale overrides by group. Omit groups to clear all. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("product_id", mcp.Description("BigCommerce product ID."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Locale code on the channel."), mcp.Required()),
			mcp.WithArray("groups", mcp.Description("Override groups to clear: basic, seo, url, storefront, preorder, featured."),
				mcp.WithStringItems()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleRemove,
	})
}

func (t *LocalizationOverridesTools) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	productID, channelID, locale, err := parseOverrideContext(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	snap, err := t.bc.GetProductLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to get overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"result":     snap,
		"api":        "GraphQL Admin product overridesForLocale",
	})
}

func (t *LocalizationOverridesTools) handleSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, channelID, locale, err := parseOverrideContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	in := bigcommerce.ProductLocaleOverrideInput{
		ProductID: productID,
		Context:   bigcommerce.LocaleContext{ChannelID: channelID, Locale: locale},
	}
	hasField := false
	if s, ok, err := optionalStringPtr(args, "name"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.Name = s
		hasField = true
	}
	if s, ok, err := optionalStringPtr(args, "description"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.Description = s
		hasField = true
	}
	if s, ok, err := optionalStringPtr(args, "page_title"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.PageTitle = s
		hasField = true
	}
	if s, ok, err := optionalStringPtr(args, "meta_description"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.MetaDescription = s
		hasField = true
	}
	if s, ok, err := optionalStringPtr(args, "url_path"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.URLPath = s
		hasField = true
	}
	if s, ok, err := optionalStringPtr(args, "warranty"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.Warranty = s
		hasField = true
	}
	if s, ok, err := optionalStringPtr(args, "availability"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.Availability = s
		hasField = true
	}
	if s, ok, err := optionalStringPtr(args, "search_keywords"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.SearchKeywords = s
		hasField = true
	}
	if s, ok, err := optionalStringPtr(args, "preorder_message"); err != nil {
		return toolError("%s", err.Error()), nil
	} else if ok {
		in.PreorderMessage = s
		hasField = true
	}
	if v, ok := args["is_featured"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return toolError("is_featured must be a boolean"), nil
		}
		in.IsFeatured = &b
		hasField = true
	}
	if !hasField {
		return toolError("provide at least one override field to set"), nil
	}

	current, err := t.bc.GetProductLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to fetch current overrides for preview: %v", err), nil
	}

	proposed := map[string]any{}
	if in.Name != nil {
		proposed["name"] = *in.Name
	}
	if in.Description != nil {
		proposed["description"] = *in.Description
	}
	if in.PageTitle != nil {
		proposed["page_title"] = *in.PageTitle
	}
	if in.MetaDescription != nil {
		proposed["meta_description"] = *in.MetaDescription
	}
	if in.URLPath != nil {
		proposed["url_path"] = *in.URLPath
	}
	if in.Warranty != nil {
		proposed["warranty"] = *in.Warranty
	}
	if in.Availability != nil {
		proposed["availability"] = *in.Availability
	}
	if in.SearchKeywords != nil {
		proposed["search_keywords"] = *in.SearchKeywords
	}
	if in.PreorderMessage != nil {
		proposed["preorder_message"] = *in.PreorderMessage
	}
	if in.IsFeatured != nil {
		proposed["is_featured"] = *in.IsFeatured
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":      "pending_confirmation",
			"product_id":  productID,
			"channel_id":  channelID,
			"locale":      locale,
			"current":     current,
			"would_apply": proposed,
			"message":     fmt.Sprintf("Will set locale overrides on product %d for channel %d / %s. Pass confirmed=true to apply.", productID, channelID, locale),
			"api":         "GraphQL Admin product set* overrides",
		})
	}

	updated, err := t.bc.SetProductLocaleOverrides(ctx, in)
	if err != nil {
		return toolError("failed to set overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"status":     "updated",
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"result":     updated,
		"api":        "GraphQL Admin product set* overrides",
	})
}

func (t *LocalizationOverridesTools) handleRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productID, channelID, locale, err := parseOverrideContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	var groups []string
	if v, ok := args["groups"]; ok && v != nil {
		groups, err = parseStringSlice(v, "groups")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
	}
	if len(groups) == 0 {
		groups = append([]string{}, bigcommerce.AllOverrideGroups...)
	}

	current, err := t.bc.GetProductLocaleOverrides(ctx, productID, channelID, locale)
	if err != nil {
		return toolError("failed to fetch current overrides for preview: %v", err), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"product_id": productID,
			"channel_id": channelID,
			"locale":     locale,
			"groups":     groups,
			"current":    current,
			"message":    fmt.Sprintf("Will remove override groups %v on product %d for channel %d / %s. Pass confirmed=true to apply.", groups, productID, channelID, locale),
			"api":        "GraphQL Admin product remove* overrides",
		})
	}

	updated, err := t.bc.RemoveProductLocaleOverrides(ctx, productID, bigcommerce.LocaleContext{
		ChannelID: channelID,
		Locale:    locale,
	}, groups)
	if err != nil {
		return toolError("failed to remove overrides for product %d: %v", productID, err), nil
	}
	return toolJSON(map[string]any{
		"status":     "removed",
		"product_id": productID,
		"channel_id": channelID,
		"locale":     locale,
		"groups":     groups,
		"result":     updated,
		"api":        "GraphQL Admin product remove* overrides",
	})
}

func parseOverrideContext(args map[string]any) (productID, channelID int, locale string, err error) {
	productID, err = argPositiveInt("product_id", args["product_id"])
	if err != nil {
		return 0, 0, "", err
	}
	channelID, err = argPositiveInt("channel_id", args["channel_id"])
	if err != nil {
		return 0, 0, "", err
	}
	locale, err = requiredStringArg(args, "locale")
	if err != nil {
		return 0, 0, "", err
	}
	return productID, channelID, locale, nil
}

func optionalStringPtr(args map[string]any, field string) (*string, bool, error) {
	v, ok := args[field]
	if !ok || v == nil {
		return nil, false, nil
	}
	s, ok := v.(string)
	if !ok {
		return nil, false, fmt.Errorf("%s must be a string", field)
	}
	// Allow empty string overrides (clear to empty display).
	out := s
	return &out, true, nil
}
