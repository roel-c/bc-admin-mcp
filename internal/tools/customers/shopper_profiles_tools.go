package customers

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

const (
	maxShopperProfileCreateBatch = 50
	maxShopperProfileDeleteIDs   = 40
)

// ShopperProfiles provides MCP handlers for /v3/shopper-profiles and the
// segments-for-a-shopper-profile read endpoint.
type ShopperProfiles struct {
	bc BigCommerceCustomersAPI
}

// NewShopperProfiles constructs shopper-profile tool handlers.
func NewShopperProfiles(bc BigCommerceCustomersAPI) *ShopperProfiles {
	return &ShopperProfiles{bc: bc}
}

// RegisterTools registers customers/shopper_profiles/* tools.
func (s *ShopperProfiles) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/shopper_profiles/list",
		Tier:    middleware.TierR0,
		Summary: "List shopper profiles (V3)",
		Description: "GET /v3/shopper-profiles — paginated list. BigCommerce does not support id:in or " +
			"customer_id filtering on this endpoint; use customers/list with include=shopper_profile_id to look up by customer. " +
			"Customer Segmentation is an Enterprise-only feature; non-enterprise stores will receive 403.",
		Tool: mcp.NewTool("customers_shopper_profiles_list",
			mcp.WithDescription("Paginated list of shopper profiles."),
			mcp.WithNumber("page", mcp.Description("Page number.")),
			mcp.WithNumber("limit", mcp.Description("Page size.")),
		),
		Handler: s.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/shopper_profiles/create",
		Tier:    middleware.TierR1,
		Summary: "Create shopper profiles for customers (V3)",
		Description: fmt.Sprintf("POST /v3/shopper-profiles — bulk create one shopper profile per registered customer. "+
			"Each customer is 1:1 with a profile; recreating an existing profile is a no-op (BC will reject duplicates). "+
			"Provide customer_ids (numeric) or profiles_batch ([{customer_id}]). Max %d profiles per call.",
			maxShopperProfileCreateBatch,
		),
		Tool: mcp.NewTool("customers_shopper_profiles_create",
			mcp.WithDescription("Create shopper profiles. Provide customer_ids or profiles_batch."),
			mcp.WithArray("customer_ids", mcp.Description("Customer numeric IDs to create profiles for."),
				mcp.Items(map[string]any{"type": "number"})),
			mcp.WithArray("profiles_batch", mcp.Description("Array of {customer_id} objects (alternative to customer_ids)."),
				mcp.Items(map[string]any{"type": "object"})),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: s.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/shopper_profiles/delete",
		Tier:    middleware.TierR2,
		Summary: "Delete shopper profiles (V3)",
		Description: fmt.Sprintf("DELETE /v3/shopper-profiles?id:in=… — removes the profile and all of its segment memberships. "+
			"Customer records themselves are not affected. Max %d ids per call. Preview required.",
			maxShopperProfileDeleteIDs,
		),
		Tool: mcp.NewTool("customers_shopper_profiles_delete",
			mcp.WithDescription("Delete shopper profiles by UUID list."),
			mcp.WithArray("shopper_profile_ids", mcp.Description("Profile UUIDs to delete."),
				mcp.Items(map[string]any{"type": "string"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: s.handleDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "customers/shopper_profiles/list_segments",
		Tier:        middleware.TierR0,
		Summary:     "List segments for a shopper profile (V3)",
		Description: "GET /v3/shopper-profiles/{shopperProfileId}/segments — all segments containing this profile.",
		Tool: mcp.NewTool("customers_shopper_profiles_list_segments",
			mcp.WithDescription("List segments containing a shopper profile."),
			mcp.WithString("shopper_profile_id", mcp.Description("Shopper profile UUID."), mcp.Required()),
		),
		Handler: s.handleListSegments,
	})
}

func (s *ShopperProfiles) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := map[string]string{}
	if v, ok := args["page"].(float64); ok && v > 0 {
		params["page"] = fmt.Sprintf("%.0f", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params["limit"] = fmt.Sprintf("%.0f", v)
	}
	out, err := s.bc.ListShopperProfiles(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list shopper profiles: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(out), "shopper_profiles": out})
}

func (s *ShopperProfiles) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	creates, err := parseShopperProfileCreates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(creates) == 0 {
		return shared.ToolError("provide customer_ids or profiles_batch with at least one entry"), nil
	}
	if len(creates) > maxShopperProfileCreateBatch {
		return shared.ToolError("create payload exceeds max of %d per call", maxShopperProfileCreateBatch), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_shopper_profiles",
			"count":   len(creates),
			"payload": creates,
			"message": "Pass confirmed=true to create shopper profiles. Existing profiles will be left as-is; BC rejects duplicates.",
		})
	}

	out, err := s.bc.CreateShopperProfiles(ctx, creates)
	if err != nil {
		return shared.ToolError("create failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "count": len(out), "shopper_profiles": out})
}

func (s *ShopperProfiles) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := shared.RequiredNonEmptyStringIDs(args, "shopper_profile_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > maxShopperProfileDeleteIDs {
		return shared.ToolError("shopper_profile_ids exceeds max of %d per call", maxShopperProfileDeleteIDs), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		if err := s.bc.DeleteShopperProfiles(ctx, ids); err != nil {
			return shared.ToolError("delete failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "deleted", "shopper_profile_ids": ids, "count": len(ids)})
	}

	return shared.ToolJSON(map[string]any{
		"status":              "preview",
		"action":              "delete_shopper_profiles",
		"shopper_profile_ids": ids,
		"would_delete":        len(ids),
		"message":             "Pass confirmed=true to delete profiles. Segment memberships referencing these profiles are removed; underlying customer records remain.",
	})
}

func (s *ShopperProfiles) handleListSegments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id := readNonEmptyString(args, "shopper_profile_id")
	if id == "" {
		return shared.ToolError("shopper_profile_id is required"), nil
	}
	segments, err := s.bc.ListSegmentsForShopperProfile(ctx, id)
	if err != nil {
		return shared.ToolError("failed to list segments for shopper profile: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"shopper_profile_id": id,
		"total":              len(segments),
		"segments":           segments,
	})
}

func parseShopperProfileCreates(args map[string]any) ([]bigcommerce.ShopperProfileCreate, error) {
	out := []bigcommerce.ShopperProfileCreate{}
	if v, ok := args["customer_ids"]; ok && v != nil {
		ids, err := intSliceFromArgs(args, "customer_ids")
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			out = append(out, bigcommerce.ShopperProfileCreate{CustomerID: id})
		}
	}
	if v, ok := args["profiles_batch"]; ok && v != nil {
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("profiles_batch must be an array of {customer_id} objects")
		}
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("profiles_batch[%d] must be an object", i)
			}
			f, ok := m["customer_id"].(float64)
			if !ok {
				return nil, fmt.Errorf("profiles_batch[%d].customer_id must be a number", i)
			}
			id := int(f)
			if id <= 0 {
				return nil, fmt.Errorf("profiles_batch[%d].customer_id must be positive", i)
			}
			out = append(out, bigcommerce.ShopperProfileCreate{CustomerID: id})
		}
	}

	if len(out) == 0 {
		return out, nil
	}
	seen := make(map[int]struct{}, len(out))
	dedup := make([]bigcommerce.ShopperProfileCreate, 0, len(out))
	for _, c := range out {
		if _, ok := seen[c.CustomerID]; ok {
			continue
		}
		seen[c.CustomerID] = struct{}{}
		dedup = append(dedup, c)
	}
	return dedup, nil
}
