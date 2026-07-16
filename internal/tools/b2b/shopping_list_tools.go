package b2b

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ============================================================
// Shopping list tools
// ============================================================

func (ct *CompanyTools) registerShoppingListTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/shopping_lists/list",
		Tier:    middleware.TierR0,
		Summary: "List shopping lists visible to a buyer",
		Tool: mcp.NewTool("b2b_shopping_lists_list",
			mcp.WithDescription("List B2B Edition shopping lists. Provide exactly one of user_id (B2B buyer user ID) or customer_id (BigCommerce customer ID). A junior buyer sees only their own lists; senior buyer/admin see all of their company's approved/pending lists."),
			mcp.WithNumber("user_id", mcp.Description("B2B buyer user ID (mutually exclusive with customer_id).")),
			mcp.WithNumber("customer_id", mcp.Description("BigCommerce customer ID (mutually exclusive with user_id).")),
			mcp.WithNumber("channel_id", mcp.Description("Filter by BigCommerce channel ID.")),
			mcp.WithNumber("created_user_id", mcp.Description("Filter by the B2B user ID who created the list.")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleShoppingListList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/shopping_lists/get",
		Tier:    middleware.TierR0,
		Summary: "Get a shopping list's detail and items",
		Tool: mcp.NewTool("b2b_shopping_lists_get",
			mcp.WithDescription("Get a shopping list's detail, including its items."),
			mcp.WithNumber("shopping_list_id", mcp.Description("Shopping list ID"), mcp.Required()),
			mcp.WithNumber("user_id", mcp.Description("Optional: check this user's permission to view the list.")),
			mcp.WithString("sort_by", mcp.Description("Optional item sort field.")),
			mcp.WithString("order_by", mcp.Description("ASC or DESC.")),
		),
		Handler: ct.handleShoppingListGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/shopping_lists/create",
		Tier:    middleware.TierR1,
		Summary: "Create a shopping list, optionally with initial items",
		Tool: mcp.NewTool("b2b_shopping_lists_create",
			mcp.WithDescription(`Create a shopping list. Provide exactly one of user_id or customer_id as the owner. Optionally include items_json: [{"productId":1,"variantId":10,"quantity":2}]. status: 0=approved, 30=draft, 40=ready for approval. Preview → confirm.`),
			mcp.WithString("name", mcp.Description("List name"), mcp.Required()),
			mcp.WithString("description", mcp.Description("List description.")),
			mcp.WithString("status", mcp.Description("0=approved, 30=draft, 40=ready for approval.")),
			mcp.WithNumber("user_id", mcp.Description("B2B buyer user ID owner (mutually exclusive with customer_id).")),
			mcp.WithNumber("customer_id", mcp.Description("BigCommerce customer ID owner (mutually exclusive with user_id).")),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID.")),
			mcp.WithString("items_json", mcp.Description(`Optional JSON array of items: [{"productId":1,"variantId":10,"quantity":2}]`)),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create.")),
		),
		Handler: ct.handleShoppingListCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/shopping_lists/update",
		Tier:    middleware.TierR1,
		Summary: "Update a shopping list's fields and/or items",
		Tool: mcp.NewTool("b2b_shopping_lists_update",
			mcp.WithDescription(`Update a shopping list. No field is required — send only what changes. If updating items_json, include every item to keep (by id, or by productId+variantId if id is omitted); setting an item's quantity to 0 removes it. Preview → confirm.`),
			mcp.WithNumber("shopping_list_id", mcp.Description("Shopping list ID"), mcp.Required()),
			mcp.WithString("name", mcp.Description("New name.")),
			mcp.WithString("description", mcp.Description("New description.")),
			mcp.WithString("status", mcp.Description("0=approved, 30=draft, 40=ready for approval.")),
			mcp.WithNumber("user_id", mcp.Description("Optional: check this user's permission to update the list.")),
			mcp.WithNumber("customer_id", mcp.Description("Optional: check this customer's permission to update the list.")),
			mcp.WithString("items_json", mcp.Description(`JSON array of items to keep/change: [{"id":5,"productId":1,"variantId":10,"quantity":3}]. Omitted existing items are NOT removed unless you include them with quantity=0.`)),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleShoppingListUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/shopping_lists/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a shopping list",
		Tool: mcp.NewTool("b2b_shopping_lists_delete",
			mcp.WithDescription("Permanently delete a shopping list. Preview → confirm."),
			mcp.WithNumber("shopping_list_id", mcp.Description("Shopping list ID"), mcp.Required()),
			mcp.WithNumber("user_id", mcp.Description("Optional: check this user's permission to delete the list.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete permanently.")),
		),
		Handler: ct.handleShoppingListDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/shopping_lists/items/remove",
		Tier:    middleware.TierR2,
		Summary: "Remove a single item from a shopping list",
		Tool: mcp.NewTool("b2b_shopping_lists_items_remove",
			mcp.WithDescription("Remove a single item from a shopping list by item ID. Preview → confirm."),
			mcp.WithNumber("shopping_list_id", mcp.Description("Shopping list ID"), mcp.Required()),
			mcp.WithNumber("item_id", mcp.Description("Item ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to remove.")),
		),
		Handler: ct.handleShoppingListItemRemove,
	})
}

func shoppingListOwnerFromArgs(args map[string]any) (userID, customerID int, err error) {
	if v, ok := args["user_id"].(float64); ok && v > 0 {
		userID = int(v)
	}
	if v, ok := args["customer_id"].(float64); ok && v > 0 {
		customerID = int(v)
	}
	if userID > 0 && customerID > 0 {
		return 0, 0, fmt.Errorf("provide only one of user_id or customer_id, not both")
	}
	return userID, customerID, nil
}

func parseShoppingListItems(args map[string]any, key string) ([]bigcommerce.B2BShoppingListItem, error) {
	raw, ok := args[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var items []bigcommerce.B2BShoppingListItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("invalid %s: %v", key, err)
	}
	return items, nil
}

func (ct *CompanyTools) handleShoppingListList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	userID, customerID, err := shoppingListOwnerFromArgs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if userID == 0 && customerID == 0 {
		return shared.ToolError("provide user_id or customer_id"), nil
	}
	params := url.Values{}
	if userID > 0 {
		params.Set("userId", fmt.Sprintf("%d", userID))
	}
	if customerID > 0 {
		params.Set("customerId", fmt.Sprintf("%d", customerID))
	}
	if v, ok := args["channel_id"].(float64); ok && v > 0 {
		params.Set("channelId", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["created_user_id"].(float64); ok && v > 0 {
		params.Set("createdUserId", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	lists, err := ct.bc.ListB2BShoppingLists(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B shopping lists: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(lists), "shopping_lists": lists})
}

func (ct *CompanyTools) handleShoppingListGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "shopping_list_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := url.Values{}
	if v, ok := args["user_id"].(float64); ok && v > 0 {
		params.Set("userId", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["sort_by"].(string); ok && v != "" {
		params.Set("sortBy", v)
	}
	if v, ok := args["order_by"].(string); ok && v != "" {
		params.Set("orderBy", v)
	}
	list, err := ct.bc.GetB2BShoppingList(ctx, id, params.Encode())
	if err != nil {
		return shared.ToolError("failed to get B2B shopping list %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"shopping_list": list})
}

func (ct *CompanyTools) handleShoppingListCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	name, _ := args["name"].(string)
	if strings.TrimSpace(name) == "" {
		return shared.ToolError("name is required"), nil
	}
	userID, customerID, err := shoppingListOwnerFromArgs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	items, err := parseShoppingListItems(args, "items_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload := bigcommerce.B2BShoppingListCreate{Name: name, UserID: userID, CustomerID: customerID, Items: items}
	if v, ok := args["description"].(string); ok {
		payload.Description = v
	}
	if v, ok := args["status"].(string); ok && v != "" {
		payload.Status = v
	} else {
		// Undocumented in the OpenAPI schema (only `name` is marked required),
		// but the live API rejects a create with a null status. Default to
		// approved (0) rather than surfacing that 422 to every caller.
		payload.Status = "0"
	}
	if v, ok := args["channel_id"].(float64); ok && v > 0 {
		payload.ChannelID = int(v)
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_shopping_list",
			"payload": payload,
			"message": fmt.Sprintf("Will create shopping list %q. Pass confirmed=true.", name),
		})
	}

	result, err := ct.bc.CreateB2BShoppingList(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create B2B shopping list: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "shopping_list": result})
}

func (ct *CompanyTools) handleShoppingListUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "shopping_list_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	items, err := parseShoppingListItems(args, "items_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload := bigcommerce.B2BShoppingListUpdate{Items: items}
	hasField := len(items) > 0
	if v, ok := args["name"].(string); ok && v != "" {
		payload.Name = v
		hasField = true
	}
	if v, ok := args["description"].(string); ok && v != "" {
		payload.Description = v
		hasField = true
	}
	if v, ok := args["status"].(string); ok && v != "" {
		payload.Status = v
		hasField = true
	}
	if v, ok := args["user_id"].(float64); ok && v > 0 {
		payload.UserID = int(v)
	}
	if v, ok := args["customer_id"].(float64); ok && v > 0 {
		payload.CustomerID = int(v)
	}
	if !hasField {
		return shared.ToolError("at least one of name, description, status, or items_json must be provided"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":           "preview",
			"action":           "update_b2b_shopping_list",
			"shopping_list_id": id,
			"payload":          payload,
			"message":          fmt.Sprintf("Will apply these fields to shopping list %d. Pass confirmed=true.", id),
		})
	}

	result, err := ct.bc.UpdateB2BShoppingList(ctx, id, payload)
	if err != nil {
		return shared.ToolError("failed to update B2B shopping list %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "shopping_list_id": id, "shopping_list": result})
}

func (ct *CompanyTools) handleShoppingListDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "shopping_list_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	userID := 0
	if v, ok := args["user_id"].(float64); ok && v > 0 {
		userID = int(v)
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":           "preview",
			"action":           "delete_b2b_shopping_list",
			"shopping_list_id": id,
			"message":          fmt.Sprintf("Will permanently delete shopping list %d. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.DeleteB2BShoppingList(ctx, id, userID); err != nil {
		return shared.ToolError("failed to delete B2B shopping list %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "shopping_list_id": id})
}

func (ct *CompanyTools) handleShoppingListItemRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	listID, err := shared.ReadPositiveInt(args, "shopping_list_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	itemID, err := shared.ReadPositiveInt(args, "item_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":           "preview",
			"action":           "remove_b2b_shopping_list_item",
			"shopping_list_id": listID,
			"item_id":          itemID,
			"message":          fmt.Sprintf("Will remove item %d from shopping list %d. Pass confirmed=true.", itemID, listID),
		})
	}

	if err := ct.bc.DeleteB2BShoppingListItem(ctx, listID, itemID); err != nil {
		return shared.ToolError("failed to remove item %d from B2B shopping list %d: %v", itemID, listID, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "removed", "shopping_list_id": listID, "item_id": itemID})
}
