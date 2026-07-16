package b2b

import (
	"context"
	"fmt"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ============================================================
// Channel tools
// ============================================================

func (ct *CompanyTools) registerChannelTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/channels/list",
		Tier:    middleware.TierR0,
		Summary: "List storefront channels known to B2B Edition",
		Tool: mcp.NewTool("b2b_channels_list",
			mcp.WithDescription("List storefront channels as seen by B2B Edition. Note: the B2B `id` and BigCommerce `channelId` differ."),
		),
		Handler: ct.handleChannelList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/channels/get",
		Tier:    middleware.TierR0,
		Summary: "Get a B2B channel by BigCommerce channel ID",
		Tool: mcp.NewTool("b2b_channels_get",
			mcp.WithDescription("Get a single storefront channel by its BigCommerce channel ID."),
			mcp.WithNumber("channel_id", mcp.Description("BigCommerce channel ID"), mcp.Required()),
		),
		Handler: ct.handleChannelGet,
	})
}

func (ct *CompanyTools) handleChannelList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	channels, err := ct.bc.ListB2BChannels(ctx)
	if err != nil {
		return shared.ToolError("failed to list B2B channels: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(channels), "channels": channels})
}

func (ct *CompanyTools) handleChannelGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "channel_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	ch, err := ct.bc.GetB2BChannel(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B channel %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"channel": ch})
}

// ============================================================
// Order tools
// ============================================================

func (ct *CompanyTools) registerOrderTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/orders/get",
		Tier:    middleware.TierR0,
		Summary: "Get the B2B view of an order (PO number, company, extra fields)",
		Tool: mcp.NewTool("b2b_orders_get",
			mcp.WithDescription("Get the B2B Edition view of an order by its BigCommerce order ID (not the B2B order ID). Includes PO number, company linkage, and extra fields."),
			mcp.WithNumber("bc_order_id", mcp.Description("BigCommerce order ID"), mcp.Required()),
		),
		Handler: ct.handleOrderGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/orders/update",
		Tier:    middleware.TierR1,
		Summary: "Set an order's PO number and/or extra fields",
		Tool: mcp.NewTool("b2b_orders_update",
			mcp.WithDescription("Update the B2B purchase-order number and/or extra fields on an order (by BigCommerce order ID). Preview → confirm."),
			mcp.WithNumber("bc_order_id", mcp.Description("BigCommerce order ID"), mcp.Required()),
			mcp.WithString("po_number", mcp.Description("Purchase-order number to set.")),
			mcp.WithString("extra_fields_json", mcp.Description(`Optional JSON array: [{"fieldName":"...","fieldValue":"..."}]`)),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleOrderUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/orders/assign_customer_orders",
		Tier:    middleware.TierR2,
		Summary: "Assign a customer's historical orders to their company",
		Tool: mcp.NewTool("b2b_orders_assign_customer_orders",
			mcp.WithDescription("Associate a buyer's pre-existing BigCommerce orders with their Company account. Useful for orders placed before the buyer joined the company. customer_id is the BigCommerce customer ID (not the B2B user ID). Preview → confirm."),
			mcp.WithNumber("customer_id", mcp.Description("BigCommerce customer ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleOrderAssignCustomer,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/orders/reassign",
		Tier:    middleware.TierR2,
		Summary: "Reassign a customer's orders to a company by customer group (Dependent-behavior stores only)",
		Tool: mcp.NewTool("b2b_orders_reassign",
			mcp.WithDescription("Reassign all of a customer's orders to a different company by BigCommerce customer group ID. Only supported on stores using legacy Dependent Companies behavior — not Independent behavior. Preview → confirm."),
			mcp.WithNumber("customer_id", mcp.Description("BigCommerce customer ID"), mcp.Required()),
			mcp.WithNumber("bc_group_id", mcp.Description("BigCommerce customer group ID for the target company"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleOrderReassign,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/orders/extra_fields",
		Tier:    middleware.TierR0,
		Summary: "List extra-field definitions configured for orders",
		Tool: mcp.NewTool("b2b_orders_extra_fields",
			mcp.WithDescription("List the extra-field (custom field) definitions configured for B2B Edition orders."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleOrderExtraFields,
	})
}

func (ct *CompanyTools) handleOrderGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "bc_order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	order, err := ct.bc.GetB2BOrder(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B order %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"order": order})
}

func (ct *CompanyTools) handleOrderUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "bc_order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload := bigcommerce.B2BOrderUpdate{}
	hasField := false
	if v, ok := args["po_number"].(string); ok && v != "" {
		payload.PONumber = v
		hasField = true
	}
	if ef, eerr := parseB2BExtraFieldsJSON(args, "extra_fields_json"); eerr != nil {
		return shared.ToolError("%s", eerr.Error()), nil
	} else if len(ef) > 0 {
		payload.ExtraFields = ef
		hasField = true
	}
	if !hasField {
		return shared.ToolError("provide po_number and/or extra_fields_json"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "update_b2b_order",
			"bc_order_id": id,
			"payload":     payload,
			"message":     fmt.Sprintf("Will update B2B fields on order %d. Pass confirmed=true.", id),
		})
	}

	order, err := ct.bc.UpdateB2BOrder(ctx, id, payload)
	if err != nil {
		return shared.ToolError("failed to update B2B order %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "bc_order_id": id, "order": order})
}

func (ct *CompanyTools) handleOrderAssignCustomer(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "customer_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "assign_customer_orders_to_company",
			"customer_id": id,
			"message":     fmt.Sprintf("Will associate BigCommerce customer %d's existing orders with their company. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.AssignCustomerOrdersToCompany(ctx, id); err != nil {
		return shared.ToolError("failed to assign customer %d orders: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "assigned", "customer_id": id})
}

func (ct *CompanyTools) handleOrderReassign(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "customer_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	groupID, err := shared.ReadPositiveInt(args, "bc_group_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "reassign_orders_to_company",
			"customer_id": id,
			"bc_group_id": groupID,
			"message":     fmt.Sprintf("Will reassign customer %d's orders to customer group %d. Dependent-behavior stores only. Pass confirmed=true.", id, groupID),
		})
	}

	if err := ct.bc.ReassignOrdersToCompany(ctx, id, groupID); err != nil {
		return shared.ToolError("failed to reassign customer %d orders: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "reassigned", "customer_id": id, "bc_group_id": groupID})
}

func (ct *CompanyTools) handleOrderExtraFields(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	defs, err := ct.bc.ListB2BOrderExtraFields(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B order extra fields: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(defs), "extra_fields": defs})
}
