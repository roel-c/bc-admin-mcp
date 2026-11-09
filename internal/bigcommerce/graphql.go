package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GraphQLError is returned when the Admin GraphQL endpoint responds with
// top-level errors (HTTP 200 with errors, or a failed request).
type GraphQLError struct {
	Messages []string
	Path     string
}

func (e *GraphQLError) Error() string {
	if len(e.Messages) == 0 {
		return "GraphQL Admin API error"
	}
	joined := strings.Join(e.Messages, "; ")
	if len(joined) > 500 {
		joined = joined[:500] + "…"
	}
	if e.Path != "" {
		return fmt.Sprintf("GraphQL Admin API error (%s): %s", e.Path, joined)
	}
	return fmt.Sprintf("GraphQL Admin API error: %s", joined)
}

// SafeError matches APIError's user-facing contract for tool handlers.
func (e *GraphQLError) SafeError() string {
	return e.Error()
}

func (c *Client) graphqlURL() string {
	return fmt.Sprintf("%s/%s/graphql", baseURL, c.storeHash)
}

type graphqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

type graphqlResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// DoGraphQL posts a typed GraphQL Admin query/mutation using the shared
// Management API auth token and rate limiter. It returns the raw "data"
// object, or a GraphQLError / APIError when the call fails.
func (c *Client) DoGraphQL(ctx context.Context, query string, variables map[string]any) (json.RawMessage, error) {
	payload := graphqlRequest{Query: query, Variables: variables}
	resp, body, err := c.Do(ctx, http.MethodPost, c.graphqlURL(), payload)
	if err != nil {
		// BC may return HTTP 400 with a GraphQL errors payload for schema/variable
		// problems — prefer those messages over a bare status code.
		if apiErr, ok := err.(*APIError); ok && len(apiErr.Body) > 0 {
			if gqlErr := graphQLErrorFromBody(apiErr.Body); gqlErr != nil {
				return nil, gqlErr
			}
		}
		return nil, err
	}
	_ = resp

	var parsed graphqlResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode GraphQL response: %w", err)
	}
	if gqlErr := graphQLErrorFromParsed(parsed); gqlErr != nil {
		return parsed.Data, gqlErr
	}
	if len(parsed.Data) == 0 || string(parsed.Data) == "null" {
		return nil, &GraphQLError{Messages: []string{"empty GraphQL data"}, Path: "graphql"}
	}
	return parsed.Data, nil
}

func graphQLErrorFromBody(body []byte) *GraphQLError {
	var parsed graphqlResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return graphQLErrorFromParsed(parsed)
}

func graphQLErrorFromParsed(parsed graphqlResponse) *GraphQLError {
	if len(parsed.Errors) == 0 {
		return nil
	}
	msgs := make([]string, 0, len(parsed.Errors))
	for _, e := range parsed.Errors {
		if strings.TrimSpace(e.Message) != "" {
			msgs = append(msgs, strings.TrimSpace(e.Message))
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return &GraphQLError{Messages: msgs, Path: "graphql"}
}

// --- Global ID helpers (Admin GraphQL resource identifiers) ---

// FormatChannelGID builds bc/store/channel/{id}.
func FormatChannelGID(channelID int) string {
	return fmt.Sprintf("bc/store/channel/%d", channelID)
}

// FormatProductGID builds bc/store/product/{id}.
func FormatProductGID(productID int) string {
	return fmt.Sprintf("bc/store/product/%d", productID)
}

// FormatCategoryGID builds bc/store/category/{id}.
func FormatCategoryGID(categoryID int) string {
	return fmt.Sprintf("bc/store/category/%d", categoryID)
}

// FormatBrandGID builds bc/store/brand/{id}.
func FormatBrandGID(brandID int) string {
	return fmt.Sprintf("bc/store/brand/%d", brandID)
}

// FormatLocaleGID builds bc/store/locale/{code}.
func FormatLocaleGID(localeCode string) string {
	return fmt.Sprintf("bc/store/locale/%s", strings.TrimSpace(localeCode))
}

// FormatProductOptionGID builds bc/store/product-option/{id}.
// Prefer FormatCatalogProductOptionGID for Translations Admin and MSF IE; the live
// GraphQL Admin APIs accept camelCase productOption IDs.
func FormatProductOptionGID(optionID int) string {
	return fmt.Sprintf("bc/store/product-option/%d", optionID)
}

// FormatCatalogProductOptionGID builds bc/store/productOption/{id}
// (Translations Admin PRODUCT_OPTIONS + MSF International Enhancements).
func FormatCatalogProductOptionGID(optionID int) string {
	return fmt.Sprintf("bc/store/productOption/%d", optionID)
}

// FormatCatalogProductOptionValueGID builds bc/store/productOptionValue/{id} (MSF IE).
func FormatCatalogProductOptionValueGID(valueID int) string {
	return fmt.Sprintf("bc/store/productOptionValue/%d", valueID)
}

// FormatProductCustomFieldGID builds bc/store/productCustomField/{id}.
func FormatProductCustomFieldGID(fieldID int) string {
	return fmt.Sprintf("bc/store/productCustomField/%d", fieldID)
}

// LocaleContext is the channel + locale pair used by override mutations.
type LocaleContext struct {
	ChannelID int
	Locale    string
}

func (lc LocaleContext) toInput() map[string]any {
	return map[string]any{
		"channelId": FormatChannelGID(lc.ChannelID),
		"locale":    strings.TrimSpace(lc.Locale),
	}
}
