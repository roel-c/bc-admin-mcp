package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// Quote response bodies (list items, detail, PDF/shipping responses) are
// deeply nested and vary with store configuration (custom products, variant
// options, currency overrides). As with invoices, these are passed through as
// generic maps rather than fully typed structs.

// ListB2BQuotes returns quotes matching optional query params (offset, limit,
// sortBy, orderBy, q, quoteNumber, company, salesRep, status, quoteTitle,
// createdBy, min/maxCreated, min/maxModified, min/maxExpired, channelIds[]).
func (c *B2BClient) ListB2BQuotes(ctx context.Context, params string) ([]map[string]any, error) {
	path := "rfq"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B quotes: %w", err)
	}
	return unmarshalMapSlice(raw, "quote")
}

// GetB2BQuote fetches full detail for a single quote by its numeric quote ID.
func (c *B2BClient) GetB2BQuote(ctx context.Context, quoteID int) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("rfq/%d", quoteID))
	if err != nil {
		return nil, fmt.Errorf("get B2B quote %d: %w", quoteID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B quote"); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateB2BQuote creates a new quote from a raw JSON body matching the
// documented quoteData_POST schema. The quote is immediately visible to the
// assigned buyer unless allowCheckout=false is set in the body.
func (c *B2BClient) CreateB2BQuote(ctx context.Context, body map[string]any) (map[string]any, error) {
	respBody, err := c.B2BPost(ctx, "rfq", body)
	if err != nil {
		return nil, fmt.Errorf("create B2B quote: %w", err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "create B2B quote"); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateB2BQuote updates an existing quote from a raw JSON body (partial
// update — omitted fields are left unchanged, except line items: the full set
// to keep must be included, per the BigCommerce API contract).
func (c *B2BClient) UpdateB2BQuote(ctx context.Context, quoteID int, body map[string]any) (map[string]any, error) {
	respBody, err := c.B2BPut(ctx, fmt.Sprintf("rfq/%d", quoteID), body)
	if err != nil {
		return nil, fmt.Errorf("update B2B quote %d: %w", quoteID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "update B2B quote"); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteB2BQuote permanently deletes a quote. To hide a quote without
// deleting it, update its status to archived instead.
func (c *B2BClient) DeleteB2BQuote(ctx context.Context, quoteID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("rfq/%d", quoteID))
	if err != nil {
		return fmt.Errorf("delete B2B quote %d: %w", quoteID, err)
	}
	return nil
}

// GenerateB2BQuoteCheckout generates a cart URL and checkout URL for a quote.
// Only valid for quotes in status New(0), In Process(2), or Updated by
// Customer(3).
func (c *B2BClient) GenerateB2BQuoteCheckout(ctx context.Context, quoteID int) (map[string]any, error) {
	body, err := c.B2BPost(ctx, fmt.Sprintf("rfq/%d/checkout", quoteID), map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("generate checkout for B2B quote %d: %w", quoteID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "generate B2B quote checkout"); err != nil {
		return nil, err
	}
	return out, nil
}

// AssignB2BQuoteToOrder associates an existing BigCommerce order with a
// quote. Only valid for quotes in status New(0), In Process(2), or Updated by
// Customer(3).
func (c *B2BClient) AssignB2BQuoteToOrder(ctx context.Context, quoteID, orderID int) error {
	body := map[string]int{"orderId": orderID}
	_, err := c.B2BPost(ctx, fmt.Sprintf("rfq/%d/ordered", quoteID), body)
	if err != nil {
		return fmt.Errorf("assign B2B quote %d to order %d: %w", quoteID, orderID, err)
	}
	return nil
}

// ExportB2BQuotePDF creates a download link for a non-buyer-facing PDF of the
// quote (includes backend info like channel and line-item cost margins).
// currency, if non-nil, overrides the quote's currency/exchange rate for the
// exported PDF.
func (c *B2BClient) ExportB2BQuotePDF(ctx context.Context, quoteID int, currency map[string]any) (map[string]any, error) {
	body := map[string]any{}
	if currency != nil {
		body["currency"] = currency
	}
	respBody, err := c.B2BPost(ctx, fmt.Sprintf("rfq/%d/pdf-export", quoteID), body)
	if err != nil {
		return nil, fmt.Errorf("export B2B quote %d pdf: %w", quoteID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "export B2B quote pdf"); err != nil {
		return nil, err
	}
	return out, nil
}

// ListB2BQuoteShippingRates returns the static/real-time shipping rates
// available to a quote (NOT custom shipping methods — see
// ListB2BQuoteCustomShippingMethods for those). Note the API path is plural
// (/shipping-rates); the singular /shipping-rate path is for
// select/remove and returns 405 if confused with this one.
func (c *B2BClient) ListB2BQuoteShippingRates(ctx context.Context, quoteID int) ([]map[string]any, error) {
	raw, err := c.B2BGetAll(ctx, fmt.Sprintf("rfq/%d/shipping-rates", quoteID))
	if err != nil {
		return nil, fmt.Errorf("list shipping rates for B2B quote %d: %w", quoteID, err)
	}
	return unmarshalMapSlice(raw, "quote shipping rate")
}

// SelectB2BQuoteShippingRate assigns a shipping method to a quote. Provide
// either shippingMethodID (a static/real-time rate ID from
// ListB2BQuoteShippingRates or ListB2BQuoteCustomShippingMethods) OR both
// customName and customCost for an ad hoc custom method.
func (c *B2BClient) SelectB2BQuoteShippingRate(ctx context.Context, quoteID int, shippingMethodID, customName string, customCost float64, hasCustomCost bool) (map[string]any, error) {
	body := map[string]any{}
	if shippingMethodID != "" {
		body["shippingMethodId"] = shippingMethodID
	}
	if customName != "" {
		body["customShippingMethodName"] = customName
	}
	if hasCustomCost {
		body["customShippingMethodCost"] = customCost
	}
	respBody, err := c.B2BPut(ctx, fmt.Sprintf("rfq/%d/shipping-rate", quoteID), body)
	if err != nil {
		return nil, fmt.Errorf("select shipping rate for B2B quote %d: %w", quoteID, err)
	}
	// Unlike other quote write endpoints, this one does not return the updated
	// quote — `data` is an empty array on success. Tolerate that shape rather
	// than failing to parse it.
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "select B2B quote shipping rate"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body carries no useful detail
	}
	return out, nil
}

// RemoveB2BQuoteShippingRate clears the shipping method currently assigned to
// a quote.
func (c *B2BClient) RemoveB2BQuoteShippingRate(ctx context.Context, quoteID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("rfq/%d/shipping-rate", quoteID))
	if err != nil {
		return fmt.Errorf("remove shipping rate for B2B quote %d: %w", quoteID, err)
	}
	return nil
}

// ListB2BQuoteCustomShippingMethods returns the custom shipping methods
// enabled in the store's Quotes settings (store-wide, not quote-scoped).
func (c *B2BClient) ListB2BQuoteCustomShippingMethods(ctx context.Context) ([]map[string]any, error) {
	raw, err := c.B2BGetAll(ctx, "rfq/custom/shipping-methods")
	if err != nil {
		return nil, fmt.Errorf("list B2B quote custom shipping methods: %w", err)
	}
	return unmarshalMapSlice(raw, "quote custom shipping method")
}

// ListB2BQuoteExtraFields returns the extra-field definitions configured for
// quotes.
func (c *B2BClient) ListB2BQuoteExtraFields(ctx context.Context, params string) ([]B2BExtraFieldDef, error) {
	path := "rfq/extra-fields"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B quote extra fields: %w", err)
	}
	out := make([]B2BExtraFieldDef, 0, len(raw))
	for _, r := range raw {
		var f B2BExtraFieldDef
		if err := json.Unmarshal(r, &f); err != nil {
			return nil, fmt.Errorf("unmarshal B2B quote extra field: %w", err)
		}
		out = append(out, f)
	}
	return out, nil
}
