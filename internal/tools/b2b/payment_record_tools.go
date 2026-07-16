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
// Payment record tools (money logged/received against invoices).
//
// Distinct from b2b/payments/* (payment METHOD definitions and per-company
// enablement, in payment_tools.go): this group covers actual payment
// records — offline payments, their processing lifecycle, and transaction
// history. Both groups' list endpoints share the REST path "/payments"; the
// deciding factor is the base URL (this group uses /ip, like invoices).
// ============================================================

func (ct *CompanyTools) registerPaymentRecordTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/list",
		Tier:    middleware.TierR0,
		Summary: "List payment records logged against invoices",
		Tool: mcp.NewTool("b2b_payment_records_list",
			mcp.WithDescription("List B2B payment records (money logged/received against invoices) — not to be confused with b2b/payments/list, which lists payment METHOD definitions."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handlePaymentRecordList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/get",
		Tier:    middleware.TierR0,
		Summary: "Get a payment record's detail",
		Tool: mcp.NewTool("b2b_payment_records_get",
			mcp.WithDescription("Get detail for a single payment record."),
			mcp.WithNumber("payment_id", mcp.Description("Payment ID"), mcp.Required()),
		),
		Handler: ct.handlePaymentRecordGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/transactions",
		Tier:    middleware.TierR0,
		Summary: "List a payment record's transaction history",
		Tool: mcp.NewTool("b2b_payment_records_transactions",
			mcp.WithDescription("List the transaction history for a payment record."),
			mcp.WithNumber("payment_id", mcp.Description("Payment ID"), mcp.Required()),
		),
		Handler: ct.handlePaymentRecordTransactions,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/operations",
		Tier:    middleware.TierR0,
		Summary: "Get the operations currently allowed on a payment record",
		Tool: mcp.NewTool("b2b_payment_records_operations",
			mcp.WithDescription("Get the operation codes currently valid for a payment record (the allowed values for b2b/payment_records/perform_operation)."),
			mcp.WithNumber("payment_id", mcp.Description("Payment ID"), mcp.Required()),
		),
		Handler: ct.handlePaymentRecordOperations,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/create_offline",
		Tier:    middleware.TierR2,
		Summary: "Log a new offline payment against one or more invoices",
		Tool: mcp.NewTool("b2b_payment_records_create_offline",
			mcp.WithDescription(`Log an offline payment (check, wire, etc.) against one or more invoices. line_items_json: [{"invoiceId":141,"amount":"25.00"}]. Preview → confirm.`),
			mcp.WithString("line_items_json", mcp.Description(`JSON array: [{"invoiceId":141,"amount":"25.00"}]`), mcp.Required()),
			mcp.WithString("currency", mcp.Description("Currency code (e.g. USD).")),
			mcp.WithString("memo", mcp.Description("Free-text memo for this payment.")),
			mcp.WithString("external_id", mcp.Description("External (ERP) payment ID.")),
			mcp.WithString("customer_id", mcp.Description("B2B company ID.")),
			mcp.WithString("external_customer_id", mcp.Description("External (ERP) customer ID.")),
			mcp.WithString("payer_name", mcp.Description(`Payer display name (default "Store offline payment").`)),
			mcp.WithString("payer_customer_id", mcp.Description("Payer's customer ID.")),
			mcp.WithString("processing_status", mcp.Description("1=Awaiting Processing, 2=Processing, 3=Completed (default), 4=Refunded.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create.")),
		),
		Handler: ct.handlePaymentRecordCreateOffline,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/update_offline",
		Tier:    middleware.TierR2,
		Summary: "Update an existing offline payment record",
		Tool: mcp.NewTool("b2b_payment_records_update_offline",
			mcp.WithDescription("Update an offline payment record. Same fields as create_offline; send only what changes. Preview → confirm."),
			mcp.WithNumber("payment_id", mcp.Description("Payment ID"), mcp.Required()),
			mcp.WithString("line_items_json", mcp.Description(`JSON array: [{"invoiceId":141,"amount":"25.00"}]`)),
			mcp.WithString("currency", mcp.Description("Currency code.")),
			mcp.WithString("memo", mcp.Description("Free-text memo.")),
			mcp.WithString("external_id", mcp.Description("External (ERP) payment ID.")),
			mcp.WithString("customer_id", mcp.Description("B2B company ID.")),
			mcp.WithString("external_customer_id", mcp.Description("External (ERP) customer ID.")),
			mcp.WithString("payer_name", mcp.Description("Payer display name.")),
			mcp.WithString("payer_customer_id", mcp.Description("Payer's customer ID.")),
			mcp.WithString("processing_status", mcp.Description("1=Awaiting Processing, 2=Processing, 3=Completed, 4=Refunded.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handlePaymentRecordUpdateOffline,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/perform_operation",
		Tier:    middleware.TierR2,
		Summary: "Perform a lifecycle operation on a payment record",
		Tool: mcp.NewTool("b2b_payment_records_perform_operation",
			mcp.WithDescription("Perform a lifecycle operation (e.g. void, refund) on a payment record. Use b2b/payment_records/operations first to see which operation codes are currently valid for this specific payment — operations may be irreversible (e.g. void/refund). Preview → confirm."),
			mcp.WithNumber("payment_id", mcp.Description("Payment ID"), mcp.Required()),
			mcp.WithString("operation_code", mcp.Description("Operation code (see b2b/payment_records/operations for valid values on this payment)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to perform the operation.")),
		),
		Handler: ct.handlePaymentRecordPerformOperation,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/update_processing_status",
		Tier:    middleware.TierR2,
		Summary: "Directly set a payment record's processing status",
		Tool: mcp.NewTool("b2b_payment_records_update_processing_status",
			mcp.WithDescription("Directly set a payment record's processing status. Preview → confirm."),
			mcp.WithNumber("payment_id", mcp.Description("Payment ID"), mcp.Required()),
			mcp.WithNumber("processing_status", mcp.Description("1=Awaiting Processing, 2=Processing, 3=Completed, 4=Refunded."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handlePaymentRecordUpdateProcessingStatus,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payment_records/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a payment record",
		Tool: mcp.NewTool("b2b_payment_records_delete",
			mcp.WithDescription("Permanently delete a payment record. This does not reverse the underlying transaction (e.g. with a payment processor) — use b2b/payment_records/perform_operation to void/refund first if applicable. Preview → confirm."),
			mcp.WithNumber("payment_id", mcp.Description("Payment ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete permanently.")),
		),
		Handler: ct.handlePaymentRecordDelete,
	})
}

func (ct *CompanyTools) handlePaymentRecordList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	records, err := ct.bc.ListB2BPaymentRecords(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B payment records: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(records), "payment_records": records})
}

func (ct *CompanyTools) handlePaymentRecordGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "payment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	record, err := ct.bc.GetB2BPaymentRecord(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B payment record %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"payment_record": record})
}

func (ct *CompanyTools) handlePaymentRecordTransactions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "payment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	txns, err := ct.bc.ListB2BPaymentTransactions(ctx, id)
	if err != nil {
		return shared.ToolError("failed to list transactions for B2B payment %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"payment_id": id, "total": len(txns), "transactions": txns})
}

func (ct *CompanyTools) handlePaymentRecordOperations(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "payment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	ops, err := ct.bc.GetB2BPaymentOperations(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get operations for B2B payment %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"payment_id": id, "operations": ops})
}

func offlinePaymentPayloadFromArgs(args map[string]any) (bigcommerce.B2BOfflinePaymentCreate, error) {
	payload := bigcommerce.B2BOfflinePaymentCreate{}
	if raw, ok := args["line_items_json"].(string); ok && strings.TrimSpace(raw) != "" {
		var items []bigcommerce.B2BOfflinePaymentLineItem
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			return payload, fmt.Errorf("invalid line_items_json: %v", err)
		}
		payload.LineItems = items
	}
	if v, ok := args["currency"].(string); ok {
		payload.Currency = v
	}
	if v, ok := args["memo"].(string); ok && v != "" {
		payload.Details = &bigcommerce.B2BOfflinePaymentDetails{Memo: v}
	}
	if v, ok := args["external_id"].(string); ok {
		payload.ExternalID = v
	}
	if v, ok := args["customer_id"].(string); ok {
		payload.CustomerID = v
	}
	if v, ok := args["external_customer_id"].(string); ok {
		payload.ExternalCustomerID = v
	}
	if v, ok := args["payer_name"].(string); ok {
		payload.PayerName = v
	}
	if v, ok := args["payer_customer_id"].(string); ok {
		payload.PayerCustomerID = v
	}
	if v, ok := args["processing_status"].(string); ok {
		payload.ProcessingStatus = v
	}
	return payload, nil
}

func (ct *CompanyTools) handlePaymentRecordCreateOffline(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	payload, err := offlinePaymentPayloadFromArgs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(payload.LineItems) == 0 {
		return shared.ToolError("line_items_json is required (a JSON array of {invoiceId, amount} objects)"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_offline_payment",
			"payload": payload,
			"message": "Will log this offline payment against the listed invoice(s). Pass confirmed=true.",
		})
	}

	result, err := ct.bc.CreateB2BOfflinePayment(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create B2B offline payment: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "payment_record": result})
}

func (ct *CompanyTools) handlePaymentRecordUpdateOffline(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "payment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload, err := offlinePaymentPayloadFromArgs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "update_b2b_offline_payment",
			"payment_id": id,
			"payload":    payload,
			"message":    fmt.Sprintf("Will apply these fields to offline payment %d. Pass confirmed=true.", id),
		})
	}

	result, err := ct.bc.UpdateB2BOfflinePayment(ctx, id, payload)
	if err != nil {
		return shared.ToolError("failed to update B2B offline payment %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "payment_id": id, "payment_record": result})
}

func (ct *CompanyTools) handlePaymentRecordPerformOperation(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "payment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	opCode, _ := args["operation_code"].(string)
	if strings.TrimSpace(opCode) == "" {
		return shared.ToolError("operation_code is required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":         "preview",
			"action":         "perform_b2b_payment_operation",
			"payment_id":     id,
			"operation_code": opCode,
			"message":        fmt.Sprintf("Will perform operation %q on payment %d. This may be irreversible (e.g. void/refund). Pass confirmed=true.", opCode, id),
		})
	}

	result, err := ct.bc.PerformB2BPaymentOperation(ctx, id, opCode)
	if err != nil {
		return shared.ToolError("failed to perform operation %q on B2B payment %d: %v", opCode, id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "performed", "payment_id": id, "operation_code": opCode, "result": result})
}

func (ct *CompanyTools) handlePaymentRecordUpdateProcessingStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "payment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	status, err := shared.ReadPositiveInt(args, "processing_status")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if status < 1 || status > 4 {
		return shared.ToolError("processing_status must be 1, 2, 3, or 4"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":            "preview",
			"action":            "update_b2b_payment_processing_status",
			"payment_id":        id,
			"processing_status": status,
			"message":           fmt.Sprintf("Will set payment %d's processing status to %d. Pass confirmed=true.", id, status),
		})
	}

	result, err := ct.bc.UpdateB2BPaymentProcessingStatus(ctx, id, status)
	if err != nil {
		return shared.ToolError("failed to update processing status for B2B payment %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "payment_id": id, "result": result})
}

func (ct *CompanyTools) handlePaymentRecordDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "payment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "delete_b2b_payment_record",
			"payment_id": id,
			"message":    fmt.Sprintf("Will permanently delete payment record %d. This does not reverse the underlying transaction. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.DeleteB2BPayment(ctx, id); err != nil {
		return shared.ToolError("failed to delete B2B payment %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "payment_id": id})
}
