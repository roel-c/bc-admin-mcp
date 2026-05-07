package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

const (
	maxChannelSummaryProducts = 5
	maxChannelSummaryChannels = 25
)

// channelMeta is the lightweight projection of a StoreChannel returned in
// channel_summary. Mirrors the catalog/channels/list summary shape so the
// agent only sees decision-relevant fields.
type channelMeta struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Platform string `json:"platform,omitempty"`
	Status   string `json:"status,omitempty"`
}

// channelListingProjection is the per-channel listing fields surfaced in the
// summary. Keeping this lean avoids leaking the variant raw JSON unless asked.
type channelListingProjection struct {
	ListingID   int    `json:"listing_id"`
	State       string `json:"state,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	ExternalID  string `json:"external_id,omitempty"`
	VariantsLen int    `json:"variants_count,omitempty"`
}

func (p *Products) registerChannelSummaryTool(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/channel_summary",
		Tier:    middleware.TierR0,
		Summary: "Per-product MSF snapshot: assignments + listing state across channels",
		Description: "Aggregates three Management API reads into one structured snapshot per product:\n" +
			"  1. GET /v3/channels (channel directory)\n" +
			"  2. GET /v3/catalog/products/channel-assignments?product_id:in=… (which channels each product is on)\n" +
			"  3. GET /v3/channels/{id}/listings?product_id:in=… for each channel that has at least one assignment for the requested products\n" +
			"Use this instead of stitching the three reads manually. " +
			"Caps: max 5 product_ids per call; up to 25 channels touched per call. " +
			"Required scopes: store_v2_products_read_only, store_channel_settings_read_only, store_channel_listings_read_only.",
		Tool: mcp.NewTool("catalog_products_channel_summary",
			mcp.WithDescription(
				"Return per-product channel assignments + listing state across all assigned channels.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Product IDs to summarize (max 5). Required."),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithBoolean("include_unassigned_channels",
				mcp.Description(
					"If true, also fetch listings on every active channel even when there is no catalog assignment, "+
						"so legacy stores with listings-without-assignments still appear. Defaults to false to keep "+
						"the call bounded.",
				),
			),
		),
		Handler: p.handleChannelSummary,
	})
}

func (p *Products) handleChannelSummary(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	productIDs, err := parseFloat64SliceToPositiveInts(args["product_ids"], "product_ids")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(productIDs) == 0 {
		return toolError("product_ids is required and must not be empty"), nil
	}
	if len(productIDs) > maxChannelSummaryProducts {
		return toolError("product_ids: maximum %d per call", maxChannelSummaryProducts), nil
	}

	includeUnassigned := false
	if v, ok := args["include_unassigned_channels"].(bool); ok {
		includeUnassigned = v
	}

	channels, err := p.bc.ListStoreChannels(ctx, nil)
	if err != nil {
		return toolError("list channels: %v", err), nil
	}
	channelByID := make(map[int]bigcommerce.StoreChannel, len(channels))
	for _, ch := range channels {
		channelByID[ch.ID] = ch
	}

	assignments, err := p.bc.ListProductChannelAssignments(ctx, map[string]string{
		"product_id:in": joinIntSlice(productIDs),
	})
	if err != nil {
		return toolError("list channel assignments: %v", err), nil
	}

	channelsForProduct := make(map[int]map[int]struct{}, len(productIDs))
	for _, pid := range productIDs {
		channelsForProduct[pid] = make(map[int]struct{})
	}
	for _, a := range assignments {
		if _, watched := channelsForProduct[a.ProductID]; !watched {
			continue
		}
		channelsForProduct[a.ProductID][a.ChannelID] = struct{}{}
	}

	channelsToQuery := map[int]struct{}{}
	for _, set := range channelsForProduct {
		for cid := range set {
			channelsToQuery[cid] = struct{}{}
		}
	}
	if includeUnassigned {
		for _, ch := range channels {
			channelsToQuery[ch.ID] = struct{}{}
		}
	}
	if len(channelsToQuery) > maxChannelSummaryChannels {
		return toolError(
			"this call would touch %d channels which exceeds the cap of %d. "+
				"Reduce the number of product_ids or set include_unassigned_channels=false.",
			len(channelsToQuery), maxChannelSummaryChannels,
		), nil
	}

	listingsByChannelByProduct := map[int]map[int]channelListingProjection{}
	for cid := range channelsToQuery {
		listings, err := p.bc.ListChannelListings(ctx, cid, map[string]string{
			"product_id:in": joinIntSlice(productIDs),
		})
		if err != nil {
			return toolError("list listings for channel %d: %v", cid, err), nil
		}
		channelListings := map[int]channelListingProjection{}
		for _, l := range listings {
			variantCount := 0
			if len(l.Variants) > 0 {
				var arr []json.RawMessage
				if jerr := json.Unmarshal(l.Variants, &arr); jerr == nil {
					variantCount = len(arr)
				}
			}
			channelListings[l.ProductID] = channelListingProjection{
				ListingID:   l.ListingID,
				State:       l.State,
				Name:        l.Name,
				Description: l.Description,
				ExternalID:  l.ExternalID,
				VariantsLen: variantCount,
			}
		}
		listingsByChannelByProduct[cid] = channelListings
	}

	type productSummary struct {
		ProductID                 int                                 `json:"product_id"`
		AssignedChannels          []channelMeta                       `json:"assigned_channels"`
		ListingsByChannel         map[string]channelListingProjection `json:"listings_by_channel"`
		ChannelsWithoutListing    []int                               `json:"channels_assigned_without_listing,omitempty"`
		ListingsWithoutAssignment []int                               `json:"channels_with_listing_but_no_assignment,omitempty"`
	}

	productSummaries := make([]productSummary, 0, len(productIDs))
	for _, pid := range productIDs {
		assignedSet := channelsForProduct[pid]
		assignedIDs := make([]int, 0, len(assignedSet))
		for cid := range assignedSet {
			assignedIDs = append(assignedIDs, cid)
		}
		sort.Ints(assignedIDs)
		assignedMeta := make([]channelMeta, 0, len(assignedIDs))
		for _, cid := range assignedIDs {
			ch, ok := channelByID[cid]
			if !ok {
				assignedMeta = append(assignedMeta, channelMeta{ID: cid})
				continue
			}
			assignedMeta = append(assignedMeta, channelMeta{
				ID: ch.ID, Name: ch.Name, Type: ch.Type, Platform: ch.Platform, Status: ch.Status,
			})
		}

		listingsForProduct := map[string]channelListingProjection{}
		channelsAssignedNoListing := []int{}
		listingsNoAssignment := []int{}

		for cid := range channelsToQuery {
			listing, has := listingsByChannelByProduct[cid][pid]
			if has {
				listingsForProduct[fmt.Sprintf("%d", cid)] = listing
				if _, isAssigned := assignedSet[cid]; !isAssigned {
					listingsNoAssignment = append(listingsNoAssignment, cid)
				}
			} else if _, isAssigned := assignedSet[cid]; isAssigned {
				channelsAssignedNoListing = append(channelsAssignedNoListing, cid)
			}
		}
		sort.Ints(channelsAssignedNoListing)
		sort.Ints(listingsNoAssignment)

		productSummaries = append(productSummaries, productSummary{
			ProductID:                 pid,
			AssignedChannels:          assignedMeta,
			ListingsByChannel:         listingsForProduct,
			ChannelsWithoutListing:    channelsAssignedNoListing,
			ListingsWithoutAssignment: listingsNoAssignment,
		})
	}

	channelDirectory := make([]channelMeta, 0, len(channels))
	for _, ch := range channels {
		channelDirectory = append(channelDirectory, channelMeta{
			ID: ch.ID, Name: ch.Name, Type: ch.Type, Platform: ch.Platform, Status: ch.Status,
		})
	}

	return toolJSON(map[string]any{
		"product_count":      len(productIDs),
		"channels_queried":   len(channelsToQuery),
		"include_unassigned": includeUnassigned,
		"channels_directory": channelDirectory,
		"products":           productSummaries,
		"apis": []string{
			"GET /v3/channels",
			"GET /v3/catalog/products/channel-assignments",
			"GET /v3/channels/{id}/listings",
		},
	})
}
