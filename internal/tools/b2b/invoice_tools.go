package b2b

import (
	"context"
	"fmt"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ============================================================
// Invoice tools (read-only)
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
