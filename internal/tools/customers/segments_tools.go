package customers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

const (
	maxSegmentWriteBatch          = 10
	maxSegmentDeleteIDs           = 40
	maxSegmentShoppersPerCall     = 50
	maxSegmentShopperRemoveIDs    = 40
	maxResolveCustomerIDsPerCall  = 50
	missingProfileCustomerIDLimit = 25
)

// CustomerSegments provides MCP handlers for /v3/segments and the
// segments↔shopper-profiles membership endpoints.
type CustomerSegments struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerSegments constructs Customer Segmentation tool handlers.
func NewCustomerSegments(bc BigCommerceCustomersAPI) *CustomerSegments {
	return &CustomerSegments{bc: bc}
}

// RegisterTools registers customers/segments/* and customers/segments/shoppers/*.
func (s *CustomerSegments) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/segments/list",
		Tier:    middleware.TierR0,
		Summary: "List customer segments (V3)",
		Description: "GET /v3/segments — paginated list of segments. Optional segment_ids (UUIDs) maps to id:in. " +
			"Note: BigCommerce Customer Segmentation is an Enterprise-only feature; non-enterprise stores will receive 403.",
		Tool: mcp.NewTool("customers_segments_list",
			mcp.WithDescription("List customer segments. Filter by segment_ids (UUIDs) or paginate."),
			mcp.WithArray("segment_ids", mcp.Description("Optional segment UUIDs (comma-joined as id:in)."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithNumber("page", mcp.Description("Page number (offset pagination).")),
			mcp.WithNumber("limit", mcp.Description("Page size.")),
		),
		Handler: s.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "customers/segments/get",
		Tier:        middleware.TierR0,
		Summary:     "Get one segment by ID (V3)",
		Description: "Wraps GET /v3/segments?id:in={segment_id} — BigCommerce has no single-segment GET.",
		Tool: mcp.NewTool("customers_segments_get",
			mcp.WithDescription("Fetch one segment by UUID."),
			mcp.WithString("segment_id", mcp.Description("Segment UUID."), mcp.Required()),
		),
		Handler: s.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/segments/create",
		Tier:    middleware.TierR1,
		Summary: "Create one or more customer segments (V3)",
		Description: fmt.Sprintf("POST /v3/segments — bulk create. Max %d per call (BigCommerce caps the store at 1000 segments total). "+
			"Either pass segments_batch (array of {name, description?}) or single-record fields name, description. "+
			"Preview first; pass confirmed=true to execute.",
			maxSegmentWriteBatch,
		),
		Tool: mcp.NewTool("customers_segments_create",
			mcp.WithDescription("Create segments (max 10 per call). Preview then confirmed=true."),
			mcp.WithArray("segments_batch", mcp.Description("Array of {name, description?} objects (BigCommerce SegmentPost)."),
				mcp.Items(map[string]any{"type": "object"})),
			mcp.WithString("name", mcp.Description("Single create: segment name.")),
			mcp.WithString("description", mcp.Description("Single create: optional description.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: s.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/segments/update",
		Tier:    middleware.TierR1,
		Summary: "Update one or more customer segments (V3)",
		Description: fmt.Sprintf("PUT /v3/segments — bulk update of segment metadata. Max %d per call. "+
			"Each row in segments_batch must include id (UUID); name and description are optional. "+
			"Preview first; pass confirmed=true to execute.",
			maxSegmentWriteBatch,
		),
		Tool: mcp.NewTool("customers_segments_update",
			mcp.WithDescription("Update segments (max 10). Each row must include id."),
			mcp.WithArray("segments_batch", mcp.Description("Array of {id, name?, description?}."),
				mcp.Items(map[string]any{"type": "object"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: s.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/segments/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete customer segments (V3)",
		Description: fmt.Sprintf("DELETE /v3/segments?id:in=… — irreversible. Removes segment metadata only; "+
			"shopper profiles previously associated with the segment are kept. Promotions or workflows that "+
			"reference these segment IDs will stop targeting members. Max %d ids per call. Preview required.",
			maxSegmentDeleteIDs,
		),
		Tool: mcp.NewTool("customers_segments_delete",
			mcp.WithDescription("Delete segments by UUID list. Preview required before confirmed=true."),
			mcp.WithArray("segment_ids", mcp.Description("Segment UUIDs to delete."), mcp.Items(map[string]any{"type": "string"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: s.handleDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/segments/shoppers/list",
		Tier:    middleware.TierR0,
		Summary: "List shopper profiles in a segment (V3)",
		Description: "GET /v3/segments/{segmentId}/shopper-profiles. NOTE: this endpoint requires the *modify* " +
			"Customers OAuth scope (store_v2_customers) — read-only tokens will receive 403 here even though it is a GET.",
		Tool: mcp.NewTool("customers_segments_shoppers_list",
			mcp.WithDescription("List shopper profiles in a segment."),
			mcp.WithString("segment_id", mcp.Description("Segment UUID."), mcp.Required()),
		),
		Handler: s.handleShoppersList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/segments/shoppers/add",
		Tier:    middleware.TierR1,
		Summary: "Add shopper profiles to a segment (V3)",
		Description: fmt.Sprintf("POST /v3/segments/{segmentId}/shopper-profiles — bulk add. Max %d profiles per call (BigCommerce limit). "+
			"Provide shopper_profile_ids (UUIDs) directly, or pass customer_ids and the tool resolves them via "+
			"GET /v3/customers?include=shopper_profile_id (max %d customer_ids per call); customers without a "+
			"shopper profile are surfaced under missing_shopper_profiles instead of being silently dropped.",
			maxSegmentShoppersPerCall, maxResolveCustomerIDsPerCall,
		),
		Tool: mcp.NewTool("customers_segments_shoppers_add",
			mcp.WithDescription("Add shopper profiles to a segment by UUID or by customer_ids."),
			mcp.WithString("segment_id", mcp.Description("Segment UUID."), mcp.Required()),
			mcp.WithArray("shopper_profile_ids", mcp.Description("Shopper profile UUIDs."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithArray("customer_ids", mcp.Description("Customer numeric IDs to resolve to shopper_profile_ids."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: s.handleShoppersAdd,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/segments/shoppers/remove",
		Tier:    middleware.TierR1,
		Summary: "Remove shopper profiles from a segment (V3)",
		Description: fmt.Sprintf("DELETE /v3/segments/{segmentId}/shopper-profiles?id:in=… — disassociates profiles "+
			"from a segment without deleting the profiles or customers. Max %d profile ids per call (chunked).",
			maxSegmentShopperRemoveIDs,
		),
		Tool: mcp.NewTool("customers_segments_shoppers_remove",
			mcp.WithDescription("Remove shopper profiles from a segment."),
			mcp.WithString("segment_id", mcp.Description("Segment UUID."), mcp.Required()),
			mcp.WithArray("shopper_profile_ids", mcp.Description("Shopper profile UUIDs to remove."),
				mcp.Items(map[string]any{"type": "string"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: s.handleShoppersRemove,
	})
}

func (s *CustomerSegments) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := map[string]string{}

	ids, err := shared.OptionalStringIDs(args, "segment_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > 0 {
		params["id:in"] = strings.Join(ids, ",")
	}
	if v, ok := args["page"].(float64); ok && v > 0 {
		params["page"] = fmt.Sprintf("%.0f", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params["limit"] = fmt.Sprintf("%.0f", v)
	}

	segments, err := s.bc.SearchSegments(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list segments: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(segments), "segments": segments})
}

func (s *CustomerSegments) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id := readNonEmptyString(args, "segment_id")
	if id == "" {
		return shared.ToolError("segment_id is required"), nil
	}
	segments, err := s.bc.GetSegmentsByIDs(ctx, []string{id})
	if err != nil {
		return shared.ToolError("failed to get segment: %v", err), nil
	}
	if len(segments) == 0 {
		return shared.ToolError("segment %s not found", id), nil
	}
	return shared.ToolJSON(segments[0])
}

func (s *CustomerSegments) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	creates, err := parseSegmentCreates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(creates) == 0 {
		return shared.ToolError("no segments to create"), nil
	}
	if len(creates) > maxSegmentWriteBatch {
		return shared.ToolError("segments_batch exceeds max of %d per call", maxSegmentWriteBatch), nil
	}
	for i, c := range creates {
		if strings.TrimSpace(c.Name) == "" {
			return shared.ToolError("segments_batch[%d]: name is required", i), nil
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_segments",
			"count":   len(creates),
			"payload": creates,
			"message": "Review payload then pass confirmed=true to execute.",
		})
	}

	out, err := s.bc.CreateSegments(ctx, creates)
	if err != nil {
		return shared.ToolError("create failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "count": len(out), "segments": out})
}

func (s *CustomerSegments) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	updates, err := parseSegmentUpdates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(updates) == 0 {
		return shared.ToolError("segments_batch is required with at least one row including id"), nil
	}
	if len(updates) > maxSegmentWriteBatch {
		return shared.ToolError("segments_batch exceeds max of %d per call", maxSegmentWriteBatch), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		ids := make([]string, 0, len(updates))
		for _, u := range updates {
			ids = append(ids, u.ID)
		}
		current, err := s.bc.GetSegmentsByIDs(ctx, ids)
		if err != nil {
			return shared.ToolError("failed to pre-fetch segments for preview: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "update_segments",
			"count":   len(updates),
			"current": current,
			"patch":   updates,
			"message": "Review patch vs current then pass confirmed=true to execute.",
		})
	}

	out, err := s.bc.UpdateSegments(ctx, updates)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "count": len(out), "segments": out})
}

func (s *CustomerSegments) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := shared.RequiredNonEmptyStringIDs(args, "segment_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > maxSegmentDeleteIDs {
		return shared.ToolError("segment_ids exceeds max of %d per call", maxSegmentDeleteIDs), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		if err := s.bc.DeleteSegments(ctx, ids); err != nil {
			return shared.ToolError("delete failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "deleted", "segment_ids": ids})
	}

	existing, err := s.bc.GetSegmentsByIDs(ctx, ids)
	if err != nil {
		return shared.ToolError("failed to pre-fetch segments for preview: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":           "preview",
		"action":           "delete_segments",
		"would_delete":     len(ids),
		"matched_segments": existing,
		"message": "Pass confirmed=true to permanently delete these segments. Shopper profiles are not removed; " +
			"any external references (promotions, scripts) that target these segment IDs will stop matching.",
	})
}

func (s *CustomerSegments) handleShoppersList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id := readNonEmptyString(args, "segment_id")
	if id == "" {
		return shared.ToolError("segment_id is required"), nil
	}
	profiles, err := s.bc.ListShopperProfilesInSegment(ctx, id)
	if err != nil {
		return shared.ToolError("failed to list shopper profiles in segment: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"segment_id":       id,
		"total":            len(profiles),
		"shopper_profiles": profiles,
		"oauth_scope_note": "BigCommerce requires the modify Customers scope (store_v2_customers) for this GET.",
	})
}

func (s *CustomerSegments) handleShoppersAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	segID := readNonEmptyString(args, "segment_id")
	if segID == "" {
		return shared.ToolError("segment_id is required"), nil
	}

	profileIDs, err := shared.OptionalStringIDs(args, "shopper_profile_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	rawCustomerIDs, err := intSliceFromArgs(args, "customer_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if len(profileIDs) == 0 && len(rawCustomerIDs) == 0 {
		return shared.ToolError("provide shopper_profile_ids and/or customer_ids"), nil
	}
	if len(rawCustomerIDs) > maxResolveCustomerIDsPerCall {
		return shared.ToolError("customer_ids exceeds max of %d per call", maxResolveCustomerIDsPerCall), nil
	}

	resolved := append([]string(nil), profileIDs...)
	var missing []int
	resolutionMap := make(map[int]string, len(rawCustomerIDs))
	if len(rawCustomerIDs) > 0 {
		params := map[string]string{
			"id:in":   shared.JoinInts(rawCustomerIDs),
			"include": "shopper_profile_id",
		}
		customers, lookupErr := s.bc.SearchCustomers(ctx, params)
		if lookupErr != nil {
			return shared.ToolError("failed to resolve customer_ids → shopper_profile_id: %v", lookupErr), nil
		}
		shopperProfileIDsByCustomer := extractShopperProfileIDs(customers)
		for _, cid := range rawCustomerIDs {
			pid := strings.TrimSpace(shopperProfileIDsByCustomer[cid])
			if pid == "" {
				missing = append(missing, cid)
				continue
			}
			resolutionMap[cid] = pid
			resolved = append(resolved, pid)
		}
	}

	resolved = dedupStrings(resolved)
	if len(resolved) == 0 {
		return shared.ToolError("no shopper_profile_ids resolved (missing profiles for customer_ids: %v). "+
			"Create profiles first via customers/shopper_profiles/create or pass shopper_profile_ids directly.", missing), nil
	}
	if len(resolved) > maxSegmentShoppersPerCall {
		return shared.ToolError("resolved shopper_profile_ids count %d exceeds max of %d per call (BigCommerce limit)",
			len(resolved), maxSegmentShoppersPerCall), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		preview := map[string]any{
			"status":              "preview",
			"action":              "add_shopper_profiles_to_segment",
			"segment_id":          segID,
			"shopper_profile_ids": resolved,
			"count":               len(resolved),
			"message":             "Pass confirmed=true to add these shopper profiles to the segment.",
		}
		if len(resolutionMap) > 0 {
			preview["resolved_from_customer_ids"] = resolutionMap
		}
		if len(missing) > 0 {
			capped := missing
			if len(capped) > missingProfileCustomerIDLimit {
				capped = capped[:missingProfileCustomerIDLimit]
			}
			preview["missing_shopper_profiles"] = capped
			preview["missing_count"] = len(missing)
		}
		return shared.ToolJSON(preview)
	}

	out, err := s.bc.AddShopperProfilesToSegment(ctx, segID, resolved)
	if err != nil {
		return shared.ToolError("add failed: %v", err), nil
	}
	resp := map[string]any{
		"status":          "added",
		"segment_id":      segID,
		"requested_count": len(resolved),
		"added_profiles":  out,
	}
	if len(resolutionMap) > 0 {
		resp["resolved_from_customer_ids"] = resolutionMap
	}
	if len(missing) > 0 {
		resp["missing_shopper_profiles"] = missing
		resp["missing_count"] = len(missing)
	}
	return shared.ToolJSON(resp)
}

func (s *CustomerSegments) handleShoppersRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	segID := readNonEmptyString(args, "segment_id")
	if segID == "" {
		return shared.ToolError("segment_id is required"), nil
	}
	ids, err := shared.RequiredNonEmptyStringIDs(args, "shopper_profile_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > maxSegmentShopperRemoveIDs {
		return shared.ToolError("shopper_profile_ids exceeds max of %d per call", maxSegmentShopperRemoveIDs), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":              "preview",
			"action":              "remove_shopper_profiles_from_segment",
			"segment_id":          segID,
			"shopper_profile_ids": ids,
			"would_remove":        len(ids),
			"message":             "Pass confirmed=true to disassociate these shopper profiles from the segment. The profiles themselves are not deleted.",
		})
	}

	if err := s.bc.RemoveShopperProfilesFromSegment(ctx, segID, ids); err != nil {
		return shared.ToolError("remove failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":              "removed",
		"segment_id":          segID,
		"shopper_profile_ids": ids,
		"count":               len(ids),
	})
}

func parseSegmentCreates(args map[string]any) ([]bigcommerce.SegmentCreate, error) {
	if v, ok := args["segments_batch"]; ok && v != nil {
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("segments_batch must be an array")
		}
		out := make([]bigcommerce.SegmentCreate, 0, len(arr))
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("segments_batch[%d] must be an object", i)
			}
			var c bigcommerce.SegmentCreate
			b, err := json.Marshal(m)
			if err != nil {
				return nil, fmt.Errorf("segments_batch[%d]: %w", i, err)
			}
			if err := json.Unmarshal(b, &c); err != nil {
				return nil, fmt.Errorf("segments_batch[%d]: %w", i, err)
			}
			out = append(out, c)
		}
		return out, nil
	}
	name, _ := args["name"].(string)
	desc, _ := args["description"].(string)
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("provide segments_batch or name (and optional description)")
	}
	return []bigcommerce.SegmentCreate{{Name: name, Description: desc}}, nil
}

func parseSegmentUpdates(args map[string]any) ([]bigcommerce.SegmentUpdate, error) {
	v, ok := args["segments_batch"]
	if !ok || v == nil {
		return nil, fmt.Errorf("segments_batch is required")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("segments_batch must be an array")
	}
	out := make([]bigcommerce.SegmentUpdate, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("segments_batch[%d] must be an object", i)
		}
		var u bigcommerce.SegmentUpdate
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("segments_batch[%d]: %w", i, err)
		}
		if err := json.Unmarshal(b, &u); err != nil {
			return nil, fmt.Errorf("segments_batch[%d]: %w", i, err)
		}
		if strings.TrimSpace(u.ID) == "" {
			return nil, fmt.Errorf("segments_batch[%d]: id is required", i)
		}
		if u.Name == nil && u.Description == nil {
			return nil, fmt.Errorf("segments_batch[%d]: at least one of name or description must be provided", i)
		}
		out = append(out, u)
	}
	return out, nil
}

func readNonEmptyString(args map[string]any, key string) string {
	v, _ := args[key].(string)
	return strings.TrimSpace(v)
}

func extractShopperProfileIDs(customers []bigcommerce.Customer) map[int]string {
	out := make(map[int]string, len(customers))
	for _, cu := range customers {
		pid := strings.TrimSpace(cu.ShopperProfileID)
		if cu.ID != 0 && pid != "" {
			out[cu.ID] = pid
		}
	}
	return out
}

func dedupStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
