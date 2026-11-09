package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Translation resource types supported in v1 catalog localization tools.
const (
	TranslationResourceProducts            = "PRODUCTS"
	TranslationResourceCategories          = "CATEGORIES"
	TranslationResourceBrands              = "BRANDS"
	TranslationResourceProductOptions      = "PRODUCT_OPTIONS"
	TranslationResourceProductCustomFields = "PRODUCT_CUSTOM_FIELDS"
)

// MaxTranslationPageSize matches the Translations Admin API page limit.
const MaxTranslationPageSize = 50

// MaxTranslationUpdateEntities caps entities per update/delete tool call.
const MaxTranslationUpdateEntities = 20

// ValidTranslationResourceTypes lists accepted resource_type values.
var ValidTranslationResourceTypes = map[string]struct{}{
	TranslationResourceProducts:            {},
	TranslationResourceCategories:          {},
	TranslationResourceBrands:              {},
	TranslationResourceProductOptions:      {},
	TranslationResourceProductCustomFields: {},
}

// TranslationField is one translatable field on a resource.
type TranslationField struct {
	FieldName   string  `json:"field_name"`
	Original    string  `json:"original"`
	Translation *string `json:"translation"`
}

// TranslationResource is one resource's translation row.
type TranslationResource struct {
	ResourceID string             `json:"resource_id"`
	Fields     []TranslationField `json:"fields"`
	Cursor     string             `json:"cursor,omitempty"`
}

// TranslationListResult is a page of translations.
type TranslationListResult struct {
	Resources   []TranslationResource `json:"resources"`
	HasNextPage bool                  `json:"has_next_page"`
	EndCursor   string                `json:"end_cursor,omitempty"`
}

// TranslationListParams filters a translations query.
type TranslationListParams struct {
	ChannelID    int
	Locale       string
	ResourceType string
	ResourceIDs  []string // optional global IDs
	First        int      // 1–50
	After        string   // optional cursor
}

// ListTranslations queries Admin GraphQL translations for a channel locale.
func (c *Client) ListTranslations(ctx context.Context, p TranslationListParams) (*TranslationListResult, error) {
	if err := validateTranslationContext(p.ChannelID, p.Locale, p.ResourceType); err != nil {
		return nil, err
	}
	first := p.First
	if first <= 0 {
		first = MaxTranslationPageSize
	}
	if first > MaxTranslationPageSize {
		first = MaxTranslationPageSize
	}

	// resourceType is an enum — inject the validated value into the document
	// rather than guessing a filters input type name across schema versions.
	vars := map[string]any{
		"channelId": FormatChannelGID(p.ChannelID),
		"localeId":  FormatLocaleGID(p.Locale),
		"first":     first,
	}
	if strings.TrimSpace(p.After) != "" {
		vars["after"] = p.After
	}

	var q string
	if len(p.ResourceIDs) > 0 {
		vars["resourceIds"] = p.ResourceIDs
		q = fmt.Sprintf(`
query($channelId: ID!, $localeId: ID!, $first: Int!, $after: String, $resourceIds: [ID!]!) {
  store {
    translations(filters: {
      resourceType: %s
      channelId: $channelId
      localeId: $localeId
      resourceIds: $resourceIds
    }, first: $first, after: $after) {
      edges {
        cursor
        node {
          resourceId
          fields { fieldName original translation }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, p.ResourceType)
	} else {
		q = fmt.Sprintf(`
query($channelId: ID!, $localeId: ID!, $first: Int!, $after: String) {
  store {
    translations(filters: {
      resourceType: %s
      channelId: $channelId
      localeId: $localeId
    }, first: $first, after: $after) {
      edges {
        cursor
        node {
          resourceId
          fields { fieldName original translation }
        }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}`, p.ResourceType)
	}

	data, err := c.DoGraphQL(ctx, q, vars)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Store struct {
			Translations struct {
				Edges []struct {
					Cursor string `json:"cursor"`
					Node   struct {
						ResourceID string `json:"resourceId"`
						Fields     []struct {
							FieldName   string  `json:"fieldName"`
							Original    string  `json:"original"`
							Translation *string `json:"translation"`
						} `json:"fields"`
					} `json:"node"`
				} `json:"edges"`
				PageInfo struct {
					HasNextPage bool   `json:"hasNextPage"`
					EndCursor   string `json:"endCursor"`
				} `json:"pageInfo"`
			} `json:"translations"`
		} `json:"store"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode translations: %w", err)
	}

	out := &TranslationListResult{
		Resources:   make([]TranslationResource, 0, len(parsed.Store.Translations.Edges)),
		HasNextPage: parsed.Store.Translations.PageInfo.HasNextPage,
		EndCursor:   parsed.Store.Translations.PageInfo.EndCursor,
	}
	for _, e := range parsed.Store.Translations.Edges {
		fields := make([]TranslationField, 0, len(e.Node.Fields))
		for _, f := range e.Node.Fields {
			fields = append(fields, TranslationField{
				FieldName:   f.FieldName,
				Original:    f.Original,
				Translation: f.Translation,
			})
		}
		out.Resources = append(out.Resources, TranslationResource{
			ResourceID: e.Node.ResourceID,
			Fields:     fields,
			Cursor:     e.Cursor,
		})
	}
	return out, nil
}

// TranslationEntityUpdate is one resource's field upserts.
type TranslationEntityUpdate struct {
	ResourceID string
	Fields     map[string]string // fieldName -> value
}

// UpdateTranslationsInput upserts translations (non-default locales only).
type UpdateTranslationsInput struct {
	ChannelID    int
	Locale       string
	ResourceType string
	Entities     []TranslationEntityUpdate
}

// UpdateTranslations upserts translations via Admin GraphQL.
func (c *Client) UpdateTranslations(ctx context.Context, in UpdateTranslationsInput) error {
	if err := validateTranslationContext(in.ChannelID, in.Locale, in.ResourceType); err != nil {
		return err
	}
	if len(in.Entities) == 0 {
		return fmt.Errorf("at least one entity is required")
	}
	if len(in.Entities) > MaxTranslationUpdateEntities {
		return fmt.Errorf("at most %d entities per update", MaxTranslationUpdateEntities)
	}

	entities := make([]map[string]any, 0, len(in.Entities))
	for i, ent := range in.Entities {
		if strings.TrimSpace(ent.ResourceID) == "" {
			return fmt.Errorf("entities[%d].resource_id is required", i)
		}
		if len(ent.Fields) == 0 {
			return fmt.Errorf("entities[%d] must include at least one field", i)
		}
		fields := make([]map[string]any, 0, len(ent.Fields))
		for name, val := range ent.Fields {
			fields = append(fields, map[string]any{
				"fieldName": name,
				"value":     val,
			})
		}
		entities = append(entities, map[string]any{
			"resourceId": ent.ResourceID,
			"fields":     fields,
		})
	}

	const m = `
mutation($input: UpdateTranslationsInput!) {
  translation {
    updateTranslations(input: $input) {
      __typename
      errors {
        __typename
        ... on Error { message }
      }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, m, map[string]any{
		"input": map[string]any{
			"resourceType": in.ResourceType,
			"channelId":    FormatChannelGID(in.ChannelID),
			"localeId":     FormatLocaleGID(in.Locale),
			"entities":     entities,
		},
	})
	if err != nil {
		return err
	}
	return decodeTranslationMutationErrors(data, "updateTranslations")
}

// TranslationEntityDelete deletes named fields on a resource.
type TranslationEntityDelete struct {
	ResourceID string
	Fields     []string // field names to clear
}

// DeleteTranslationsInput deletes translation fields.
type DeleteTranslationsInput struct {
	ChannelID    int
	Locale       string
	ResourceType string
	Resources    []TranslationEntityDelete
}

// DeleteTranslations deletes translation fields via Admin GraphQL.
func (c *Client) DeleteTranslations(ctx context.Context, in DeleteTranslationsInput) error {
	if err := validateTranslationContext(in.ChannelID, in.Locale, in.ResourceType); err != nil {
		return err
	}
	if len(in.Resources) == 0 {
		return fmt.Errorf("at least one resource is required")
	}
	if len(in.Resources) > MaxTranslationUpdateEntities {
		return fmt.Errorf("at most %d resources per delete", MaxTranslationUpdateEntities)
	}

	resources := make([]map[string]any, 0, len(in.Resources))
	for i, r := range in.Resources {
		if strings.TrimSpace(r.ResourceID) == "" {
			return fmt.Errorf("resources[%d].resource_id is required", i)
		}
		if len(r.Fields) == 0 {
			return fmt.Errorf("resources[%d] must include at least one field name", i)
		}
		resources = append(resources, map[string]any{
			"resourceId": r.ResourceID,
			"fields":     r.Fields,
		})
	}

	const m = `
mutation($input: DeleteTranslationsInput!) {
  translation {
    deleteTranslations(input: $input) {
      __typename
      errors {
        __typename
        ... on Error { message }
      }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, m, map[string]any{
		"input": map[string]any{
			"resourceType": in.ResourceType,
			"channelId":    FormatChannelGID(in.ChannelID),
			"localeId":     FormatLocaleGID(in.Locale),
			"resources":    resources,
		},
	})
	if err != nil {
		return err
	}
	return decodeTranslationMutationErrors(data, "deleteTranslations")
}

// DefaultLocaleCode returns the default locale code for a channel, if present.
func (c *Client) DefaultLocaleCode(ctx context.Context, channelID int) (string, error) {
	locales, err := c.ListChannelLocales(ctx, channelID)
	if err != nil {
		return "", err
	}
	for _, loc := range locales {
		if loc.IsDefault {
			return loc.Code, nil
		}
	}
	return "", fmt.Errorf("channel %d has no default locale", channelID)
}

func validateTranslationContext(channelID int, locale, resourceType string) error {
	if channelID <= 0 {
		return fmt.Errorf("channel_id must be a positive integer")
	}
	if strings.TrimSpace(locale) == "" {
		return fmt.Errorf("locale is required")
	}
	rt := strings.TrimSpace(resourceType)
	if rt == "" {
		return fmt.Errorf("resource_type is required")
	}
	if _, ok := ValidTranslationResourceTypes[rt]; !ok {
		return fmt.Errorf("unsupported resource_type %q", resourceType)
	}
	return nil
}

func decodeTranslationMutationErrors(data json.RawMessage, field string) error {
	var parsed struct {
		Translation map[string]struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"translation"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return fmt.Errorf("decode translation mutation: %w", err)
	}
	result, ok := parsed.Translation[field]
	if !ok {
		return nil
	}
	if msgs := mutationErrorMessages(result.Errors); len(msgs) > 0 {
		return &GraphQLError{Messages: msgs, Path: "translation." + field}
	}
	return nil
}

// FormatTranslationResourceID builds a global resource ID from type + numeric id.
func FormatTranslationResourceID(resourceType string, id int) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("resource id must be positive")
	}
	switch resourceType {
	case TranslationResourceProducts:
		return FormatProductGID(id), nil
	case TranslationResourceCategories:
		return FormatCategoryGID(id), nil
	case TranslationResourceBrands:
		return FormatBrandGID(id), nil
	case TranslationResourceProductOptions:
		// Live Admin translations use camelCase productOption IDs (same as MSF IE),
		// not the hyphenated product-option form in older docs.
		return FormatCatalogProductOptionGID(id), nil
	case TranslationResourceProductCustomFields:
		return FormatProductCustomFieldGID(id), nil
	default:
		return "", fmt.Errorf("unsupported resource_type %q", resourceType)
	}
}
