package customers

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

const (
	defaultCustomerMetafieldPermissionSet = "app_only"
	maxBulkCustomerMetafieldTargets       = 50
)

// CustomerMetafieldListSearchFilters maps tool params to /v3/customers/metafields keys.
var CustomerMetafieldListSearchFilters = []shared.SearchFilter{
	{ToolKey: "customer_id_in", BCKey: "customer_id:in", Kind: "string"},
	{ToolKey: "key", BCKey: "key", Kind: "string"},
	{ToolKey: "key_in", BCKey: "key:in", Kind: "string"},
	{ToolKey: "namespace", BCKey: "namespace", Kind: "string"},
	{ToolKey: "namespace_in", BCKey: "namespace:in", Kind: "string"},
	{ToolKey: "namespace_like", BCKey: "namespace:like", Kind: "string"},
}

var customerMetafieldListNonDataKeys = map[string]bool{
	"sort": true, "page": true, "limit": true, "include_fields": true, "exclude_fields": true,
}

// CustomerMetafields provides MCP handlers for /v3/customers/{id}/metafields and
// /v3/customers/metafields.
type CustomerMetafields struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerMetafields constructs the customer metafield tool handlers.
func NewCustomerMetafields(bc BigCommerceCustomersAPI) *CustomerMetafields {
	return &CustomerMetafields{bc: bc}
}

// RegisterTools registers customers/metafields/{list,set,delete,bulk_set,bulk_delete}.
func (m *CustomerMetafields) RegisterTools(reg *discovery.Registry) {
	maxStr := fmt.Sprintf("%d", maxBulkCustomerMetafieldTargets)

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/metafields/list",
		Tier:    middleware.TierR0,
		Summary: "List customer metafields (V3)",
		Description: "If customer_id is supplied, lists metafields for that single customer " +
			"(GET /v3/customers/{id}/metafields). Otherwise queries the cross-customer endpoint " +
			"(GET /v3/customers/metafields) with optional filters such as customer_ids, namespace, " +
			"or key.",
		Tool: mcp.NewTool("customers_metafields_list",
			mcp.WithDescription("List metafields for one customer (customer_id) or filter across customers."),
			mcp.WithNumber("customer_id", mcp.Description("Single customer scope. Omit to list across customers.")),
			mcp.WithBoolean("list_all", mcp.Description("Cross-customer mode: page through every metafield (required if no filter is provided).")),
			mcp.WithArray("customer_ids", mcp.Description("Cross-customer mode: customer_id:in filter."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithString("namespace", mcp.Description("Cross-customer mode: exact namespace filter.")),
			mcp.WithString("namespace_in", mcp.Description("Cross-customer mode: namespace:in filter.")),
			mcp.WithString("namespace_like", mcp.Description("Cross-customer mode: namespace:like filter.")),
			mcp.WithString("key", mcp.Description("Cross-customer mode: exact key filter.")),
			mcp.WithString("key_in", mcp.Description("Cross-customer mode: key:in filter.")),
			mcp.WithNumber("page", mcp.Description("Page number.")),
			mcp.WithNumber("limit", mcp.Description("Page size.")),
		),
		Handler: m.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/metafields/set",
		Tier:    middleware.TierR1,
		Summary: "Create or update one customer metafield (upsert by namespace+key)",
		Description: "Sets a metafield on one customer using the per-customer endpoint. " +
			"If permission_set is omitted, new metafields default to app_only (not Storefront-readable). " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("customers_metafields_set",
			mcp.WithDescription("Upsert a metafield on a customer by namespace+key."),
			mcp.WithNumber("customer_id", mcp.Description("Customer ID."), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("Metafield namespace."), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key."), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value (string)."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description.")),
			mcp.WithString("permission_set", mcp.Description("Optional. One of app_only (default), read, write, read_and_sf_access, write_and_sf_access.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: m.handleSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "customers/metafields/delete",
		Tier:        middleware.TierR1,
		Summary:     "Delete one customer metafield (V3)",
		Description: "Deletes by metafield_id or by namespace+key on a single customer. Preview then confirmed=true.",
		Tool: mcp.NewTool("customers_metafields_delete",
			mcp.WithDescription("Delete a customer metafield by metafield_id or namespace+key."),
			mcp.WithNumber("customer_id", mcp.Description("Customer ID."), mcp.Required()),
			mcp.WithNumber("metafield_id", mcp.Description("Metafield ID (mutually exclusive with namespace+key).")),
			mcp.WithString("namespace", mcp.Description("Namespace (with key).")),
			mcp.WithString("key", mcp.Description("Key (with namespace).")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: m.handleDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/metafields/bulk_set",
		Tier:    middleware.TierR1,
		Summary: "Set the same metafield (namespace+key+value) on many customers",
		Description: "Applies one metafield upsert to each customer_id (sequential per-customer API calls). " +
			"Maximum " + maxStr + " customers per call. Optional permission_set; new rows default to app_only. " +
			"Preview then confirmed=true.",
		Tool: mcp.NewTool("customers_metafields_bulk_set",
			mcp.WithDescription("Bulk upsert the same namespace+key+value on many customers."),
			mcp.WithArray("customer_ids", mcp.Description("Array of customer IDs (deduped; max "+maxStr+")."),
				mcp.Items(map[string]any{"type": "number"}), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("Metafield namespace."), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key."), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description applied to each upsert.")),
			mcp.WithString("permission_set", mcp.Description("Optional; default app_only on create per customer.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: m.handleBulkSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/metafields/bulk_delete",
		Tier:    middleware.TierR1,
		Summary: "Delete a metafield (namespace+key) from many customers",
		Description: "Removes the metafield matching namespace+key from each customer that has it; " +
			"customers without that metafield are skipped. Maximum " + maxStr + " customers per call. " +
			"Preview then confirmed=true.",
		Tool: mcp.NewTool("customers_metafields_bulk_delete",
			mcp.WithDescription("Bulk delete metafield by namespace+key across customers (skips customers where it does not exist)."),
			mcp.WithArray("customer_ids", mcp.Description("Array of customer IDs."),
				mcp.Items(map[string]any{"type": "number"}), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("Metafield namespace."), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: m.handleBulkDelete,
	})
}

func (m *CustomerMetafields) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	if v, ok := args["customer_id"].(float64); ok && int(v) > 0 {
		mfs, err := m.bc.ListCustomerMetafields(ctx, int(v))
		if err != nil {
			return shared.ToolError("failed to list metafields: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{
			"customer_id": int(v),
			"total":       len(mfs),
			"metafields":  mfs,
		})
	}

	listAll := shared.ReadBool(args, "list_all")
	params, err := shared.ExtractFilters(args, CustomerMetafieldListSearchFilters)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if ids, ierr := intSliceFromArgs(args, "customer_ids"); ierr == nil && len(ids) > 0 {
		params["customer_id:in"] = shared.JoinInts(ids)
	} else if ierr != nil {
		return shared.ToolError("%s", ierr.Error()), nil
	}
	if v, ok := args["page"].(float64); ok && v > 0 {
		params["page"] = fmt.Sprintf("%.0f", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params["limit"] = fmt.Sprintf("%.0f", v)
	}

	hasData := shared.HasDataFilterBCParams(params, CustomerMetafieldListSearchFilters, customerMetafieldListNonDataKeys) ||
		params["customer_id:in"] != ""
	if !listAll && !hasData {
		return shared.ToolError("provide customer_id, a filter (customer_ids, namespace, key, …), or list_all=true."), nil
	}

	mfs, err := m.bc.SearchAllCustomerMetafields(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list metafields: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(mfs), "metafields": mfs})
}

func (m *CustomerMetafields) handleSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	customerID, err := shared.ReadPositiveInt(args, "customer_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	ns, key, value, desc, permSet, err := parseMetafieldSetFields(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	existing, err := m.bc.ListCustomerMetafields(ctx, customerID)
	if err != nil {
		return shared.ToolError("failed to list existing metafields: %v", err), nil
	}
	var existingMF *bigcommerce.Metafield
	for i := range existing {
		if existing[i].Namespace == ns && existing[i].Key == key {
			existingMF = &existing[i]
			break
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		action := "create"
		preview := map[string]any{
			"status":      "pending_confirmation",
			"customer_id": customerID,
			"namespace":   ns,
			"key":         key,
			"value":       value,
		}
		var effectivePerm string
		if existingMF != nil {
			action = "update"
			preview["existing_value"] = existingMF.Value
			preview["existing_permission_set"] = existingMF.PermissionSet
			preview["metafield_id"] = existingMF.ID
			if permSet != "" {
				effectivePerm = permSet
			} else {
				effectivePerm = existingMF.PermissionSet
			}
		} else {
			if permSet != "" {
				effectivePerm = permSet
			} else {
				effectivePerm = defaultCustomerMetafieldPermissionSet
			}
		}
		preview["action"] = action
		preview["permission_set"] = effectivePerm
		preview["permission_note"] = shared.AppOnlyMetafieldPermissionNote
		if desc != "" {
			preview["description"] = desc
		}
		preview["message"] = fmt.Sprintf(
			"Will %s metafield %s.%s on customer %d. Pass confirmed=true to execute.",
			action, ns, key, customerID,
		)
		return shared.ToolJSON(preview)
	}

	payload := bigcommerce.Metafield{
		Namespace:     ns,
		Key:           key,
		Value:         value,
		Description:   desc,
		PermissionSet: permSet,
	}

	if existingMF != nil {
		if payload.PermissionSet == "" {
			payload.PermissionSet = existingMF.PermissionSet
		}
		updated, uerr := m.bc.UpdateCustomerMetafield(ctx, customerID, existingMF.ID, payload)
		if uerr != nil {
			return shared.ToolError("update failed: %v", uerr), nil
		}
		return shared.ToolJSON(map[string]any{
			"status":    "updated",
			"metafield": updated,
			"message":   fmt.Sprintf("Metafield %s.%s updated on customer %d.", ns, key, customerID),
		})
	}
	if payload.PermissionSet == "" {
		payload.PermissionSet = defaultCustomerMetafieldPermissionSet
	}
	created, cerr := m.bc.CreateCustomerMetafield(ctx, customerID, payload)
	if cerr != nil {
		return shared.ToolError("create failed: %v", cerr), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":    "created",
		"metafield": created,
		"message":   fmt.Sprintf("Metafield %s.%s created on customer %d.", ns, key, customerID),
	})
}

func (m *CustomerMetafields) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	customerID, err := shared.ReadPositiveInt(args, "customer_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	mfID, ns, key, err := parseMetafieldDeleteSelector(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if mfID == 0 {
		existing, lerr := m.bc.ListCustomerMetafields(ctx, customerID)
		if lerr != nil {
			return shared.ToolError("failed to list metafields: %v", lerr), nil
		}
		for _, mf := range existing {
			if mf.Namespace == ns && mf.Key == key {
				mfID = mf.ID
				break
			}
		}
		if mfID == 0 {
			return shared.ToolError("no metafield found with namespace %q key %q on customer %d", ns, key, customerID), nil
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		preview := map[string]any{
			"status":       "pending_confirmation",
			"customer_id":  customerID,
			"metafield_id": mfID,
			"message":      fmt.Sprintf("Will delete metafield %d from customer %d. Pass confirmed=true to execute.", mfID, customerID),
		}
		if ns != "" {
			preview["namespace"] = ns
			preview["key"] = key
		}
		return shared.ToolJSON(preview)
	}

	if err := m.bc.DeleteCustomerMetafield(ctx, customerID, mfID); err != nil {
		return shared.ToolError("delete failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":  "deleted",
		"message": fmt.Sprintf("Metafield %d deleted from customer %d.", mfID, customerID),
	})
}

func (m *CustomerMetafields) handleBulkSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := bulkCustomerIDs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	ns, key, value, desc, permSet, err := parseMetafieldSetFields(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	type row struct {
		CustomerID    int    `json:"customer_id"`
		Action        string `json:"action"`
		EffectivePerm string `json:"effective_permission_set,omitempty"`
		HasExisting   bool   `json:"has_existing"`
		ExistingValue string `json:"existing_value,omitempty"`
		MetafieldID   int    `json:"metafield_id,omitempty"`
	}
	rows := make([]row, 0, len(ids))
	existingByCustomer := make(map[int]*bigcommerce.Metafield, len(ids))
	for _, cid := range ids {
		existing, listErr := m.bc.ListCustomerMetafields(ctx, cid)
		if listErr != nil {
			return shared.ToolError("failed to list metafields for customer %d: %v", cid, listErr), nil
		}
		var existingMF *bigcommerce.Metafield
		for i := range existing {
			if existing[i].Namespace == ns && existing[i].Key == key {
				existingMF = &existing[i]
				break
			}
		}
		existingByCustomer[cid] = existingMF
		r := row{CustomerID: cid, HasExisting: existingMF != nil}
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
				r.EffectivePerm = defaultCustomerMetafieldPermissionSet
			}
		}
		rows = append(rows, r)
	}

	if !middleware.IsConfirmedFromArgs(args) {
		preview := map[string]any{
			"status":          "pending_confirmation",
			"customer_count":  len(ids),
			"namespace":       ns,
			"key":             key,
			"value":           value,
			"permission_note": shared.AppOnlyMetafieldPermissionNote,
			"per_customer":    rows,
			"message": fmt.Sprintf(
				"Will upsert metafield %s.%s on %d customer(s). Pass confirmed=true to execute.",
				ns, key, len(ids),
			),
		}
		if desc != "" {
			preview["description"] = desc
		}
		if permSet != "" {
			preview["permission_set"] = permSet
		}
		return shared.ToolJSON(preview)
	}

	var succeeded, failed int
	var results []map[string]any
	var errs []map[string]any

	for _, cid := range ids {
		existingMF := existingByCustomer[cid]
		payload := bigcommerce.Metafield{
			Namespace: ns, Key: key, Value: value, Description: desc, PermissionSet: permSet,
		}
		var (
			action  string
			mf      *bigcommerce.Metafield
			execErr error
		)
		if existingMF != nil {
			if payload.PermissionSet == "" {
				payload.PermissionSet = existingMF.PermissionSet
			}
			mf, execErr = m.bc.UpdateCustomerMetafield(ctx, cid, existingMF.ID, payload)
			action = "updated"
		} else {
			if payload.PermissionSet == "" {
				payload.PermissionSet = defaultCustomerMetafieldPermissionSet
			}
			mf, execErr = m.bc.CreateCustomerMetafield(ctx, cid, payload)
			action = "created"
		}
		if execErr != nil {
			failed++
			errs = append(errs, map[string]any{"customer_id": cid, "error": execErr.Error()})
			continue
		}
		succeeded++
		results = append(results, map[string]any{
			"customer_id": cid,
			"action":      action,
			"metafield":   mf,
		})
	}

	status := "completed"
	if failed > 0 {
		status = "partial_success"
	}
	out := map[string]any{
		"status":    status,
		"total":     len(ids),
		"succeeded": succeeded,
		"failed":    failed,
		"results":   results,
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return shared.ToolJSON(out)
}

func (m *CustomerMetafields) handleBulkDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := bulkCustomerIDs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	ns, key, err := parseRequiredNamespaceKey(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	type prevRow struct {
		CustomerID   int  `json:"customer_id"`
		HasMetafield bool `json:"has_metafield"`
		MetafieldID  int  `json:"metafield_id,omitempty"`
	}
	prevRows := make([]prevRow, 0, len(ids))
	mfByCustomer := make(map[int]int, len(ids))
	for _, cid := range ids {
		existing, lerr := m.bc.ListCustomerMetafields(ctx, cid)
		if lerr != nil {
			return shared.ToolError("failed to list metafields for customer %d: %v", cid, lerr), nil
		}
		var mfID int
		for _, mf := range existing {
			if mf.Namespace == ns && mf.Key == key {
				mfID = mf.ID
				break
			}
		}
		mfByCustomer[cid] = mfID
		prevRows = append(prevRows, prevRow{CustomerID: cid, HasMetafield: mfID > 0, MetafieldID: mfID})
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":         "pending_confirmation",
			"customer_count": len(ids),
			"namespace":      ns,
			"key":            key,
			"per_customer":   prevRows,
			"message": fmt.Sprintf(
				"Will delete metafield %s.%s where present on %d customer(s). Pass confirmed=true to execute.",
				ns, key, len(ids),
			),
		})
	}

	var succeeded, skipped, failed int
	var results []map[string]any
	var errs []map[string]any
	for _, cid := range ids {
		mfID := mfByCustomer[cid]
		if mfID == 0 {
			skipped++
			results = append(results, map[string]any{
				"customer_id": cid,
				"status":      "skipped",
				"reason":      "metafield not found",
			})
			continue
		}
		if delErr := m.bc.DeleteCustomerMetafield(ctx, cid, mfID); delErr != nil {
			failed++
			errs = append(errs, map[string]any{"customer_id": cid, "error": delErr.Error()})
			continue
		}
		succeeded++
		results = append(results, map[string]any{
			"customer_id":  cid,
			"status":       "deleted",
			"metafield_id": mfID,
		})
	}

	status := "completed"
	if failed > 0 {
		status = "partial_success"
	}
	out := map[string]any{
		"status":    status,
		"total":     len(ids),
		"succeeded": succeeded,
		"skipped":   skipped,
		"failed":    failed,
		"results":   results,
	}
	if len(errs) > 0 {
		out["errors"] = errs
	}
	return shared.ToolJSON(out)
}

func parseMetafieldSetFields(args map[string]any) (namespace, key, value, description, permissionSet string, err error) {
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

	ps, perr := shared.ParseOptionalPermissionSet(args)
	if perr != nil {
		return "", "", "", "", "", perr
	}

	return ns, k, val, desc, ps, nil
}

func parseRequiredNamespaceKey(args map[string]any) (namespace, key string, err error) {
	nsRaw, ok := args["namespace"]
	if !ok {
		return "", "", fmt.Errorf("namespace is required")
	}
	ns, sOk := nsRaw.(string)
	if !sOk || strings.TrimSpace(ns) == "" {
		return "", "", fmt.Errorf("namespace must be a non-empty string")
	}
	kRaw, ok := args["key"]
	if !ok {
		return "", "", fmt.Errorf("key is required")
	}
	k, sOk := kRaw.(string)
	if !sOk || strings.TrimSpace(k) == "" {
		return "", "", fmt.Errorf("key must be a non-empty string")
	}
	return strings.TrimSpace(ns), strings.TrimSpace(k), nil
}

func parseMetafieldDeleteSelector(args map[string]any) (mfID int, namespace, key string, err error) {
	_, hasMFID := args["metafield_id"]
	_, hasNS := args["namespace"]
	_, hasKey := args["key"]
	if hasMFID && (hasNS || hasKey) {
		return 0, "", "", fmt.Errorf("use metafield_id alone, or namespace + key; do not combine")
	}
	if !hasMFID && (!hasNS || !hasKey) {
		return 0, "", "", fmt.Errorf("provide metafield_id, or both namespace and key")
	}
	if hasMFID {
		f, fOk := args["metafield_id"].(float64)
		if !fOk {
			return 0, "", "", fmt.Errorf("metafield_id must be a number")
		}
		id := int(f)
		if id <= 0 {
			return 0, "", "", fmt.Errorf("metafield_id must be positive")
		}
		return id, "", "", nil
	}
	ns, _ := args["namespace"].(string)
	k, _ := args["key"].(string)
	ns = strings.TrimSpace(ns)
	k = strings.TrimSpace(k)
	if ns == "" || k == "" {
		return 0, "", "", fmt.Errorf("namespace and key must be non-empty")
	}
	return 0, ns, k, nil
}

func bulkCustomerIDs(args map[string]any) ([]int, error) {
	ids, err := requiredPositiveIntIDs(args, "customer_ids")
	if err != nil {
		return nil, err
	}
	seen := make(map[int]struct{}, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > maxBulkCustomerMetafieldTargets {
		return nil, fmt.Errorf("customer_ids exceeds maximum of %d for bulk metafield operations", maxBulkCustomerMetafieldTargets)
	}
	return out, nil
}
