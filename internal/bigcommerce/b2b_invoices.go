package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// Invoice and receipt response bodies are deeply nested and vary by store
// configuration (custom fields, cost lines, tax breakdowns). Rather than
// modeling every field, these read-only endpoints are passed through as
// generic maps so callers see whatever BigCommerce returns.
//
// Unlike the rest of the B2B Management API, the Invoice Management group
// (invoices, receipts, receipt-lines) is served from a distinct base path —
// https://api-b2b.bigcommerce.com/api/v3/io/ip — rather than the standard
// .../io base used by companies/users/quotes/orders/channels/roles. All paths
// below are therefore prefixed with "ip/".

// ListB2BInvoices returns invoices matching optional query params (e.g.
// offset/limit/orderBy/sortBy/searchBy/q/status/customerId).
func (c *B2BClient) ListB2BInvoices(ctx context.Context, params string) ([]map[string]any, error) {
	path := "ip/invoices"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B invoices: %w", err)
	}
	return unmarshalMapSlice(raw, "invoice")
}

// GetB2BInvoice fetches a single invoice by its (string) invoice ID.
func (c *B2BClient) GetB2BInvoice(ctx context.Context, invoiceID string) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("ip/invoices/%s", invoiceID))
	if err != nil {
		return nil, fmt.Errorf("get B2B invoice %s: %w", invoiceID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B invoice"); err != nil {
		return nil, err
	}
	return out, nil
}

// DownloadB2BInvoicePDF returns the download info (e.g. a signed URL) for an
// invoice's PDF.
func (c *B2BClient) DownloadB2BInvoicePDF(ctx context.Context, invoiceID string) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("ip/invoices/%s/download-pdf", invoiceID))
	if err != nil {
		return nil, fmt.Errorf("download B2B invoice %s pdf: %w", invoiceID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "download B2B invoice pdf"); err != nil {
		return nil, err
	}
	return out, nil
}

// ListB2BInvoiceExtraFields returns the extra-field definitions configured for
// invoices.
func (c *B2BClient) ListB2BInvoiceExtraFields(ctx context.Context, params string) ([]B2BExtraFieldDef, error) {
	path := "ip/invoices/extra-fields"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B invoice extra fields: %w", err)
	}
	out := make([]B2BExtraFieldDef, 0, len(raw))
	for _, r := range raw {
		var f B2BExtraFieldDef
		if err := json.Unmarshal(r, &f); err != nil {
			return nil, fmt.Errorf("unmarshal B2B invoice extra field: %w", err)
		}
		out = append(out, f)
	}
	return out, nil
}

// ---- Receipts ----

// ListB2BReceipts returns receipts matching optional query params.
func (c *B2BClient) ListB2BReceipts(ctx context.Context, params string) ([]map[string]any, error) {
	path := "ip/receipts"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B receipts: %w", err)
	}
	return unmarshalMapSlice(raw, "receipt")
}

// GetB2BReceipt fetches a single receipt by its (string) receipt ID.
func (c *B2BClient) GetB2BReceipt(ctx context.Context, receiptID string) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("ip/receipts/%s", receiptID))
	if err != nil {
		return nil, fmt.Errorf("get B2B receipt %s: %w", receiptID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B receipt"); err != nil {
		return nil, err
	}
	return out, nil
}

// ListB2BReceiptLines returns receipt line items across all receipts, matching
// optional query params (e.g. paymentStatus[], searchBy, q).
func (c *B2BClient) ListB2BReceiptLines(ctx context.Context, params string) ([]map[string]any, error) {
	path := "ip/receipt-lines"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B receipt lines: %w", err)
	}
	return unmarshalMapSlice(raw, "receipt line")
}

// ListB2BLinesOfReceipt returns the line items belonging to a single receipt.
func (c *B2BClient) ListB2BLinesOfReceipt(ctx context.Context, receiptID, params string) ([]map[string]any, error) {
	path := fmt.Sprintf("ip/receipts/%s/lines", receiptID)
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list lines of B2B receipt %s: %w", receiptID, err)
	}
	return unmarshalMapSlice(raw, "receipt line")
}

// GetB2BReceiptLine fetches a single line item on a receipt.
func (c *B2BClient) GetB2BReceiptLine(ctx context.Context, receiptID, lineID string) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("ip/receipts/%s/lines/%s", receiptID, lineID))
	if err != nil {
		return nil, fmt.Errorf("get B2B receipt %s line %s: %w", receiptID, lineID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B receipt line"); err != nil {
		return nil, err
	}
	return out, nil
}

// unmarshalMapSlice unmarshals a slice of raw JSON objects into
// []map[string]any, used by read-only list endpoints whose item shape is too
// variable (or too deeply nested) to warrant a fully typed struct.
func unmarshalMapSlice(raw []json.RawMessage, label string) ([]map[string]any, error) {
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		m := map[string]any{}
		if err := json.Unmarshal(r, &m); err != nil {
			return nil, fmt.Errorf("unmarshal B2B %s: %w", label, err)
		}
		out = append(out, m)
	}
	return out, nil
}
