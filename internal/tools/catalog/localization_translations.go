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

// LocalizationTranslationsTools manages Translations Admin GraphQL for catalog resources.
type LocalizationTranslationsTools struct {
	bc BigCommerceAPI
}

// NewLocalizationTranslationsTools constructs translation tools.
func NewLocalizationTranslationsTools(bc BigCommerceAPI) *LocalizationTranslationsTools {
	return &LocalizationTranslationsTools{bc: bc}
}

// RegisterTools registers catalog/localization/translations/* tools.
func (t *LocalizationTranslationsTools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/translations/list",
		Tier:    middleware.TierR0,
		Summary: "List catalog translations for a channel locale",
		Description: "Lists translations via GraphQL Admin store.translations. " +
			"resource_type: PRODUCTS, CATEGORIES, BRANDS, PRODUCT_OPTIONS, PRODUCT_CUSTOM_FIELDS. " +
			"Page size capped at 50. Optional resource_ids (global IDs) or resource_numeric_ids.",
		Tool: mcp.NewTool("catalog_localization_translations_list",
			mcp.WithDescription(
				"List translations for a channel locale and resource type. Optional resource filters and cursor pagination.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Non-default or any configured locale code (e.g. es)."), mcp.Required()),
			mcp.WithString("resource_type", mcp.Description("PRODUCTS, CATEGORIES, BRANDS, PRODUCT_OPTIONS, or PRODUCT_CUSTOM_FIELDS."), mcp.Required()),
			mcp.WithArray("resource_ids", mcp.Description("Optional global resource IDs (e.g. bc/store/product/130)."), mcp.WithStringItems()),
			mcp.WithArray("resource_numeric_ids", mcp.Description("Optional numeric IDs; converted server-side for the resource_type."), mcp.WithNumberItems()),
			mcp.WithNumber("limit", mcp.Description("Page size 1–50 (default 50).")),
			mcp.WithString("after", mcp.Description("Pagination cursor from a previous response.")),
		),
		Handler: t.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/translations/update",
		Tier:    middleware.TierR1,
		Summary: "Upsert catalog field translations (preview → confirm)",
		Description: "Updates translations via GraphQL Admin translation.updateTranslations. " +
			"Rejects writes to the channel default locale. entities: [{resource_id|resource_numeric_id, fields:{name:value}}]. " +
			"Max 20 entities per call. Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_translations_update",
			mcp.WithDescription(
				"Upsert translations for catalog resources on a non-default locale. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Target locale code (must not be the channel default)."), mcp.Required()),
			mcp.WithString("resource_type", mcp.Description("PRODUCTS, CATEGORIES, BRANDS, PRODUCT_OPTIONS, or PRODUCT_CUSTOM_FIELDS."), mcp.Required()),
			mcp.WithArray("entities", mcp.Description(
				"Array of {resource_id or resource_numeric_id, fields:{field_name: value}} objects.",
			)),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/localization/translations/delete",
		Tier:    middleware.TierR1,
		Summary: "Delete catalog field translations (preview → confirm)",
		Description: "Deletes translation fields via GraphQL Admin translation.deleteTranslations. " +
			"Rejects default-locale writes. resources: [{resource_id|resource_numeric_id, fields:[...]}]. " +
			"Max 20 resources. Preview → confirmed=true.",
		Tool: mcp.NewTool("catalog_localization_translations_delete",
			mcp.WithDescription(
				"Delete translation fields for catalog resources on a non-default locale. Preview first; pass confirmed=true.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID."), mcp.Required()),
			mcp.WithString("locale", mcp.Description("Target locale code (must not be the channel default)."), mcp.Required()),
			mcp.WithString("resource_type", mcp.Description("PRODUCTS, CATEGORIES, BRANDS, PRODUCT_OPTIONS, or PRODUCT_CUSTOM_FIELDS."), mcp.Required()),
			mcp.WithArray("resources", mcp.Description(
				"Array of {resource_id or resource_numeric_id, fields:[field_name,...]} objects.",
			)),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to apply after reviewing the preview.")),
		),
		Handler: t.handleDelete,
	})
}

func (t *LocalizationTranslationsTools) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, locale, resourceType, err := parseTranslationContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	params := bigcommerce.TranslationListParams{
		ChannelID:    chID,
		Locale:       locale,
		ResourceType: resourceType,
	}
	if v, ok := args["limit"]; ok && v != nil {
		n, err := argPositiveInt("limit", v)
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		params.First = n
	}
	if v, ok := args["after"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return toolError("after must be a string"), nil
		}
		params.After = strings.TrimSpace(s)
	}

	resourceIDs, err := collectResourceIDs(args, resourceType)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	params.ResourceIDs = resourceIDs

	result, err := t.bc.ListTranslations(ctx, params)
	if err != nil {
		return toolError("failed to list translations: %v", err), nil
	}
	return toolJSON(map[string]any{
		"channel_id":    chID,
		"locale":        locale,
		"resource_type": resourceType,
		"total":         len(result.Resources),
		"resources":     result.Resources,
		"has_next_page": result.HasNextPage,
		"end_cursor":    result.EndCursor,
		"api":           "GraphQL Admin store.translations",
	})
}

func (t *LocalizationTranslationsTools) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, locale, resourceType, err := parseTranslationContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if err := t.rejectDefaultLocale(ctx, chID, locale); err != nil {
		return toolError("%s", err.Error()), nil
	}

	entities, err := parseTranslationEntities(args["entities"], resourceType)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(entities) == 0 {
		return toolError("entities must contain at least one item"), nil
	}
	if len(entities) > bigcommerce.MaxTranslationUpdateEntities {
		return toolError("at most %d entities per update", bigcommerce.MaxTranslationUpdateEntities), nil
	}

	// Preview: fetch current translations for these resource IDs.
	ids := make([]string, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.ResourceID)
	}
	current, err := t.bc.ListTranslations(ctx, bigcommerce.TranslationListParams{
		ChannelID:    chID,
		Locale:       locale,
		ResourceType: resourceType,
		ResourceIDs:  ids,
		First:        bigcommerce.MaxTranslationPageSize,
	})
	if err != nil {
		return toolError("failed to fetch current translations for preview: %v", err), nil
	}

	would := make([]map[string]any, 0, len(entities))
	for _, e := range entities {
		would = append(would, map[string]any{
			"resource_id": e.ResourceID,
			"fields":      e.Fields,
		})
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":        "pending_confirmation",
			"channel_id":    chID,
			"locale":        locale,
			"resource_type": resourceType,
			"current":       current.Resources,
			"would_apply":   would,
			"message":       fmt.Sprintf("Will upsert translations for %d %s on channel %d / %s. Pass confirmed=true to apply.", len(entities), resourceType, chID, locale),
			"api":           "GraphQL Admin translation.updateTranslations",
		})
	}

	if err := t.bc.UpdateTranslations(ctx, bigcommerce.UpdateTranslationsInput{
		ChannelID:    chID,
		Locale:       locale,
		ResourceType: resourceType,
		Entities:     entities,
	}); err != nil {
		return toolError("failed to update translations: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":        "updated",
		"channel_id":    chID,
		"locale":        locale,
		"resource_type": resourceType,
		"entity_count":  len(entities),
		"api":           "GraphQL Admin translation.updateTranslations",
	})
}

func (t *LocalizationTranslationsTools) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, locale, resourceType, err := parseTranslationContext(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if err := t.rejectDefaultLocale(ctx, chID, locale); err != nil {
		return toolError("%s", err.Error()), nil
	}

	resources, err := parseTranslationDeletes(args["resources"], resourceType)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(resources) == 0 {
		return toolError("resources must contain at least one item"), nil
	}
	if len(resources) > bigcommerce.MaxTranslationUpdateEntities {
		return toolError("at most %d resources per delete", bigcommerce.MaxTranslationUpdateEntities), nil
	}

	ids := make([]string, 0, len(resources))
	for _, r := range resources {
		ids = append(ids, r.ResourceID)
	}
	current, err := t.bc.ListTranslations(ctx, bigcommerce.TranslationListParams{
		ChannelID:    chID,
		Locale:       locale,
		ResourceType: resourceType,
		ResourceIDs:  ids,
		First:        bigcommerce.MaxTranslationPageSize,
	})
	if err != nil {
		return toolError("failed to fetch current translations for preview: %v", err), nil
	}

	would := make([]map[string]any, 0, len(resources))
	for _, r := range resources {
		would = append(would, map[string]any{
			"resource_id": r.ResourceID,
			"fields":      r.Fields,
		})
	}

	if !middleware.IsConfirmed(request) {
		return toolJSON(map[string]any{
			"status":        "pending_confirmation",
			"channel_id":    chID,
			"locale":        locale,
			"resource_type": resourceType,
			"current":       current.Resources,
			"would_delete":  would,
			"message":       fmt.Sprintf("Will delete translation fields for %d %s on channel %d / %s. Pass confirmed=true to apply.", len(resources), resourceType, chID, locale),
			"api":           "GraphQL Admin translation.deleteTranslations",
		})
	}

	if err := t.bc.DeleteTranslations(ctx, bigcommerce.DeleteTranslationsInput{
		ChannelID:    chID,
		Locale:       locale,
		ResourceType: resourceType,
		Resources:    resources,
	}); err != nil {
		return toolError("failed to delete translations: %v", err), nil
	}
	return toolJSON(map[string]any{
		"status":         "deleted",
		"channel_id":     chID,
		"locale":         locale,
		"resource_type":  resourceType,
		"resource_count": len(resources),
		"api":            "GraphQL Admin translation.deleteTranslations",
	})
}

func (t *LocalizationTranslationsTools) rejectDefaultLocale(ctx context.Context, channelID int, locale string) error {
	def, err := t.bc.DefaultLocaleCode(ctx, channelID)
	if err != nil {
		return fmt.Errorf("failed to resolve channel default locale: %w", err)
	}
	if strings.EqualFold(def, locale) {
		return fmt.Errorf("cannot write translations for default locale %q on channel %d; use catalog management APIs for default-locale content", def, channelID)
	}
	return nil
}

func parseTranslationContext(args map[string]any) (channelID int, locale, resourceType string, err error) {
	channelID, err = argPositiveInt("channel_id", args["channel_id"])
	if err != nil {
		return 0, "", "", err
	}
	locale, err = requiredStringArg(args, "locale")
	if err != nil {
		return 0, "", "", err
	}
	resourceType, err = requiredStringArg(args, "resource_type")
	if err != nil {
		return 0, "", "", err
	}
	resourceType = strings.ToUpper(resourceType)
	if _, ok := bigcommerce.ValidTranslationResourceTypes[resourceType]; !ok {
		return 0, "", "", fmt.Errorf("unsupported resource_type %q", resourceType)
	}
	return channelID, locale, resourceType, nil
}

func collectResourceIDs(args map[string]any, resourceType string) ([]string, error) {
	var out []string
	if v, ok := args["resource_ids"]; ok && v != nil {
		ids, err := parseStringSlice(v, "resource_ids")
		if err != nil {
			return nil, err
		}
		out = append(out, ids...)
	}
	if v, ok := args["resource_numeric_ids"]; ok && v != nil {
		nums, err := parseFloat64SliceToPositiveInts(v, "resource_numeric_ids")
		if err != nil {
			return nil, err
		}
		for _, n := range nums {
			gid, err := bigcommerce.FormatTranslationResourceID(resourceType, n)
			if err != nil {
				return nil, err
			}
			out = append(out, gid)
		}
	}
	return out, nil
}

func parseTranslationEntities(v any, resourceType string) ([]bigcommerce.TranslationEntityUpdate, error) {
	if v == nil {
		return nil, fmt.Errorf("entities is required")
	}
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("entities must be an array")
	}
	out := make([]bigcommerce.TranslationEntityUpdate, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("entities[%d] must be an object", i)
		}
		rid, err := resolveResourceID(m, resourceType, fmt.Sprintf("entities[%d]", i))
		if err != nil {
			return nil, err
		}
		fieldsRaw, ok := m["fields"]
		if !ok || fieldsRaw == nil {
			return nil, fmt.Errorf("entities[%d].fields is required", i)
		}
		fieldsMap, ok := fieldsRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("entities[%d].fields must be an object of field_name -> string", i)
		}
		fields := make(map[string]string, len(fieldsMap))
		for k, val := range fieldsMap {
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("entities[%d].fields[%s] must be a string", i, k)
			}
			fields[k] = s
		}
		if len(fields) == 0 {
			return nil, fmt.Errorf("entities[%d].fields must not be empty", i)
		}
		out = append(out, bigcommerce.TranslationEntityUpdate{
			ResourceID: rid,
			Fields:     fields,
		})
	}
	return out, nil
}

func parseTranslationDeletes(v any, resourceType string) ([]bigcommerce.TranslationEntityDelete, error) {
	if v == nil {
		return nil, fmt.Errorf("resources is required")
	}
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("resources must be an array")
	}
	out := make([]bigcommerce.TranslationEntityDelete, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("resources[%d] must be an object", i)
		}
		rid, err := resolveResourceID(m, resourceType, fmt.Sprintf("resources[%d]", i))
		if err != nil {
			return nil, err
		}
		fields, err := parseStringSlice(m["fields"], fmt.Sprintf("resources[%d].fields", i))
		if err != nil {
			return nil, err
		}
		if len(fields) == 0 {
			return nil, fmt.Errorf("resources[%d].fields must not be empty", i)
		}
		out = append(out, bigcommerce.TranslationEntityDelete{
			ResourceID: rid,
			Fields:     fields,
		})
	}
	return out, nil
}

func resolveResourceID(m map[string]any, resourceType, label string) (string, error) {
	if v, ok := m["resource_id"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("%s.resource_id must be a string", label)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return "", fmt.Errorf("%s.resource_id must not be empty", label)
		}
		return s, nil
	}
	if v, ok := m["resource_numeric_id"]; ok && v != nil {
		n, err := argPositiveInt("resource_numeric_id", v)
		if err != nil {
			return "", fmt.Errorf("%s.%s", label, err.Error())
		}
		return bigcommerce.FormatTranslationResourceID(resourceType, n)
	}
	return "", fmt.Errorf("%s requires resource_id or resource_numeric_id", label)
}
