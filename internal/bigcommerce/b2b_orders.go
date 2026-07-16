package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// B2BOrderUpdate is the request body for PUT /orders/{bcOrderId}: the B2B
// purchase-order number and/or extra fields.
type B2BOrderUpdate struct {
	PONumber    string          `json:"poNumber,omitempty"`
	ExtraFields []B2BExtraField `json:"extraFields,omitempty"`
}

// GetB2BOrder returns the B2B view of an order (PO number, company linkage,
// extra fields) by its BigCommerce order ID. The response shape is passed
// through as a generic object.
func (c *B2BClient) GetB2BOrder(ctx context.Context, bcOrderID int) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("orders/%d", bcOrderID))
	if err != nil {
		return nil, fmt.Errorf("get B2B order %d: %w", bcOrderID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B order"); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateB2BOrder sets the PO number and/or extra fields on an order by its
// BigCommerce order ID.
func (c *B2BClient) UpdateB2BOrder(ctx context.Context, bcOrderID int, payload B2BOrderUpdate) (map[string]any, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("orders/%d", bcOrderID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B order %d: %w", bcOrderID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "update B2B order"); err != nil {
		// Some responses are minimal; treat a parse miss as success.
		return map[string]any{}, nil //nolint:nilerr // response body shape varies
	}
	return out, nil
}

// AssignCustomerOrdersToCompany associates a buyer's pre-existing BigCommerce
// orders with their Company account (PUT /customers/{customerId}/orders/b2b).
// customerID is the BigCommerce customer ID, not the B2B user ID.
func (c *B2BClient) AssignCustomerOrdersToCompany(ctx context.Context, customerID int) error {
	_, err := c.B2BPut(ctx, fmt.Sprintf("customers/%d/orders/b2b", customerID), nil)
	if err != nil {
		return fmt.Errorf("assign customer %d orders to company: %w", customerID, err)
	}
	return nil
}

// ReassignOrdersToCompany reassigns all of a customer's orders to a different
// company by BigCommerce customer group ID
// (PUT /customers/{customerId}/orders/company). Only supported on stores using
// legacy Dependent Companies behavior.
func (c *B2BClient) ReassignOrdersToCompany(ctx context.Context, customerID, bcGroupID int) error {
	body := map[string]int{"bcGroupId": bcGroupID}
	_, err := c.B2BPut(ctx, fmt.Sprintf("customers/%d/orders/company", customerID), body)
	if err != nil {
		return fmt.Errorf("reassign customer %d orders to group %d: %w", customerID, bcGroupID, err)
	}
	return nil
}

// ListB2BOrderExtraFields returns the extra-field definitions configured for
// orders. Optional params support offset/limit pagination.
func (c *B2BClient) ListB2BOrderExtraFields(ctx context.Context, params string) ([]B2BExtraFieldDef, error) {
	path := "orders/extra-fields"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B order extra fields: %w", err)
	}
	out := make([]B2BExtraFieldDef, 0, len(raw))
	for _, r := range raw {
		var f B2BExtraFieldDef
		if err := json.Unmarshal(r, &f); err != nil {
			return nil, fmt.Errorf("unmarshal B2B order extra field: %w", err)
		}
		out = append(out, f)
	}
	return out, nil
}
