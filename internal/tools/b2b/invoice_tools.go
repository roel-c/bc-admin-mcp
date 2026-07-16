package b2b

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ============================================================
// Invoice tools
// ============================================================

func (ct *CompanyTools) registerInvoiceTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/invoices/list",
		Tier:    middleware.TierR0,
		Summary: "List B2B invoices with filters and sorting",
		Tool: mcp.NewTool("b2b_invoices_list",
			mcp.WithDescription("List B2B Edition invoices. Filter with search_by + q (search fields: invoiceNumber, type, orderNumber, purchaseOrderNumber, customerId, externalCustomerId); sort with sort_by + order_by."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
			mcp.WithString("search_by", mcp.Description("Field to search: invoiceNumber, type, orderNumber, purchaseOrderNumber, customerId, externalCustomerId.")),
			mcp.WithString("q", mcp.Description("Search term for search_by (or all supported fields if search_by is omitted).")),
			mcp.WithString("sort_by", mcp.Description("Sort field: invoiceNumber, createdAt, customerId, externalCustomerId, dueDate, updatedAt, isPendingPayment, openBalance, originalBalance, status.")),
			mcp.WithString("order_by", mcp.Description("ASC or DESC.")),
			mcp.WithString("status", mcp.Description("Filter by status code: 0, 1, or 2.")),
			mcp.WithNumber("customer_id", mcp.Description("Filter by BigCommerce customer ID.")),
		),
		Handler: ct.handleInvoiceList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/invoices/get",
		Tier:    middleware.TierR0,
		Summary: "Get invoice detail by invoice ID",
		Tool: mcp.NewTool("b2b_invoices_get",
			mcp.WithDescription("Get a single B2B invoice's full detail (line items, balance, billing address) by invoice ID."),
			mcp.WithString("invoice_id", mcp.Description("Invoice ID"), mcp.Required()),
		),
		Handler: ct.handleInvoiceGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/invoices/download_pdf",
		Tier:    middleware.TierR0,
		Summary: "Get a download link for an invoice's PDF",
		Tool: mcp.NewTool("b2b_invoices_download_pdf",
			mcp.WithDescription("Get the PDF download info for a B2B invoice."),
			mcp.WithString("invoice_id", mcp.Description("Invoice ID"), mcp.Required()),
		),
		Handler: ct.handleInvoiceDownloadPDF,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/invoices/extra_fields",
		Tier:    middleware.TierR0,
		Summary: "List extra-field definitions configured for invoices",
		Tool: mcp.NewTool("b2b_invoices_extra_fields",
			mcp.WithDescription("List the extra-field (custom field) definitions configured for B2B Edition invoices."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleInvoiceExtraFields,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/invoices/create",
		Tier:    middleware.TierR2,
		Summary: "Create an invoice from a raw JSON body",
		Tool: mcp.NewTool("b2b_invoices_create",
			mcp.WithDescription("Create a B2B invoice from invoice_json, matching the documented create schema (invoiceNumber, dueDate, status [0=open,1=partially paid,2=completed], orderNumber, purchaseOrderNumber, originalBalance, openBalance, details, customerId [B2B company ID], channelId, etc.). Use b2b/invoices/get on an existing invoice to see an example shape. Preview → confirm."),
			mcp.WithString("invoice_json", mcp.Description("JSON object matching the invoice create body."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create.")),
		),
		Handler: ct.handleInvoiceCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/invoices/create_from_order",
		Tier:    middleware.TierR2,
		Summary: "Generate an invoice from an existing BigCommerce order",
		Tool: mcp.NewTool("b2b_invoices_create_from_order",
			mcp.WithDescription("Generate a B2B invoice using an existing BigCommerce order's data. Internally resolves the BC order ID to B2B Edition's own order ID first. Preview → confirm."),
			mcp.WithNumber("order_id", mcp.Description("BigCommerce order ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create.")),
		),
		Handler: ct.handleInvoiceCreateFromOrder,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/invoices/update",
		Tier:    middleware.TierR2,
		Summary: "Update an invoice from a raw JSON body",
		Tool: mcp.NewTool("b2b_invoices_update",
			mcp.WithDescription("Update a B2B invoice from invoice_json. No field is required — send only what changes. IMPORTANT: updating `details` completely replaces the existing value rather than merging it. Preview → confirm."),
			mcp.WithString("invoice_id", mcp.Description("Invoice ID"), mcp.Required()),
			mcp.WithString("invoice_json", mcp.Description("JSON object with the fields to update."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleInvoiceUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/invoices/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete an invoice",
		Tool: mcp.NewTool("b2b_invoices_delete",
			mcp.WithDescription("Permanently delete a B2B invoice. Preview → confirm."),
			mcp.WithString("invoice_id", mcp.Description("Invoice ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete permanently.")),
		),
		Handler: ct.handleInvoiceDelete,
	})

	// ---- Receipts ----

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/receipts/list",
		Tier:    middleware.TierR0,
		Summary: "List B2B payment receipts",
		Tool: mcp.NewTool("b2b_receipts_list",
			mcp.WithDescription("List B2B Edition payment receipts."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
			mcp.WithString("q", mcp.Description("Search term.")),
			mcp.WithString("search_by", mcp.Description("Field to search.")),
			mcp.WithString("sort_by", mcp.Description("Sort field (default createdAt).")),
			mcp.WithString("order_by", mcp.Description("ASC or DESC (default DESC).")),
		),
		Handler: ct.handleReceiptList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/receipts/get",
		Tier:    middleware.TierR0,
		Summary: "Get a payment receipt by ID",
		Tool: mcp.NewTool("b2b_receipts_get",
			mcp.WithDescription("Get a single B2B payment receipt by receipt ID."),
			mcp.WithString("receipt_id", mcp.Description("Receipt ID"), mcp.Required()),
		),
		Handler: ct.handleReceiptGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/receipts/lines/list_all",
		Tier:    middleware.TierR0,
		Summary: "List receipt line items across all receipts",
		Tool: mcp.NewTool("b2b_receipts_lines_list_all",
			mcp.WithDescription("List receipt line items across all B2B receipts (not scoped to one receipt)."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
			mcp.WithString("q", mcp.Description("Search term.")),
			mcp.WithString("search_by", mcp.Description("Field to search.")),
			mcp.WithString("sort_by", mcp.Description("Sort field.")),
			mcp.WithString("order_by", mcp.Description("ASC or DESC.")),
		),
		Handler: ct.handleReceiptLinesListAll,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/receipts/lines/list_for_receipt",
		Tier:    middleware.TierR0,
		Summary: "List line items belonging to one receipt",
		Tool: mcp.NewTool("b2b_receipts_lines_list_for_receipt",
			mcp.WithDescription("List the line items on a single B2B receipt."),
			mcp.WithString("receipt_id", mcp.Description("Receipt ID"), mcp.Required()),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleReceiptLinesListForReceipt,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/receipts/lines/get",
		Tier:    middleware.TierR0,
		Summary: "Get a single receipt line item",
		Tool: mcp.NewTool("b2b_receipts_lines_get",
			mcp.WithDescription("Get a single line item on a B2B receipt."),
			mcp.WithString("receipt_id", mcp.Description("Receipt ID"), mcp.Required()),
			mcp.WithString("line_id", mcp.Description("Receipt line ID"), mcp.Required()),
		),
		Handler: ct.handleReceiptLineGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/receipts/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a receipt",
		Tool: mcp.NewTool("b2b_receipts_delete",
			mcp.WithDescription("Permanently delete a B2B payment receipt. Preview → confirm."),
			mcp.WithString("receipt_id", mcp.Description("Receipt ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete permanently.")),
		),
		Handler: ct.handleReceiptDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/receipts/lines/delete",
		Tier:    middleware.TierR2,
		Summary: "Permanently delete a single receipt line",
		Tool: mcp.NewTool("b2b_receipts_lines_delete",
			mcp.WithDescription("Permanently delete a single line from a B2B payment receipt. Preview → confirm."),
			mcp.WithString("receipt_id", mcp.Description("Receipt ID"), mcp.Required()),
			mcp.WithString("line_id", mcp.Description("Receipt line ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete permanently.")),
		),
		Handler: ct.handleReceiptLineDelete,
	})
}

// ---- invoice handlers ----

func invoiceListParams(args map[string]any) string {
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["search_by"].(string); ok && v != "" {
		params.Set("searchBy", v)
	}
	if v, ok := args["q"].(string); ok && v != "" {
		params.Set("q", v)
	}
	if v, ok := args["sort_by"].(string); ok && v != "" {
		params.Set("sortBy", v)
	}
	if v, ok := args["order_by"].(string); ok && v != "" {
		params.Set("orderBy", v)
	}
	if v, ok := args["status"].(string); ok && v != "" {
		params.Set("status", v)
	}
	if v, ok := args["customer_id"].(float64); ok && v > 0 {
		params.Set("customerId", fmt.Sprintf("%d", int(v)))
	}
	return params.Encode()
}

func (ct *CompanyTools) handleInvoiceList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	invoices, err := ct.bc.ListB2BInvoices(ctx, invoiceListParams(args))
	if err != nil {
		return shared.ToolError("failed to list B2B invoices: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(invoices), "invoices": invoices})
}

func (ct *CompanyTools) handleInvoiceGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, _ := args["invoice_id"].(string)
	if id == "" {
		return shared.ToolError("invoice_id is required"), nil
	}
	inv, err := ct.bc.GetB2BInvoice(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B invoice %s: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"invoice": inv})
}

func (ct *CompanyTools) handleInvoiceDownloadPDF(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, _ := args["invoice_id"].(string)
	if id == "" {
		return shared.ToolError("invoice_id is required"), nil
	}
	result, err := ct.bc.DownloadB2BInvoicePDF(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get PDF for B2B invoice %s: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"invoice_id": id, "download": result})
}

func (ct *CompanyTools) handleInvoiceExtraFields(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	defs, err := ct.bc.ListB2BInvoiceExtraFields(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B invoice extra fields: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(defs), "extra_fields": defs})
}

func parseInvoiceJSONBody(args map[string]any, key string) (map[string]any, error) {
	raw, ok := args[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%s is required (a JSON object)", key)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		return nil, fmt.Errorf("invalid %s: %v", key, err)
	}
	return body, nil
}

func (ct *CompanyTools) handleInvoiceCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	body, err := parseInvoiceJSONBody(args, "invoice_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_invoice",
			"payload": body,
			"message": "Will create this invoice. Pass confirmed=true.",
		})
	}

	invoice, err := ct.bc.CreateB2BInvoice(ctx, body)
	if err != nil {
		return shared.ToolError("failed to create B2B invoice: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "invoice": invoice})
}

// b2bOrderInternalID extracts B2B Edition's internal numeric order ID (the
// "id" field) from a GetB2BOrder response, which is distinct from the
// BigCommerce order ID ("bcOrderId").
func b2bOrderInternalID(order map[string]any) (int, error) {
	idVal, ok := order["id"]
	if !ok {
		return 0, fmt.Errorf("B2B order response missing internal id field")
	}
	switch v := idVal.(type) {
	case float64:
		return int(v), nil
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("B2B order internal id %q is not numeric", v)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("B2B order internal id has unexpected type %T", v)
	}
}

func (ct *CompanyTools) handleInvoiceCreateFromOrder(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "create_b2b_invoice_from_order",
			"order_id": orderID,
			"message":  fmt.Sprintf("Will generate an invoice from order %d. Pass confirmed=true.", orderID),
		})
	}

	// The Invoice Management API's create-from-order endpoint expects B2B
	// Edition's own internal order ID, not the BigCommerce order ID — the two
	// are different numbers (see GetB2BOrder's "id" vs "bcOrderId" fields).
	// Resolve it first so callers can keep using the familiar BC order ID.
	b2bOrder, err := ct.bc.GetB2BOrder(ctx, orderID)
	if err != nil {
		return shared.ToolError("failed to resolve B2B order for BC order %d: %v", orderID, err), nil
	}
	internalID, err := b2bOrderInternalID(b2bOrder)
	if err != nil {
		return shared.ToolError("order %d: %v", orderID, err), nil
	}

	invoice, err := ct.bc.CreateB2BInvoiceFromOrder(ctx, internalID)
	if err != nil {
		return shared.ToolError("failed to create B2B invoice from order %d: %v", orderID, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "order_id": orderID, "invoice": invoice})
}

func (ct *CompanyTools) handleInvoiceUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, _ := args["invoice_id"].(string)
	if id == "" {
		return shared.ToolError("invoice_id is required"), nil
	}
	body, err := parseInvoiceJSONBody(args, "invoice_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "update_b2b_invoice",
			"invoice_id": id,
			"payload":    body,
			"message":    fmt.Sprintf("Will apply these fields to invoice %s. Pass confirmed=true.", id),
		})
	}

	invoice, err := ct.bc.UpdateB2BInvoice(ctx, id, body)
	if err != nil {
		return shared.ToolError("failed to update B2B invoice %s: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "invoice_id": id, "invoice": invoice})
}

func (ct *CompanyTools) handleInvoiceDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, _ := args["invoice_id"].(string)
	if id == "" {
		return shared.ToolError("invoice_id is required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "delete_b2b_invoice",
			"invoice_id": id,
			"message":    fmt.Sprintf("Will permanently delete invoice %s. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.DeleteB2BInvoice(ctx, id); err != nil {
		return shared.ToolError("failed to delete B2B invoice %s: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "invoice_id": id})
}

// ---- receipt handlers ----

func receiptListParams(args map[string]any) string {
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["q"].(string); ok && v != "" {
		params.Set("q", v)
	}
	if v, ok := args["search_by"].(string); ok && v != "" {
		params.Set("searchBy", v)
	}
	if v, ok := args["sort_by"].(string); ok && v != "" {
		params.Set("sortBy", v)
	}
	if v, ok := args["order_by"].(string); ok && v != "" {
		params.Set("orderBy", v)
	}
	return params.Encode()
}

func (ct *CompanyTools) handleReceiptList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	receipts, err := ct.bc.ListB2BReceipts(ctx, receiptListParams(args))
	if err != nil {
		return shared.ToolError("failed to list B2B receipts: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(receipts), "receipts": receipts})
}

func (ct *CompanyTools) handleReceiptGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, _ := args["receipt_id"].(string)
	if id == "" {
		return shared.ToolError("receipt_id is required"), nil
	}
	r, err := ct.bc.GetB2BReceipt(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B receipt %s: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"receipt": r})
}

func (ct *CompanyTools) handleReceiptLinesListAll(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	lines, err := ct.bc.ListB2BReceiptLines(ctx, receiptListParams(args))
	if err != nil {
		return shared.ToolError("failed to list B2B receipt lines: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(lines), "lines": lines})
}

func (ct *CompanyTools) handleReceiptLinesListForReceipt(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, _ := args["receipt_id"].(string)
	if id == "" {
		return shared.ToolError("receipt_id is required"), nil
	}
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	lines, err := ct.bc.ListB2BLinesOfReceipt(ctx, id, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list lines for B2B receipt %s: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"receipt_id": id, "total": len(lines), "lines": lines})
}

func (ct *CompanyTools) handleReceiptLineGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	receiptID, _ := args["receipt_id"].(string)
	lineID, _ := args["line_id"].(string)
	if receiptID == "" || lineID == "" {
		return shared.ToolError("receipt_id and line_id are required"), nil
	}
	line, err := ct.bc.GetB2BReceiptLine(ctx, receiptID, lineID)
	if err != nil {
		return shared.ToolError("failed to get B2B receipt %s line %s: %v", receiptID, lineID, err), nil
	}
	return shared.ToolJSON(map[string]any{"line": line})
}

func (ct *CompanyTools) handleReceiptDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, _ := args["receipt_id"].(string)
	if id == "" {
		return shared.ToolError("receipt_id is required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "delete_b2b_receipt",
			"receipt_id": id,
			"message":    fmt.Sprintf("Will permanently delete receipt %s. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.DeleteB2BReceipt(ctx, id); err != nil {
		return shared.ToolError("failed to delete B2B receipt %s: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "receipt_id": id})
}

func (ct *CompanyTools) handleReceiptLineDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	receiptID, _ := args["receipt_id"].(string)
	lineID, _ := args["line_id"].(string)
	if receiptID == "" || lineID == "" {
		return shared.ToolError("receipt_id and line_id are required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "delete_b2b_receipt_line",
			"receipt_id": receiptID,
			"line_id":    lineID,
			"message":    fmt.Sprintf("Will permanently delete line %s from receipt %s. Pass confirmed=true.", lineID, receiptID),
		})
	}

	if err := ct.bc.DeleteB2BReceiptLine(ctx, receiptID, lineID); err != nil {
		return shared.ToolError("failed to delete line %s from B2B receipt %s: %v", lineID, receiptID, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "receipt_id": receiptID, "line_id": lineID})
}
