package catalog

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

// maxBulkVariantMetafieldTargets caps how many distinct variants a single bulk
// variant metafield call may touch (sequential API calls per variant).
const maxBulkVariantMetafieldTargets = 50

// maxBulkCrossProductVariantOps caps total variant-level metafield operations
// (one upsert or delete each) across all products in bulk_set_products /
// bulk_delete_products, to keep a single MCP call within reasonable time.
const maxBulkCrossProductVariantOps = 500

// variant_scope values for bulk tools that target many products.
const (
	variantScopeAllVariants        = "all_variants"
	variantScopeFirstVariantOnly   = "first_variant_only"
	variantScopeSKUContains        = "sku_contains"
)

// ParseBulkVariantMetafieldVariantIDs extracts variant_ids for bulk variant
// metafield tools; dedupes; enforces max.
func ParseBulkVariantMetafieldVariantIDs(args map[string]any) ([]int, error) {
	raw, ok := args["variant_ids"]
	if !ok {
		return nil, fmt.Errorf("variant_ids is required")
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("variant_ids must be an array")
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("variant_ids must be non-empty")
	}
	seen := make(map[int]struct{})
	out := make([]int, 0, len(arr))
	for i, item := range arr {
		f, fOk := item.(float64)
		if !fOk {
			return nil, fmt.Errorf("variant_ids[%d] must be a number", i)
		}
		id := int(f)
		if id <= 0 {
			return nil, fmt.Errorf("variant_ids[%d] must be a positive variant id", i)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("variant_ids must contain at least one valid id")
	}
	if len(out) > maxBulkVariantMetafieldTargets {
		return nil, fmt.Errorf("variant_ids exceeds maximum of %d for bulk variant metafield operations", maxBulkVariantMetafieldTargets)
	}
	return out, nil
}

// BulkSingleProductVariantSelection is either explicit variant_ids or a
// case-insensitive substring match on variant SKU (mutually exclusive).
type BulkSingleProductVariantSelection struct {
	VariantIDs  []int
	SKUContains string
}

// ParseBulkSingleProductVariantSelection parses bulk_set / bulk_delete variant
// targeting for a single product (exported for unit tests).
func ParseBulkSingleProductVariantSelection(args map[string]any) (*BulkSingleProductVariantSelection, error) {
	rawNeedle, hasNeedleKey := args["variant_sku_contains"]
	needle := ""
	if hasNeedleKey {
		s, ok := rawNeedle.(string)
		if !ok {
			return nil, fmt.Errorf("variant_sku_contains must be a string")
		}
		needle = strings.TrimSpace(s)
	}

	rawIDs, hasIDsKey := args["variant_ids"]
	hasNonemptyIDs := false
	if hasIDsKey {
		arr, ok := rawIDs.([]any)
		hasNonemptyIDs = ok && len(arr) > 0
	}

	if needle != "" && hasNonemptyIDs {
		return nil, fmt.Errorf("use only one of: variant_ids or variant_sku_contains")
	}
	if needle != "" {
		return &BulkSingleProductVariantSelection{SKUContains: needle}, nil
	}
	if !hasIDsKey {
		return nil, fmt.Errorf("provide variant_ids or variant_sku_contains")
	}
	ids, err := ParseBulkVariantMetafieldVariantIDs(args)
	if err != nil {
		return nil, err
	}
	return &BulkSingleProductVariantSelection{VariantIDs: ids}, nil
}

func filterVariantIDsBySKUContains(variants []bigcommerce.Variant, needle string) []int {
	nl := strings.ToLower(needle)
	out := make([]int, 0)
	for i := range variants {
		if strings.Contains(strings.ToLower(variants[i].SKU), nl) {
			out = append(out, variants[i].ID)
		}
	}
	return out
}

func (p *Products) resolveVariantIDsForSingleProductBulk(
	ctx context.Context,
	productID int,
	sel *BulkSingleProductVariantSelection,
) ([]int, error) {
	if sel.SKUContains != "" {
		variants, err := p.bc.ListVariantsForProduct(ctx, productID)
		if err != nil {
			return nil, err
		}
		vids := filterVariantIDsBySKUContains(variants, sel.SKUContains)
		if len(vids) == 0 {
			return nil, fmt.Errorf("no variants on product %d have SKU containing %q (match is case-insensitive)", productID, sel.SKUContains)
		}
		if len(vids) > maxBulkVariantMetafieldTargets {
			return nil, fmt.Errorf(
				"variant_sku_contains matched %d variants (max %d); use a more specific substring or pass explicit variant_ids",
				len(vids), maxBulkVariantMetafieldTargets,
			)
		}
		return vids, nil
	}
	if err := validateVariantIDsOnProduct(ctx, p.bc, productID, sel.VariantIDs); err != nil {
		return nil, err
	}
	return sel.VariantIDs, nil
}

func parseBulkVariantMetafieldProductTarget(args map[string]any) (productID int, sku string, productName string, err error) {
	if err := parseProductMetafieldTargetArgs(args, &productID, &sku, &productName); err != nil {
		return 0, "", "", err
	}
	return productID, sku, productName, nil
}

func validateVariantIDsOnProduct(
	ctx context.Context,
	bc BigCommerceAPI,
	productID int,
	variantIDs []int,
) error {
	variants, err := bc.ListVariantsForProduct(ctx, productID)
	if err != nil {
		return fmt.Errorf("list variants for product %d: %w", productID, err)
	}
	valid := make(map[int]struct{}, len(variants))
	for i := range variants {
		valid[variants[i].ID] = struct{}{}
	}
	for _, vid := range variantIDs {
		if _, ok := valid[vid]; !ok {
			return fmt.Errorf("variant %d not found on product %d", vid, productID)
		}
	}
	return nil
}

// RegisterVariantMetafieldBulkTools registers bulk variant metafield tools.
func (p *Products) RegisterVariantMetafieldBulkTools(reg *discovery.Registry) {
	maxStr := strconv.Itoa(maxBulkVariantMetafieldTargets)
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/metafields/bulk_set",
		Tier:    middleware.TierR1,
		Summary: "Set the same metafield (namespace+key+value) on many variants of one product",
		Description: "Resolves one product, then upserts one metafield on each target variant (sequential API calls). " +
			"Target variants with variant_ids OR variant_sku_contains (case-insensitive substring on variant SKU), not both. " +
			"Maximum " + maxStr + " variants per call. Optional permission_set; new rows default to app_only. Preview then confirmed=true.",
		Tool: mcp.NewTool("catalog_products_variants_metafields_bulk_set",
			mcp.WithDescription(
				"Bulk upsert the same namespace+key+value on many variants. "+
					"Product: exactly one of product_id, sku, or product_name. "+
					"Targets: variant_ids (deduped, max "+maxStr+") OR variant_sku_contains (e.g. \"-XYZ-\" matches any variant SKU containing that substring, case-insensitive). "+
					"Required: namespace, key, value. Optional description, permission_set.",
			),
			mcp.WithNumber("product_id", mcp.Description("Numeric product ID")),
			mcp.WithString("sku", mcp.Description("Exact product SKU")),
			mcp.WithString("product_name", mcp.Description("Exact product name")),
			mcp.WithArray("variant_ids",
				mcp.Description("Variant IDs on the resolved product (deduped; max "+maxStr+"); omit if using variant_sku_contains"),
				mcp.WithNumberItems(),
			),
			mcp.WithString("variant_sku_contains",
				mcp.Description("Case-insensitive substring; all matching variants on the product (max "+maxStr+"); mutually exclusive with variant_ids"),
			),
			mcp.WithString("namespace", mcp.Description("Metafield namespace"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key"), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description applied to each upsert")),
			mcp.WithString("permission_set", mcp.Description("Optional; default app_only on create per variant")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleVariantMetafieldsBulkSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/metafields/bulk_delete",
		Tier:    middleware.TierR1,
		Summary: "Delete a metafield (namespace+key) from many variants of one product",
		Description: "Removes the metafield matching namespace+key from each target variant that has it; " +
			"variants without that metafield are skipped. Target with variant_ids OR variant_sku_contains, not both. " +
			"Maximum " + maxStr + " variants per call. Preview then confirmed=true.",
		Tool: mcp.NewTool("catalog_products_variants_metafields_bulk_delete",
			mcp.WithDescription(
				"Bulk delete metafield by namespace+key. variant_ids (max "+maxStr+
					") OR variant_sku_contains (case-insensitive substring on variant SKU). Skips variants where it does not exist.",
			),
			mcp.WithNumber("product_id", mcp.Description("Numeric product ID")),
			mcp.WithString("sku", mcp.Description("Exact product SKU")),
			mcp.WithString("product_name", mcp.Description("Exact product name")),
			mcp.WithArray("variant_ids",
				mcp.Description("Variant IDs on the resolved product; omit if using variant_sku_contains"),
				mcp.WithNumberItems(),
			),
			mcp.WithString("variant_sku_contains",
				mcp.Description("Case-insensitive substring match on variant SKU; mutually exclusive with variant_ids"),
			),
			mcp.WithString("namespace", mcp.Description("Metafield namespace"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleVariantMetafieldsBulkDelete,
	})

	maxProductsStr := strconv.Itoa(maxBulkProductMetafieldTargets)
	maxOpsStr := strconv.Itoa(maxBulkCrossProductVariantOps)
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/metafields/bulk_set_products",
		Tier:    middleware.TierR1,
		Summary: "Set the same variant metafield on many variants across many products",
		Description: "Uses product_ids (max " + maxProductsStr + " per call) and variant_scope: " +
			"all_variants, first_variant_only, or sku_contains (with variant_sku_contains: case-insensitive substring on variant SKU). " +
			"Total variant operations per call may not exceed " + maxOpsStr + ". Preview then confirmed=true.",
		Tool: mcp.NewTool("catalog_products_variants_metafields_bulk_set_products",
			mcp.WithDescription(
				"Cross-product bulk upsert: same namespace+key+value on many variants. "+
					"product_ids (max "+maxProductsStr+"); variant_scope: all_variants, first_variant_only, or sku_contains. "+
					"When sku_contains, set variant_sku_contains (non-empty). Products with no matching variants are skipped. "+
					"Max "+maxOpsStr+" total variant metafield writes per call. Optional description, permission_set.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("BigCommerce product IDs (deduped; max "+maxProductsStr+")"),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithString("variant_scope",
				mcp.Description("all_variants | first_variant_only | sku_contains (requires variant_sku_contains)"),
				mcp.Required(),
			),
			mcp.WithString("variant_sku_contains",
				mcp.Description("Required when variant_scope is sku_contains: case-insensitive substring matched against each variant SKU"),
			),
			mcp.WithString("namespace", mcp.Description("Metafield namespace"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key"), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description applied to each upsert")),
			mcp.WithString("permission_set", mcp.Description("Optional; default app_only on create per variant")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleVariantMetafieldsBulkSetProducts,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/metafields/bulk_delete_products",
		Tier:    middleware.TierR1,
		Summary: "Delete a variant metafield (namespace+key) across many variants on many products",
		Description: "Same product_ids and variant_scope as bulk_set_products. " +
			"Skips variant/product pairs where the metafield is absent. " +
			"Max " + maxOpsStr + " total variant operations per call. Preview then confirmed=true.",
		Tool: mcp.NewTool("catalog_products_variants_metafields_bulk_delete_products",
			mcp.WithDescription(
				"Cross-product bulk delete by namespace+key. product_ids (max "+maxProductsStr+"); "+
					"variant_scope all_variants, first_variant_only, or sku_contains (+ variant_sku_contains). Max "+maxOpsStr+" operations per call.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Product IDs (deduped; max "+maxProductsStr+")"),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithString("variant_scope",
				mcp.Description("all_variants | first_variant_only | sku_contains"),
				mcp.Required(),
			),
			mcp.WithString("variant_sku_contains",
				mcp.Description("Required when variant_scope is sku_contains"),
			),
			mcp.WithString("namespace", mcp.Description("Metafield namespace"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleVariantMetafieldsBulkDeleteProducts,
	})
}

func parseVariantScopeForCrossProduct(args map[string]any) (scope string, skuNeedle string, err error) {
	raw, ok := args["variant_scope"]
	if !ok {
		return "", "", fmt.Errorf("variant_scope is required")
	}
	s, ok := raw.(string)
	if !ok {
		return "", "", fmt.Errorf("variant_scope must be a string")
	}
	s = strings.TrimSpace(s)

	hasFilterArg := false
	if rawN, ok := args["variant_sku_contains"]; ok {
		if str, ok := rawN.(string); ok && strings.TrimSpace(str) != "" {
			hasFilterArg = true
		}
	}

	switch s {
	case variantScopeAllVariants, variantScopeFirstVariantOnly:
		if hasFilterArg {
			return "", "", fmt.Errorf("variant_sku_contains is only valid when variant_scope is %q", variantScopeSKUContains)
		}
		return s, "", nil
	case variantScopeSKUContains:
		rawN, ok := args["variant_sku_contains"]
		if !ok {
			return "", "", fmt.Errorf("variant_sku_contains is required when variant_scope is %q", variantScopeSKUContains)
		}
		needle, ok := rawN.(string)
		if !ok {
			return "", "", fmt.Errorf("variant_sku_contains must be a string")
		}
		needle = strings.TrimSpace(needle)
		if needle == "" {
			return "", "", fmt.Errorf("variant_sku_contains must be non-empty when variant_scope is %q", variantScopeSKUContains)
		}
		return s, needle, nil
	default:
		return "", "", fmt.Errorf(
			"variant_scope must be %q, %q, or %q",
			variantScopeAllVariants, variantScopeFirstVariantOnly, variantScopeSKUContains,
		)
	}
}

func variantIDsForScope(variants []bigcommerce.Variant, scope string, skuNeedle string) ([]int, error) {
	switch scope {
	case variantScopeSKUContains:
		if strings.TrimSpace(skuNeedle) == "" {
			return nil, fmt.Errorf("internal: empty variant_sku_contains")
		}
		return filterVariantIDsBySKUContains(variants, skuNeedle), nil
	case variantScopeAllVariants:
		if len(variants) == 0 {
			return nil, fmt.Errorf("no variants on product")
		}
		out := make([]int, 0, len(variants))
		for i := range variants {
			out = append(out, variants[i].ID)
		}
		return out, nil
	case variantScopeFirstVariantOnly:
		if len(variants) == 0 {
			return nil, fmt.Errorf("no variants on product")
		}
		return []int{variants[0].ID}, nil
	default:
		return nil, fmt.Errorf("invalid variant_scope")
	}
}

type crossProductVariantPlan struct {
	ProductID  int   `json:"product_id"`
	VariantIDs []int `json:"variant_ids"`
}

func buildCrossProductVariantPlans(
	ctx context.Context,
	bc BigCommerceAPI,
	productIDs []int,
	scope string,
	skuNeedle string,
) ([]crossProductVariantPlan, int, error) {
	plans := make([]crossProductVariantPlan, 0, len(productIDs))
	total := 0
	for _, pid := range productIDs {
		variants, err := bc.ListVariantsForProduct(ctx, pid)
		if err != nil {
			return nil, 0, fmt.Errorf("list variants for product %d: %w", pid, err)
		}
		vids, err := variantIDsForScope(variants, scope, skuNeedle)
		if err != nil {
			return nil, 0, fmt.Errorf("product %d: %w", pid, err)
		}
		if len(vids) == 0 {
			plans = append(plans, crossProductVariantPlan{ProductID: pid, VariantIDs: vids})
			continue
		}
		total += len(vids)
		if total > maxBulkCrossProductVariantOps {
			return nil, 0, fmt.Errorf(
				"total variant operations would be %d (exceeds maximum %d for one call); use fewer product_ids, first_variant_only, a more specific variant_sku_contains, or split into multiple calls",
				total, maxBulkCrossProductVariantOps,
			)
		}
		plans = append(plans, crossProductVariantPlan{ProductID: pid, VariantIDs: vids})
	}
	if total == 0 {
		if scope == variantScopeSKUContains {
			return nil, 0, fmt.Errorf(
				"no variants matched variant_sku_contains %q for any of the given products",
				skuNeedle,
			)
		}
		return nil, 0, fmt.Errorf("no variant operations to perform")
	}
	return plans, total, nil
}

func (p *Products) handleVariantMetafieldsBulkSetProducts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	productIDs, err := ParseBulkProductMetafieldProductIDs(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	scope, skuNeedle, err := parseVariantScopeForCrossProduct(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	ns, key, value, desc, permSet, err := parseBulkMetafieldCommonFields(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	plans, totalOps, err := buildCrossProductVariantPlans(ctx, p.bc, productIDs, scope, skuNeedle)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)

	if !confirmed {
		preview := map[string]any{
			"status":                   "pending_confirmation",
			"product_count":            len(productIDs),
			"total_variant_operations": totalOps,
			"variant_scope":            scope,
			"namespace":                ns,
			"key":                      key,
			"value":                    value,
			"permission_note":          "New rows default to app_only unless permission_set is set.",
			"per_product":              plans,
			"preview_note":             "Per-variant create vs update is resolved at execute time.",
			"message": fmt.Sprintf(
				"Will upsert metafield %s.%s on %d variant operation(s) across %d product(s). Pass confirmed=true to execute.",
				ns, key, totalOps, len(productIDs),
			),
		}
		if permSet != "" {
			preview["permission_set"] = permSet
		}
		if desc != "" {
			preview["description"] = desc
		}
		if scope == variantScopeSKUContains {
			preview["variant_sku_contains"] = skuNeedle
		}
		return toolJSON(preview)
	}

	var succeeded, failed int
	var results []map[string]any
	var errs []map[string]any

	for _, plan := range plans {
		for _, vid := range plan.VariantIDs {
			action, mf, execErr := executeVariantMetafieldUpsert(ctx, p.bc, plan.ProductID, vid, ns, key, value, desc, permSet)
			if execErr != nil {
				failed++
				errs = append(errs, map[string]any{
					"product_id": plan.ProductID, "variant_id": vid, "error": execErr.Error(),
				})
				continue
			}
			succeeded++
			results = append(results, map[string]any{
				"product_id": plan.ProductID,
				"variant_id": vid,
				"action":     action,
				"metafield":  mf,
			})
		}
	}

	out := map[string]any{
		"status":                   "completed",
		"product_count":            len(productIDs),
		"total_variant_operations": totalOps,
		"succeeded":                succeeded,
		"failed":                   failed,
		"results":                  results,
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return toolJSON(out)
}

func (p *Products) handleVariantMetafieldsBulkDeleteProducts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	productIDs, err := ParseBulkProductMetafieldProductIDs(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	scope, skuNeedle, err := parseVariantScopeForCrossProduct(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	nsRaw, ok := args["namespace"]
	if !ok {
		return toolError("namespace is required"), nil
	}
	ns, sOk := nsRaw.(string)
	if !sOk || strings.TrimSpace(ns) == "" {
		return toolError("namespace must be a non-empty string"), nil
	}
	ns = strings.TrimSpace(ns)

	kRaw, ok := args["key"]
	if !ok {
		return toolError("key is required"), nil
	}
	k, sOk := kRaw.(string)
	if !sOk || strings.TrimSpace(k) == "" {
		return toolError("key must be a non-empty string"), nil
	}
	k = strings.TrimSpace(k)

	plans, totalOps, err := buildCrossProductVariantPlans(ctx, p.bc, productIDs, scope, skuNeedle)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)

	if !confirmed {
		prev := map[string]any{
			"status":                   "pending_confirmation",
			"product_count":            len(productIDs),
			"total_variant_operations": totalOps,
			"variant_scope":            scope,
			"namespace":                ns,
			"key":                      k,
			"per_product":              plans,
			"message": fmt.Sprintf(
				"Will delete metafield %s.%s where present across %d variant operation(s) on %d product(s). Pass confirmed=true to execute.",
				ns, k, totalOps, len(productIDs),
			),
		}
		if scope == variantScopeSKUContains {
			prev["variant_sku_contains"] = skuNeedle
		}
		return toolJSON(prev)
	}

	var succeeded, skipped, failed int
	var results []map[string]any
	var errs []map[string]any

	for _, plan := range plans {
		for _, vid := range plan.VariantIDs {
			mfID, resErr := resolveMetafieldIDForVariant(ctx, p.bc, plan.ProductID, vid, ns, k)
			if resErr != nil {
				failed++
				errs = append(errs, map[string]any{
					"product_id": plan.ProductID, "variant_id": vid, "error": resErr.Error(),
				})
				continue
			}
			if mfID == 0 {
				skipped++
				results = append(results, map[string]any{
					"product_id": plan.ProductID,
					"variant_id": vid,
					"status":     "skipped",
					"reason":     "metafield not found",
				})
				continue
			}
			if delErr := variantMetafieldOps(p.bc, plan.ProductID).Delete(ctx, vid, mfID); delErr != nil {
				failed++
				errs = append(errs, map[string]any{
					"product_id": plan.ProductID, "variant_id": vid, "error": delErr.Error(),
				})
				continue
			}
			succeeded++
			results = append(results, map[string]any{
				"product_id":   plan.ProductID,
				"variant_id":   vid,
				"status":       "deleted",
				"metafield_id": mfID,
			})
		}
	}

	out := map[string]any{
		"status":                   "completed",
		"product_count":            len(productIDs),
		"total_variant_operations": totalOps,
		"succeeded":                succeeded,
		"skipped":                  skipped,
		"failed":                   failed,
		"results":                  results,
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return toolJSON(out)
}

func (p *Products) handleVariantMetafieldsBulkSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	pid, psku, pname, err := parseBulkVariantMetafieldProductTarget(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	sel, err := ParseBulkSingleProductVariantSelection(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	ns, key, value, desc, permSet, err := parseBulkMetafieldCommonFields(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	productID, err := p.resolveProductIDFromMetafieldParts(ctx, pid, psku, pname)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	vids, err := p.resolveVariantIDsForSingleProductBulk(ctx, productID, sel)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)

	type row struct {
		VariantID       int    `json:"variant_id"`
		Action          string `json:"action"`
		EffectivePerm   string `json:"effective_permission_set,omitempty"`
		HasExisting     bool   `json:"has_existing"`
		ExistingValue   string `json:"existing_value,omitempty"`
		MetafieldID     int    `json:"metafield_id,omitempty"`
	}

	rows := make([]row, 0, len(vids))
	for _, vid := range vids {
		existing, listErr := p.bc.ListVariantMetafields(ctx, productID, vid)
		if listErr != nil {
			return toolError("failed to list metafields for variant %d: %v", vid, listErr), nil
		}
		var existingMF *bigcommerce.Metafield
		for i := range existing {
			if existing[i].Namespace == ns && existing[i].Key == key {
				existingMF = &existing[i]
				break
			}
		}
		r := row{VariantID: vid, HasExisting: existingMF != nil}
		if existingMF != nil {
			r.Action = "update"
			r.ExistingValue = existingMF.Value
			r.MetafieldID = existingMF.ID
			if permSet != "" {
				r.EffectivePerm = permSet
			} else {
				r.EffectivePerm = existingMF.PermissionSet
			}
		} else {
			r.Action = "create"
			if permSet != "" {
				r.EffectivePerm = permSet
			} else {
				r.EffectivePerm = defaultVariantMetafieldPermissionSet
			}
		}
		rows = append(rows, r)
	}

	if !confirmed {
		preview := map[string]any{
			"status":          "pending_confirmation",
			"product_id":      productID,
			"variant_count":   len(vids),
			"namespace":       ns,
			"key":             key,
			"value":           value,
			"permission_note": "New rows default to app_only unless permission_set is set.",
			"per_variant":     rows,
			"message": fmt.Sprintf(
				"Will upsert metafield %s.%s on %d variant(s) of product %d. Pass confirmed=true to execute.",
				ns, key, len(vids), productID,
			),
		}
		if sel.SKUContains != "" {
			preview["variant_sku_contains"] = sel.SKUContains
		}
		if desc != "" {
			preview["description"] = desc
		}
		if permSet != "" {
			preview["permission_set"] = permSet
		}
		return toolJSON(preview)
	}

	var succeeded int
	var failed int
	var results []map[string]any
	var errs []map[string]any

	for _, vid := range vids {
		action, mf, execErr := executeVariantMetafieldUpsert(ctx, p.bc, productID, vid, ns, key, value, desc, permSet)
		if execErr != nil {
			failed++
			errs = append(errs, map[string]any{"variant_id": vid, "error": execErr.Error()})
			continue
		}
		succeeded++
		results = append(results, map[string]any{
			"variant_id": vid,
			"action":     action,
			"metafield":  mf,
		})
	}

	out := map[string]any{
		"status":     "completed",
		"product_id": productID,
		"total":      len(vids),
		"succeeded":  succeeded,
		"failed":     failed,
		"results":    results,
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return toolJSON(out)
}

func (p *Products) handleVariantMetafieldsBulkDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	pid, psku, pname, err := parseBulkVariantMetafieldProductTarget(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	sel, err := ParseBulkSingleProductVariantSelection(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	nsRaw, ok := args["namespace"]
	if !ok {
		return toolError("namespace is required"), nil
	}
	ns, sOk := nsRaw.(string)
	if !sOk || strings.TrimSpace(ns) == "" {
		return toolError("namespace must be a non-empty string"), nil
	}
	ns = strings.TrimSpace(ns)

	kRaw, ok := args["key"]
	if !ok {
		return toolError("key is required"), nil
	}
	k, sOk := kRaw.(string)
	if !sOk || strings.TrimSpace(k) == "" {
		return toolError("key must be a non-empty string"), nil
	}
	k = strings.TrimSpace(k)

	productID, err := p.resolveProductIDFromMetafieldParts(ctx, pid, psku, pname)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	vids, err := p.resolveVariantIDsForSingleProductBulk(ctx, productID, sel)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)

	type prevRow struct {
		VariantID    int  `json:"variant_id"`
		HasMetafield bool `json:"has_metafield"`
		MetafieldID  int  `json:"metafield_id,omitempty"`
	}
	prevRows := make([]prevRow, 0, len(vids))
	for _, vid := range vids {
		mfID, resErr := resolveMetafieldIDForVariant(ctx, p.bc, productID, vid, ns, k)
		if resErr != nil {
			return toolError("failed to resolve metafield for variant %d: %v", vid, resErr), nil
		}
		prevRows = append(prevRows, prevRow{
			VariantID:    vid,
			HasMetafield: mfID > 0,
			MetafieldID:  mfID,
		})
	}

	if !confirmed {
		prev := map[string]any{
			"status":        "pending_confirmation",
			"product_id":    productID,
			"variant_count": len(vids),
			"namespace":     ns,
			"key":           k,
			"per_variant":   prevRows,
			"message": fmt.Sprintf(
				"Will delete metafield %s.%s where present on %d variant(s) of product %d. Pass confirmed=true to execute.",
				ns, k, len(vids), productID,
			),
		}
		if sel.SKUContains != "" {
			prev["variant_sku_contains"] = sel.SKUContains
		}
		return toolJSON(prev)
	}

	var succeeded, skipped, failed int
	var results []map[string]any
	var errs []map[string]any

	for _, vid := range vids {
		mfID, resErr := resolveMetafieldIDForVariant(ctx, p.bc, productID, vid, ns, k)
		if resErr != nil {
			failed++
			errs = append(errs, map[string]any{"variant_id": vid, "error": resErr.Error()})
			continue
		}
		if mfID == 0 {
			skipped++
			results = append(results, map[string]any{
				"variant_id": vid,
				"status":     "skipped",
				"reason":     "metafield not found",
			})
			continue
		}
		if delErr := variantMetafieldOps(p.bc, productID).Delete(ctx, vid, mfID); delErr != nil {
			failed++
			errs = append(errs, map[string]any{"variant_id": vid, "error": delErr.Error()})
			continue
		}
		succeeded++
		results = append(results, map[string]any{
			"variant_id":   vid,
			"status":       "deleted",
			"metafield_id": mfID,
		})
	}

	out := map[string]any{
		"status":     "completed",
		"product_id": productID,
		"total":      len(vids),
		"succeeded":  succeeded,
		"skipped":    skipped,
		"failed":     failed,
		"results":    results,
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return toolJSON(out)
}
