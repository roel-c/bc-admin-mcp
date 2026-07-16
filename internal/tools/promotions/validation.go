// Package promotions hosts MCP tool handlers for the BigCommerce Promotions
// API (V3: /v3/promotions). Slice 1 covers AUTOMATIC promotions; slice 2
// covers COUPON promotions and the /codes + /codegen sub-resources.
//
// The promotions rule engine is deeply polymorphic: each rule contains
// exactly one of five action shapes (cart_items / cart_value / shipping /
// gift_item / fixed_price_set), an optional condition tree (cart / and / or
// / not), and a recursive item-matcher (products / categories / brands /
// variants plus and / or / not operators). Rather than translate the entire
// shape into a typed Go AST, we accept the BigCommerce JSON verbatim and
// validate the shape at this layer before posting to BC. This keeps the
// surface honest with BC's own schema while catching the obvious 422-class
// errors before they round-trip.
package promotions

import (
	"fmt"
	"strings"
)

// validActionKeys enumerates the five action shapes documented by BC. A
// rule.action object must have exactly one of these keys at top level.
var validActionKeys = map[string]struct{}{
	"cart_items":      {},
	"cart_value":      {},
	"shipping":        {},
	"gift_item":       {},
	"fixed_price_set": {},
}

// validStrategies enumerates cart_items.strategy values.
var validStrategies = map[string]struct{}{
	"LEAST_EXPENSIVE": {},
	"MOST_EXPENSIVE":  {},
}

// validNotificationTypes enumerates notification.type values.
var validNotificationTypes = map[string]struct{}{
	"PROMOTION": {},
	"UPSELL":    {},
	"ELIGIBLE":  {},
	"APPLIED":   {},
}

// validNotificationLocations enumerates notification.locations[] values.
var validNotificationLocations = map[string]struct{}{
	"HOME_PAGE":     {},
	"PRODUCT_PAGE":  {},
	"CART_PAGE":     {},
	"CHECKOUT_PAGE": {},
}

// validStatuses enumerates promotion.status values writable via API. INVALID
// is read-only (BC sets it on rule transitions) and rejected here.
var validStatusesWritable = map[string]struct{}{
	"ENABLED":  {},
	"DISABLED": {},
}

// validCouponTypes enumerates coupon_type values for COUPON promotions.
var validCouponTypes = map[string]struct{}{
	"SINGLE": {},
	"BULK":   {},
}

// couponOnlyOuterFields lists outer-level promotion fields that only make
// sense when redemption_type=COUPON. They're rejected on AUTOMATIC writes.
var couponOnlyOuterFields = []string{
	"coupon_type",
	"coupon_overrides_other_promotions",
	"coupon_overrides_automatic_when_offering_higher_discounts",
	"multiple_codes",
}

// deprecatedCouponField is the deprecated override flag. Per slice-2 design
// (REJECT mode) we never accept it on writes — operators should use
// coupon_overrides_other_promotions instead.
const deprecatedCouponField = "coupon_overrides_automatic_when_offering_higher_discounts"

// validatePromotionDraft runs every shape check on a draft (create or
// update) payload. It returns a single error joining all violations so
// callers can surface them in one round-trip.
//
// Required at this level:
//   - rules[] is present and non-empty
//   - each rule has exactly one valid action shape
//   - each action's required sub-fields are present and well-typed
//   - condition/item-matcher trees only use documented keys
//   - customer.group_ids and customer.excluded_group_ids cannot both be set
//   - notifications[] (when present) use known type/locations
//   - status (when set) is ENABLED or DISABLED
//   - currency_code (when set) is "*" or a 3-letter uppercase code
func validatePromotionDraft(payload map[string]any) error {
	var errs []string

	if v, ok := payload["status"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			errs = append(errs, "status must be a string")
		} else if _, ok := validStatusesWritable[s]; !ok {
			errs = append(errs, fmt.Sprintf("status must be ENABLED or DISABLED (INVALID is read-only); got %q", s))
		}
	}

	if v, ok := payload["currency_code"]; ok && v != nil {
		if s, ok := v.(string); ok {
			if err := validateCurrencyCode(s); err != nil {
				errs = append(errs, err.Error())
			}
		} else {
			errs = append(errs, "currency_code must be a string")
		}
	}

	if v, ok := payload["customer"]; ok && v != nil {
		if err := validateCustomer(v); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if v, ok := payload["notifications"]; ok && v != nil {
		if err := validateNotifications(v); err != nil {
			errs = append(errs, err.Error())
		}
	}

	rules, ok := payload["rules"]
	if !ok || rules == nil {
		errs = append(errs, "rules is required and must contain at least one rule")
	} else {
		if err := validateRules(rules); err != nil {
			errs = append(errs, err.Error())
		}
	}

	// Redemption-type-aware coupon checks. Both create and update paths
	// stamp redemption_type on the merged payload before calling here.
	if err := validateCouponConsistency(payload); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid promotion payload: %s", strings.Join(errs, "; "))
}

// validateCouponConsistency enforces cross-field rules that depend on
// redemption_type. The function is intentionally tolerant when
// redemption_type is absent: callers are expected to stamp it before
// validating, but we still reject the deprecated field unconditionally.
func validateCouponConsistency(payload map[string]any) error {
	// Always reject the deprecated override flag, regardless of redemption_type.
	if _, ok := payload[deprecatedCouponField]; ok {
		return fmt.Errorf("%s is deprecated and not accepted by these tools; use coupon_overrides_other_promotions instead", deprecatedCouponField)
	}

	rt, _ := payload["redemption_type"].(string)
	switch strings.ToUpper(rt) {
	case "AUTOMATIC":
		// Coupon-only fields don't apply to AUTOMATIC promotions.
		var bad []string
		for _, k := range couponOnlyOuterFields {
			if _, ok := payload[k]; ok {
				bad = append(bad, k)
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("AUTOMATIC promotions cannot set coupon-only fields: %s", strings.Join(bad, ", "))
		}
		return nil
	case "COUPON":
		return validateCouponOuterFields(payload)
	default:
		// No redemption_type stamped (defensive). We still flag the deprecated
		// field via the early-return above and otherwise let the call through.
		return nil
	}
}

// validateCouponOuterFields enforces the COUPON-promotion-specific rules
// documented by BigCommerce:
//   - coupon_type ∈ SINGLE | BULK (default SINGLE on read; allowed on writes)
//   - coupon_overrides_other_promotions=true requires
//     can_be_used_with_other_promotions=false (BC 422 otherwise)
//   - multiple_codes is only meaningful when coupon_type=BULK; we warn-via-
//     reject if it's set on a SINGLE coupon promotion since BC ignores it
//     anyway and the operator is likely confused.
func validateCouponOuterFields(payload map[string]any) error {
	var errs []string

	ct, hasCT := payload["coupon_type"]
	if hasCT && ct != nil {
		s, ok := ct.(string)
		if !ok {
			errs = append(errs, "coupon_type must be a string")
		} else if _, ok := validCouponTypes[strings.ToUpper(s)]; !ok {
			errs = append(errs, fmt.Sprintf("coupon_type must be SINGLE or BULK; got %q", s))
		}
	}

	overridesRaw, hasOverrides := payload["coupon_overrides_other_promotions"]
	if hasOverrides && overridesRaw != nil {
		overrides, ok := overridesRaw.(bool)
		if !ok {
			errs = append(errs, "coupon_overrides_other_promotions must be a boolean")
		} else if overrides {
			cb, hasCB := payload["can_be_used_with_other_promotions"]
			cbBool, _ := cb.(bool)
			// BC default for can_be_used_with_other_promotions is true; the
			// override flag is only valid when the merchant has explicitly
			// set it to false.
			if !hasCB || cbBool {
				errs = append(errs, "coupon_overrides_other_promotions=true requires can_be_used_with_other_promotions=false (BigCommerce rejects otherwise)")
			}
		}
	}

	if mc, ok := payload["multiple_codes"]; ok && mc != nil {
		// Only meaningful on BULK; warn-via-reject so operators don't think
		// they're configuring something that BC silently ignores.
		ctStr, _ := payload["coupon_type"].(string)
		if strings.ToUpper(ctStr) != "BULK" {
			errs = append(errs, "multiple_codes is only valid on coupon_type=BULK promotions")
		}
		if _, ok := mc.(map[string]any); !ok {
			errs = append(errs, "multiple_codes must be an object")
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

func validateCurrencyCode(s string) error {
	if s == "*" {
		return nil
	}
	if len(s) != 3 {
		return fmt.Errorf("currency_code must be a 3-letter ISO-4217 code or %q; got %q", "*", s)
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("currency_code must be uppercase A-Z; got %q", s)
		}
	}
	return nil
}

func validateCustomer(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("customer must be an object")
	}
	groupNonEmpty := arrayHasItems(m["group_ids"])
	excludedNonEmpty := arrayHasItems(m["excluded_group_ids"])
	if groupNonEmpty && excludedNonEmpty {
		return fmt.Errorf("customer.group_ids and customer.excluded_group_ids cannot both be non-empty (BigCommerce rejects with 422)")
	}
	return nil
}

func arrayHasItems(v any) bool {
	a, ok := v.([]any)
	return ok && len(a) > 0
}

func validateNotifications(v any) error {
	a, ok := v.([]any)
	if !ok {
		return fmt.Errorf("notifications must be an array")
	}
	for i, raw := range a {
		obj, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("notifications[%d] must be an object", i)
		}
		t, _ := obj["type"].(string)
		if t == "" {
			return fmt.Errorf("notifications[%d].type is required", i)
		}
		if _, ok := validNotificationTypes[t]; !ok {
			return fmt.Errorf("notifications[%d].type must be one of PROMOTION/UPSELL/ELIGIBLE/APPLIED; got %q", i, t)
		}
		locs, ok := obj["locations"].([]any)
		if !ok || len(locs) == 0 {
			return fmt.Errorf("notifications[%d].locations is required and must be a non-empty array", i)
		}
		for j, l := range locs {
			s, ok := l.(string)
			if !ok {
				return fmt.Errorf("notifications[%d].locations[%d] must be a string", i, j)
			}
			if _, ok := validNotificationLocations[s]; !ok {
				return fmt.Errorf("notifications[%d].locations[%d] must be one of HOME_PAGE/PRODUCT_PAGE/CART_PAGE/CHECKOUT_PAGE; got %q", i, j, s)
			}
		}
	}
	return nil
}

// validateRules verifies the rules array. BC documents that 10+ rules per
// promotion is discouraged; we don't block here but the tools layer surfaces
// a soft warning.
func validateRules(v any) error {
	a, ok := v.([]any)
	if !ok {
		return fmt.Errorf("rules must be an array")
	}
	if len(a) == 0 {
		return fmt.Errorf("rules must contain at least one rule")
	}
	for i, raw := range a {
		obj, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("rules[%d] must be an object", i)
		}
		if err := validateRule(obj); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
	}
	return nil
}

func validateRule(rule map[string]any) error {
	action, ok := rule["action"].(map[string]any)
	if !ok {
		return fmt.Errorf("action is required and must be an object")
	}
	if err := validateAction(action); err != nil {
		return fmt.Errorf("action: %w", err)
	}
	if cond, ok := rule["condition"]; ok && cond != nil {
		if err := validateCondition(cond); err != nil {
			return fmt.Errorf("condition: %w", err)
		}
	}
	return nil
}

// validateAction enforces "exactly one of the five action shapes" and the
// minimum required fields per shape.
func validateAction(action map[string]any) error {
	matched := ""
	for k := range action {
		if _, ok := validActionKeys[k]; ok {
			if matched != "" {
				return fmt.Errorf("must contain exactly one of cart_items/cart_value/shipping/gift_item/fixed_price_set; saw both %q and %q", matched, k)
			}
			matched = k
		}
	}
	if matched == "" {
		return fmt.Errorf("must contain exactly one of cart_items/cart_value/shipping/gift_item/fixed_price_set")
	}
	body, _ := action[matched].(map[string]any)
	switch matched {
	case "cart_items":
		return validateCartItemsAction(body)
	case "cart_value":
		return validateCartValueAction(body)
	case "shipping":
		return validateShippingAction(body)
	case "gift_item":
		return validateGiftItemAction(body)
	case "fixed_price_set":
		return validateFixedPriceSetAction(body)
	}
	return nil
}

func validateCartItemsAction(b map[string]any) error {
	if b == nil {
		return fmt.Errorf("cart_items must be an object")
	}
	disc, ok := b["discount"].(map[string]any)
	if !ok {
		return fmt.Errorf("cart_items.discount is required")
	}
	if err := validateDiscount(disc); err != nil {
		return fmt.Errorf("cart_items.discount: %w", err)
	}
	if v, ok := b["strategy"]; ok && v != nil {
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("cart_items.strategy must be a string")
		}
		if _, ok := validStrategies[s]; !ok {
			return fmt.Errorf("cart_items.strategy must be LEAST_EXPENSIVE or MOST_EXPENSIVE; got %q", s)
		}
	}
	if items, ok := b["items"]; ok && items != nil {
		if err := validateItemMatcher(items); err != nil {
			return fmt.Errorf("cart_items.items: %w", err)
		}
	}
	return nil
}

func validateCartValueAction(b map[string]any) error {
	if b == nil {
		return fmt.Errorf("cart_value must be an object")
	}
	disc, ok := b["discount"].(map[string]any)
	if !ok {
		return fmt.Errorf("cart_value.discount is required")
	}
	return validateDiscount(disc)
}

func validateShippingAction(b map[string]any) error {
	if b == nil {
		return fmt.Errorf("shipping must be an object")
	}
	free, ok := b["free_shipping"].(bool)
	if !ok || !free {
		return fmt.Errorf("shipping.free_shipping must be true (the only documented shipping action shape)")
	}
	return nil
}

func validateGiftItemAction(b map[string]any) error {
	if b == nil {
		return fmt.Errorf("gift_item must be an object")
	}
	pid, ok := b["product_id"].(float64)
	if !ok || pid <= 0 {
		return fmt.Errorf("gift_item.product_id is required and must be a positive integer")
	}
	q, ok := b["quantity"].(float64)
	if !ok || q < 1 {
		return fmt.Errorf("gift_item.quantity is required and must be >= 1")
	}
	return nil
}

func validateFixedPriceSetAction(b map[string]any) error {
	if b == nil {
		return fmt.Errorf("fixed_price_set must be an object")
	}
	if _, ok := b["price"]; !ok {
		return fmt.Errorf("fixed_price_set.price is required (string-encoded amount)")
	}
	if items, ok := b["items"]; ok && items != nil {
		if err := validateItemMatcher(items); err != nil {
			return fmt.Errorf("fixed_price_set.items: %w", err)
		}
	}
	return nil
}

// validateDiscount enforces "exactly one of percentage_amount or fixed_amount".
// BigCommerce stores amounts as strings (e.g. "10", "5.0000").
func validateDiscount(d map[string]any) error {
	_, hasPct := d["percentage_amount"]
	_, hasFixed := d["fixed_amount"]
	if hasPct && hasFixed {
		return fmt.Errorf("must contain exactly one of percentage_amount or fixed_amount, not both")
	}
	if !hasPct && !hasFixed {
		return fmt.Errorf("must contain exactly one of percentage_amount or fixed_amount")
	}
	return nil
}

// validateItemMatcher walks the recursive item matcher used by
// cart_items.items, fixed_price_set.items, and inside cart conditions. The
// documented leaves are products / categories / brands / variants (each a
// non-empty integer array); the operators are and / or (arrays of matchers)
// and not (single matcher).
func validateItemMatcher(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("item matcher must be an object")
	}
	keyCount := 0
	for k, val := range m {
		switch k {
		case "products", "categories", "brands", "variants":
			keyCount++
			arr, ok := val.([]any)
			if !ok || len(arr) == 0 {
				return fmt.Errorf("%s must be a non-empty array of integers", k)
			}
			for i, x := range arr {
				if _, ok := x.(float64); !ok {
					return fmt.Errorf("%s[%d] must be an integer", k, i)
				}
			}
		case "and", "or":
			keyCount++
			arr, ok := val.([]any)
			if !ok || len(arr) == 0 {
				return fmt.Errorf("%s must be a non-empty array of item matchers", k)
			}
			for i, child := range arr {
				if err := validateItemMatcher(child); err != nil {
					return fmt.Errorf("%s[%d]: %w", k, i, err)
				}
			}
		case "not":
			keyCount++
			if err := validateItemMatcher(val); err != nil {
				return fmt.Errorf("not: %w", err)
			}
		default:
			return fmt.Errorf("unknown item matcher key %q (allowed: products/categories/brands/variants/and/or/not)", k)
		}
	}
	if keyCount == 0 {
		return fmt.Errorf("item matcher must contain at least one of products/categories/brands/variants/and/or/not")
	}
	return nil
}

// validateCondition walks a rule.condition. It is a one-of: cart / and / or
// / not. The cart variant additionally embeds an item matcher under items.
func validateCondition(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("condition must be an object")
	}
	matched := ""
	for k := range m {
		switch k {
		case "cart", "and", "or", "not":
			if matched != "" {
				return fmt.Errorf("condition must contain exactly one of cart/and/or/not; saw both %q and %q", matched, k)
			}
			matched = k
		default:
			return fmt.Errorf("unknown condition key %q (allowed: cart/and/or/not)", k)
		}
	}
	if matched == "" {
		return fmt.Errorf("condition must contain exactly one of cart/and/or/not")
	}
	switch matched {
	case "cart":
		return validateCartCondition(m["cart"])
	case "and", "or":
		arr, ok := m[matched].([]any)
		if !ok || len(arr) == 0 {
			return fmt.Errorf("%s must be a non-empty array of conditions", matched)
		}
		for i, child := range arr {
			if err := validateCondition(child); err != nil {
				return fmt.Errorf("%s[%d]: %w", matched, i, err)
			}
		}
	case "not":
		return validateCondition(m["not"])
	}
	return nil
}

func validateCartCondition(v any) error {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("cart must be an object")
	}
	if items, ok := m["items"]; ok && items != nil {
		if err := validateItemMatcher(items); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	}
	if mq, ok := m["minimum_quantity"]; ok && mq != nil {
		if _, ok := mq.(float64); !ok {
			return fmt.Errorf("minimum_quantity must be an integer")
		}
	}
	return nil
}

// maxCouponCodeLength mirrors BigCommerce's documented per-code length cap.
const maxCouponCodeLength = 50

// validateCouponCodeCharset rejects coupon codes that don't match BC's
// documented charset (letters, numbers, spaces, underscores, hyphens) or
// exceed the 50-character cap. This catches the common 422 cases before the
// POST goes out.
func validateCouponCodeCharset(code string) error {
	if code == "" {
		return fmt.Errorf("code is required")
	}
	if len(code) > maxCouponCodeLength {
		return fmt.Errorf("code exceeds %d characters (got %d)", maxCouponCodeLength, len(code))
	}
	for i, r := range code {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == ' ' || r == '_' || r == '-':
		default:
			return fmt.Errorf("code contains invalid character %q at position %d (allowed: letters, numbers, spaces, underscores, hyphens)", r, i)
		}
	}
	return nil
}

// validCodegenFormats enumerates the documented BC codegen formats. We
// accept the documented values verbatim and let unknown values pass through
// to BC for any newer formats they may add.
var validCodegenFormats = map[string]struct{}{
	"NUMBERS":      {},
	"LETTERS":      {},
	"ALPHANUMERIC": {},
}

// validateCodeGenRequest enforces the documented bounds for /codegen so the
// tools layer can surface validation errors before round-tripping.
//
//	batch_size: 1..250 (BC hard cap is 250)
//	length:     6..16 when set (BC documents this range; excludes prefix/suffix)
//	format:     uppercase enum when set
func validateCodeGenRequest(batchSize int, length int, format string) error {
	var errs []string
	if batchSize <= 0 {
		errs = append(errs, "batch_size must be at least 1")
	}
	if batchSize > 250 {
		errs = append(errs, fmt.Sprintf("batch_size cannot exceed BigCommerce's per-call limit of 250 (got %d)", batchSize))
	}
	if length != 0 {
		if length < 6 || length > 16 {
			errs = append(errs, fmt.Sprintf("length must be between 6 and 16 when set; got %d", length))
		}
	}
	if format != "" {
		if _, ok := validCodegenFormats[strings.ToUpper(format)]; !ok {
			errs = append(errs, fmt.Sprintf("format must be one of NUMBERS/LETTERS/ALPHANUMERIC; got %q", format))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("invalid codegen request: %s", strings.Join(errs, "; "))
}
