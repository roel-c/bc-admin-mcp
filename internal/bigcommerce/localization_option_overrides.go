package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ProductOptionLocaleSnapshot is global option info plus locale overlays.
type ProductOptionLocaleSnapshot struct {
	ProductID string                     `json:"product_id"`
	Options   []ProductOptionWithOverlay `json:"options"`
}

// ProductOptionWithOverlay is one variant option and its locale override (if any).
type ProductOptionWithOverlay struct {
	ID          string                      `json:"id"`
	DisplayName string                      `json:"display_name"`
	IsShared    bool                        `json:"is_shared"`
	Values      []ProductOptionLocaleValue  `json:"values"`
	Overrides   *ProductOptionLocaleOverlay `json:"overrides_for_locale"`
}

// ProductOptionLocaleValue is an option value label in GraphQL Admin (global ID form).
type ProductOptionLocaleValue struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// ProductOptionLocaleOverlay holds locale-specific option display fields.
type ProductOptionLocaleOverlay struct {
	DisplayName *string                    `json:"display_name"`
	Values      []ProductOptionLocaleValue `json:"values"`
}

// ProductOptionOverrideValue is one value label to set for a locale.
type ProductOptionOverrideValue struct {
	ValueID        string // global ID or empty when ValueNumericID set
	ValueNumericID int
	Label          string
}

// ProductOptionOverrideEntry is one option overlay to set for a locale.
type ProductOptionOverrideEntry struct {
	OptionID        string // global ID or empty when OptionNumericID set
	OptionNumericID int
	DisplayName     *string
	Values          []ProductOptionOverrideValue
}

// SetProductOptionOverridesInput applies option label overlays for a channel locale.
type SetProductOptionOverridesInput struct {
	ProductID int
	Context   LocaleContext
	Options   []ProductOptionOverrideEntry
}

// RemoveProductOptionOverridesInput clears option overlays for a channel locale.
// When OptionIDs / OptionNumericIDs are empty, all non-shared options with overlays are cleared.
type RemoveProductOptionOverridesInput struct {
	ProductID        int
	Context          LocaleContext
	OptionIDs        []string
	OptionNumericIDs []int
}

const dropdownDisplayNameField = "DROPDOWN_PRODUCT_OPTION_DISPLAY_NAME_FIELD"

// GetProductOptionLocaleOverrides returns global options + locale overlays.
func (c *Client) GetProductOptionLocaleOverrides(ctx context.Context, productID, channelID int, locale string) (*ProductOptionLocaleSnapshot, error) {
	if err := validateOverrideContext(productID, LocaleContext{ChannelID: channelID, Locale: locale}); err != nil {
		return nil, err
	}
	const q = `
query($productId: ID!, $channelId: ID!, $locale: String!) {
  store {
    product(id: $productId) {
      id
      options(first: 50) {
        edges {
          node {
            id
            displayName
            isShared
            values { id label }
            overridesForLocale(localeContext: { channelId: $channelId, locale: $locale }) {
              displayName
              values { id label }
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
				ID      string `json:"id"`
				Options struct {
					Edges []struct {
						Node gqlOptionNode `json:"node"`
					} `json:"edges"`
				} `json:"options"`
			} `json:"product"`
		} `json:"store"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode product option overrides: %w", err)
	}
	if parsed.Store.Product == nil {
		return nil, fmt.Errorf("product %d not found", productID)
	}
	out := &ProductOptionLocaleSnapshot{ProductID: parsed.Store.Product.ID}
	for _, e := range parsed.Store.Product.Options.Edges {
		out.Options = append(out.Options, mapOptionNode(e.Node))
	}
	if out.Options == nil {
		out.Options = []ProductOptionWithOverlay{}
	}
	return out, nil
}

type gqlOptionNode struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	IsShared    bool   `json:"isShared"`
	Values      []struct {
		ID    string `json:"id"`
		Label string `json:"label"`
	} `json:"values"`
	OverridesForLocale *struct {
		DisplayName *string `json:"displayName"`
		Values      []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"values"`
	} `json:"overridesForLocale"`
}

func mapOptionNode(n gqlOptionNode) ProductOptionWithOverlay {
	opt := ProductOptionWithOverlay{
		ID:          n.ID,
		DisplayName: n.DisplayName,
		IsShared:    n.IsShared,
		Values:      make([]ProductOptionLocaleValue, 0, len(n.Values)),
	}
	for _, v := range n.Values {
		opt.Values = append(opt.Values, ProductOptionLocaleValue{ID: v.ID, Label: v.Label})
	}
	if n.OverridesForLocale == nil {
		opt.Overrides = &ProductOptionLocaleOverlay{Values: []ProductOptionLocaleValue{}}
		return opt
	}
	ol := &ProductOptionLocaleOverlay{
		DisplayName: n.OverridesForLocale.DisplayName,
		Values:      make([]ProductOptionLocaleValue, 0, len(n.OverridesForLocale.Values)),
	}
	for _, v := range n.OverridesForLocale.Values {
		ol.Values = append(ol.Values, ProductOptionLocaleValue{ID: v.ID, Label: v.Label})
	}
	opt.Overrides = ol
	return opt
}

// SetProductOptionLocaleOverrides sets dropdown option display name/value labels for a locale.
func (c *Client) SetProductOptionLocaleOverrides(ctx context.Context, in SetProductOptionOverridesInput) (*ProductOptionLocaleSnapshot, error) {
	if err := validateOverrideContext(in.ProductID, in.Context); err != nil {
		return nil, err
	}
	if len(in.Options) == 0 {
		return nil, fmt.Errorf("options is required and must not be empty")
	}

	optionInputs := make([]map[string]any, 0, len(in.Options))
	for i, opt := range in.Options {
		optionGID, err := resolveCatalogOptionGID(opt.OptionID, opt.OptionNumericID)
		if err != nil {
			return nil, fmt.Errorf("options[%d]: %w", i, err)
		}
		dropdown := map[string]any{}
		if opt.DisplayName != nil {
			dropdown["displayName"] = *opt.DisplayName
		}
		if len(opt.Values) > 0 {
			vals := make([]map[string]any, 0, len(opt.Values))
			for j, v := range opt.Values {
				valueGID, err := resolveCatalogOptionValueGID(v.ValueID, v.ValueNumericID)
				if err != nil {
					return nil, fmt.Errorf("options[%d].values[%d]: %w", i, j, err)
				}
				if strings.TrimSpace(v.Label) == "" {
					return nil, fmt.Errorf("options[%d].values[%d]: label is required", i, j)
				}
				vals = append(vals, map[string]any{
					"valueId": valueGID,
					"label":   v.Label,
				})
			}
			dropdown["values"] = vals
		}
		if len(dropdown) == 0 {
			return nil, fmt.Errorf("options[%d]: provide display_name and/or values", i)
		}
		optionInputs = append(optionInputs, map[string]any{
			"optionId": optionGID,
			"data":     map[string]any{"dropdown": dropdown},
		})
	}

	const mut = `
mutation($input: SetProductOptionsInformationInput!) {
  product {
    setProductOptionsInformation(input: $input) {
      product { id }
    }
  }
}`
	if err := c.runProductLocaleMutation(ctx, mut, map[string]any{
		"productId":     FormatProductGID(in.ProductID),
		"localeContext": in.Context.toInput(),
		"data":          map[string]any{"options": optionInputs},
	}); err != nil {
		return nil, err
	}
	return c.GetProductOptionLocaleOverrides(ctx, in.ProductID, in.Context.ChannelID, in.Context.Locale)
}

// RemoveProductOptionLocaleOverrides clears dropdown option overlays for a locale.
func (c *Client) RemoveProductOptionLocaleOverrides(ctx context.Context, in RemoveProductOptionOverridesInput) (*ProductOptionLocaleSnapshot, error) {
	if err := validateOverrideContext(in.ProductID, in.Context); err != nil {
		return nil, err
	}

	snap, err := c.GetProductOptionLocaleOverrides(ctx, in.ProductID, in.Context.ChannelID, in.Context.Locale)
	if err != nil {
		return nil, err
	}

	wanted := map[string]bool{}
	for _, id := range in.OptionIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = true
		}
	}
	for _, n := range in.OptionNumericIDs {
		if n > 0 {
			wanted[FormatCatalogProductOptionGID(n)] = true
		}
	}
	filter := len(wanted) > 0

	optionInputs := make([]map[string]any, 0)
	for _, opt := range snap.Options {
		if opt.IsShared {
			continue
		}
		if filter && !wanted[opt.ID] {
			// Also accept numeric match if caller passed numeric-only filter that
			// doesn't equal the exact GID string (already normalized above).
			continue
		}
		if !filter && !optionHasOverlay(opt) {
			continue
		}
		valueIDs := make([]string, 0, len(opt.Values))
		for _, v := range opt.Values {
			valueIDs = append(valueIDs, v.ID)
		}
		dropdown := map[string]any{
			"fields": []string{dropdownDisplayNameField},
		}
		if len(valueIDs) > 0 {
			dropdown["values"] = map[string]any{"ids": valueIDs}
		}
		optionInputs = append(optionInputs, map[string]any{
			"optionId": opt.ID,
			"data":     map[string]any{"dropdown": dropdown},
		})
	}
	if filter && len(optionInputs) == 0 {
		return nil, fmt.Errorf("no matching product options found to remove")
	}
	if len(optionInputs) == 0 {
		return snap, nil
	}

	const mut = `
mutation($input: RemoveProductOptionsOverridesInput!) {
  product {
    removeProductOptionsOverrides(input: $input) {
      product { id }
    }
  }
}`
	if err := c.runProductLocaleMutation(ctx, mut, map[string]any{
		"productId":     FormatProductGID(in.ProductID),
		"localeContext": in.Context.toInput(),
		"data":          map[string]any{"options": optionInputs},
	}); err != nil {
		return nil, err
	}
	return c.GetProductOptionLocaleOverrides(ctx, in.ProductID, in.Context.ChannelID, in.Context.Locale)
}

func optionHasOverlay(opt ProductOptionWithOverlay) bool {
	if opt.Overrides == nil {
		return false
	}
	if opt.Overrides.DisplayName != nil && strings.TrimSpace(*opt.Overrides.DisplayName) != "" {
		return true
	}
	return len(opt.Overrides.Values) > 0
}

func resolveCatalogOptionGID(id string, numericID int) (string, error) {
	id = strings.TrimSpace(id)
	if id != "" {
		return id, nil
	}
	if numericID > 0 {
		return FormatCatalogProductOptionGID(numericID), nil
	}
	return "", fmt.Errorf("option_id or option_numeric_id is required")
}

func resolveCatalogOptionValueGID(id string, numericID int) (string, error) {
	id = strings.TrimSpace(id)
	if id != "" {
		return id, nil
	}
	if numericID > 0 {
		return FormatCatalogProductOptionValueGID(numericID), nil
	}
	return "", fmt.Errorf("value_id or value_numeric_id is required")
}

// ParseTrailingNumericID extracts the trailing integer from a BC global ID.
func ParseTrailingNumericID(gid string) (int, error) {
	gid = strings.TrimSpace(gid)
	if gid == "" {
		return 0, fmt.Errorf("empty global id")
	}
	i := strings.LastIndex(gid, "/")
	if i < 0 || i == len(gid)-1 {
		return 0, fmt.Errorf("invalid global id %q", gid)
	}
	n, err := strconv.Atoi(gid[i+1:])
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid numeric suffix in global id %q", gid)
	}
	return n, nil
}
