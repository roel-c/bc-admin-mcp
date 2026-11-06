package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// LocalizationLocalesTools manages channel locale configuration via GraphQL Admin.
type LocalizationLocalesTools struct {
	bc BigCommerceAPI
}

// NewLocalizationLocalesTools constructs locale tools.
func NewLocalizationLocalesTools(bc BigCommerceAPI) *LocalizationLocalesTools {
	return &LocalizationLocalesTools{bc: bc}
}

// RegisterTools registers catalog/localization/locales/* tools.
func (t *LocalizationLocalesTools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/locales/list",
		Tier:    middleware.TierR0,
		Summary: "List locales for a sales channel (optional platform-supported codes)",
		Description: "Lists channel locales via GraphQL Admin (code, status, is_default). " +
			"Pass include_supported=true to also return platform-supported locale codes. " +
			"Each storefront supports up to 5 locales.",
		Tool: mcp.NewTool("catalog_localization_locales_list",
			mcp.WithDescription(
				"List locales configured on a BigCommerce channel. Optional include_supported returns platform locale codes.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithBoolean("include_supported", mcp.Description("When true, also return platform-supported locale codes.")),
		),
		Handler: t.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/locales/add",
		Tier:    middleware.TierR1,
		Summary: "Add a locale to a channel (preview → confirm)",
		Description: "Adds a locale via GraphQL Admin locale.addLocale. " +
			"Status defaults to INACTIVE. Preview checks the 5-locale cap. Pass confirmed=true to apply.",
		Tool: mcp.NewTool("catalog_localization_locales_add",
			mcp.WithDescription(
				"Add a locale to a channel. Preview first; pass confirmed=true to apply. Max 5 locales per channel.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("code", mcp.Description("Locale code (e.g. fr, es, en-CA)."), mcp.Required()),
			mcp.WithString("status", mcp.Description("ACTIVE or INACTIVE (default INACTIVE).")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleAdd,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/locales/update",
		Tier:    middleware.TierR1,
		Summary: "Update a channel locale status or default flag (preview → confirm)",
		Description: "Updates a locale via GraphQL Admin locale.updateLocale. " +
			"Provide status and/or is_default. Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_locales_update",
			mcp.WithDescription(
				"Update a channel locale's status and/or is_default. Preview first; pass confirmed=true to apply.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("code", mcp.Description("Locale code to update."), mcp.Required()),
			mcp.WithString("status", mcp.Description("ACTIVE or INACTIVE.")),
			mcp.WithBoolean("is_default", mcp.Description("When true, make this locale the channel default.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/locales/delete",
		Tier:    middleware.TierR2,
		Summary: "Delete a non-default channel locale (preview → confirm)",
		Description: "Deletes a locale via GraphQL Admin locale.deleteLocale. " +
			"Fails closed if the locale is missing or is the channel default. Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_locales_delete",
			mcp.WithDescription(
				"Delete a non-default locale from a channel. Preview first; pass confirmed=true to apply.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("code", mcp.Description("Locale code to delete."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleDelete,
	})
}

func (t *LocalizationLocalesTools) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, err := argPositiveInt("channel_id", args["channel_id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	includeSupported := false
	if v, ok := args["include_supported"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return toolError("include_supported must be a boolean"), nil
		}
		includeSupported = b
	}

	locales, err := t.bc.ListChannelLocales(ctx, chID)
	if err != nil {
		return toolError("failed to list locales for channel %d: %v", chID, err), nil
	}

	out := map[string]any{
		"channel_id": chID,
		"total":      len(locales),
		"locales":    locales,
		"max_locales": bigcommerce.MaxChannelLocales,
		"api":        "GraphQL Admin store.locales",
	}
	if includeSupported {
		supported, err := t.bc.ListSupportedLocales(ctx)
		if err != nil {
			return toolError("failed to list supported locales: %v", err), nil
		}
		out["supported_locales"] = supported
		out["supported_total"] = len(supported)
	}
	return toolJSON(out)
}

func (t *LocalizationLocalesTools) handleAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, err := argPositiveInt("channel_id", args["channel_id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	code, err := requiredStringArg(args, "code")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	status := bigcommerce.LocaleStatusInactive
	if v, ok := args["status"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return toolError("status must be a string"), nil
		}
		s = strings.ToUpper(strings.TrimSpace(s))
		if s != bigcommerce.LocaleStatusActive && s != bigcommerce.LocaleStatusInactive {
			return toolError("status must be ACTIVE or INACTIVE"), nil
		}
		status = s
	}

	current, err := t.bc.ListChannelLocales(ctx, chID)
	if err != nil {
		return toolError("failed to list locales for preview: %v", err), nil
	}
	for _, loc := range current {
		if strings.EqualFold(loc.Code, code) {
			return toolError("locale %q already exists on channel %d", code, chID), nil
		}
	}
	if len(current) >= bigcommerce.MaxChannelLocales {
		return toolError("channel %d already has %d locales (max %d)", chID, len(current), bigcommerce.MaxChannelLocales), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":           "pending_confirmation",
			"channel_id":       chID,
			"current_locales":  current,
			"would_add":        map[string]any{"code": code, "status": status},
			"message":          fmt.Sprintf("Will add locale %s (%s) to channel %d. Pass confirmed=true to apply.", code, status, chID),
			"api":              "GraphQL Admin locale.addLocale",
		})
	}

	added, err := t.bc.AddLocale(ctx, bigcommerce.AddLocaleInput{
		ChannelID: chID,
		Code:      code,
		Status:    status,
	})
	if err != nil {
		return toolError("failed to add locale %s on channel %d: %v", code, chID, err), nil
	}
	return toolJSON(map[string]any{
		"status":  "added",
		"locale":  added,
		"channel_id": chID,
		"api":     "GraphQL Admin locale.addLocale",
	})
}

func (t *LocalizationLocalesTools) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, err := argPositiveInt("channel_id", args["channel_id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	code, err := requiredStringArg(args, "code")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	in := bigcommerce.UpdateLocaleInput{ChannelID: chID, Code: code}
	hasChange := false
	if v, ok := args["status"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return toolError("status must be a string"), nil
		}
		s = strings.ToUpper(strings.TrimSpace(s))
		if s != bigcommerce.LocaleStatusActive && s != bigcommerce.LocaleStatusInactive {
			return toolError("status must be ACTIVE or INACTIVE"), nil
		}
		in.Status = s
		hasChange = true
	}
	if v, ok := args["is_default"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return toolError("is_default must be a boolean"), nil
		}
		in.IsDefault = &b
		hasChange = true
	}
	if !hasChange {
		return toolError("at least one of status or is_default must be provided"), nil
	}

	current, err := t.findLocale(ctx, chID, code)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	would := map[string]any{
		"code":       current.Code,
		"status":     current.Status,
		"is_default": current.IsDefault,
	}
	if in.Status != "" {
		would["status"] = in.Status
	}
	if in.IsDefault != nil {
		would["is_default"] = *in.IsDefault
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":      "pending_confirmation",
			"channel_id":  chID,
			"current":     current,
			"would_apply": would,
			"message":     fmt.Sprintf("Will update locale %s on channel %d. Pass confirmed=true to apply.", code, chID),
			"api":         "GraphQL Admin locale.updateLocale",
		})
	}

	updated, err := t.bc.UpdateLocale(ctx, in)
	if err != nil {
		return toolError("failed to update locale %s on channel %d: %v", code, chID, err), nil
	}
	return toolJSON(map[string]any{
		"status":     "updated",
		"locale":     updated,
		"channel_id": chID,
		"api":        "GraphQL Admin locale.updateLocale",
	})
}

func (t *LocalizationLocalesTools) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, err := argPositiveInt("channel_id", args["channel_id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	code, err := requiredStringArg(args, "code")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	current, err := t.findLocale(ctx, chID, code)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if current.IsDefault {
		return toolError("cannot delete default locale %q on channel %d; set another default first", code, chID), nil
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":     "pending_confirmation",
			"channel_id": chID,
			"current":    current,
			"message":    fmt.Sprintf("Will delete locale %s from channel %d. Pass confirmed=true to apply.", code, chID),
			"api":        "GraphQL Admin locale.deleteLocale",
		})
	}

	deleted, err := t.bc.DeleteLocale(ctx, chID, code)
	if err != nil {
		return toolError("failed to delete locale %s on channel %d: %v", code, chID, err), nil
	}
	return toolJSON(map[string]any{
		"status":     "deleted",
		"code":       deleted,
		"channel_id": chID,
		"api":        "GraphQL Admin locale.deleteLocale",
	})
}

func (t *LocalizationLocalesTools) findLocale(ctx context.Context, channelID int, code string) (*bigcommerce.ChannelLocale, error) {
	locales, err := t.bc.ListChannelLocales(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to list locales for preview: %w", err)
	}
	for i := range locales {
		if strings.EqualFold(locales[i].Code, code) {
			return &locales[i], nil
		}
	}
	return nil, fmt.Errorf("locale %q not found on channel %d", code, channelID)
}

func requiredStringArg(args map[string]any, field string) (string, error) {
	v, ok := args[field]
	if !ok || v == nil {
		return "", fmt.Errorf("%s is required", field)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", field)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%s must not be empty", field)
	}
	return s, nil
}
