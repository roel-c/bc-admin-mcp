package promotions

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// Hard caps and soft thresholds for the automatic-promotions tools.
const (
	maxPromotionDeleteIDs        = 40 // BC documents 50; we leave headroom for URL length and request batching
	rulesPerPromotionSoftWarn    = 10 // BC recommends ≤ 10 rules per promotion
	activePromotionsSoftWarnAt   = 100
	activePromotionsSearchPageSz = 250 // page size we use for the active-count gate on create
)

// hardPinAutomatic is the value forced onto every list call from this tool —
// keeps coupon promotions out of the AUTOMATIC tree and (eventually) lets a
// future coupon tool keep its own clean filter.
const hardPinAutomatic = "automatic"

// validSortFields enumerates the documented sort columns on
// GET /v3/promotions.
var validSortFields = map[string]struct{}{
	"id":         {},
	"name":       {},
	"priority":   {},
	"start_date": {},
}

var validSortDirections = map[string]struct{}{
	"asc":  {},
	"desc": {},
}

// AutomaticPromotions holds tool handlers for /v3/promotions where the
// redemption_type is hard-pinned to AUTOMATIC.
type AutomaticPromotions struct {
	bc    BigCommercePromotionsAPI
	cache *session.Store
}

// NewAutomaticPromotions constructs the AUTOMATIC promotions tool handlers.
func NewAutomaticPromotions(bc BigCommercePromotionsAPI, cache *session.Store) *AutomaticPromotions {
	return &AutomaticPromotions{bc: bc, cache: cache}
}

// RegisterTools wires the marketing/promotions/automatic/* tools into the
// discovery registry.
func (a *AutomaticPromotions) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/automatic/list",
		Tier:    middleware.TierR0,
		Summary: "List automatic promotions (V3)",
		Description: "GET /v3/promotions with redemption_type pinned to 'automatic'. " +
			"Filters: id, name, query (matches name), currency_code, status (ENABLED/DISABLED/INVALID), channels (channel ids). " +
			"Sort: id|name|priority|start_date with direction asc|desc. Page/limit pagination (BC default 50). " +
			"Coupon promotions live under marketing/promotions/coupon.",
		Tool: mcp.NewTool("marketing_promotions_automatic_list",
			mcp.WithDescription("List automatic promotions. Filters and sort honor BigCommerce GET /v3/promotions semantics."),
			mcp.WithNumber("id", mcp.Description("Exact promotion id filter.")),
			mcp.WithString("name", mcp.Description("Exact name filter.")),
			mcp.WithString("query", mcp.Description("Substring match against name (BigCommerce 'query' parameter).")),
			mcp.WithString("currency_code", mcp.Description("Filter by transactional currency (e.g. USD).")),
			mcp.WithString("status", mcp.Description("ENABLED | DISABLED | INVALID.")),
			mcp.WithArray("channels", mcp.Description("Channel ids to include."), mcp.Items(map[string]any{"type": "integer"})),
			mcp.WithString("sort", mcp.Description("Sort field: id | name | priority | start_date.")),
			mcp.WithString("direction", mcp.Description("Sort direction: asc | desc.")),
			mcp.WithNumber("page", mcp.Description("Page number (default 1).")),
			mcp.WithNumber("limit", mcp.Description("Page size (default 50).")),
		),
		Handler: a.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "marketing/promotions/automatic/get",
		Tier:        middleware.TierR0,
		Summary:     "Get one automatic promotion (V3)",
		Description: "GET /v3/promotions/{id}. Errors if the promotion's redemption_type is COUPON; use marketing/promotions/coupon/get for those.",
		Tool: mcp.NewTool("marketing_promotions_automatic_get",
			mcp.WithDescription("Fetch a single automatic promotion by id."),
			mcp.WithNumber("promotion_id", mcp.Description("Promotion id."), mcp.Required()),
		),
		Handler: a.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/automatic/create",
		Tier:    middleware.TierR2,
		Summary: "Create an automatic promotion (V3)",
		Description: fmt.Sprintf("POST /v3/promotions with redemption_type forced to AUTOMATIC. "+
			"Pass the full promotion as 'promotion' (object). Shape is validated locally before posting "+
			"(rules required, action one-of, condition tree, item matchers, notifications, customer.group_ids "+
			"vs excluded_group_ids mutual exclusion). Soft-warn surfaces when the store already has ≥%d ENABLED "+
			"promotions (BC recommends < %d). R2 high-risk: preview shows the resolved payload, %d-rule warning "+
			"if applicable; pass confirmed=true to execute.",
			activePromotionsSoftWarnAt, activePromotionsSoftWarnAt, rulesPerPromotionSoftWarn,
		),
		Tool: mcp.NewTool("marketing_promotions_automatic_create",
			mcp.WithDescription("Create an automatic promotion. Preview required; pass confirmed=true to execute."),
			mcp.WithObject("promotion", mcp.Description("Full BigCommerce promotion object (rules, notifications, etc.). redemption_type is overridden to AUTOMATIC."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing the preview.")),
		),
		Handler: a.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/automatic/update",
		Tier:    middleware.TierR2,
		Summary: "Update an automatic promotion (V3)",
		Description: "PUT /v3/promotions/{id}. BigCommerce replaces the document on PUT; this tool fetches " +
			"current then merges. Top-level scalars in 'patch' override current. If 'rules' is present in the " +
			"patch it REPLACES the entire current rules array. For positional rule edits, use 'rules_patch=[{index, replace_with}]' " +
			"to swap individual rule entries. 'notifications' likewise replaces in full when provided. " +
			"redemption_type is read-only after create. R2 high-risk: preview shows current vs would-apply; pass confirmed=true.",
		Tool: mcp.NewTool("marketing_promotions_automatic_update",
			mcp.WithDescription("Update an automatic promotion via fetch-merge-PUT."),
			mcp.WithNumber("promotion_id", mcp.Description("Promotion id to update."), mcp.Required()),
			mcp.WithObject("patch", mcp.Description("Fields to set. 'rules' (if present) replaces in full.")),
			mcp.WithArray("rules_patch", mcp.Description("Positional rule edits: [{index, replace_with}]. Indices apply to the CURRENT rules array."),
				mcp.Items(map[string]any{"type": "object"})),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing the preview.")),
		),
		Handler: a.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/automatic/set_status",
		Tier:    middleware.TierR2,
		Summary: "Enable / disable an automatic promotion (V3)",
		Description: "Convenience wrapper over update — flips status to ENABLED or DISABLED without touching rules. " +
			"Preview-then-confirm.",
		Tool: mcp.NewTool("marketing_promotions_automatic_set_status",
			mcp.WithDescription("Enable or disable an automatic promotion."),
			mcp.WithNumber("promotion_id", mcp.Description("Promotion id."), mcp.Required()),
			mcp.WithString("status", mcp.Description("ENABLED or DISABLED."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to apply.")),
		),
		Handler: a.handleSetStatus,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/automatic/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete automatic promotions (V3)",
		Description: fmt.Sprintf("DELETE /v3/promotions?id:in=… — destructive. Max %d ids per call (BC documents 50; "+
			"we leave headroom). BigCommerce returns 422 when a promotion still has coupon codes attached; "+
			"the error surfaces with a hint. Preview required; pass confirmed=true to execute.",
			maxPromotionDeleteIDs,
		),
		Tool: mcp.NewTool("marketing_promotions_automatic_delete",
			mcp.WithDescription("Delete automatic promotions by id. Preview required."),
			mcp.WithArray("promotion_ids", mcp.Description("Promotion ids to delete (max 40 per call)."),
				mcp.Items(map[string]any{"type": "integer"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to apply.")),
		),
		Handler: a.handleDelete,
	})
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func (a *AutomaticPromotions) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := bigcommerce.PromotionListParams{RedemptionType: hardPinAutomatic}

	if id, ok, err := readOptionalPositiveInt(args, "id"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.ID = id
	}
	if s, ok := readOptionalString(args, "name"); ok {
		params.Name = s
	}
	if s, ok := readOptionalString(args, "query"); ok {
		params.Query = s
	}
	if s, ok := readOptionalString(args, "currency_code"); ok {
		params.CurrencyCode = strings.ToUpper(s)
	}
	if s, ok := readOptionalString(args, "status"); ok {
		params.Status = s
	}
	if s, ok := readOptionalString(args, "sort"); ok {
		if _, ok := validSortFields[s]; !ok {
			return shared.ToolError("sort must be one of id/name/priority/start_date; got %q", s), nil
		}
		params.Sort = s
	}
	if s, ok := readOptionalString(args, "direction"); ok {
		if _, ok := validSortDirections[s]; !ok {
			return shared.ToolError("direction must be asc or desc; got %q", s), nil
		}
		params.Direction = s
	}
	if page, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.Page = page
	}
	if limit, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.Limit = limit
	}
	if chans, err := optionalIntArray(args, "channels"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else {
		params.Channels = chans
	}

	results, err := a.bc.SearchPromotions(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list promotions: %v", err), nil
	}
	// Defensive filter — BC sometimes ignores the redemption_type filter on
	// older stores. We only return AUTOMATIC entries from this tool.
	filtered := make([]bigcommerce.Promotion, 0, len(results))
	for _, p := range results {
		if p.RedemptionType == "" || p.RedemptionType == bigcommerce.PromotionRedemptionAutomatic {
			filtered = append(filtered, p)
		}
	}

	return shared.ToolJSON(map[string]any{
		"total":      len(filtered),
		"promotions": filtered,
		"filters":    summarizeListFilters(params),
	})
}

// ---------------------------------------------------------------------------
// get
// ---------------------------------------------------------------------------

func (a *AutomaticPromotions) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "promotion_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	p, err := a.bc.GetPromotion(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get promotion %d: %v", id, err), nil
	}
	if p.RedemptionType == bigcommerce.PromotionRedemptionCoupon {
		return shared.ToolError("promotion %d has redemption_type=COUPON; use the coupon tools to read it", id), nil
	}
	return shared.ToolJSON(p)
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

func (a *AutomaticPromotions) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	raw, ok := args["promotion"].(map[string]any)
	if !ok || len(raw) == 0 {
		return shared.ToolError("promotion is required and must be an object"), nil
	}

	// Force redemption_type to AUTOMATIC — this tool deliberately can't create
	// coupon promotions (those will live under marketing/promotions/coupon).
	payload := cloneMap(raw)
	payload["redemption_type"] = bigcommerce.PromotionRedemptionAutomatic

	if err := validatePromotionDraft(payload); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	warnings := []string{}
	if rules, ok := payload["rules"].([]any); ok && len(rules) > rulesPerPromotionSoftWarn {
		warnings = append(warnings, fmt.Sprintf("rules has %d entries; BigCommerce recommends <= %d rules per promotion", len(rules), rulesPerPromotionSoftWarn))
	}

	if !middleware.IsConfirmedFromArgs(args) {
		preview := map[string]any{
			"status":   "preview",
			"action":   "create_automatic_promotion",
			"payload":  payload,
			"warnings": warnings,
			"message":  "Review payload then pass confirmed=true to execute.",
		}
		// Active-count soft warn: only when the user is actually about to commit.
		// We still surface it on preview so they can see it before confirming.
		if active, err := a.countActivePromotions(ctx); err == nil {
			preview["active_promotion_count"] = active
			if active >= activePromotionsSoftWarnAt {
				preview["warnings"] = append(preview["warnings"].([]string),
					fmt.Sprintf("store already has %d ENABLED promotions; BigCommerce recommends fewer than %d for performance", active, activePromotionsSoftWarnAt))
			}
		}
		return shared.ToolJSON(preview)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return shared.ToolError("failed to marshal payload: %v", err), nil
	}
	created, err := a.bc.CreatePromotion(ctx, body)
	if err != nil {
		return shared.ToolError("create failed: %v", err), nil
	}
	resp := map[string]any{
		"status":    "created",
		"promotion": created,
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	return shared.ToolJSON(resp)
}

// countActivePromotions paginates through ENABLED automatic promotions just
// far enough to cross the soft-warn threshold. Bounded so we never fetch the
// whole catalogue when a store has thousands of historical promotions.
func (a *AutomaticPromotions) countActivePromotions(ctx context.Context) (int, error) {
	page := 1
	total := 0
	for total < activePromotionsSoftWarnAt+1 {
		params := bigcommerce.PromotionListParams{
			RedemptionType: hardPinAutomatic,
			Status:         bigcommerce.PromotionStatusEnabled,
			Limit:          activePromotionsSearchPageSz,
			Page:           page,
		}
		results, err := a.bc.SearchPromotions(ctx, params)
		if err != nil {
			return 0, err
		}
		total += len(results)
		if len(results) < activePromotionsSearchPageSz {
			break
		}
		page++
	}
	return total, nil
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func (a *AutomaticPromotions) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "promotion_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	patch, _ := args["patch"].(map[string]any)
	rulesPatchRaw, hasRulesPatch := args["rules_patch"]
	if len(patch) == 0 && !hasRulesPatch {
		return shared.ToolError("provide patch (object) and/or rules_patch (array)"), nil
	}

	// Cache the fetched promotion under an update-scoped key so the confirm
	// call reuses the preview's snapshot instead of issuing a second GET. The
	// key is namespaced to this handler to avoid colliding with other tools.
	cacheKey := fmt.Sprintf("promotion_update:%d", id)
	current, err := session.CacheOrFetch(a.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Promotion, error) {
		return a.bc.GetPromotion(ctx, id)
	})
	if err != nil {
		return shared.ToolError("failed to fetch current promotion %d: %v", id, err), nil
	}
	if current.RedemptionType == bigcommerce.PromotionRedemptionCoupon {
		return shared.ToolError("promotion %d has redemption_type=COUPON; use the coupon tools to update it", id), nil
	}

	merged, rulesReplaced, err := buildUpdatePayload(current, patch, rulesPatchRaw)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	merged["redemption_type"] = bigcommerce.PromotionRedemptionAutomatic

	if err := validatePromotionDraft(merged); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	warnings := []string{}
	if rulesReplaced {
		warnings = append(warnings, "rules array will be REPLACED in full (BigCommerce PUT replaces the document; merge is top-level only)")
	}
	if rules, ok := merged["rules"].([]any); ok && len(rules) > rulesPerPromotionSoftWarn {
		warnings = append(warnings, fmt.Sprintf("rules has %d entries; BigCommerce recommends <= %d rules per promotion", len(rules), rulesPerPromotionSoftWarn))
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "update_automatic_promotion",
			"current":     current,
			"would_apply": merged,
			"warnings":    warnings,
			"message":     "Review current vs would_apply then pass confirmed=true to execute.",
		})
	}

	body, err := json.Marshal(merged)
	if err != nil {
		return shared.ToolError("failed to marshal payload: %v", err), nil
	}
	updated, err := a.bc.UpdatePromotion(ctx, id, body)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	// Snapshot is now stale — drop it so a later preview refetches.
	a.cache.ForContext(ctx).Delete(cacheKey)
	resp := map[string]any{"status": "updated", "promotion": updated}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	return shared.ToolJSON(resp)
}

// buildUpdatePayload merges patch (top-level scalars) onto current, applies
// any positional rules_patch, and reports whether the rules array was
// (effectively) replaced. The caller still validates the merged map before
// posting.
func buildUpdatePayload(current *bigcommerce.Promotion, patch map[string]any, rulesPatchRaw any) (map[string]any, bool, error) {
	merged, err := promotionToMap(current)
	if err != nil {
		return nil, false, fmt.Errorf("failed to serialize current promotion: %w", err)
	}

	rulesReplaced := false

	for k, v := range patch {
		if k == "rules" {
			rulesReplaced = true
		}
		if k == "id" || k == "redemption_type" || k == "current_uses" || k == "created_from" {
			return nil, false, fmt.Errorf("patch may not include read-only field %q", k)
		}
		merged[k] = v
	}

	if rulesPatchRaw != nil {
		arr, ok := rulesPatchRaw.([]any)
		if !ok {
			return nil, false, fmt.Errorf("rules_patch must be an array of {index, replace_with}")
		}
		curRules, _ := merged["rules"].([]any)
		for i, raw := range arr {
			obj, ok := raw.(map[string]any)
			if !ok {
				return nil, false, fmt.Errorf("rules_patch[%d] must be an object {index, replace_with}", i)
			}
			idxF, ok := obj["index"].(float64)
			if !ok {
				return nil, false, fmt.Errorf("rules_patch[%d].index must be a number", i)
			}
			idx := int(idxF)
			if idx < 0 || idx >= len(curRules) {
				return nil, false, fmt.Errorf("rules_patch[%d].index %d is out of range (current rules length: %d)", i, idx, len(curRules))
			}
			replace, ok := obj["replace_with"].(map[string]any)
			if !ok {
				return nil, false, fmt.Errorf("rules_patch[%d].replace_with must be an object (a complete rule)", i)
			}
			curRules[idx] = replace
		}
		merged["rules"] = curRules
	}

	return merged, rulesReplaced, nil
}

// promotionToMap round-trips a Promotion through json so the merge layer can
// edit nested raw messages by key without our caring about which fields are
// typed scalars vs json.RawMessage.
func promotionToMap(p *bigcommerce.Promotion) (map[string]any, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	delete(m, "id")
	delete(m, "current_uses")
	delete(m, "created_from")
	return m, nil
}

// ---------------------------------------------------------------------------
// set_status
// ---------------------------------------------------------------------------

func (a *AutomaticPromotions) handleSetStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "promotion_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	status, _ := args["status"].(string)
	status = strings.ToUpper(strings.TrimSpace(status))
	if _, ok := validStatusesWritable[status]; !ok {
		return shared.ToolError("status must be ENABLED or DISABLED; got %q", status), nil
	}

	current, err := a.bc.GetPromotion(ctx, id)
	if err != nil {
		return shared.ToolError("failed to fetch current promotion %d: %v", id, err), nil
	}
	if current.RedemptionType == bigcommerce.PromotionRedemptionCoupon {
		return shared.ToolError("promotion %d has redemption_type=COUPON; use the coupon tools to update it", id), nil
	}
	if current.Status == status {
		return shared.ToolJSON(map[string]any{
			"status":         "noop",
			"promotion_id":   id,
			"current_status": current.Status,
			"message":        "promotion is already in the requested status",
		})
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":         "preview",
			"action":         "set_status",
			"promotion_id":   id,
			"current_status": current.Status,
			"would_apply":    status,
			"message":        "Pass confirmed=true to apply.",
		})
	}

	merged, err := promotionToMap(current)
	if err != nil {
		return shared.ToolError("failed to serialize current promotion: %v", err), nil
	}
	merged["status"] = status
	merged["redemption_type"] = bigcommerce.PromotionRedemptionAutomatic

	body, err := json.Marshal(merged)
	if err != nil {
		return shared.ToolError("failed to marshal payload: %v", err), nil
	}
	updated, err := a.bc.UpdatePromotion(ctx, id, body)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "promotion": updated})
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func (a *AutomaticPromotions) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := requiredPositiveIntIDs(args, "promotion_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > maxPromotionDeleteIDs {
		return shared.ToolError("promotion_ids exceeds max of %d per call", maxPromotionDeleteIDs), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		if err := a.bc.DeletePromotionsByIDs(ctx, ids); err != nil {
			msg := err.Error()
			if strings.Contains(strings.ToLower(msg), "coupon") || strings.Contains(msg, "422") {
				return shared.ToolError("delete failed: %v — at least one promotion still has coupon codes attached. Use marketing/promotions/coupon/codes/delete to remove them first, or call marketing/promotions/coupon/delete with delete_codes_first=true.", err), nil
			}
			return shared.ToolError("delete failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "deleted", "promotion_ids": ids})
	}

	// Preview — fetch each id and surface name + status + current_uses.
	previews := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		p, err := a.bc.GetPromotion(ctx, id)
		if err != nil {
			previews = append(previews, map[string]any{"id": id, "error": err.Error()})
			continue
		}
		previews = append(previews, map[string]any{
			"id":              p.ID,
			"name":            p.Name,
			"status":          p.Status,
			"current_uses":    p.CurrentUses,
			"redemption_type": p.RedemptionType,
		})
	}
	return shared.ToolJSON(map[string]any{
		"status":       "preview",
		"action":       "delete_automatic_promotions",
		"would_delete": len(ids),
		"matched":      previews,
		"message": "Pass confirmed=true to permanently delete these promotions. " +
			"BigCommerce returns 422 if any promotion still has coupon codes attached.",
	})
}

// ---------------------------------------------------------------------------
// shared helpers
// ---------------------------------------------------------------------------

func readOptionalString(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

func optionalIntArray(args map[string]any, key string) ([]int, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of integers", key)
	}
	out := make([]int, 0, len(arr))
	for i, x := range arr {
		f, ok := x.(float64)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be an integer", key, i)
		}
		if f != math.Trunc(f) {
			return nil, fmt.Errorf("%s[%d] must be an integer", key, i)
		}
		out = append(out, int(f))
	}
	return out, nil
}

func requiredPositiveIntIDs(args map[string]any, key string) ([]int, error) {
	raw, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	out := make([]int, 0, len(arr))
	for i, item := range arr {
		f, ok := item.(float64)
		if !ok {
			return nil, fmt.Errorf("each %s entry must be a number", key)
		}
		if f != math.Trunc(f) {
			return nil, fmt.Errorf("%s[%d] must be an integer", key, i)
		}
		id := int(f)
		if id <= 0 {
			return nil, fmt.Errorf("each %s entry must be a positive integer", key)
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s must contain at least one id", key)
	}
	return out, nil
}

func readOptionalPositiveInt(args map[string]any, key string) (int, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return 0, false, fmt.Errorf("%s must be a positive integer", key)
	}
	n := int(f)
	if n <= 0 {
		return 0, false, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, true, nil
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func summarizeListFilters(p bigcommerce.PromotionListParams) map[string]any {
	out := map[string]any{"redemption_type": hardPinAutomatic}
	if p.ID != 0 {
		out["id"] = p.ID
	}
	if p.Name != "" {
		out["name"] = p.Name
	}
	if p.Query != "" {
		out["query"] = p.Query
	}
	if p.CurrencyCode != "" {
		out["currency_code"] = p.CurrencyCode
	}
	if p.Status != "" {
		out["status"] = p.Status
	}
	if len(p.Channels) > 0 {
		out["channels"] = p.Channels
	}
	if p.Sort != "" {
		out["sort"] = p.Sort
	}
	if p.Direction != "" {
		out["direction"] = p.Direction
	}
	if p.Page > 0 {
		out["page"] = p.Page
	}
	if p.Limit > 0 {
		out["limit"] = p.Limit
	}
	return out
}
