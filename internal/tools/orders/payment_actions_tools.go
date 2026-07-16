package orders

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// Payments holds handlers for orders/payments/* and orders/refunds/*.
type Payments struct {
	bc BigCommerceOrdersAPI
}

// NewPayments constructs payment-action handlers.
func NewPayments(bc BigCommerceOrdersAPI) *Payments {
	return &Payments{bc: bc}
}

// RegisterTools wires payment-action tools into discovery.
func (p *Payments) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/payments/actions/list",
		Tier:        middleware.TierR0,
		Summary:     "List payment actions for one order (V3)",
		Description: "GET /v3/orders/{id}/payment_actions.",
		Tool: mcp.NewTool("orders_payments_actions_list",
			mcp.WithDescription("List payment actions for an order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: p.handleListPaymentActions,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/payments/transactions/list",
		Tier:        middleware.TierR0,
		Summary:     "List transactions for one order (V3)",
		Description: "GET /v3/orders/{id}/transactions. Useful for parity checks against payment actions/refunds.",
		Tool: mcp.NewTool("orders_payments_transactions_list",
			mcp.WithDescription("List transactions for one order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: p.handleListTransactions,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/payments/capture",
		Tier:    middleware.TierR3,
		Summary: "Capture payment for one order (V3)",
		Description: "POST /v3/orders/{id}/payment_actions/capture. Financially sensitive. " +
			"Per-order preview and explicit confirmation required.",
		Tool: mcp.NewTool("orders_payments_capture",
			mcp.WithDescription("Capture payment for one order. Preview first; confirmed=true to execute."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: p.handleCapture,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/payments/void",
		Tier:    middleware.TierR3,
		Summary: "Void payment for one order (V3)",
		Description: "POST /v3/orders/{id}/payment_actions/void. Financially sensitive. " +
			"Per-order preview and explicit confirmation required.",
		Tool: mcp.NewTool("orders_payments_void",
			mcp.WithDescription("Void payment for one order. Preview first; confirmed=true to execute."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: p.handleVoid,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/refunds/list",
		Tier:        middleware.TierR0,
		Summary:     "List refunds for one order (V3)",
		Description: "GET /v3/orders/{id}/payment_actions/refunds.",
		Tool: mcp.NewTool("orders_refunds_list",
			mcp.WithDescription("List refunds for an order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithString("transaction_id", mcp.Description("Optional transaction filter.")),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: p.handleListRefunds,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/refunds/legacy_list",
		Tier:        middleware.TierR0,
		Summary:     "List legacy refunds for one order (V2)",
		Description: "GET /v2/orders/{id}/refunds. Legacy reference endpoint for parity checks.",
		Tool: mcp.NewTool("orders_refunds_legacy_list",
			mcp.WithDescription("List legacy V2 refund rows for one order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: p.handleListLegacyRefunds,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/refunds/quote",
		Tier:    middleware.TierR2,
		Summary: "Create refund quote for one order (V3)",
		Description: "POST /v3/orders/{id}/payment_actions/refund_quotes. " +
			"Use before create to avoid refund 422 validation failures. Preview required.",
		Tool: mcp.NewTool("orders_refunds_quote",
			mcp.WithDescription("Create a refund quote payload. Preview first; confirmed=true to execute."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithObject("quote", mcp.Description("Refund quote payload (BigCommerce shape)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: p.handleCreateRefundQuote,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/refunds/create",
		Tier:    middleware.TierR3,
		Summary: "Create refund for one order (V3)",
		Description: "POST /v3/orders/{id}/payment_actions/refunds. Financially sensitive. " +
			"Process sequentially per order. Preview and explicit confirmation required.",
		Tool: mcp.NewTool("orders_refunds_create",
			mcp.WithDescription("Create a refund for one order. Preview first; confirmed=true to execute."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithObject("refund", mcp.Description("Refund payload (BigCommerce shape)."), mcp.Required()),
			mcp.WithString("transaction_id", mcp.Description("Optional transaction id query filter.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: p.handleCreateRefund,
	})
}

func (p *Payments) handleListPaymentActions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.OrderPaymentActionListParams{}
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
	rows, err := p.bc.ListOrderPaymentActions(ctx, orderID, params)
	if err != nil {
		return shared.ToolError("failed to list payment actions: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":        orderID,
		"total":           len(rows),
		"payment_actions": rows,
	})
}

func (p *Payments) handleListTransactions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.OrderTransactionListParams{}
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
	rows, err := p.bc.ListOrderTransactions(ctx, orderID, params)
	if err != nil {
		return shared.ToolError("failed to list transactions: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":     orderID,
		"total":        len(rows),
		"transactions": rows,
	})
}

func (p *Payments) handleListRefunds(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.OrderRefundListParams{}
	if txID, ok, err := readOptionalTrimmedString(args, "transaction_id"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.TransactionID = txID
	}
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
	rows, err := p.bc.ListOrderRefunds(ctx, orderID, params)
	if err != nil {
		return shared.ToolError("failed to list refunds: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id": orderID,
		"total":    len(rows),
		"refunds":  rows,
	})
}

func (p *Payments) handleListLegacyRefunds(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.OrderLegacyRefundListParams{}
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
	rows, err := p.bc.ListOrderLegacyRefunds(ctx, orderID, params)
	if err != nil {
		return shared.ToolError("failed to list legacy refunds: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":       orderID,
		"total":          len(rows),
		"legacy_refunds": rows,
	})
}

func (p *Payments) handleCapture(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		current, gerr := p.bc.GetOrder(ctx, orderID, bigcommerce.OrderGetParams{})
		if gerr != nil {
			return shared.ToolError("failed to fetch order %d for preview — resolve this before confirming a capture: %v", orderID, gerr), nil
		}
		preview := map[string]any{
			"status":   "preview",
			"action":   "capture_payment",
			"order_id": orderID,
			"message": "Capture is asynchronous and financially sensitive. " +
				"Pass confirmed=true to execute.",
		}
		if current != nil {
			preview["current_status_id"] = current.StatusID
			preview["current_status"] = current.Status
		}
		return shared.ToolJSON(preview)
	}
	data, err := p.bc.CreateOrderPaymentCapture(ctx, orderID)
	if err != nil {
		return shared.ToolError("capture failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":         "completed",
		"action":         "capture_payment",
		"order_id":       orderID,
		"payment_action": data,
	})
}

func (p *Payments) handleVoid(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		current, gerr := p.bc.GetOrder(ctx, orderID, bigcommerce.OrderGetParams{})
		if gerr != nil {
			return shared.ToolError("failed to fetch order %d for preview — resolve this before confirming a void: %v", orderID, gerr), nil
		}
		preview := map[string]any{
			"status":   "preview",
			"action":   "void_payment",
			"order_id": orderID,
			"message": "Void is asynchronous and financially sensitive. " +
				"Pass confirmed=true to execute.",
		}
		if current != nil {
			preview["current_status_id"] = current.StatusID
			preview["current_status"] = current.Status
		}
		return shared.ToolJSON(preview)
	}
	data, err := p.bc.CreateOrderPaymentVoid(ctx, orderID)
	if err != nil {
		return shared.ToolError("void failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":         "completed",
		"action":         "void_payment",
		"order_id":       orderID,
		"payment_action": data,
	})
}

func (p *Payments) handleCreateRefundQuote(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload, err := requiredObjectPayload(args, "quote")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "create_refund_quote",
			"order_id": orderID,
			"payload":  payload,
			"message":  "Review payload, then pass confirmed=true to execute.",
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return shared.ToolError("failed to marshal quote payload: %v", err), nil
	}
	data, err := p.bc.CreateOrderRefundQuote(ctx, orderID, raw)
	if err != nil {
		return shared.ToolError("refund quote failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status": "completed",
		"quote":  data,
	})
}

func (p *Payments) handleCreateRefund(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload, err := requiredObjectPayload(args, "refund")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	transactionID, _, err := readOptionalTrimmedString(args, "transaction_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		current, gerr := p.bc.GetOrder(ctx, orderID, bigcommerce.OrderGetParams{})
		if gerr != nil {
			return shared.ToolError("failed to fetch order %d for preview — resolve this before confirming a refund: %v", orderID, gerr), nil
		}
		preview := map[string]any{
			"status":   "preview",
			"action":   "create_refund",
			"order_id": orderID,
			"payload":  payload,
			"message": "Financially sensitive action. Refunds should be processed sequentially per order. " +
				"Pass confirmed=true to execute.",
		}
		if transactionID != "" {
			preview["transaction_id"] = transactionID
		}
		if current != nil {
			preview["current_status_id"] = current.StatusID
			preview["current_status"] = current.Status
		}
		return shared.ToolJSON(preview)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return shared.ToolError("failed to marshal refund payload: %v", err), nil
	}
	data, err := p.bc.CreateOrderRefund(ctx, orderID, raw, transactionID)
	if err != nil {
		return shared.ToolError("refund failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "completed",
		"order_id": orderID,
		"refund":   data,
	})
}

func requiredObjectPayload(args map[string]any, key string) (map[string]any, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	if len(obj) == 0 {
		return nil, fmt.Errorf("%s must not be empty", key)
	}
	return obj, nil
}
