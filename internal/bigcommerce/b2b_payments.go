package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// B2BPaymentMethod is a store-wide payment method definition, as returned by
// GET /payments.
type B2BPaymentMethod struct {
	ID           int    `json:"id"`
	PaymentCode  string `json:"paymentCode"`
	PaymentTitle string `json:"paymentTitle"`
}

// B2BCompanyPaymentMethod is a payment method's availability for a specific
// company, as returned by GET /companies/{companyId}/payments. Field names
// differ from B2BPaymentMethod (paymentId/code vs id/paymentCode) — this
// matches the live API response, not just the documented schema.
type B2BCompanyPaymentMethod struct {
	PaymentID    int    `json:"paymentId"`
	Code         string `json:"code"`
	PaymentTitle string `json:"paymentTitle"`
	IsEnabled    bool   `json:"isEnabled"`
}

// B2BCompanyCredit is a company's store credit / purchase-on-credit settings.
type B2BCompanyCredit struct {
	CreditEnabled   bool     `json:"creditEnabled"`
	CreditCurrency  string   `json:"creditCurrency,omitempty"`
	AvailableCredit *float64 `json:"availableCredit,omitempty"`
	LimitPurchases  bool     `json:"limitPurchases"`
	CreditHold      bool     `json:"creditHold"`
}

// B2BPaymentTerms is a company's net-terms (payment-on-terms) configuration.
// PaymentTerms is documented as a string enum ("0","5","15","30","45","60")
// but observed live as a number — flexString tolerates either.
type B2BPaymentTerms struct {
	IsEnabled    bool       `json:"isEnabled"`
	PaymentTerms flexString `json:"paymentTerms"`
}

// ListB2BPaymentMethods returns the store's payment method definitions.
func (c *B2BClient) ListB2BPaymentMethods(ctx context.Context) ([]B2BPaymentMethod, error) {
	raw, err := c.B2BGetAll(ctx, "payments")
	if err != nil {
		return nil, fmt.Errorf("list B2B payment methods: %w", err)
	}
	out := make([]B2BPaymentMethod, 0, len(raw))
	for _, r := range raw {
		var m B2BPaymentMethod
		if err := json.Unmarshal(r, &m); err != nil {
			return nil, fmt.Errorf("unmarshal B2B payment method: %w", err)
		}
		out = append(out, m)
	}
	return out, nil
}

// ListB2BCompanyPaymentMethods returns payment methods and their
// enabled/disabled state for a specific company.
func (c *B2BClient) ListB2BCompanyPaymentMethods(ctx context.Context, companyID int) ([]B2BCompanyPaymentMethod, error) {
	raw, err := c.B2BGetAll(ctx, fmt.Sprintf("companies/%d/payments", companyID))
	if err != nil {
		return nil, fmt.Errorf("list B2B company %d payment methods: %w", companyID, err)
	}
	out := make([]B2BCompanyPaymentMethod, 0, len(raw))
	for _, r := range raw {
		var m B2BCompanyPaymentMethod
		if err := json.Unmarshal(r, &m); err != nil {
			return nil, fmt.Errorf("unmarshal B2B company payment method: %w", err)
		}
		out = append(out, m)
	}
	return out, nil
}

// ListB2BActivePaymentMethods returns currently-enabled payment methods across
// all companies (optionally filtered to one via a companyId param). If a
// method is enabled on multiple companies, each appears as a separate row.
func (c *B2BClient) ListB2BActivePaymentMethods(ctx context.Context, params string) ([]map[string]any, error) {
	path := "company-payment-methods"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B active payment methods: %w", err)
	}
	return unmarshalMapSlice(raw, "active payment method")
}

// GetB2BCompanyCredit returns a company's credit settings. Fails if the
// Company credit feature is disabled on the store.
func (c *B2BClient) GetB2BCompanyCredit(ctx context.Context, companyID int) (*B2BCompanyCredit, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("companies/%d/credit", companyID))
	if err != nil {
		return nil, fmt.Errorf("get B2B company %d credit: %w", companyID, err)
	}
	var out B2BCompanyCredit
	if err := b2bUnmarshalSingle(body, &out, "get B2B company credit"); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetB2BCompanyPaymentTerms returns a company's net-terms configuration. If
// payment on terms is disabled for the company, paymentTerms reflects the
// store-level default.
func (c *B2BClient) GetB2BCompanyPaymentTerms(ctx context.Context, companyID int) (*B2BPaymentTerms, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("companies/%d/payment-terms", companyID))
	if err != nil {
		return nil, fmt.Errorf("get B2B company %d payment terms: %w", companyID, err)
	}
	var out B2BPaymentTerms
	if err := b2bUnmarshalSingle(body, &out, "get B2B company payment terms"); err != nil {
		return nil, err
	}
	return &out, nil
}

// B2BCompanyPaymentMethodUpdate is one entry in the PUT /companies/{id}/payments
// request body: enables/disables a payment method for the company.
type B2BCompanyPaymentMethodUpdate struct {
	Code      string `json:"code"`
	IsEnabled bool   `json:"isEnabled"`
}

// UpdateB2BCompanyPaymentMethods enables/disables payment methods for a
// company. Only the methods listed in updates are affected.
func (c *B2BClient) UpdateB2BCompanyPaymentMethods(ctx context.Context, companyID int, updates []B2BCompanyPaymentMethodUpdate) error {
	body := map[string]any{"payments": updates}
	_, err := c.B2BPut(ctx, fmt.Sprintf("companies/%d/payments", companyID), body)
	if err != nil {
		return fmt.Errorf("update B2B company %d payment methods: %w", companyID, err)
	}
	return nil
}

// UpdateB2BCompanyCredit updates a company's credit settings. Fails if the
// store's Company Credit feature is disabled. The request body is the same
// shape as B2BCompanyCredit (the GET response).
func (c *B2BClient) UpdateB2BCompanyCredit(ctx context.Context, companyID int, payload B2BCompanyCredit) (*B2BCompanyCredit, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("companies/%d/credit", companyID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B company %d credit: %w", companyID, err)
	}
	var out B2BCompanyCredit
	if err := b2bUnmarshalSingle(body, &out, "update B2B company credit"); err != nil {
		return &payload, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return &out, nil
}

// UpdateB2BCompanyPaymentTerms updates a company's net-terms configuration.
// paymentTerms must be one of "0","5","15","30","45","60" and is ignored by
// BC (defaults to the store-level value) when isEnabled is false.
func (c *B2BClient) UpdateB2BCompanyPaymentTerms(ctx context.Context, companyID int, isEnabled bool, paymentTerms string) (*B2BPaymentTerms, error) {
	body := map[string]any{"isEnabled": isEnabled, "paymentTerms": paymentTerms}
	respBody, err := c.B2BPut(ctx, fmt.Sprintf("companies/%d/payment-terms", companyID), body)
	if err != nil {
		return nil, fmt.Errorf("update B2B company %d payment terms: %w", companyID, err)
	}
	var out B2BPaymentTerms
	if err := b2bUnmarshalSingle(respBody, &out, "update B2B company payment terms"); err != nil {
		return &B2BPaymentTerms{IsEnabled: isEnabled, PaymentTerms: flexString(paymentTerms)}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return &out, nil
}
