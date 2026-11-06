package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// ProductLocaleOverrides is the resolved global + override snapshot for a product.
type ProductLocaleOverrides struct {
	ProductID           string                    `json:"product_id"`
	BasicInformation    *ProductBasicInformation  `json:"basic_information,omitempty"`
	OverridesForLocale  *LocaleProductOverrides   `json:"overrides_for_locale"`
	OverridesForChannel *ChannelProductOverrides  `json:"overrides_for_channel,omitempty"`
}

// ProductBasicInformation holds global product name/description.
type ProductBasicInformation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// LocaleProductOverrides holds per-locale overlay fields (null groups mean inherit).
type LocaleProductOverrides struct {
	BasicInformation  *ProductBasicInformation  `json:"basic_information"`
	SEOInformation    *ProductSEOInformation    `json:"seo_information"`
	URLPath           *ProductURLPath           `json:"url_path"`
	StorefrontDetails *ProductStorefrontDetails `json:"storefront_details"`
	PreOrderSettings  *ProductPreOrderSettings  `json:"pre_order_settings"`
}

// ChannelProductOverrides holds channel-scoped (all locales) overlays.
type ChannelProductOverrides struct {
	IsFeatured *bool `json:"is_featured"`
}

// ProductSEOInformation holds SEO overlay fields.
type ProductSEOInformation struct {
	PageTitle       string `json:"page_title"`
	MetaDescription string `json:"meta_description"`
}

// ProductURLPath holds a product URL path overlay.
type ProductURLPath struct {
	Path string `json:"path"`
}

// ProductStorefrontDetails holds storefront detail overlays.
type ProductStorefrontDetails struct {
	Warranty               string `json:"warranty"`
	AvailabilityDescription string `json:"availability"`
	SearchKeywords         string `json:"search_keywords"`
}

// ProductPreOrderSettings holds pre-order message overlay.
type ProductPreOrderSettings struct {
	Message string `json:"message"`
}

// ProductLocaleOverrideInput is the use-case payload for set/remove orchestration.
// Pointer/optional fields: only non-nil pointers (or set flags) are applied.
type ProductLocaleOverrideInput struct {
	ProductID int
	Context   LocaleContext

	Name        *string
	Description *string

	PageTitle       *string
	MetaDescription *string

	URLPath *string

	Warranty       *string
	Availability   *string
	SearchKeywords *string

	PreorderMessage *string

	IsFeatured *bool
}

// ProductLocaleOverrideGroups names removable field groups.
const (
	OverrideGroupBasic      = "basic"
	OverrideGroupSEO        = "seo"
	OverrideGroupURL        = "url"
	OverrideGroupStorefront = "storefront"
	OverrideGroupPreorder   = "preorder"
	OverrideGroupFeatured   = "featured"
)

// AllOverrideGroups is the default remove set when groups are omitted.
var AllOverrideGroups = []string{
	OverrideGroupBasic,
	OverrideGroupSEO,
	OverrideGroupURL,
	OverrideGroupStorefront,
	OverrideGroupPreorder,
	OverrideGroupFeatured,
}

// GetProductLocaleOverrides queries global + locale/channel overlays for a product.
func (c *Client) GetProductLocaleOverrides(ctx context.Context, productID, channelID int, locale string) (*ProductLocaleOverrides, error) {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return nil, fmt.Errorf("locale is required")
	}
	const q = `
query($ids: [ID!]!, $channelId: ID!, $locale: String!) {
  store {
    products(filters: {ids: $ids}) {
      edges {
        node {
          id
          basicInformation { name description }
          overridesForLocale(localeContext: { channelId: $channelId, locale: $locale }) {
            basicInformation { name description }
            seoInformation { pageTitle metaDescription }
            urlPath { path }
            storefrontDetails { warranty availabilityDescription searchKeywords }
            preOrderSettings { message }
          }
          overridesForChannel(channelId: $channelId) {
            isFeatured
          }
        }
      }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, q, map[string]any{
		"ids":       []string{FormatProductGID(productID)},
		"channelId": FormatChannelGID(channelID),
		"locale":    locale,
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Store struct {
			Products struct {
				Edges []struct {
					Node gqlProductOverrideNode `json:"node"`
				} `json:"edges"`
			} `json:"products"`
		} `json:"store"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode product locale overrides: %w", err)
	}
	if len(parsed.Store.Products.Edges) == 0 {
		return nil, fmt.Errorf("product %d not found", productID)
	}
	return mapProductOverrideNode(parsed.Store.Products.Edges[0].Node), nil
}

type gqlProductOverrideNode struct {
	ID               string `json:"id"`
	BasicInformation *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"basicInformation"`
	OverridesForLocale *struct {
		BasicInformation *struct {
			Name        *string `json:"name"`
			Description *string `json:"description"`
		} `json:"basicInformation"`
		SEOInformation *struct {
			PageTitle       *string `json:"pageTitle"`
			MetaDescription *string `json:"metaDescription"`
		} `json:"seoInformation"`
		URLPath *struct {
			Path *string `json:"path"`
		} `json:"urlPath"`
		StorefrontDetails *struct {
			Warranty                *string `json:"warranty"`
			AvailabilityDescription *string `json:"availabilityDescription"`
			SearchKeywords          *string `json:"searchKeywords"`
		} `json:"storefrontDetails"`
		PreOrderSettings *struct {
			Message *string `json:"message"`
		} `json:"preOrderSettings"`
	} `json:"overridesForLocale"`
	OverridesForChannel *struct {
		IsFeatured *bool `json:"isFeatured"`
	} `json:"overridesForChannel"`
}

func mapProductOverrideNode(n gqlProductOverrideNode) *ProductLocaleOverrides {
	out := &ProductLocaleOverrides{ProductID: n.ID}
	if n.BasicInformation != nil {
		out.BasicInformation = &ProductBasicInformation{
			Name:        n.BasicInformation.Name,
			Description: n.BasicInformation.Description,
		}
	}
	if n.OverridesForChannel != nil {
		out.OverridesForChannel = &ChannelProductOverrides{IsFeatured: n.OverridesForChannel.IsFeatured}
	}
	if n.OverridesForLocale == nil {
		out.OverridesForLocale = &LocaleProductOverrides{}
		return out
	}
	ol := &LocaleProductOverrides{}
	if b := n.OverridesForLocale.BasicInformation; b != nil {
		ol.BasicInformation = &ProductBasicInformation{
			Name:        derefStr(b.Name),
			Description: derefStr(b.Description),
		}
	}
	if s := n.OverridesForLocale.SEOInformation; s != nil {
		ol.SEOInformation = &ProductSEOInformation{
			PageTitle:       derefStr(s.PageTitle),
			MetaDescription: derefStr(s.MetaDescription),
		}
	}
	if u := n.OverridesForLocale.URLPath; u != nil {
		ol.URLPath = &ProductURLPath{Path: derefStr(u.Path)}
	}
	if sf := n.OverridesForLocale.StorefrontDetails; sf != nil {
		ol.StorefrontDetails = &ProductStorefrontDetails{
			Warranty:                derefStr(sf.Warranty),
			AvailabilityDescription: derefStr(sf.AvailabilityDescription),
			SearchKeywords:          derefStr(sf.SearchKeywords),
		}
	}
	if p := n.OverridesForLocale.PreOrderSettings; p != nil {
		ol.PreOrderSettings = &ProductPreOrderSettings{Message: derefStr(p.Message)}
	}
	out.OverridesForLocale = ol
	return out
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// SetProductLocaleOverrides applies the provided overlay fields sequentially.
func (c *Client) SetProductLocaleOverrides(ctx context.Context, in ProductLocaleOverrideInput) (*ProductLocaleOverrides, error) {
	if err := validateOverrideContext(in.ProductID, in.Context); err != nil {
		return nil, err
	}
	productGID := FormatProductGID(in.ProductID)
	localeCtx := in.Context.toInput()

	if in.Name != nil || in.Description != nil {
		data := map[string]any{}
		if in.Name != nil {
			data["name"] = *in.Name
		}
		if in.Description != nil {
			data["description"] = *in.Description
		}
		if err := c.runProductLocaleMutation(ctx, `
mutation($input: SetProductBasicInformationInput!) {
  product { setProductBasicInformation(input: $input) { product { id } } }
}`, map[string]any{
			"productId":     productGID,
			"localeContext": localeCtx,
			"data":          data,
		}); err != nil {
			return nil, err
		}
	}

	if in.PageTitle != nil || in.MetaDescription != nil {
		data := map[string]any{}
		if in.PageTitle != nil {
			data["pageTitle"] = *in.PageTitle
		}
		if in.MetaDescription != nil {
			data["metaDescription"] = *in.MetaDescription
		}
		if err := c.runProductLocaleMutation(ctx, `
mutation($input: SetProductSeoInformationInput!) {
  product { setProductSeoInformation(input: $input) { product { id } } }
}`, map[string]any{
			"productId":     productGID,
			"localeContext": localeCtx,
			"data":          data,
		}); err != nil {
			return nil, err
		}
	}

	if in.URLPath != nil {
		if err := c.runProductLocaleMutation(ctx, `
mutation($input: SetProductUrlPathInput!) {
  product { setProductUrlPath(input: $input) { product { id } } }
}`, map[string]any{
			"productId":     productGID,
			"localeContext": localeCtx,
			"data":          map[string]any{"path": *in.URLPath},
		}); err != nil {
			return nil, err
		}
	}

	if in.Warranty != nil || in.Availability != nil || in.SearchKeywords != nil {
		data := map[string]any{}
		if in.Warranty != nil {
			data["warranty"] = *in.Warranty
		}
		if in.Availability != nil {
			data["availabilityDescription"] = *in.Availability
		}
		if in.SearchKeywords != nil {
			data["searchKeywords"] = *in.SearchKeywords
		}
		if err := c.runProductLocaleMutation(ctx, `
mutation($input: SetProductStorefrontDetailsInput!) {
  product { setProductStorefrontDetails(input: $input) { product { id } } }
}`, map[string]any{
			"productId":     productGID,
			"localeContext": localeCtx,
			"data":          data,
		}); err != nil {
			return nil, err
		}
	}

	if in.PreorderMessage != nil {
		if err := c.runProductLocaleMutation(ctx, `
mutation($input: SetProductPreOrderSettingsInput!) {
  product { setProductPreOrderSettings(input: $input) { product { id } } }
}`, map[string]any{
			"productId":     productGID,
			"localeContext": localeCtx,
			"data":          map[string]any{"message": *in.PreorderMessage},
		}); err != nil {
			return nil, err
		}
	}

	if in.IsFeatured != nil {
		if err := c.runProductLocaleMutation(ctx, `
mutation($input: SetProductIsFeaturedInput!) {
  product { setProductIsFeatured(input: $input) { product { id } } }
}`, map[string]any{
			"productId": productGID,
			"channelId": FormatChannelGID(in.Context.ChannelID),
			"data":      map[string]any{"isFeatured": *in.IsFeatured},
		}); err != nil {
			return nil, err
		}
	}

	return c.GetProductLocaleOverrides(ctx, in.ProductID, in.Context.ChannelID, in.Context.Locale)
}

// RemoveProductLocaleOverrides clears the named groups (or all supported groups).
func (c *Client) RemoveProductLocaleOverrides(ctx context.Context, productID int, lc LocaleContext, groups []string) (*ProductLocaleOverrides, error) {
	if err := validateOverrideContext(productID, lc); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		groups = append([]string{}, AllOverrideGroups...)
	}
	productGID := FormatProductGID(productID)
	localeCtx := lc.toInput()

	for _, g := range groups {
		switch strings.ToLower(strings.TrimSpace(g)) {
		case OverrideGroupBasic:
			if err := c.runProductLocaleMutation(ctx, `
mutation($input: RemoveProductBasicInformationOverridesInput!) {
  product { removeProductBasicInformationOverrides(input: $input) { product { id } } }
}`, map[string]any{
				"productId":     productGID,
				"localeContext": localeCtx,
			}); err != nil {
				return nil, err
			}
		case OverrideGroupSEO:
			if err := c.runProductLocaleMutation(ctx, `
mutation($input: RemoveProductSeoInformationOverridesInput!) {
  product { removeProductSeoInformationOverrides(input: $input) { product { id } } }
}`, map[string]any{
				"productId":     productGID,
				"localeContext": localeCtx,
			}); err != nil {
				return nil, err
			}
		case OverrideGroupURL:
			if err := c.runProductLocaleMutation(ctx, `
mutation($input: RemoveProductUrlPathOverrideInput!) {
  product { removeProductUrlPathOverride(input: $input) { product { id } } }
}`, map[string]any{
				"productId":     productGID,
				"localeContext": localeCtx,
			}); err != nil {
				return nil, err
			}
		case OverrideGroupStorefront:
			if err := c.runProductLocaleMutation(ctx, `
mutation($input: RemoveProductStorefrontDetailsOverridesInput!) {
  product { removeProductStorefrontDetailsOverrides(input: $input) { product { id } } }
}`, map[string]any{
				"productId":     productGID,
				"localeContext": localeCtx,
			}); err != nil {
				return nil, err
			}
		case OverrideGroupPreorder:
			if err := c.runProductLocaleMutation(ctx, `
mutation($input: RemoveProductPreOrderSettingsOverridesInput!) {
  product { removeProductPreOrderSettingsOverrides(input: $input) { product { id } } }
}`, map[string]any{
				"productId":     productGID,
				"localeContext": localeCtx,
			}); err != nil {
				return nil, err
			}
		case OverrideGroupFeatured:
			if err := c.runProductLocaleMutation(ctx, `
mutation($input: RemoveProductIsFeaturedOverrideInput!) {
  product { removeProductIsFeaturedOverride(input: $input) { product { id } } }
}`, map[string]any{
				"productId": productGID,
				"channelId": FormatChannelGID(lc.ChannelID),
			}); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unknown override group %q; valid: basic, seo, url, storefront, preorder, featured", g)
		}
	}

	return c.GetProductLocaleOverrides(ctx, productID, lc.ChannelID, lc.Locale)
}

func validateOverrideContext(productID int, lc LocaleContext) error {
	if productID <= 0 {
		return fmt.Errorf("product_id must be a positive integer")
	}
	if lc.ChannelID <= 0 {
		return fmt.Errorf("channel_id must be a positive integer")
	}
	if strings.TrimSpace(lc.Locale) == "" {
		return fmt.Errorf("locale is required")
	}
	return nil
}

func (c *Client) runProductLocaleMutation(ctx context.Context, query string, input map[string]any) error {
	_, err := c.DoGraphQL(ctx, query, map[string]any{"input": input})
	return err
}
