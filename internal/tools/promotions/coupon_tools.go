package promotions

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// hardPinCoupon mirrors hardPinAutomatic — every list call from this tool
// forces redemption_type=coupon so AUTOMATIC promotions never leak into the
// coupon tree.
const hardPinCoupon = "coupon"

// cascadeDeleteCodesCap bounds how many codes a single coupon/delete call
// will cascade through before refusing. Walks the BC cursor to collect ids
// then deletes in chunks of maxCouponCodeIDInDelete; the cap keeps a single
// tool invocation from issuing hundreds of DELETEs against /codes.
const cascadeDeleteCodesCap = 1000

// CouponPromotions holds tool handlers for /v3/promotions where the
// redemption_type is hard-pinned to COUPON. Mirror of AutomaticPromotions —
// kept parallel rather than refactored into a shared base so each tool tree
// reads cleanly on its own.
type CouponPromotions struct {
	bc    BigCommercePromotionsAPI
	cache *session.Store
}

// NewCouponPromotions constructs the COUPON promotions tool handlers.
func NewCouponPromotions(bc BigCommercePromotionsAPI, cache *session.Store) *CouponPromotions {
	return &CouponPromotions{bc: bc, cache: cache}
}

// RegisterTools wires the marketing/promotions/coupon/* tools into the
// discovery registry.
func (c *CouponPromotions) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/list",
		Tier:    middleware.TierR0,
		Summary: "List coupon promotions (V3)",
		Description: "GET /v3/promotions with redemption_type pinned to 'coupon'. " +
			"Filters: id, name, code (matches the assigned code), query (matches name), currency_code, " +
			"status (ENABLED/DISABLED/INVALID), channels (channel ids). " +
			"Sort: id|name|priority|start_date with direction asc|desc. Page/limit pagination (BC default 50). " +
			"Automatic promotions live under marketing/promotions/automatic.",
		Tool: mcp.NewTool("marketing_promotions_coupon_list",
			mcp.WithDescription("List coupon promotions. Filters and sort honor BigCommerce GET /v3/promotions semantics."),
			mcp.WithNumber("id", mcp.Description("Exact promotion id filter.")),
			mcp.WithString("name", mcp.Description("Exact name filter.")),
			mcp.WithString("code", mcp.Description("Match an assigned coupon code (full string, no partial match).")),
			mcp.WithString("query", mcp.Description("Substring match against name (BigCommerce 'query' parameter).")),
			mcp.WithString("currency_code", mcp.Description("Filter by transactional currency (e.g. USD).")),
			mcp.WithString("status", mcp.Description("ENABLED | DISABLED | INVALID.")),
			mcp.WithArray("channels", mcp.Description("Channel ids to include."), mcp.Items(map[string]any{"type": "integer"})),
			mcp.WithString("sort", mcp.Description("Sort field: id | name | priority | start_date.")),
			mcp.WithString("direction", mcp.Description("Sort direction: asc | desc.")),
			mcp.WithNumber("page", mcp.Description("Page number (default 1).")),
			mcp.WithNumber("limit", mcp.Description("Page size (default 50).")),
		),
		Handler: c.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "marketing/promotions/coupon/get",
		Tier:        middleware.TierR0,
		Summary:     "Get one coupon promotion (V3)",
		Description: "GET /v3/promotions/{id}. Errors if the promotion's redemption_type is AUTOMATIC; use the marketing/promotions/automatic tools instead.",
		Tool: mcp.NewTool("marketing_promotions_coupon_get",
			mcp.WithDescription("Fetch a single coupon promotion by id."),
			mcp.WithNumber("promotion_id", mcp.Description("Promotion id."), mcp.Required()),
		),
		Handler: c.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/create",
		Tier:    middleware.TierR2,
		Summary: "Create a coupon promotion (V3)",
		Description: fmt.Sprintf("POST /v3/promotions with redemption_type forced to COUPON. "+
			"Pass the full promotion as 'promotion' (object). Coupon-specific cross-field validation: "+
			"coupon_type ∈ SINGLE | BULK; coupon_overrides_other_promotions=true requires "+
			"can_be_used_with_other_promotions=false (BC 422 otherwise); multiple_codes is only valid on BULK. "+
			"The deprecated coupon_overrides_automatic_when_offering_higher_discounts is rejected — use "+
			"coupon_overrides_other_promotions instead. Soft-warn surfaces when the store already has ≥%d ENABLED "+
			"promotions (BC recommends < %d). After creation, codes are added separately via marketing/promotions/coupon/codes/* tools. "+
			"R2 high-risk: preview shows the resolved payload; pass confirmed=true to execute.",
			activePromotionsSoftWarnAt, activePromotionsSoftWarnAt,
		),
		Tool: mcp.NewTool("marketing_promotions_coupon_create",
			mcp.WithDescription("Create a coupon promotion. Preview required; pass confirmed=true to execute."),
			mcp.WithObject("promotion", mcp.Description("Full BigCommerce promotion object. redemption_type is overridden to COUPON. "+
				"Set coupon_type=BULK to enable code generation via /codegen tooling."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing the preview.")),
		),
		Handler: c.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/update",
		Tier:    middleware.TierR2,
		Summary: "Update a coupon promotion (V3)",
		Description: "PUT /v3/promotions/{id}. BigCommerce replaces the document on PUT; this tool fetches " +
			"current then merges. Top-level scalars in 'patch' override current. If 'rules' is present in the " +
			"patch it REPLACES the entire current rules array; for positional rule edits use " +
			"'rules_patch=[{index, replace_with}]'. redemption_type is read-only after create. Coupon-specific " +
			"cross-field validation runs on the merged payload (see coupon/create description). " +
			"R2 high-risk: preview shows current vs would-apply; pass confirmed=true.",
		Tool: mcp.NewTool("marketing_promotions_coupon_update",
			mcp.WithDescription("Update a coupon promotion via fetch-merge-PUT."),
			mcp.WithNumber("promotion_id", mcp.Description("Promotion id to update."), mcp.Required()),
			mcp.WithObject("patch", mcp.Description("Fields to set. 'rules' (if present) replaces in full.")),
			mcp.WithArray("rules_patch", mcp.Description("Positional rule edits: [{index, replace_with}]. Indices apply to the CURRENT rules array."),
				mcp.Items(map[string]any{"type": "object"})),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing the preview.")),
		),
		Handler: c.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/set_status",
		Tier:    middleware.TierR2,
		Summary: "Enable / disable a coupon promotion (V3)",
		Description: "Convenience wrapper over update — flips status to ENABLED or DISABLED without touching rules. " +
			"Preview-then-confirm.",
		Tool: mcp.NewTool("marketing_promotions_coupon_set_status",
			mcp.WithDescription("Enable or disable a coupon promotion."),
			mcp.WithNumber("promotion_id", mcp.Description("Promotion id."), mcp.Required()),
			mcp.WithString("status", mcp.Description("ENABLED or DISABLED."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to apply.")),
		),
		Handler: c.handleSetStatus,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete coupon promotions (V3)",
		Description: fmt.Sprintf("DELETE /v3/promotions?id:in=… — destructive. Max %d ids per call. "+
			"BigCommerce returns 422 when a promotion still has coupon codes attached. By default this tool "+
			"refuses with a hint pointing at marketing/promotions/coupon/codes/delete. Set delete_codes_first=true "+
			"to cascade: the tool walks the codes for each promotion, deletes them in chunks of %d, then deletes "+
			"the promotion. Cascade is bounded at %d codes per promotion to keep a single invocation reviewable; "+
			"above that cap, clean up codes via the codes tools first. Preview required; pass confirmed=true to execute.",
			maxPromotionDeleteIDs, maxCouponCodeDeleteIDs, cascadeDeleteCodesCap,
		),
		Tool: mcp.NewTool("marketing_promotions_coupon_delete",
			mcp.WithDescription("Delete coupon promotions by id. Optionally cascade to delete attached codes."),
			mcp.WithArray("promotion_ids", mcp.Description("Promotion ids to delete (max 40 per call)."),
				mcp.Items(map[string]any{"type": "integer"}), mcp.Required()),
			mcp.WithBoolean("delete_codes_first", mcp.Description("If true, delete each promotion's coupon codes (chunked) before deleting the promotion. Bounded by an internal cap.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to apply.")),
		),
		Handler: c.handleDelete,
	})
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func (c *CouponPromotions) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := bigcommerce.PromotionListParams{RedemptionType: hardPinCoupon}

	if id, ok, err := readOptionalPositiveInt(args, "id"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.ID = id
	}
	if s, ok := readOptionalString(args, "name"); ok {
		params.Name = s
	}
	if s, ok := readOptionalString(args, "code"); ok {
		params.Code = s
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

	results, err := c.bc.SearchPromotions(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list promotions: %v", err), nil
	}
	// Defensive filter — BC sometimes ignores the redemption_type filter on
	// older stores. We only return COUPON entries from this tool.
	filtered := make([]bigcommerce.Promotion, 0, len(results))
	for _, p := range results {
		if p.RedemptionType == "" || p.RedemptionType == bigcommerce.PromotionRedemptionCoupon {
			filtered = append(filtered, p)
		}
	}

	return shared.ToolJSON(map[string]any{
		"total":      len(filtered),
		"promotions": filtered,
		"filters":    summarizeCouponListFilters(params),
	})
}

// ---------------------------------------------------------------------------
// get
// ---------------------------------------------------------------------------

func (c *CouponPromotions) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "promotion_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	p, err := c.bc.GetPromotion(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get promotion %d: %v", id, err), nil
	}
	if p.RedemptionType == bigcommerce.PromotionRedemptionAutomatic {
		return shared.ToolError("promotion %d has redemption_type=AUTOMATIC; use the automatic tools to read it", id), nil
	}
	return shared.ToolJSON(p)
}

// ---------------------------------------------------------------------------
// create
// ---------------------------------------------------------------------------

func (c *CouponPromotions) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	raw, ok := args["promotion"].(map[string]any)
	if !ok || len(raw) == 0 {
		return shared.ToolError("promotion is required and must be an object"), nil
	}

	// Force redemption_type to COUPON — this tool deliberately can't create
	// automatic promotions.
	payload := cloneMap(raw)
	payload["redemption_type"] = bigcommerce.PromotionRedemptionCoupon

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
			"action":   "create_coupon_promotion",
			"payload":  payload,
			"warnings": warnings,
			"message":  "Review payload then pass confirmed=true to execute. Codes are added afterwards via marketing/promotions/coupon/codes/* tools.",
		}
		if active, err := c.countActivePromotions(ctx); err == nil {
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
	created, err := c.bc.CreatePromotion(ctx, body)
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

// countActivePromotions paginates ENABLED COUPON promotions for the soft-warn
// threshold check on create. Bounded so we never fetch the whole catalogue.
func (c *CouponPromotions) countActivePromotions(ctx context.Context) (int, error) {
	page := 1
	total := 0
	for total < activePromotionsSoftWarnAt+1 {
		params := bigcommerce.PromotionListParams{
			RedemptionType: hardPinCoupon,
			Status:         bigcommerce.PromotionStatusEnabled,
			Limit:          activePromotionsSearchPageSz,
			Page:           page,
		}
		results, err := c.bc.SearchPromotions(ctx, params)
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

func (c *CouponPromotions) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	// Cache the fetched promotion under an update-scoped key so confirm reuses
	// the preview snapshot instead of a second GET (namespaced to this handler).
	cacheKey := fmt.Sprintf("coupon_update:%d", id)
	current, err := session.CacheOrFetch(c.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Promotion, error) {
		return c.bc.GetPromotion(ctx, id)
	})
	if err != nil {
		return shared.ToolError("failed to fetch current promotion %d: %v", id, err), nil
	}
	if current.RedemptionType == bigcommerce.PromotionRedemptionAutomatic {
		return shared.ToolError("promotion %d has redemption_type=AUTOMATIC; use the automatic tools to update it", id), nil
	}

	merged, rulesReplaced, err := buildUpdatePayload(current, patch, rulesPatchRaw)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	merged["redemption_type"] = bigcommerce.PromotionRedemptionCoupon

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
			"action":      "update_coupon_promotion",
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
	updated, err := c.bc.UpdatePromotion(ctx, id, body)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	c.cache.ForContext(ctx).Delete(cacheKey)
	resp := map[string]any{"status": "updated", "promotion": updated}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	return shared.ToolJSON(resp)
}

// ---------------------------------------------------------------------------
// set_status
// ---------------------------------------------------------------------------

func (c *CouponPromotions) handleSetStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	current, err := c.bc.GetPromotion(ctx, id)
	if err != nil {
		return shared.ToolError("failed to fetch current promotion %d: %v", id, err), nil
	}
	if current.RedemptionType == bigcommerce.PromotionRedemptionAutomatic {
		return shared.ToolError("promotion %d has redemption_type=AUTOMATIC; use the automatic tools to update it", id), nil
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
	merged["redemption_type"] = bigcommerce.PromotionRedemptionCoupon

	body, err := json.Marshal(merged)
	if err != nil {
		return shared.ToolError("failed to marshal payload: %v", err), nil
	}
	updated, err := c.bc.UpdatePromotion(ctx, id, body)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "promotion": updated})
}

// ---------------------------------------------------------------------------
// delete (with optional cascade)
// ---------------------------------------------------------------------------

func (c *CouponPromotions) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := requiredPositiveIntIDs(args, "promotion_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > maxPromotionDeleteIDs {
		return shared.ToolError("promotion_ids exceeds max of %d per call", maxPromotionDeleteIDs), nil
	}
	cascade, _ := args["delete_codes_first"].(bool)

	if middleware.IsConfirmedFromArgs(args) {
		return c.executeDelete(ctx, ids, cascade)
	}

	// Preview — fetch each id, surface name/status/uses, and (best-effort)
	// the attached coupon-code count from the first /codes page.
	previews := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		p, err := c.bc.GetPromotion(ctx, id)
		if err != nil {
			previews = append(previews, map[string]any{"id": id, "error": err.Error()})
			continue
		}
		entry := map[string]any{
			"id":              p.ID,
			"name":            p.Name,
			"status":          p.Status,
			"current_uses":    p.CurrentUses,
			"redemption_type": p.RedemptionType,
		}
		if codes, err := c.bc.ListCouponCodes(ctx, id, bigcommerce.CouponCodeListParams{Limit: 50}); err == nil {
			entry["attached_codes_first_page"] = len(codes.Codes)
			entry["has_more_code_pages"] = codes.Cursor.After != ""
			if len(codes.Codes) > 0 {
				sample := make([]string, 0, 5)
				for i := 0; i < len(codes.Codes) && i < 5; i++ {
					sample = append(sample, codes.Codes[i].Code)
				}
				entry["attached_codes_sample"] = sample
			}
		}
		previews = append(previews, entry)
	}
	message := "Pass confirmed=true to permanently delete these promotions. " +
		"BigCommerce returns 422 if any promotion still has coupon codes attached. " +
		"Set delete_codes_first=true to cascade through attached codes (bounded)."
	if cascade {
		message = fmt.Sprintf("Pass confirmed=true to permanently delete these promotions AND their attached codes (cascade bounded at %d codes per promotion).", cascadeDeleteCodesCap)
	}
	return shared.ToolJSON(map[string]any{
		"status":             "preview",
		"action":             "delete_coupon_promotions",
		"would_delete":       len(ids),
		"matched":            previews,
		"delete_codes_first": cascade,
		"message":            message,
	})
}

// executeDelete performs the delete after the user confirms. When cascade is
// true we walk each promotion's codes via cursor pagination, refusing if the
// total exceeds cascadeDeleteCodesCap, then chunk-delete codes followed by
// the promotion delete batch.
func (c *CouponPromotions) executeDelete(ctx context.Context, ids []int, cascade bool) (*mcp.CallToolResult, error) {
	cascadeReport := map[string]any{}
	if cascade {
		for _, id := range ids {
			codeIDs, truncated, err := c.collectAllCodeIDs(ctx, id)
			if err != nil {
				return shared.ToolError("failed to enumerate coupon codes for promotion %d: %v", id, err), nil
			}
			if truncated {
				return shared.ToolError("promotion %d has more than %d coupon codes; clean them up via marketing/promotions/coupon/codes/delete first (then re-run delete with delete_codes_first=true or unset)", id, cascadeDeleteCodesCap), nil
			}
			if len(codeIDs) > 0 {
				if err := c.bc.DeleteCouponCodes(ctx, id, codeIDs); err != nil {
					return shared.ToolError("cascade: failed to delete coupon codes for promotion %d: %v", id, err), nil
				}
				cascadeReport[fmt.Sprintf("%d", id)] = len(codeIDs)
			}
		}
	}
	if err := c.bc.DeletePromotionsByIDs(ctx, ids); err != nil {
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "coupon") || strings.Contains(msg, "422") {
			return shared.ToolError("delete failed: %v — at least one promotion still has coupon codes attached. Use marketing/promotions/coupon/codes/delete to remove them, or re-run with delete_codes_first=true.", err), nil
		}
		return shared.ToolError("delete failed: %v", err), nil
	}
	resp := map[string]any{"status": "deleted", "promotion_ids": ids, "delete_codes_first": cascade}
	if len(cascadeReport) > 0 {
		resp["codes_deleted_per_promotion"] = cascadeReport
	}
	return shared.ToolJSON(resp)
}

// collectAllCodeIDs walks the cursor for /v3/promotions/{id}/codes and
// returns every code id (or signals truncation when the cap is exceeded).
func (c *CouponPromotions) collectAllCodeIDs(ctx context.Context, promotionID int) (ids []int, truncated bool, err error) {
	cursor := ""
	for {
		page, err := c.bc.ListCouponCodes(ctx, promotionID, bigcommerce.CouponCodeListParams{After: cursor, Limit: 250})
		if err != nil {
			return nil, false, err
		}
		for _, code := range page.Codes {
			if code.ID > 0 {
				ids = append(ids, code.ID)
				if len(ids) > cascadeDeleteCodesCap {
					return nil, true, nil
				}
			}
		}
		if page.Cursor.After == "" || len(page.Codes) == 0 {
			break
		}
		cursor = page.Cursor.After
	}
	return ids, false, nil
}

// summarizeCouponListFilters mirrors summarizeListFilters but stamps the
// hard-pin as 'coupon' so list responses are unambiguous.
func summarizeCouponListFilters(p bigcommerce.PromotionListParams) map[string]any {
	out := map[string]any{"redemption_type": hardPinCoupon}
	if p.ID != 0 {
		out["id"] = p.ID
	}
	if p.Name != "" {
		out["name"] = p.Name
	}
	if p.Code != "" {
		out["code"] = p.Code
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
