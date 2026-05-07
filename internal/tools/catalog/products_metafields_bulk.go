package catalog

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// maxBulkProductMetafieldTargets caps how many distinct products a single bulk
// metafield tool call may touch (sequential API calls per product).
const maxBulkProductMetafieldTargets = 50

// ParseBulkProductMetafieldProductIDs extracts product_ids for bulk metafield tools; dedupes; enforces max.
func ParseBulkProductMetafieldProductIDs(args map[string]any) ([]int, error) {
	raw, ok := args["product_ids"]
	if !ok {
		return nil, fmt.Errorf("product_ids is required")
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("product_ids must be an array")
	}
	if len(arr) == 0 {
		return nil, fmt.Errorf("product_ids must be non-empty")
	}
	seen := make(map[int]struct{})
	out := make([]int, 0, len(arr))
	for i, item := range arr {
		f, fOk := item.(float64)
		if !fOk {
			return nil, fmt.Errorf("product_ids[%d] must be a number", i)
		}
		id := int(f)
		if id <= 0 {
			return nil, fmt.Errorf("product_ids[%d] must be a positive product id", i)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("product_ids must contain at least one valid id")
	}
	if len(out) > maxBulkProductMetafieldTargets {
		return nil, fmt.Errorf("product_ids exceeds maximum of %d for bulk metafield operations", maxBulkProductMetafieldTargets)
	}
	return out, nil
}

// executeProductMetafieldUpsert applies namespace+key upsert on one product.
func executeProductMetafieldUpsert(
	ctx context.Context,
	bc BigCommerceAPI,
	productID int,
	namespace, key, value, description, permissionSet string,
) (action string, mf *bigcommerce.Metafield, err error) {
	return metafieldUpsertExecute(
		ctx, productID, namespace, key, value, description, permissionSet,
		productMetafieldOps(bc, productID),
		defaultProductMetafieldPermissionSet,
		&metafieldUpsertOptions{PreserveEmptyPermissionOnUpdate: true},
	)
}

// resolveMetafieldIDForProduct returns metafield id for namespace+key or 0 if absent.
func resolveMetafieldIDForProduct(ctx context.Context, bc BigCommerceAPI, productID int, namespace, key string) (int, error) {
	return metafieldResolveIDByNamespaceKey(ctx, productID, namespace, key, productMetafieldOps(bc, productID))
}

// RegisterProductMetafieldBulkTools registers bulk metafield tools (many products).
func (p *Products) RegisterProductMetafieldBulkTools(reg *discovery.Registry) {
	maxStr := strconv.Itoa(maxBulkProductMetafieldTargets)
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/metafields/bulk_set",
		Tier:    middleware.TierR1,
		Summary: "Set the same metafield (namespace+key+value) on many products",
		Description: "Applies one metafield upsert to each product_id (sequential API calls). " +
			"Maximum " + maxStr + " products per call. " +
			"Optional permission_set; new rows default to app_only. Preview then confirmed=true.",
		Tool: mcp.NewTool("catalog_products_metafields_bulk_set",
			mcp.WithDescription(
				"Bulk upsert the same namespace+key+value on many products. "+
					"Provide product_ids (array of positive ints, max "+maxStr+"), "+
					"namespace, key, value. Optional description, permission_set.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Array of BigCommerce product IDs (deduped; max "+maxStr+")"),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithString("namespace", mcp.Description("Metafield namespace"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key"), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description applied to each upsert")),
			mcp.WithString("permission_set", mcp.Description("Optional; default app_only on create per product")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleProductMetafieldsBulkSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/metafields/bulk_delete",
		Tier:    middleware.TierR1,
		Summary: "Delete a metafield (namespace+key) from many products",
		Description: "Removes the metafield matching namespace+key from each product that has it; " +
			"products without that metafield are skipped. " +
			"Maximum " + maxStr + " products per call. Preview then confirmed=true.",
		Tool: mcp.NewTool("catalog_products_metafields_bulk_delete",
			mcp.WithDescription(
				"Bulk delete metafield by namespace+key across product_ids (max "+maxStr+
					"). Skips products where it does not exist.",
			),
			mcp.WithArray("product_ids",
				mcp.Description("Array of product IDs"),
				mcp.WithNumberItems(),
				mcp.Required(),
			),
			mcp.WithString("namespace", mcp.Description("Metafield namespace"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleProductMetafieldsBulkDelete,
	})
}

func (p *Products) handleProductMetafieldsBulkSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := ParseBulkProductMetafieldProductIDs(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	ns, key, value, desc, permSet, err := parseBulkMetafieldCommonFields(args)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)

	type row struct {
		ProductID     int    `json:"product_id"`
		Action        string `json:"action"`
		EffectivePerm string `json:"effective_permission_set,omitempty"`
		HasExisting   bool   `json:"has_existing"`
		ExistingValue string `json:"existing_value,omitempty"`
		MetafieldID   int    `json:"metafield_id,omitempty"`
	}

	rows := make([]row, 0, len(ids))
	for _, pid := range ids {
		existing, listErr := p.bc.ListProductMetafields(ctx, pid)
		if listErr != nil {
			return toolError("failed to list metafields for product %d: %v", pid, listErr), nil
		}
		var existingMF *bigcommerce.Metafield
		for i := range existing {
			if existing[i].Namespace == ns && existing[i].Key == key {
				existingMF = &existing[i]
				break
			}
		}
		r := row{ProductID: pid, HasExisting: existingMF != nil}
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
				r.EffectivePerm = defaultProductMetafieldPermissionSet
			}
		}
		rows = append(rows, r)
	}

	if !confirmed {
		preview := map[string]any{
			"status":          "pending_confirmation",
			"product_count":   len(ids),
			"namespace":       ns,
			"key":             key,
			"value":           value,
			"permission_note": "New rows default to app_only unless permission_set is set.",
			"per_product":     rows,
			"message": fmt.Sprintf(
				"Will upsert metafield %s.%s on %d product(s). Pass confirmed=true to execute.",
				ns, key, len(ids),
			),
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

	for _, pid := range ids {
		action, mf, execErr := executeProductMetafieldUpsert(ctx, p.bc, pid, ns, key, value, desc, permSet)
		if execErr != nil {
			failed++
			errs = append(errs, map[string]any{"product_id": pid, "error": execErr.Error()})
			continue
		}
		succeeded++
		results = append(results, map[string]any{
			"product_id": pid,
			"action":     action,
			"metafield":  mf,
		})
	}

	out := map[string]any{
		"status":    "completed",
		"total":     len(ids),
		"succeeded": succeeded,
		"failed":    failed,
		"results":   results,
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return toolJSON(out)
}

func (p *Products) handleProductMetafieldsBulkDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := ParseBulkProductMetafieldProductIDs(args)
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

	confirmed := middleware.IsConfirmed(request)

	type prevRow struct {
		ProductID    int  `json:"product_id"`
		HasMetafield bool `json:"has_metafield"`
		MetafieldID  int  `json:"metafield_id,omitempty"`
	}
	prevRows := make([]prevRow, 0, len(ids))
	for _, pid := range ids {
		mfID, resErr := resolveMetafieldIDForProduct(ctx, p.bc, pid, ns, k)
		if resErr != nil {
			return toolError("failed to resolve metafield for product %d: %v", pid, resErr), nil
		}
		prevRows = append(prevRows, prevRow{
			ProductID:    pid,
			HasMetafield: mfID > 0,
			MetafieldID:  mfID,
		})
	}

	if !confirmed {
		return toolJSON(map[string]any{
			"status":        "pending_confirmation",
			"product_count": len(ids),
			"namespace":     ns,
			"key":           k,
			"per_product":   prevRows,
			"message": fmt.Sprintf(
				"Will delete metafield %s.%s where present on %d product(s). Pass confirmed=true to execute.",
				ns, k, len(ids),
			),
		})
	}

	var succeeded, skipped, failed int
	var results []map[string]any
	var errs []map[string]any

	for _, pid := range ids {
		mfID, resErr := resolveMetafieldIDForProduct(ctx, p.bc, pid, ns, k)
		if resErr != nil {
			failed++
			errs = append(errs, map[string]any{"product_id": pid, "error": resErr.Error()})
			continue
		}
		if mfID == 0 {
			skipped++
			results = append(results, map[string]any{
				"product_id": pid,
				"status":     "skipped",
				"reason":     "metafield not found",
			})
			continue
		}
		if delErr := productMetafieldOps(p.bc, pid).Delete(ctx, pid, mfID); delErr != nil {
			failed++
			errs = append(errs, map[string]any{"product_id": pid, "error": delErr.Error()})
			continue
		}
		succeeded++
		results = append(results, map[string]any{
			"product_id":   pid,
			"status":       "deleted",
			"metafield_id": mfID,
		})
	}

	out := map[string]any{
		"status":    "completed",
		"total":     len(ids),
		"succeeded": succeeded,
		"skipped":   skipped,
		"failed":    failed,
		"results":   results,
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return toolJSON(out)
}

func parseBulkMetafieldCommonFields(args map[string]any) (namespace, key, value, description, permissionSet string, err error) {
	nsRaw, ok := args["namespace"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("namespace is required")
	}
	ns, sOk := nsRaw.(string)
	if !sOk || strings.TrimSpace(ns) == "" {
		return "", "", "", "", "", fmt.Errorf("namespace must be a non-empty string")
	}
	ns = strings.TrimSpace(ns)

	kRaw, ok := args["key"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("key is required")
	}
	k, sOk := kRaw.(string)
	if !sOk || strings.TrimSpace(k) == "" {
		return "", "", "", "", "", fmt.Errorf("key must be a non-empty string")
	}
	k = strings.TrimSpace(k)

	vRaw, ok := args["value"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("value is required")
	}
	val, sOk := vRaw.(string)
	if !sOk {
		return "", "", "", "", "", fmt.Errorf("value must be a string")
	}

	var desc string
	if v, ok := args["description"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return "", "", "", "", "", fmt.Errorf("description must be a string")
		}
		desc = s
	}

	ps, err := ParseOptionalPermissionSet(args)
	if err != nil {
		return "", "", "", "", "", err
	}

	return ns, k, val, desc, ps, nil
}
