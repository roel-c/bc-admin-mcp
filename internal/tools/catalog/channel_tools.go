package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// ChannelTools exposes read-only MCP tools for BigCommerce Management API channels
// and MSF-related catalog tree discovery (GET /v3/channels, GET /v3/catalog/trees).
type ChannelTools struct {
	bc BigCommerceAPI
}

// NewChannelTools constructs channel list handlers.
func NewChannelTools(bc BigCommerceAPI) *ChannelTools {
	return &ChannelTools{bc: bc}
}

// RegisterTools registers catalog/channels/* tools (channels, trees, listings).
func (c *ChannelTools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/channels/list",
		Tier:    middleware.TierR0,
		Summary: "List sales channels for the connected BigCommerce store (MSF / routing context)",
		Description: "Returns channels for the merchant’s store using Store Management GET /v3/channels " +
			"(same OAuth token and store as other catalog tools). " +
			"Requires OAuth scope store_channel_settings (or equivalent) on the API account. " +
			"Optional type and status filters match the Management API query parameters.",
		Tool: mcp.NewTool("catalog_channels_list",
			mcp.WithDescription(
				"Request channels for the connected BigCommerce store (GET /v3/channels). "+
					"Optional filters: type (e.g. storefront), status (e.g. active). "+
					"Response includes active_storefront_channel_count — values > 1 usually mean multi-storefront catalog operations should specify channel_id / tree context.",
			),
			mcp.WithString("type", mcp.Description("Optional filter passed as type= to the API (e.g. storefront).")),
			mcp.WithString("status", mcp.Description("Optional filter passed as status= to the API (e.g. active).")),
		),
		Handler: c.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/channels/category_trees",
		Tier:    middleware.TierR0,
		Summary: "List category trees for MSF (optional filter by channel_id)",
		Description: "Calls GET /v3/catalog/trees with optional channel_id:in filter. " +
			"Each tree includes channel IDs it is associated with — use with catalog/channels/list " +
			"to pick the correct tree_id for category operations on a storefront. " +
			"Requires Products OAuth scope (store_v2_products_read_only or store_v2_products) on the API account.",
		Tool: mcp.NewTool("catalog_channels_category_trees",
			mcp.WithDescription(
				"List category trees for the store. Pass channel_id to restrict to trees linked to that BigCommerce channel (multi-storefront).",
			),
			mcp.WithNumber("channel_id", mcp.Description("Optional BigCommerce channel id; sent as channel_id:in to the API.")),
		),
		Handler: c.handleCategoryTrees,
	})
	c.registerListingTools(reg)
}

func (c *ChannelTools) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := make(map[string]string)

	if v, ok := args["type"]; ok {
		s, ok := v.(string)
		if !ok {
			return toolError("type must be a string"), nil
		}
		if t := strings.TrimSpace(s); t != "" {
			params["type"] = t
		}
	}
	if v, ok := args["status"]; ok {
		s, ok := v.(string)
		if !ok {
			return toolError("status must be a string"), nil
		}
		if t := strings.TrimSpace(s); t != "" {
			params["status"] = t
		}
	}

	var query map[string]string
	if len(params) > 0 {
		query = params
	}

	channels, err := c.bc.ListStoreChannels(ctx, query)
	if err != nil {
		return toolError("failed to list channels: %v", err), nil
	}

	activeStorefronts := 0
	for i := range channels {
		if channels[i].Type != "storefront" {
			continue
		}
		switch channels[i].Status {
		case "active", "prelaunch":
			activeStorefronts++
		default:
		}
	}

	return toolJSON(map[string]any{
		"total":                           len(channels),
		"channels":                        channels,
		"active_storefront_channel_count": activeStorefronts,
		"multi_storefront_likely":         activeStorefronts > 1,
		"api":                             "GET /v3/channels (Management API)",
	})
}

func (c *ChannelTools) handleCategoryTrees(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	var query map[string]string

	if v, ok := args["channel_id"]; ok && v != nil {
		ch, err := argPositiveInt("channel_id", v)
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
		query = map[string]string{"channel_id:in": fmt.Sprintf("%d", ch)}
	}

	trees, err := c.bc.ListCategoryTrees(ctx, query)
	if err != nil {
		return toolError("failed to list category trees: %v", err), nil
	}

	return toolJSON(map[string]any{
		"total": len(trees),
		"trees": trees,
		"api":   "GET /v3/catalog/trees (Management API)",
	})
}

func argPositiveInt(field string, v any) (int, error) {
	switch n := v.(type) {
	case float64:
		if n <= 0 || n != float64(int(n)) {
			return 0, fmt.Errorf("%s must be a positive integer", field)
		}
		return int(n), nil
	case int:
		if n <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", field)
		}
		return n, nil
	case int64:
		if n <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", field)
		}
		return int(n), nil
	default:
		return 0, fmt.Errorf("%s must be a number", field)
	}
}
