package bigcommerce

import (
	"context"
	"fmt"
)

// This file covers the "Invoice Management > Payments" resource group:
// payment records (money logged/received against invoices), distinct from
// the "Payments" group in b2b_payments.go (payment METHOD definitions and
// per-company enablement). Confusingly, both groups' list endpoints share the
// path "/payments" — the deciding factor is the base URL: this group lives at
// the /ip base (like invoices/receipts), while payment methods use the
// standard base. See docs/B2B.md and FOLLOW-UPS.md for details.

// B2BOfflinePaymentLineItem allocates part of an offline payment to a
// specific invoice.
type B2BOfflinePaymentLineItem struct {
	InvoiceID int    `json:"invoiceId"`
	Amount    string `json:"amount"`
}

// B2BOfflinePaymentDetails holds free-text metadata for an offline payment.
type B2BOfflinePaymentDetails struct {
	Memo string `json:"memo,omitempty"`
}

// B2BOfflinePaymentCreate is the request body for POST /payments/offline and
// PUT /payments/offline/{id} (same shape for both). ProcessingStatus:
// "1"=Awaiting Processing, "2"=Processing, "3"=Completed (default),
// "4"=Refunded.
type B2BOfflinePaymentCreate struct {
	LineItems          []B2BOfflinePaymentLineItem `json:"lineItems,omitempty"`
	Currency           string                      `json:"currency,omitempty"`
	Details            *B2BOfflinePaymentDetails   `json:"details,omitempty"`
	ExternalID         string                      `json:"externalId,omitempty"`
	CustomerID         string                      `json:"customerId,omitempty"`
	ExternalCustomerID string                      `json:"externalCustomerId,omitempty"`
	PayerName          string                      `json:"payerName,omitempty"`
	PayerCustomerID    string                      `json:"payerCustomerId,omitempty"`
	ProcessingStatus   string                      `json:"processingStatus,omitempty"`
}

// Payment record response bodies are deeply nested (module data, fees, line
// items linking to invoices) and vary by payment module, so read endpoints
// are passed through as generic maps.

// ListB2BPaymentRecords returns payment records (money logged against
// invoices), optionally filtered/sorted via params.
func (c *B2BClient) ListB2BPaymentRecords(ctx context.Context, params string) ([]map[string]any, error) {
	path := "ip/payments"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B payment records: %w", err)
	}
	return unmarshalMapSlice(raw, "payment record")
}

// GetB2BPaymentRecord fetches a single payment record's detail.
func (c *B2BClient) GetB2BPaymentRecord(ctx context.Context, paymentID int) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("ip/payments/%d", paymentID))
	if err != nil {
		return nil, fmt.Errorf("get B2B payment record %d: %w", paymentID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B payment record"); err != nil {
		return nil, err
	}
	return out, nil
}

// ListB2BPaymentTransactions returns the transaction history for a payment.
func (c *B2BClient) ListB2BPaymentTransactions(ctx context.Context, paymentID int) ([]map[string]any, error) {
	raw, err := c.B2BGetAll(ctx, fmt.Sprintf("ip/payments/%d/transactions", paymentID))
	if err != nil {
		return nil, fmt.Errorf("list transactions for B2B payment %d: %w", paymentID, err)
	}
	return unmarshalMapSlice(raw, "payment transaction")
}

// GetB2BPaymentOperations returns the operations currently allowed on a
// payment (the valid operationCode values for PerformB2BPaymentOperation).
func (c *B2BClient) GetB2BPaymentOperations(ctx context.Context, paymentID int) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("ip/payments/%d/operations", paymentID))
	if err != nil {
		return nil, fmt.Errorf("get operations for B2B payment %d: %w", paymentID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B payment operations"); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateB2BOfflinePayment logs a new offline payment (e.g. check, wire)
// against one or more invoices.
func (c *B2BClient) CreateB2BOfflinePayment(ctx context.Context, payload B2BOfflinePaymentCreate) (map[string]any, error) {
	body, err := c.B2BPost(ctx, "ip/payments/offline", payload)
	if err != nil {
		return nil, fmt.Errorf("create B2B offline payment: %w", err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "create B2B offline payment"); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateB2BOfflinePayment updates an existing offline payment record.
func (c *B2BClient) UpdateB2BOfflinePayment(ctx context.Context, paymentID int, payload B2BOfflinePaymentCreate) (map[string]any, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("ip/payments/offline/%d", paymentID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B offline payment %d: %w", paymentID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "update B2B offline payment"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}

// PerformB2BPaymentOperation performs a lifecycle operation (e.g. void,
// refund — see GetB2BPaymentOperations for the codes valid on this specific
// payment) on a payment.
func (c *B2BClient) PerformB2BPaymentOperation(ctx context.Context, paymentID int, operationCode string) (map[string]any, error) {
	body := map[string]string{"operationCode": operationCode}
	respBody, err := c.B2BPost(ctx, fmt.Sprintf("ip/payments/%d/operations", paymentID), body)
	if err != nil {
		return nil, fmt.Errorf("perform operation %q on B2B payment %d: %w", operationCode, paymentID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "perform B2B payment operation"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}

// UpdateB2BPaymentProcessingStatus sets a payment's processing status
// directly (1=Awaiting Processing, 2=Processing, 3=Completed, 4=Refunded).
func (c *B2BClient) UpdateB2BPaymentProcessingStatus(ctx context.Context, paymentID int, processingStatus int) (map[string]any, error) {
	body := map[string]int{"processingStatus": processingStatus}
	respBody, err := c.B2BPut(ctx, fmt.Sprintf("ip/payments/%d/processing-status", paymentID), body)
	if err != nil {
		return nil, fmt.Errorf("update processing status for B2B payment %d: %w", paymentID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "update B2B payment processing status"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}

// DeleteB2BPayment permanently deletes a payment record.
func (c *B2BClient) DeleteB2BPayment(ctx context.Context, paymentID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("ip/payments/%d", paymentID))
	if err != nil {
		return fmt.Errorf("delete B2B payment %d: %w", paymentID, err)
	}
	return nil
}
