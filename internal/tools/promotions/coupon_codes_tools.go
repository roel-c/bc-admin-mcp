package promotions

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// codeGenBatchSizeCap mirrors BigCommerce's documented per-call hard cap.
const codeGenBatchSizeCap = 250

// maxCouponCodeDeleteIDs caps coupon-code deletes per tool invocation. BC
// documents 50 per call; we leave headroom (matching the promotion-delete
// cap of 40) for URL length and concurrent-request budget.
const maxCouponCodeDeleteIDs = 40

// CouponCodes holds tool handlers for /v3/promotions/{id}/codes and the
// /codegen sub-resource. All tools require an existing parent promotion id
// and surface BigCommerce's per-endpoint constraints (cursor pagination,
// charset rules, batch caps) as explicit tool-layer checks.
type CouponCodes struct {
	bc BigCommercePromotionsAPI
}

// NewCouponCodes constructs the coupon-codes tool handlers.
func NewCouponCodes(bc BigCommercePromotionsAPI) *CouponCodes {
	return &CouponCodes{bc: bc}
}

// RegisterTools wires the marketing/promotions/coupon/codes/* tools into the
// discovery registry.
func (cc *CouponCodes) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/codes/list",
		Tier:    middleware.TierR0,
		Summary: "List coupon codes for a promotion (V3)",
		Description: "GET /v3/promotions/{promotion_id}/codes. Cursor pagination via before/after; pass the cursor " +
			"value returned in the previous response. BigCommerce default rate limit on this endpoint is 10 " +
			"concurrent requests (lower than other coupon-codes endpoints).",
		Tool: mcp.NewTool("marketing_promotions_coupon_codes_list",
			mcp.WithDescription("List coupon codes for a promotion (cursor-paginated)."),
			mcp.WithNumber("promotion_id", mcp.Description("Parent promotion id."), mcp.Required()),
			mcp.WithString("after", mcp.Description("Cursor for the next page (from a previous response).")),
			mcp.WithString("before", mcp.Description("Cursor for the previous page.")),
			mcp.WithNumber("limit", mcp.Description("Page size (BigCommerce default applies when unset).")),
		),
		Handler: cc.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/codes/create_single",
		Tier:    middleware.TierR1,
		Summary: "Create one coupon code on a promotion (V3)",
		Description: fmt.Sprintf("POST /v3/promotions/{promotion_id}/codes. Adds a single coupon code. "+
			"BigCommerce constraints: code is required, max %d characters; allowed characters are letters, "+
			"numbers, spaces, underscores, and hyphens. max_uses=0 means unlimited; the parent promotion's "+
			"max_uses overrides this. There is NO update endpoint — coupon codes are immutable. To change a "+
			"code's max_uses, delete and recreate it. Preview required; pass confirmed=true to execute.",
			maxCouponCodeLength,
		),
		Tool: mcp.NewTool("marketing_promotions_coupon_codes_create_single",
			mcp.WithDescription("Create a single coupon code on a coupon promotion."),
			mcp.WithNumber("promotion_id", mcp.Description("Parent promotion id (must be coupon_type=SINGLE or BULK)."), mcp.Required()),
			mcp.WithString("code", mcp.Description("The code string. Letters/numbers/spaces/underscores/hyphens, max 50 chars."), mcp.Required()),
			mcp.WithNumber("max_uses", mcp.Description("Total uses across all customers; 0 = unlimited.")),
			mcp.WithNumber("max_uses_per_customer", mcp.Description("Per-customer use cap; 0 = unlimited.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to apply.")),
		),
		Handler: cc.handleCreateSingle,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/codes/generate_bulk",
		Tier:    middleware.TierR2,
		Summary: "Generate a batch of coupon codes (V3)",
		Description: fmt.Sprintf("POST /v3/promotions/{promotion_id}/codegen. Mints up to %d codes per call from "+
			"a template (prefix, suffix, length, format). Parent promotion MUST have coupon_type=BULK; the "+
			"tool fetches it and refuses on SINGLE promotions before posting. length (when set) must be 6..16. "+
			"format ∈ NUMBERS | LETTERS | ALPHANUMERIC. BigCommerce recommends keeping the total per-store code "+
			"count under 2 million for performance. R2 high-risk (each batch grants discounts to %d customers); "+
			"preview required.",
			codeGenBatchSizeCap, codeGenBatchSizeCap,
		),
		Tool: mcp.NewTool("marketing_promotions_coupon_codes_generate_bulk",
			mcp.WithDescription("Generate a bulk batch of coupon codes for a BULK coupon promotion."),
			mcp.WithNumber("promotion_id", mcp.Description("Parent promotion id (must be coupon_type=BULK)."), mcp.Required()),
			mcp.WithNumber("batch_size", mcp.Description("Number of codes to generate (1..250)."), mcp.Required()),
			mcp.WithString("prefix", mcp.Description("Optional prefix prepended to every generated code.")),
			mcp.WithString("suffix", mcp.Description("Optional suffix appended to every generated code.")),
			mcp.WithNumber("length", mcp.Description("Generated-portion length (6..16, excludes prefix/suffix).")),
			mcp.WithString("format", mcp.Description("NUMBERS | LETTERS | ALPHANUMERIC.")),
			mcp.WithString("separator", mcp.Description("Separator inserted between prefix/code/suffix.")),
			mcp.WithNumber("max_uses", mcp.Description("Total uses per generated code; 0 = unlimited.")),
			mcp.WithNumber("max_uses_per_customer", mcp.Description("Per-customer use cap per generated code; 0 = unlimited.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to apply.")),
		),
		Handler: cc.handleGenerateBulk,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/coupon/codes/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete coupon codes by id (V3)",
		Description: fmt.Sprintf("DELETE /v3/promotions/{promotion_id}/codes?id:in=… — destructive. "+
			"Max %d ids per call (BC documents 50; we leave headroom). Use this before "+
			"marketing/promotions/coupon/delete on a promotion that still has codes, or as the cleanup path "+
			"after a generate_bulk run. Preview required; pass confirmed=true to execute.",
			maxCouponCodeDeleteIDs,
		),
		Tool: mcp.NewTool("marketing_promotions_coupon_codes_delete",
			mcp.WithDescription("Delete coupon codes from a promotion. Preview required."),
			mcp.WithNumber("promotion_id", mcp.Description("Parent promotion id."), mcp.Required()),
			mcp.WithArray("code_ids", mcp.Description("Coupon code ids to delete (max 40 per call)."),
				mcp.Items(map[string]any{"type": "integer"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to apply.")),
		),
		Handler: cc.handleDelete,
	})
}

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func (cc *CouponCodes) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "promotion_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.CouponCodeListParams{}
	if s, ok := readOptionalString(args, "after"); ok {
		params.After = s
	}
	if s, ok := readOptionalString(args, "before"); ok {
		params.Before = s
	}
	if limit, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.Limit = limit
	}

	resp, err := cc.bc.ListCouponCodes(ctx, id, params)
	if err != nil {
		return shared.ToolError("failed to list coupon codes for promotion %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"promotion_id":  id,
		"total_in_page": len(resp.Codes),
		"codes":         resp.Codes,
		"cursor":        resp.Cursor,
		"has_more":      resp.Cursor.After != "",
	})
}

// ---------------------------------------------------------------------------
// create_single
// ---------------------------------------------------------------------------

func (cc *CouponCodes) handleCreateSingle(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "promotion_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	code, _ := args["code"].(string)
	code = strings.TrimSpace(code)
	if err := validateCouponCodeCharset(code); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	payload := bigcommerce.CouponCodeCreate{Code: code}
	warnings := []string{}

	if v, ok := args["max_uses"]; ok && v != nil {
		f, ok := v.(float64)
		if !ok || f < 0 || f != math.Trunc(f) {
			return shared.ToolError("max_uses must be a non-negative integer (0 = unlimited)"), nil
		}
		n := int(f)
		payload.MaxUses = &n
	}
	if v, ok := args["max_uses_per_customer"]; ok && v != nil {
		f, ok := v.(float64)
		if !ok || f < 0 || f != math.Trunc(f) {
			return shared.ToolError("max_uses_per_customer must be a non-negative integer (0 = unlimited)"), nil
		}
		n := int(f)
		payload.MaxUsesPerCustomer = &n
	}

	// Pre-flight: parent must be a coupon promotion (not AUTOMATIC). Fetching
	// also lets us surface the parent's max_uses-overrides-code behavior.
	parent, err := cc.bc.GetPromotion(ctx, id)
	if err != nil {
		return shared.ToolError("failed to fetch parent promotion %d: %v", id, err), nil
	}
	if parent.RedemptionType == bigcommerce.PromotionRedemptionAutomatic {
		return shared.ToolError("promotion %d is AUTOMATIC; coupon codes can only be added to COUPON promotions", id), nil
	}
	if parent.MaxUses != nil && *parent.MaxUses > 0 {
		warnings = append(warnings, fmt.Sprintf("parent promotion has max_uses=%d which OVERRIDES this code's max_uses (BigCommerce documented behavior)", *parent.MaxUses))
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":       "preview",
			"action":       "create_coupon_code",
			"promotion_id": id,
			"payload":      payload,
			"warnings":     warnings,
			"message":      "Coupon codes are IMMUTABLE. To change max_uses later, delete and recreate. Pass confirmed=true to execute.",
		})
	}

	created, err := cc.bc.CreateCouponCode(ctx, id, payload)
	if err != nil {
		return shared.ToolError("create coupon code failed: %v", err), nil
	}
	resp := map[string]any{
		"status":       "created",
		"promotion_id": id,
		"code":         created,
	}
	if len(warnings) > 0 {
		resp["warnings"] = warnings
	}
	return shared.ToolJSON(resp)
}

// ---------------------------------------------------------------------------
// generate_bulk (codegen)
// ---------------------------------------------------------------------------

func (cc *CouponCodes) handleGenerateBulk(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "promotion_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	batchSize, err := shared.ReadPositiveInt(args, "batch_size")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	req := bigcommerce.CodeGenRequest{BatchSize: batchSize}
	if s, ok := readOptionalString(args, "prefix"); ok {
		req.Prefix = s
	}
	if s, ok := readOptionalString(args, "suffix"); ok {
		req.Suffix = s
	}
	if s, ok := readOptionalString(args, "format"); ok {
		req.Format = strings.ToUpper(s)
	}
	if s, ok := readOptionalString(args, "separator"); ok {
		req.Separator = s
	}
	if v, ok := args["length"]; ok && v != nil {
		f, ok := v.(float64)
		if !ok || f < 0 || f != math.Trunc(f) {
			return shared.ToolError("length must be a non-negative integer"), nil
		}
		req.Length = int(f)
	}
	if v, ok := args["max_uses"]; ok && v != nil {
		f, ok := v.(float64)
		if !ok || f < 0 || f != math.Trunc(f) {
			return shared.ToolError("max_uses must be a non-negative integer"), nil
		}
		n := int(f)
		req.MaxUses = &n
	}
	if v, ok := args["max_uses_per_customer"]; ok && v != nil {
		f, ok := v.(float64)
		if !ok || f < 0 || f != math.Trunc(f) {
			return shared.ToolError("max_uses_per_customer must be a non-negative integer"), nil
		}
		n := int(f)
		req.MaxUsesPerCustomer = &n
	}

	if err := validateCodeGenRequest(req.BatchSize, req.Length, req.Format); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	// Pre-flight: parent must exist and be coupon_type=BULK.
	parent, err := cc.bc.GetPromotion(ctx, id)
	if err != nil {
		return shared.ToolError("failed to fetch parent promotion %d: %v", id, err), nil
	}
	if parent.RedemptionType == bigcommerce.PromotionRedemptionAutomatic {
		return shared.ToolError("promotion %d is AUTOMATIC; codegen requires a COUPON promotion", id), nil
	}
	if strings.ToUpper(parent.CouponType) != bigcommerce.CouponTypeBulk {
		return shared.ToolError("promotion %d has coupon_type=%q; codegen requires coupon_type=BULK. Update the promotion to BULK before generating codes, or use codes/create_single for individual codes.", id, parent.CouponType), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":       "preview",
			"action":       "generate_coupon_codes",
			"promotion_id": id,
			"request":      req,
			"parent": map[string]any{
				"name":        parent.Name,
				"coupon_type": parent.CouponType,
				"status":      parent.Status,
			},
			"message": fmt.Sprintf("Will generate %d codes on promotion %d. BigCommerce caps batch_size at %d per call; for larger runs, repeat. Pass confirmed=true to execute.", req.BatchSize, id, codeGenBatchSizeCap),
		})
	}

	generated, err := cc.bc.GenerateCouponCodes(ctx, id, req)
	if err != nil {
		return shared.ToolError("codegen failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":          "generated",
		"promotion_id":    id,
		"generated_count": len(generated),
		"sample":          sampleCodes(generated, 5),
		"request":         req,
	})
}

// sampleCodes returns up to n code strings from the head of the slice — the
// codegen response can be large, so the tool returns a small sample plus the
// total count rather than the entire payload.
func sampleCodes(codes []bigcommerce.CouponCode, n int) []string {
	if n > len(codes) {
		n = len(codes)
	}
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, codes[i].Code)
	}
	return out
}

// ---------------------------------------------------------------------------
// delete
// ---------------------------------------------------------------------------

func (cc *CouponCodes) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "promotion_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	codeIDs, err := requiredPositiveIntIDs(args, "code_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(codeIDs) > maxCouponCodeDeleteIDs {
		return shared.ToolError("code_ids exceeds max of %d per call", maxCouponCodeDeleteIDs), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		if err := cc.bc.DeleteCouponCodes(ctx, id, codeIDs); err != nil {
			return shared.ToolError("delete coupon codes failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{
			"status":       "deleted",
			"promotion_id": id,
			"code_ids":     codeIDs,
		})
	}
	return shared.ToolJSON(map[string]any{
		"status":          "preview",
		"action":          "delete_coupon_codes",
		"promotion_id":    id,
		"would_delete_ct": len(codeIDs),
		"code_ids":        codeIDs,
		"message":         "Pass confirmed=true to permanently delete these coupon codes.",
	})
}
