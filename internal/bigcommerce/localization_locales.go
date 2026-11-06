package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// MaxChannelLocales is BigCommerce's documented per-storefront locale cap.
const MaxChannelLocales = 5

// ChannelLocale is a language configured on a sales channel.
type ChannelLocale struct {
	Code      string `json:"code"`
	Status    string `json:"status"`
	IsDefault bool   `json:"is_default"`
}

// SupportedLocale is a platform-supported locale code from i18n.supportedLocales.
type SupportedLocale struct {
	Code                    string `json:"code"`
	EnglishName             string `json:"english_name"`
	HasBuiltInTranslations  bool   `json:"has_built_in_translations"`
}

// LocaleStatus values accepted by add/update locale mutations.
const (
	LocaleStatusActive   = "ACTIVE"
	LocaleStatusInactive = "INACTIVE"
)

// ListChannelLocales returns locales configured for a channel.
func (c *Client) ListChannelLocales(ctx context.Context, channelID int) ([]ChannelLocale, error) {
	const q = `
query($channelId: ID!) {
  store {
    locales(input: { channelId: $channelId }) {
      edges {
        node { code status isDefault }
      }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, q, map[string]any{
		"channelId": FormatChannelGID(channelID),
	})
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Store struct {
			Locales struct {
				Edges []struct {
					Node struct {
						Code      string `json:"code"`
						Status    string `json:"status"`
						IsDefault bool   `json:"isDefault"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"locales"`
		} `json:"store"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode channel locales: %w", err)
	}
	out := make([]ChannelLocale, 0, len(parsed.Store.Locales.Edges))
	for _, e := range parsed.Store.Locales.Edges {
		out = append(out, ChannelLocale{
			Code:      e.Node.Code,
			Status:    e.Node.Status,
			IsDefault: e.Node.IsDefault,
		})
	}
	return out, nil
}

// ListSupportedLocales returns platform-supported locale codes.
func (c *Client) ListSupportedLocales(ctx context.Context) ([]SupportedLocale, error) {
	const q = `
query {
  i18n {
    supportedLocales {
      edges {
        node { code englishName hasBuiltInTranslations }
      }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, q, nil)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		I18n struct {
			SupportedLocales struct {
				Edges []struct {
					Node struct {
						Code                   string `json:"code"`
						EnglishName            string `json:"englishName"`
						HasBuiltInTranslations bool   `json:"hasBuiltInTranslations"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"supportedLocales"`
		} `json:"i18n"`
	}
	// Some schemas return supportedLocales as a plain list; try edges first then fallback.
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode supported locales: %w", err)
	}
	out := make([]SupportedLocale, 0, len(parsed.I18n.SupportedLocales.Edges))
	for _, e := range parsed.I18n.SupportedLocales.Edges {
		out = append(out, SupportedLocale{
			Code:                   e.Node.Code,
			EnglishName:            e.Node.EnglishName,
			HasBuiltInTranslations: e.Node.HasBuiltInTranslations,
		})
	}
	if len(out) > 0 {
		return out, nil
	}
	// Fallback: non-connection list shape seen in some doc examples.
	var alt struct {
		I18n struct {
			SupportedLocales []struct {
				Code                   string `json:"code"`
				EnglishName            string `json:"englishName"`
				HasBuiltInTranslations bool   `json:"hasBuiltInTranslations"`
			} `json:"supportedLocales"`
		} `json:"i18n"`
	}
	if err := json.Unmarshal(data, &alt); err != nil {
		return out, nil
	}
	for _, n := range alt.I18n.SupportedLocales {
		out = append(out, SupportedLocale{
			Code:                   n.Code,
			EnglishName:            n.EnglishName,
			HasBuiltInTranslations: n.HasBuiltInTranslations,
		})
	}
	return out, nil
}

// AddLocaleInput configures a new channel locale.
type AddLocaleInput struct {
	ChannelID int
	Code      string
	Status    string // ACTIVE or INACTIVE; empty defaults to INACTIVE server-side if omitted
}

// AddLocale adds a locale to a channel.
func (c *Client) AddLocale(ctx context.Context, in AddLocaleInput) (*ChannelLocale, error) {
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return nil, fmt.Errorf("locale code is required")
	}
	status := strings.TrimSpace(in.Status)
	if status == "" {
		status = LocaleStatusInactive
	}
	const m = `
mutation($input: AddLocaleInput!) {
  locale {
    addLocale(input: $input) {
      locale { code status isDefault }
      errors { ... on Error { message } }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, m, map[string]any{
		"input": map[string]any{
			"channelId": FormatChannelGID(in.ChannelID),
			"code":      code,
			"status":    status,
		},
	})
	if err != nil {
		return nil, err
	}
	return decodeLocaleMutation(data, "addLocale")
}

// UpdateLocaleInput updates status and/or default flag for a channel locale.
type UpdateLocaleInput struct {
	ChannelID int
	Code      string
	Status    string // optional
	IsDefault *bool  // optional
}

// UpdateLocale updates a channel locale.
func (c *Client) UpdateLocale(ctx context.Context, in UpdateLocaleInput) (*ChannelLocale, error) {
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return nil, fmt.Errorf("locale code is required")
	}
	input := map[string]any{
		"channelId": FormatChannelGID(in.ChannelID),
		"code":      code,
	}
	if s := strings.TrimSpace(in.Status); s != "" {
		input["status"] = s
	}
	if in.IsDefault != nil {
		input["isDefault"] = *in.IsDefault
	}
	const m = `
mutation($input: UpdateLocaleInput!) {
  locale {
    updateLocale(input: $input) {
      locale { code status isDefault }
      errors { ... on Error { message } }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, m, map[string]any{"input": input})
	if err != nil {
		return nil, err
	}
	return decodeLocaleMutation(data, "updateLocale")
}

// DeleteLocale removes a locale from a channel.
func (c *Client) DeleteLocale(ctx context.Context, channelID int, code string) (string, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", fmt.Errorf("locale code is required")
	}
	const m = `
mutation($input: DeleteLocaleInput!) {
  locale {
    deleteLocale(input: $input) {
      code
      errors { ... on Error { message } }
    }
  }
}`
	data, err := c.DoGraphQL(ctx, m, map[string]any{
		"input": map[string]any{
			"channelId": FormatChannelGID(channelID),
			"code":      code,
		},
	})
	if err != nil {
		return "", err
	}
	var parsed struct {
		Locale struct {
			DeleteLocale struct {
				Code   string `json:"code"`
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors"`
			} `json:"deleteLocale"`
		} `json:"locale"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return "", fmt.Errorf("decode deleteLocale: %w", err)
	}
	if msgs := mutationErrorMessages(parsed.Locale.DeleteLocale.Errors); len(msgs) > 0 {
		return "", &GraphQLError{Messages: msgs, Path: "locale.deleteLocale"}
	}
	return parsed.Locale.DeleteLocale.Code, nil
}

func decodeLocaleMutation(data json.RawMessage, field string) (*ChannelLocale, error) {
	var parsed struct {
		Locale map[string]struct {
			Locale *struct {
				Code      string `json:"code"`
				Status    string `json:"status"`
				IsDefault bool   `json:"isDefault"`
			} `json:"locale"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"locale"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode locale mutation: %w", err)
	}
	result, ok := parsed.Locale[field]
	if !ok {
		return nil, fmt.Errorf("locale mutation field %q missing in response", field)
	}
	if msgs := mutationErrorMessages(result.Errors); len(msgs) > 0 {
		return nil, &GraphQLError{Messages: msgs, Path: "locale." + field}
	}
	if result.Locale == nil {
		return nil, fmt.Errorf("locale mutation %q returned no locale", field)
	}
	return &ChannelLocale{
		Code:      result.Locale.Code,
		Status:    result.Locale.Status,
		IsDefault: result.Locale.IsDefault,
	}, nil
}

func mutationErrorMessages(errs []struct {
	Message string `json:"message"`
}) []string {
	out := make([]string, 0, len(errs))
	for _, e := range errs {
		if s := strings.TrimSpace(e.Message); s != "" {
			out = append(out, s)
		}
	}
	return out
}
