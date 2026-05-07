package orders

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

const defaultOrderMetafieldPermissionSet = "app_only"

// OrderMetafields holds handlers for orders/management/metafields/*.
type OrderMetafields struct {
	bc BigCommerceOrdersAPI
}

// NewOrderMetafields constructs order metafield handlers.
func NewOrderMetafields(bc BigCommerceOrdersAPI) *OrderMetafields {
	return &OrderMetafields{bc: bc}
}

// RegisterTools wires order metafield tools into discovery.
func (m *OrderMetafields) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/metafields/list",
		Tier:        middleware.TierR0,
		Summary:     "List metafields on one order (V3)",
		Description: "GET /v3/orders/{id}/metafields with optional page/limit.",
		Tool: mcp.NewTool("orders_management_metafields_list",
			mcp.WithDescription("List metafields on one order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: m.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/metafields/set",
		Tier:    middleware.TierR1,
		Summary: "Create or update one order metafield (upsert by namespace+key)",
		Description: "POST or PUT /v3/orders/{id}/metafields. " +
			"Defaults new metafields to app_only when permission_set is omitted. " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("orders_management_metafields_set",
			mcp.WithDescription("Upsert one order metafield by namespace+key."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("Metafield namespace."), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key."), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value (string)."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description.")),
			mcp.WithString("permission_set", mcp.Description("Optional permission_set; defaults to app_only for new metafields.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: m.handleSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/metafields/delete",
		Tier:        middleware.TierR1,
		Summary:     "Delete one order metafield (V3)",
		Description: "DELETE /v3/orders/{id}/metafields/{metafield_id}. Delete by metafield_id or namespace+key. Preview then confirmed=true.",
		Tool: mcp.NewTool("orders_management_metafields_delete",
			mcp.WithDescription("Delete one order metafield by metafield_id or namespace+key."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("metafield_id", mcp.Description("Metafield id (mutually exclusive with namespace+key).")),
			mcp.WithString("namespace", mcp.Description("Namespace (use with key).")),
			mcp.WithString("key", mcp.Description("Key (use with namespace).")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: m.handleDelete,
	})
}

func (m *OrderMetafields) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.OrderMetafieldListParams{}
	if page, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.Page = page
	}
	if limit, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		if limit > maxOrdersListLimit {
			return shared.ToolError("limit must be <= %d", maxOrdersListLimit), nil
		}
		params.Limit = limit
	}
	rows, err := m.bc.ListOrderMetafields(ctx, orderID, params)
	if err != nil {
		return shared.ToolError("failed to list order metafields: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":   orderID,
		"total":      len(rows),
		"metafields": rows,
	})
}

func (m *OrderMetafields) handleSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	ns, key, value, desc, permSet, err := parseOrderMetafieldSetFields(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	existing, err := m.bc.ListOrderMetafields(ctx, orderID, bigcommerce.OrderMetafieldListParams{})
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
			"status":    "pending_confirmation",
			"order_id":  orderID,
			"namespace": ns,
			"key":       key,
			"value":     value,
		}
		var effectivePerm string
		if existingMF != nil {
			action = "update"
			preview["metafield_id"] = existingMF.ID
			preview["existing_value"] = existingMF.Value
			preview["existing_permission_set"] = existingMF.PermissionSet
			if permSet != "" {
				effectivePerm = permSet
			} else {
				effectivePerm = existingMF.PermissionSet
			}
		} else if permSet != "" {
			effectivePerm = permSet
		} else {
			effectivePerm = defaultOrderMetafieldPermissionSet
		}
		preview["action"] = action
		preview["permission_set"] = effectivePerm
		preview["permission_note"] = shared.AppOnlyMetafieldPermissionNote
		if desc != "" {
			preview["description"] = desc
		}
		preview["message"] = fmt.Sprintf(
			"Will %s metafield %s.%s on order %d. Pass confirmed=true to execute.",
			action, ns, key, orderID,
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
		updated, uerr := m.bc.UpdateOrderMetafield(ctx, orderID, existingMF.ID, payload)
		if uerr != nil {
			return shared.ToolError("update failed: %v", uerr), nil
		}
		return shared.ToolJSON(map[string]any{
			"status":    "updated",
			"metafield": updated,
			"message":   fmt.Sprintf("Metafield %s.%s updated on order %d.", ns, key, orderID),
		})
	}

	if payload.PermissionSet == "" {
		payload.PermissionSet = defaultOrderMetafieldPermissionSet
	}
	created, cerr := m.bc.CreateOrderMetafield(ctx, orderID, payload)
	if cerr != nil {
		return shared.ToolError("create failed: %v", cerr), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":    "created",
		"metafield": created,
		"message":   fmt.Sprintf("Metafield %s.%s created on order %d.", ns, key, orderID),
	})
}

func (m *OrderMetafields) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	mfID, ns, key, err := parseOrderMetafieldDeleteSelector(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if mfID == 0 {
		existing, lerr := m.bc.ListOrderMetafields(ctx, orderID, bigcommerce.OrderMetafieldListParams{})
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
			return shared.ToolError("no metafield found with namespace %q key %q on order %d", ns, key, orderID), nil
		}
	}
	if !middleware.IsConfirmedFromArgs(args) {
		preview := map[string]any{
			"status":       "pending_confirmation",
			"order_id":     orderID,
			"metafield_id": mfID,
			"message":      fmt.Sprintf("Will delete metafield %d from order %d. Pass confirmed=true to execute.", mfID, orderID),
		}
		if ns != "" {
			preview["namespace"] = ns
			preview["key"] = key
		}
		return shared.ToolJSON(preview)
	}
	if err := m.bc.DeleteOrderMetafield(ctx, orderID, mfID); err != nil {
		return shared.ToolError("delete failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":  "deleted",
		"message": fmt.Sprintf("Metafield %d deleted from order %d.", mfID, orderID),
	})
}

func parseOrderMetafieldSetFields(args map[string]any) (namespace, key, value, description, permissionSet string, err error) {
	nsRaw, ok := args["namespace"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("namespace is required")
	}
	ns, sOK := nsRaw.(string)
	if !sOK || strings.TrimSpace(ns) == "" {
		return "", "", "", "", "", fmt.Errorf("namespace must be a non-empty string")
	}
	ns = strings.TrimSpace(ns)

	keyRaw, ok := args["key"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("key is required")
	}
	k, sOK := keyRaw.(string)
	if !sOK || strings.TrimSpace(k) == "" {
		return "", "", "", "", "", fmt.Errorf("key must be a non-empty string")
	}
	k = strings.TrimSpace(k)

	valueRaw, ok := args["value"]
	if !ok {
		return "", "", "", "", "", fmt.Errorf("value is required")
	}
	val, sOK := valueRaw.(string)
	if !sOK {
		return "", "", "", "", "", fmt.Errorf("value must be a string")
	}

	desc := ""
	if v, ok := args["description"]; ok {
		s, ok := v.(string)
		if !ok {
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

func parseOrderMetafieldDeleteSelector(args map[string]any) (mfID int, namespace, key string, err error) {
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
		id, err := shared.ReadPositiveInt(args, "metafield_id")
		if err != nil {
			return 0, "", "", err
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
