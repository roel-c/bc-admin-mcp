package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ProductCustomFieldLocaleSnapshot is global custom fields plus locale overlays.
type ProductCustomFieldLocaleSnapshot struct {
	ProductID    string                          `json:"product_id"`
	CustomFields []ProductCustomFieldWithOverlay `json:"custom_fields"`
}

// ProductCustomFieldWithOverlay is one custom field and its locale override.
type ProductCustomFieldWithOverlay struct {
	ID        string                           `json:"id"`
	Name      string                           `json:"name"`
	Value     string                           `json:"value"`
	Overrides *ProductCustomFieldLocaleOverlay `json:"overrides_for_locale"`
}

// ProductCustomFieldLocaleOverlay holds locale-specific custom field attributes.
type ProductCustomFieldLocaleOverlay struct {
	Name      *string `json:"name"`
	Value     *string `json:"value"`
	IsVisible *bool   `json:"is_visible"`
}

// ProductCustomFieldOverrideEntry is one custom-field overlay to set.
type ProductCustomFieldOverrideEntry struct {
	CustomFieldID        string
	CustomFieldNumericID int
	Name                 *string
	Value                *string
	IsVisible            *bool
}

// SetProductCustomFieldOverridesInput applies custom-field overlays for a channel locale.
type SetProductCustomFieldOverridesInput struct {
	ProductID int
	Context   LocaleContext
	Fields    []ProductCustomFieldOverrideEntry
}

// Custom field override attribute names for remove mutations.
const (
	CustomFieldAttrName      = "NAME"
	CustomFieldAttrValue     = "VALUE"
	CustomFieldAttrIsVisible = "IS_VISIBLE"
)

// AllCustomFieldOverrideAttrs clears name, value, and visibility overlays.
var AllCustomFieldOverrideAttrs = []string{
	CustomFieldAttrName,
	CustomFieldAttrValue,
	CustomFieldAttrIsVisible,
}

// RemoveProductCustomFieldOverridesInput clears custom-field overlays for a locale.
// When field IDs are empty, all custom fields on the product are cleared for the locale.
type RemoveProductCustomFieldOverridesInput struct {
	ProductID             int
	Context               LocaleContext
	CustomFieldIDs        []string
	CustomFieldNumericIDs []int
	Attributes            []string // NAME, VALUE, IS_VISIBLE; empty = all
}

// GetProductCustomFieldLocaleOverrides returns global custom fields + locale overlays.
func (c *Client) GetProductCustomFieldLocaleOverrides(ctx context.Context, productID, channelID int, locale string) (*ProductCustomFieldLocaleSnapshot, error) {
	if err := validateOverrideContext(productID, LocaleContext{ChannelID: channelID, Locale: locale}); err != nil {
		return nil, err
	}
	const q = `
query($productId: ID!, $channelId: ID!, $locale: String!) {
  store {
    product(id: $productId) {
      id
      customFields {
        edges {
          node {
            id
            name
            value
            overrides(context: { channelId: $channelId, locale: $locale }) {
              edges {
                node {
                  ... on ProductCustomFieldOverridesForChannelLocale {
                    name
                    value
                    isVisible
                    context { channelId locale }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, q, map[string]any{
		"productId": FormatProductGID(productID),
		"channelId": FormatChannelGID(channelID),
		"locale":    strings.TrimSpace(locale),
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Store struct {
			Product *struct {
				ID           string `json:"id"`
				CustomFields struct {
					Edges []struct {
						Node gqlCustomFieldNode `json:"node"`
					} `json:"edges"`
				} `json:"customFields"`
			} `json:"product"`
		} `json:"store"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode product custom field overrides: %w", err)
	}
	if parsed.Store.Product == nil {
		return nil, fmt.Errorf("product %d not found", productID)
	}
	out := &ProductCustomFieldLocaleSnapshot{ProductID: parsed.Store.Product.ID}
	for _, e := range parsed.Store.Product.CustomFields.Edges {
		out.CustomFields = append(out.CustomFields, mapCustomFieldNode(e.Node))
	}
	if out.CustomFields == nil {
		out.CustomFields = []ProductCustomFieldWithOverlay{}
	}
	return out, nil
}

type gqlCustomFieldNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Value     string `json:"value"`
	Overrides struct {
		Edges []struct {
			Node *struct {
				Name      *string `json:"name"`
				Value     *string `json:"value"`
				IsVisible *bool   `json:"isVisible"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"overrides"`
}

func mapCustomFieldNode(n gqlCustomFieldNode) ProductCustomFieldWithOverlay {
	cf := ProductCustomFieldWithOverlay{
		ID:    n.ID,
		Name:  n.Name,
		Value: n.Value,
	}
	if len(n.Overrides.Edges) == 0 || n.Overrides.Edges[0].Node == nil {
		cf.Overrides = nil
		return cf
	}
	node := n.Overrides.Edges[0].Node
	cf.Overrides = &ProductCustomFieldLocaleOverlay{
		Name:      node.Name,
		Value:     node.Value,
		IsVisible: node.IsVisible,
	}
	return cf
}

// SetProductCustomFieldLocaleOverrides sets channel-locale overlays only (does not change global name/value).
func (c *Client) SetProductCustomFieldLocaleOverrides(ctx context.Context, in SetProductCustomFieldOverridesInput) (*ProductCustomFieldLocaleSnapshot, error) {
	if err := validateOverrideContext(in.ProductID, in.Context); err != nil {
		return nil, err
	}
	if len(in.Fields) == 0 {
		return nil, fmt.Errorf("fields is required and must not be empty")
	}

	dataItems := make([]map[string]any, 0, len(in.Fields))
	for i, f := range in.Fields {
		cfGID, err := resolveCustomFieldGID(f.CustomFieldID, f.CustomFieldNumericID)
		if err != nil {
			return nil, fmt.Errorf("fields[%d]: %w", i, err)
		}
		if f.Name == nil && f.Value == nil && f.IsVisible == nil {
			return nil, fmt.Errorf("fields[%d]: provide name, value, and/or is_visible", i)
		}
		ovData := map[string]any{}
		if f.Name != nil {
			ovData["name"] = *f.Name
		}
		if f.Value != nil {
			ovData["value"] = *f.Value
		}
		if f.IsVisible != nil {
			ovData["isVisible"] = *f.IsVisible
		}
		dataItems = append(dataItems, map[string]any{
			"customFieldId": cfGID,
			"overrides": []map[string]any{
				{
					"channelLocaleOverrides": map[string]any{
						"context": in.Context.toInput(),
						"data":    ovData,
					},
				},
			},
		})
	}

	const mut = `
mutation($input: UpdateProductCustomFieldsInput!) {
  product {
    updateProductCustomFields(input: $input) {
      product { id }
    }
  }
}`
	if err := c.runProductLocaleMutation(ctx, mut, map[string]any{
		"productId": FormatProductGID(in.ProductID),
		"data":      dataItems,
	}); err != nil {
		return nil, err
	}
	return c.GetProductCustomFieldLocaleOverrides(ctx, in.ProductID, in.Context.ChannelID, in.Context.Locale)
}

// RemoveProductCustomFieldLocaleOverrides clears custom-field overlays for a locale.
func (c *Client) RemoveProductCustomFieldLocaleOverrides(ctx context.Context, in RemoveProductCustomFieldOverridesInput) (*ProductCustomFieldLocaleSnapshot, error) {
	if err := validateOverrideContext(in.ProductID, in.Context); err != nil {
		return nil, err
	}

	snap, err := c.GetProductCustomFieldLocaleOverrides(ctx, in.ProductID, in.Context.ChannelID, in.Context.Locale)
	if err != nil {
		return nil, err
	}

	wanted := map[string]bool{}
	for _, id := range in.CustomFieldIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = true
		}
	}
	for _, n := range in.CustomFieldNumericIDs {
		if n > 0 {
			wanted[FormatProductCustomFieldGID(n)] = true
		}
	}
	filter := len(wanted) > 0

	attrs := in.Attributes
	if len(attrs) == 0 {
		attrs = append([]string{}, AllCustomFieldOverrideAttrs...)
	}
	for i, a := range attrs {
		attrs[i] = strings.ToUpper(strings.TrimSpace(a))
		switch attrs[i] {
		case CustomFieldAttrName, CustomFieldAttrValue, CustomFieldAttrIsVisible:
		default:
			return nil, fmt.Errorf("unknown custom field attribute %q; valid: NAME, VALUE, IS_VISIBLE", a)
		}
	}

	dataItems := make([]map[string]any, 0)
	for _, cf := range snap.CustomFields {
		if filter && !wanted[cf.ID] {
			continue
		}
		if !filter && cf.Overrides == nil {
			continue
		}
		item := map[string]any{
			"customFieldId": cf.ID,
			"channelLocaleContextData": map[string]any{
				"context":    in.Context.toInput(),
				"attributes": attrs,
			},
		}
		dataItems = append(dataItems, item)
	}
	if filter && len(dataItems) == 0 {
		return nil, fmt.Errorf("no matching custom fields found to remove")
	}
	if len(dataItems) == 0 {
		return snap, nil
	}

	const mut = `
mutation($input: RemoveProductCustomFieldsOverridesInput!) {
  product {
    removeProductCustomFieldsOverrides(input: $input) {
      product { id }
    }
  }
}`
	if err := c.runProductLocaleMutation(ctx, mut, map[string]any{
		"productId": FormatProductGID(in.ProductID),
		"data":      dataItems,
	}); err != nil {
		return nil, err
	}
	return c.GetProductCustomFieldLocaleOverrides(ctx, in.ProductID, in.Context.ChannelID, in.Context.Locale)
}

func resolveCustomFieldGID(id string, numericID int) (string, error) {
	id = strings.TrimSpace(id)
	if id != "" {
		return id, nil
	}
	if numericID > 0 {
		return FormatProductCustomFieldGID(numericID), nil
	}
	return "", fmt.Errorf("custom_field_id or custom_field_numeric_id is required")
}
