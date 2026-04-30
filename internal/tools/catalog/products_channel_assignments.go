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

const (
	maxChannelAssignListProducts = 100
	maxChannelAssignListChannels = 20
	maxChannelAssignPairsPerCall = 500
	maxChannelRemoveProducts     = 100
	maxChannelRemoveChannels     = 20
)

func (p *Products) registerChannelAssignmentTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/channel_assignments/list",
		Tier:    middleware.TierR0,
		Summary: "List product ↔ channel assignments (MSF)",
		Description: "GET /v3/catalog/products/channel-assignments. Requires at least one of product_ids or channel_ids " +
			"(comma lists sent as product_id:in / channel_id:in). Respects store pagination limits. " +
			"Needs Products scope (store_v2_products_read_only or store_v2_products).",
		Tool: mcp.NewTool("catalog_products_channel_assignments_list",
			mcp.WithDescription(
				"List which products are assigned to which channels. Provide product_ids and/or channel_ids (each non-empty array).",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Filter: BigCommerce product_id:in (max 100 ids)."),
				mcp.WithNumberItems(),
			),
			mcp.WithArray("channel_ids",
				mcp.Description("Filter: BigCommerce channel_id:in (max 20 ids)."),
				mcp.WithNumberItems(),
			),
		),
		Handler: p.handleListChannelAssignments,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/channel_assignments/assign",
		Tier:    middleware.TierR1,
		Summary: "Assign products to channels (additive PUT)",
		Description: "PUT /v3/catalog/products/channel-assignments — each product is assigned to each listed channel (cartesian). " +
			"Additive; BigCommerce discourages parallel PUTs touching the same product IDs. " +
			"Max 500 (product, channel) pairs per call; chunked by server ProductBatchSize. " +
			"Preview first; pass confirmed=true. Products + channel OAuth scopes required.",
		Tool: mcp.NewTool("catalog_products_channel_assignments_assign",
			mcp.WithDescription(
				"Assign products to sales channels for MSF. Provide product_ids and channel_ids; preview then confirmed=true.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Product IDs to assign."),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithArray("channel_ids",
				mcp.Description("Channel IDs (from catalog/channels/list) to assign each product to."),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: p.handleAssignChannels,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/channel_assignments/remove",
		Tier:    middleware.TierR2,
		Summary: "Remove product ↔ channel assignments (DELETE)",
		Description: "DELETE /v3/catalog/products/channel-assignments with product_id:in and optionally channel_id:in. " +
			"Requires non-empty product_ids (channel-only delete is not exposed for safety). " +
			"If channel_ids is omitted, removes those products from all channels they are assigned to. " +
			"Preview then confirmed=true. Destructive for listing visibility — tier R2.",
		Tool: mcp.NewTool("catalog_products_channel_assignments_remove",
			mcp.WithDescription(
				"Remove catalog channel assignments. product_ids required; optional channel_ids narrows removal. Preview then confirmed=true.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Product IDs (product_id:in); max 100 per call."),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithArray("channel_ids",
				mcp.Description("Optional channel IDs (channel_id:in); max 20. Omit to drop all channel links for listed products."),
				mcp.WithNumberItems(),
			),
			mcp.WithBoolean("confirmed",
				mcp.Description("Set to true to execute after reviewing the preview."),
			),
		),
		Handler: p.handleRemoveChannelAssignments,
	})
}

func (p *Products) handleListChannelAssignments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	var productIDs, channelIDs []int
	var err error

	if v, ok := args["product_ids"]; ok {
		productIDs, err = parseFloat64SliceToPositiveInts(v, "product_ids")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
	}
	if v, ok := args["channel_ids"]; ok {
		channelIDs, err = parseFloat64SliceToPositiveInts(v, "channel_ids")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
	}
	if len(productIDs) == 0 && len(channelIDs) == 0 {
		return toolError("at least one of product_ids or channel_ids must be a non-empty array"), nil
	}
	if len(productIDs) > maxChannelAssignListProducts {
		return toolError("product_ids: maximum %d ids per request", maxChannelAssignListProducts), nil
	}
	if len(channelIDs) > maxChannelAssignListChannels {
		return toolError("channel_ids: maximum %d ids per request", maxChannelAssignListChannels), nil
	}

	q := make(map[string]string)
	if len(productIDs) > 0 {
		q["product_id:in"] = joinIntSlice(productIDs)
	}
	if len(channelIDs) > 0 {
		q["channel_id:in"] = joinIntSlice(channelIDs)
	}

	rows, err := p.bc.ListProductChannelAssignments(ctx, q)
	if err != nil {
		return toolError("list channel assignments: %v", err), nil
	}

	return toolJSON(map[string]any{
		"total": len(rows),
		"data":  rows,
		"api":   "GET /v3/catalog/products/channel-assignments",
	})
}

func (p *Products) handleAssignChannels(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productIDs, err := parseFloat64SliceToPositiveInts(args["product_ids"], "product_ids")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(productIDs) == 0 {
		return toolError("product_ids is required and must not be empty"), nil
	}
	channelIDs, err := parseFloat64SliceToPositiveInts(args["channel_ids"], "channel_ids")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(channelIDs) == 0 {
		return toolError("channel_ids is required and must not be empty"), nil
	}

	assignments := make([]bigcommerce.ProductChannelAssignment, 0, len(productIDs)*len(channelIDs))
	for _, pid := range productIDs {
		for _, cid := range channelIDs {
			assignments = append(assignments, bigcommerce.ProductChannelAssignment{
				ProductID: pid,
				ChannelID: cid,
			})
		}
	}
	if len(assignments) > maxChannelAssignPairsPerCall {
		return toolError("too many pairs (%d); maximum %d product×channel assignments per call — split into multiple calls",
			len(assignments), maxChannelAssignPairsPerCall), nil
	}

	confirmed := middleware.IsConfirmed(request)
	if !confirmed {
		sample := assignments
		if len(sample) > 8 {
			sample = sample[:8]
		}
		return toolJSON(map[string]any{
			"status":             "pending_confirmation",
			"total_assignments":  len(assignments),
			"product_count":      len(productIDs),
			"channel_count":      len(channelIDs),
			"sample_assignments": sample,
			"message": fmt.Sprintf(
				"%d channel assignment(s) will be created (%d products × %d channels). "+
					"BigCommerce recommends avoiding parallel assignment requests for the same product IDs. "+
					"Pass confirmed=true to execute.",
				len(assignments), len(productIDs), len(channelIDs),
			),
		})
	}

	if err := p.bc.UpsertProductChannelAssignments(ctx, assignments); err != nil {
		return toolError("assign channels: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":  "completed",
		"message": fmt.Sprintf("Upserted %d product–channel assignment(s).", len(assignments)),
	})
}

func (p *Products) handleRemoveChannelAssignments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	productIDs, err := parseFloat64SliceToPositiveInts(args["product_ids"], "product_ids")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(productIDs) == 0 {
		return toolError("product_ids is required and must not be empty"), nil
	}
	if len(productIDs) > maxChannelRemoveProducts {
		return toolError("product_ids: maximum %d per call", maxChannelRemoveProducts), nil
	}

	var channelIDs []int
	if v, ok := args["channel_ids"]; ok && v != nil {
		channelIDs, err = parseFloat64SliceToPositiveInts(v, "channel_ids")
		if err != nil {
			return toolError("%s", err.Error()), nil
		}
	}
	if len(channelIDs) > maxChannelRemoveChannels {
		return toolError("channel_ids: maximum %d per call", maxChannelRemoveChannels), nil
	}

	confirmed := middleware.IsConfirmed(request)
	if !confirmed {
		msg := fmt.Sprintf(
			"Will DELETE channel assignments for %d product(s)", len(productIDs),
		)
		if len(channelIDs) > 0 {
			msg += fmt.Sprintf(" limited to %d channel(s)", len(channelIDs))
		} else {
			msg += " on all channels they are assigned to"
		}
		msg += ". Pass confirmed=true to execute."
		return toolJSON(map[string]any{
			"status":        "pending_confirmation",
			"product_ids":   productIDs,
			"channel_ids":   channelIDs,
			"product_id:in": joinIntSlice(productIDs),
			"channel_id:in": joinIntSlice(channelIDs),
			"message":       msg,
			"api":           "DELETE /v3/catalog/products/channel-assignments",
		})
	}

	if err := p.bc.DeleteProductChannelAssignments(ctx, productIDs, channelIDs); err != nil {
		return toolError("remove channel assignments: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":  "completed",
		"message": "Channel assignment delete request completed.",
	})
}

func joinIntSlice(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return strings.Join(parts, ",")
}
