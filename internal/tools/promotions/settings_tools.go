package promotions

import (
	"context"
	"fmt"
	"math"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// PromotionSettings holds tool handlers for /v3/promotions/settings — the
// store-wide policies that sit above individual promotions and govern how
// price overrides, zero-price products, multi-coupon checkout, and price
// cascades behave for every promotion.
type PromotionSettingsTools struct {
	bc    BigCommercePromotionsAPI
	cache *session.Store
}

// NewPromotionSettingsTools constructs the settings tool handlers.
func NewPromotionSettingsTools(bc BigCommercePromotionsAPI, cache *session.Store) *PromotionSettingsTools {
	return &PromotionSettingsTools{bc: bc, cache: cache}
}

// RegisterTools wires the marketing/promotions/settings/* tools into the
// discovery registry.
func (p *PromotionSettingsTools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/settings/get",
		Tier:    middleware.TierR0,
		Summary: "Get store-wide promotion settings (V3)",
		Description: "GET /v3/promotions/settings. Returns the four documented store-wide policy flags: " +
			"`promotions_triggered_by_products_with_zero_product_price` (default false), " +
			"`promotions_apply_on_products_with_custom_product_price` (default false), " +
			"`number_of_coupons_allowed_at_checkout` (default 1; values >1 are Enterprise-only), " +
			"`promotions_applied_on_original_product_price` (default true). Read-only.",
		Tool: mcp.NewTool("marketing_promotions_settings_get",
			mcp.WithDescription("Read store-wide promotion policy settings."),
		),
		Handler: p.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "marketing/promotions/settings/update",
		Tier:    middleware.TierR2,
		Summary: "Update store-wide promotion settings (V3)",
		Description: fmt.Sprintf("PUT /v3/promotions/settings. Fetch-merge-PUT: only fields supplied in this call "+
			"are changed; everything else carries forward from current. Bounds enforced client-side: "+
			"`number_of_coupons_allowed_at_checkout` ∈ 1..%d (BC's documented max); booleans type-checked. "+
			"Soft-warn surfaces in preview when raising `number_of_coupons_allowed_at_checkout` above 1 "+
			"(Enterprise-plan-only feature; non-Enterprise stores 403). Identical-to-current patches "+
			"short-circuit to `noop` without making the PUT call. R2 high-risk: settings affect every "+
			"order and every channel; preview required, pass confirmed=true to execute.",
			bigcommerce.MaxCouponsAtCheckout,
		),
		Tool: mcp.NewTool("marketing_promotions_settings_update",
			mcp.WithDescription("Update store-wide promotion settings via fetch-merge-PUT. Preview required."),
			mcp.WithBoolean("promotions_triggered_by_products_with_zero_product_price",
				mcp.Description("When true, $0 line items can satisfy promotion conditions.")),
			mcp.WithBoolean("promotions_apply_on_products_with_custom_product_price",
				mcp.Description("When true, line items with manual / cart-API price overrides are eligible for promotions.")),
			mcp.WithNumber("number_of_coupons_allowed_at_checkout",
				mcp.Description("Maximum coupon codes per order (1..5). Values >1 require Enterprise plan.")),
			mcp.WithBoolean("promotions_applied_on_original_product_price",
				mcp.Description("When true, discounts compute against original list prices; when false they cascade against running discounted subtotals.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing the preview.")),
		),
		Handler: p.handleUpdate,
	})
}

// ---------------------------------------------------------------------------
// get
// ---------------------------------------------------------------------------

func (p *PromotionSettingsTools) handleGet(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	settings, err := p.bc.GetPromotionSettings(ctx)
	if err != nil {
		return shared.ToolError("failed to fetch promotion settings: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"settings": settings,
		"notes": []string{
			"`number_of_coupons_allowed_at_checkout` > 1 is an Enterprise-plan feature.",
			"All settings are store-wide (apply to every channel and every order).",
		},
	})
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func (p *PromotionSettingsTools) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	// Collect typed patches, rejecting wrong-typed inputs early.
	patch, warnings, err := readSettingsPatch(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if patch.IsEmpty() {
		return shared.ToolError("no settings supplied — provide at least one of: promotions_triggered_by_products_with_zero_product_price, promotions_apply_on_products_with_custom_product_price, number_of_coupons_allowed_at_checkout, promotions_applied_on_original_product_price"), nil
	}

	// Cache the settings snapshot so the confirm call reuses the preview's
	// fetch instead of a second GET within the TTL window.
	const settingsCacheKey = "promotion_settings_update"
	current, err := session.CacheOrFetch(p.cache.ForContext(ctx), settingsCacheKey, func() (*bigcommerce.PromotionSettings, error) {
		return p.bc.GetPromotionSettings(ctx)
	})
	if err != nil {
		return shared.ToolError("failed to fetch current promotion settings: %v", err), nil
	}

	merged := mergeSettingsPatch(*current, patch)

	if merged == *current {
		return shared.ToolJSON(map[string]any{
			"status":   "noop",
			"current":  current,
			"message":  "supplied patch matches current settings; no PUT issued",
			"warnings": warnings,
		})
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "update_promotion_settings",
			"current":     current,
			"would_apply": merged,
			"warnings":    warnings,
			"message":     "Settings are store-wide and affect every order. Pass confirmed=true to execute.",
		})
	}

	updated, err := p.bc.UpdatePromotionSettings(ctx, merged)
	if err != nil {
		hint := ""
		if patch.NumberOfCouponsSet && merged.NumberOfCouponsAllowedAtCheckout > 1 {
			hint = " — number_of_coupons_allowed_at_checkout > 1 requires an Enterprise plan; verify the store's plan tier"
		}
		return shared.ToolError("update failed: %v%s", err, hint), nil
	}
	p.cache.ForContext(ctx).Delete(settingsCacheKey)
	return shared.ToolJSON(map[string]any{
		"status":   "updated",
		"settings": updated,
		"warnings": warnings,
	})
}

// settingsPatch is an internal optional-fields shape used to track which of
// the four documented settings the operator actually supplied. We need the
// "set vs unset" distinction so booleans-set-to-false don't get clobbered
// by the merge logic.
type settingsPatch struct {
	ZeroPriceTriggers     bool
	ZeroPriceTriggersSet  bool
	CustomPriceApplies    bool
	CustomPriceAppliesSet bool
	NumberOfCoupons       int
	NumberOfCouponsSet    bool
	OriginalPriceApply    bool
	OriginalPriceApplySet bool
}

func readSettingsPatch(args map[string]any) (settingsPatch, []string, error) {
	var p settingsPatch
	var warnings []string

	if v, ok := args["promotions_triggered_by_products_with_zero_product_price"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return p, nil, fmt.Errorf("promotions_triggered_by_products_with_zero_product_price must be a boolean")
		}
		p.ZeroPriceTriggers = b
		p.ZeroPriceTriggersSet = true
	}
	if v, ok := args["promotions_apply_on_products_with_custom_product_price"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return p, nil, fmt.Errorf("promotions_apply_on_products_with_custom_product_price must be a boolean")
		}
		p.CustomPriceApplies = b
		p.CustomPriceAppliesSet = true
	}
	if v, ok := args["promotions_applied_on_original_product_price"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return p, nil, fmt.Errorf("promotions_applied_on_original_product_price must be a boolean")
		}
		p.OriginalPriceApply = b
		p.OriginalPriceApplySet = true
	}
	if v, ok := args["number_of_coupons_allowed_at_checkout"]; ok && v != nil {
		f, ok := v.(float64)
		if !ok {
			return p, nil, fmt.Errorf("number_of_coupons_allowed_at_checkout must be an integer")
		}
		if f != math.Trunc(f) {
			return p, nil, fmt.Errorf("number_of_coupons_allowed_at_checkout must be an integer")
		}
		n := int(f)
		if n < 1 || n > bigcommerce.MaxCouponsAtCheckout {
			return p, nil, fmt.Errorf("number_of_coupons_allowed_at_checkout must be between 1 and %d (got %d)", bigcommerce.MaxCouponsAtCheckout, n)
		}
		p.NumberOfCoupons = n
		p.NumberOfCouponsSet = true
		if n > 1 {
			warnings = append(warnings, "raising number_of_coupons_allowed_at_checkout above 1 is an Enterprise-plan feature; non-Enterprise stores will return 403")
		}
	}
	return p, warnings, nil
}

// length returns how many fields the operator actually supplied; used to
// short-circuit when no patch fields were provided at all.
func (s settingsPatch) length() int {
	n := 0
	if s.ZeroPriceTriggersSet {
		n++
	}
	if s.CustomPriceAppliesSet {
		n++
	}
	if s.NumberOfCouponsSet {
		n++
	}
	if s.OriginalPriceApplySet {
		n++
	}
	return n
}

// IsEmpty reports whether the patch contains no supplied fields.
func (s settingsPatch) IsEmpty() bool { return s.length() == 0 }

// mergeSettingsPatch overlays the operator's typed patch onto the current
// settings; unset fields carry forward unchanged.
func mergeSettingsPatch(current bigcommerce.PromotionSettings, p settingsPatch) bigcommerce.PromotionSettings {
	out := current
	if p.ZeroPriceTriggersSet {
		out.PromotionsTriggeredByZeroPriceProducts = p.ZeroPriceTriggers
	}
	if p.CustomPriceAppliesSet {
		out.PromotionsApplyOnCustomPricedProducts = p.CustomPriceApplies
	}
	if p.NumberOfCouponsSet {
		out.NumberOfCouponsAllowedAtCheckout = p.NumberOfCoupons
	}
	if p.OriginalPriceApplySet {
		out.PromotionsAppliedOnOriginalProductPrice = p.OriginalPriceApply
	}
	return out
}
