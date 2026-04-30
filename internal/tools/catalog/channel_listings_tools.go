package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

const (
	maxListingsPerJSONBatch  = 10
	maxListingsJSONBytes     = 256 * 1024
	maxListingsProductFilter = 50
)

// validListingStates are the enum values BigCommerce documents for product
// listing state in /v3/channels/{channel_id}/listings (CreateMultipleListingsReq /
// UpdateMultipleListingsReq). Validated at the tool boundary so the agent
// gets a structured error before BigCommerce returns 422.
var validListingStates = map[string]struct{}{
	"active":             {},
	"disabled":           {},
	"error":              {},
	"pending":            {},
	"pending_disable":    {},
	"pending_delete":     {},
	"partially_rejected": {},
	"queued":             {},
	"rejected":           {},
	"submitted":          {},
	"deleted":            {},
}

// validVariantListingStates is the variant-level subset (no partially_rejected per OpenAPI).
var validVariantListingStates = map[string]struct{}{
	"active":          {},
	"disabled":        {},
	"error":           {},
	"pending":         {},
	"pending_disable": {},
	"pending_delete":  {},
	"queued":          {},
	"rejected":        {},
	"submitted":       {},
	"deleted":         {},
}

func (c *ChannelTools) registerListingTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/channels/listings/list",
		Tier:    middleware.TierR0,
		Summary: "List channel product listings (MSF / marketplaces / POS)",
		Description: "GET /v3/channels/{channel_id}/listings with cursor pagination (limit + after) until exhausted or cap. " +
			"Optional product_id:in filter. Requires **store_channel_listings_read_only** or **store_channel_listings** (or channel settings read scopes per BC). " +
			"Returns at most 2000 rows per call.",
		Tool: mcp.NewTool("catalog_channels_listings_list",
			mcp.WithDescription(
				"List listings for one channel. Use catalog/channels/list for channel_id. Optional product_ids narrows by product_id:in.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID"), mcp.Required()),
			mcp.WithArray("product_ids",
				mcp.Description("Optional filter: up to 50 product IDs passed as product_id:in."),
				mcp.WithNumberItems(),
			),
		),
		Handler: c.handleListingsList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/channels/listings/create",
		Tier:    middleware.TierR1,
		Summary: "Create channel listings (POST)",
		Description: "POST /v3/channels/{channel_id}/listings — body is a JSON **array** of listing objects. " +
			"Per BigCommerce, each object needs **product_id**, **state**, and **variants** (each variant: **product_id**, **variant_id**, **state**). " +
			"Valid `state`: active, disabled, error, pending, pending_disable, pending_delete, partially_rejected, queued, rejected, submitted, deleted. " +
			"Optional **name** / **description** for channel-specific copy. Max **10** listings per call; max 256KiB JSON. " +
			"Preview then **confirmed=true**. Typical for marketplaces / POS / non-storefront channels.",
		Tool: mcp.NewTool("catalog_channels_listings_create",
			mcp.WithDescription(
				"Create listings on a channel. listings_json must be a JSON array. See tool description for required fields.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID"), mcp.Required()),
			mcp.WithString("listings_json", mcp.Description("JSON array of listing objects (max 10)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: c.handleListingsCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/channels/listings/update",
		Tier:    middleware.TierR2,
		Summary: "Update channel listings (PUT)",
		Description: "PUT /v3/channels/{channel_id}/listings — partial updates supported. " +
			"Each object must include **listing_id**, **product_id**, **state**, and **variants** (per BigCommerce OpenAPI). " +
			"Valid `state`: active, disabled, error, pending, pending_disable, pending_delete, partially_rejected, queued, rejected, submitted, deleted. " +
			"If you have more than 10 listings, call this tool sequentially (do not parallelize on the same product IDs). " +
			"Preview then **confirmed=true**. Requires **store_channel_listings** modify scope.",
		Tool: mcp.NewTool("catalog_channels_listings_update",
			mcp.WithDescription(
				"Update existing listings. listings_json is a JSON array; include listing_id from catalog/channels/listings/list.",
			),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID"), mcp.Required()),
			mcp.WithString("listings_json", mcp.Description("JSON array of listing objects (max 10)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: c.handleListingsUpdate,
	})
}

func (c *ChannelTools) handleListingsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, err := argPositiveInt("channel_id", args["channel_id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	extra := map[string]string{}
	if v, ok := args["product_ids"]; ok && v != nil {
		ids, perr := parseFloat64SliceToPositiveInts(v, "product_ids")
		if perr != nil {
			return toolError("%s", perr.Error()), nil
		}
		if len(ids) == 0 {
			return toolError("product_ids, if provided, must not be empty"), nil
		}
		if len(ids) > maxListingsProductFilter {
			return toolError("product_ids: maximum %d ids", maxListingsProductFilter), nil
		}
		extra["product_id:in"] = joinIntSlice(ids)
	}

	rows, err := c.bc.ListChannelListings(ctx, chID, extra)
	if err != nil {
		return toolError("list channel listings: %v", err), nil
	}

	return toolJSON(map[string]any{
		"channel_id": chID,
		"total":      len(rows),
		"listings":   rows,
		"api":        "GET /v3/channels/{channel_id}/listings",
	})
}

func (c *ChannelTools) handleListingsCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return c.handleListingsWrite(ctx, request, false)
}

func (c *ChannelTools) handleListingsUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return c.handleListingsWrite(ctx, request, true)
}

func (c *ChannelTools) handleListingsWrite(ctx context.Context, request mcp.CallToolRequest, isUpdate bool) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	chID, err := argPositiveInt("channel_id", args["channel_id"])
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	raw, ok := args["listings_json"].(string)
	if !ok {
		return toolError("listings_json must be a string containing a JSON array"), nil
	}
	if len(raw) > maxListingsJSONBytes {
		return toolError("listings_json exceeds maximum size (%d bytes)", maxListingsJSONBytes), nil
	}

	arr, err := parseListingsJSONArray(raw, maxListingsPerJSONBatch)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if isUpdate {
		if err := validateUpdateListingItems(arr); err != nil {
			return toolError("%s", err.Error()), nil
		}
	} else {
		if err := validateCreateListingItems(arr); err != nil {
			return toolError("%s", err.Error()), nil
		}
	}

	confirmed := middleware.IsConfirmed(request)
	if !confirmed {
		action := "create (POST)"
		api := "POST /v3/channels/{channel_id}/listings"
		if isUpdate {
			action = "update (PUT)"
			api = "PUT /v3/channels/{channel_id}/listings"
		}
		return toolJSON(map[string]any{
			"status":        "pending_confirmation",
			"channel_id":    chID,
			"listing_count": len(arr),
			"action":        action,
			"api":           api,
			"message":       fmt.Sprintf("Will %s %d listing object(s). Pass confirmed=true to execute.", action, len(arr)),
		})
	}

	var respBody []byte
	if isUpdate {
		respBody, err = c.bc.UpdateChannelListings(ctx, chID, arr)
	} else {
		respBody, err = c.bc.CreateChannelListings(ctx, chID, arr)
	}
	if err != nil {
		return toolError("channel listings request failed: %v", err), nil
	}

	var parsed any
	if len(respBody) > 0 {
		_ = json.Unmarshal(respBody, &parsed)
	}
	out := map[string]any{
		"status":     "completed",
		"channel_id": chID,
	}
	if parsed != nil {
		out["response"] = parsed
	} else {
		out["raw_response_length"] = len(respBody)
	}
	return toolJSON(out)
}

func parseListingsJSONArray(listingsJSON string, max int) ([]any, error) {
	s := strings.TrimSpace(listingsJSON)
	if s == "" {
		return nil, fmt.Errorf("listings_json must be non-empty")
	}
	var arr []any
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return nil, fmt.Errorf("listings_json: %w", err)
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("listings_json must be a JSON array with at least one object")
	}
	if len(arr) > max {
		return nil, fmt.Errorf("listings_json: at most %d objects per call", max)
	}
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("listings_json[%d]: must be a JSON object", i)
		}
		arr[i] = m
	}
	return arr, nil
}

func validateCreateListingItems(arr []any) error {
	for i, item := range arr {
		m := item.(map[string]any)
		if _, err := mapPositiveInt(m, "product_id"); err != nil {
			return fmt.Errorf("listings_json[%d].product_id: %w", i, err)
		}
		state, err := mapStringNonEmpty(m, "state")
		if err != nil {
			return fmt.Errorf("listings_json[%d].state: %w", i, err)
		}
		if _, ok := validListingStates[state]; !ok {
			return fmt.Errorf("listings_json[%d].state: %q is not a valid BigCommerce listing state (%s)", i, state, listingStatesHelpText(validListingStates))
		}
		if err := mapVariantsArray(m, "listings_json", i, true); err != nil {
			return err
		}
	}
	return nil
}

func validateUpdateListingItems(arr []any) error {
	for i, item := range arr {
		m := item.(map[string]any)
		if _, err := mapPositiveInt(m, "listing_id"); err != nil {
			return fmt.Errorf("listings_json[%d].listing_id: %w", i, err)
		}
		if _, err := mapPositiveInt(m, "product_id"); err != nil {
			return fmt.Errorf("listings_json[%d].product_id: %w", i, err)
		}
		state, err := mapStringNonEmpty(m, "state")
		if err != nil {
			return fmt.Errorf("listings_json[%d].state: %w", i, err)
		}
		if _, ok := validListingStates[state]; !ok {
			return fmt.Errorf("listings_json[%d].state: %q is not a valid BigCommerce listing state (%s)", i, state, listingStatesHelpText(validListingStates))
		}
		if err := mapVariantsArray(m, "listings_json", i, true); err != nil {
			return err
		}
	}
	return nil
}

func listingStatesHelpText(set map[string]struct{}) string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "valid: " + strings.Join(keys, ", ")
}

func mapVariantsArray(m map[string]any, prefix string, idx int, requireNonEmpty bool) error {
	v, ok := m["variants"]
	if !ok || v == nil {
		return fmt.Errorf("%s[%d]: variants is required", prefix, idx)
	}
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		if requireNonEmpty {
			return fmt.Errorf("%s[%d]: variants must be a non-empty array", prefix, idx)
		}
		return nil
	}
	for j, item := range arr {
		vm, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("%s[%d].variants[%d]: must be an object", prefix, idx, j)
		}
		if _, err := mapPositiveInt(vm, "product_id"); err != nil {
			return fmt.Errorf("%s[%d].variants[%d].product_id: %w", prefix, idx, j, err)
		}
		if _, err := mapPositiveInt(vm, "variant_id"); err != nil {
			return fmt.Errorf("%s[%d].variants[%d].variant_id: %w", prefix, idx, j, err)
		}
		state, err := mapStringNonEmpty(vm, "state")
		if err != nil {
			return fmt.Errorf("%s[%d].variants[%d].state: %w", prefix, idx, j, err)
		}
		if _, ok := validVariantListingStates[state]; !ok {
			return fmt.Errorf("%s[%d].variants[%d].state: %q is not a valid BigCommerce variant listing state (%s)",
				prefix, idx, j, state, listingStatesHelpText(validVariantListingStates))
		}
	}
	return nil
}

func mapPositiveInt(m map[string]any, key string) (int, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	switch n := v.(type) {
	case float64:
		if n <= 0 || n != float64(int(n)) {
			return 0, fmt.Errorf("must be a positive integer")
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("must be a number")
	}
}

func mapStringNonEmpty(m map[string]any, key string) (string, error) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", fmt.Errorf("%s is required", key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("%s must not be empty", key)
	}
	return s, nil
}
