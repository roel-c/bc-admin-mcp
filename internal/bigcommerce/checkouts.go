package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// Checkout represents a BigCommerce checkout (GET /v3/checkouts/{id}).
// The checkout shares the same UUID as its originating cart. It extends the
// cart with billing address, shipping consignments, coupon codes, and totals.
type Checkout struct {
	ID                       string                `json:"id"`
	Cart                     *Cart                 `json:"cart,omitempty"`
	BillingAddress           *CheckoutAddress      `json:"billing_address,omitempty"`
	Consignments             []CheckoutConsignment `json:"consignments,omitempty"`
	Coupons                  []CheckoutCoupon      `json:"coupons,omitempty"`
	OrderID                  *int                  `json:"order_id,omitempty"`
	ShippingCostBeforeDiscount float64             `json:"shipping_cost_before_discount"`
	SubtotalExTax            float64               `json:"subtotal_ex_tax"`
	SubtotalIncTax           float64               `json:"subtotal_inc_tax"`
	SubtotalTax              float64               `json:"subtotal_tax"`
	GrandTotal               float64               `json:"grand_total"`
	ChannelID                int                   `json:"channel_id,omitempty"`
	CustomerMessage          string                `json:"customer_message,omitempty"`
	CreatedTime              string                `json:"created_time,omitempty"`
	UpdatedTime              string                `json:"updated_time,omitempty"`
}

// CheckoutAddress is a billing or shipping address on a checkout.
type CheckoutAddress struct {
	// BigCommerce returns checkout address IDs as strings (not ints), unlike
	// most other resources — using string here avoids an unmarshal error on
	// the SetBillingAddress / consignment responses.
	ID            string `json:"id,omitempty"`
	FirstName     string `json:"first_name,omitempty"`
	LastName      string `json:"last_name,omitempty"`
	Email         string `json:"email,omitempty"`
	Company       string `json:"company,omitempty"`
	Address1      string `json:"address1,omitempty"`
	Address2      string `json:"address2,omitempty"`
	City          string `json:"city,omitempty"`
	StateOrProvince string `json:"state_or_province,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	Country       string `json:"country,omitempty"`
	CountryCode   string `json:"country_code,omitempty"`
	Phone         string `json:"phone,omitempty"`
}

// CheckoutConsignment is a shipping group (address + line items + shipping option).
type CheckoutConsignment struct {
	ID                     string                  `json:"id,omitempty"`
	ShippingAddress        *CheckoutAddress        `json:"address,omitempty"`
	LineItemIDs            []map[string]any        `json:"line_items,omitempty"`
	SelectedShippingOption *CheckoutShippingOption `json:"selected_shipping_option,omitempty"`
	AvailableShippingOptions []CheckoutShippingOption `json:"available_shipping_options,omitempty"`
}

// CheckoutShippingOption represents one available shipping method on a consignment.
type CheckoutShippingOption struct {
	ID              string  `json:"id"`
	Type            string  `json:"type,omitempty"`
	Description     string  `json:"description,omitempty"`
	ImageURL        string  `json:"image_url,omitempty"`
	Cost            float64 `json:"cost"`
	TransitTime     string  `json:"transit_time,omitempty"`
	AdditionalDescription string `json:"additional_description,omitempty"`
}

// CheckoutCoupon is a coupon code applied to a checkout.
type CheckoutCoupon struct {
	ID             string  `json:"id,omitempty"`
	Code           string  `json:"code"`
	CouponType     string  `json:"coupon_type,omitempty"`
	DiscountedAmount float64 `json:"discounted_amount"`
}

// CheckoutAddressInput is the request body for setting/updating a billing address.
type CheckoutAddressInput struct {
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Email         string `json:"email,omitempty"`
	Company       string `json:"company,omitempty"`
	Address1      string `json:"address1"`
	Address2      string `json:"address2,omitempty"`
	City          string `json:"city"`
	StateOrProvince string `json:"state_or_province,omitempty"`
	StateOrProvinceCode string `json:"state_or_province_code,omitempty"`
	PostalCode    string `json:"postal_code,omitempty"`
	CountryCode   string `json:"country_code"`
	Phone         string `json:"phone,omitempty"`
}

// CheckoutConsignmentInput is the request body for adding a consignment.
type CheckoutConsignmentInput struct {
	Address   CheckoutAddressInput `json:"address"`
	LineItems []ConsignmentLineItem `json:"line_items"`
}

// ConsignmentLineItem maps a cart item ID to a quantity for a consignment.
type ConsignmentLineItem struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

// CheckoutConsignmentUpdate is the request body for updating a consignment
// (typically to select a shipping option).
type CheckoutConsignmentUpdate struct {
	ShippingOptionID string `json:"shipping_option_id,omitempty"`
	Address          *CheckoutAddressInput `json:"address,omitempty"`
	LineItems        []ConsignmentLineItem `json:"line_items,omitempty"`
}

// CheckoutOrderResult is the response from POST /v3/checkouts/{id}/orders.
type CheckoutOrderResult struct {
	ID int `json:"id"`
}

// ---- Client methods ----

func (c *Client) unmarshalCheckout(body []byte, op string) (*Checkout, error) {
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s: parse response: %w", op, err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("%s: response missing data", op)
	}
	var co Checkout
	if err := json.Unmarshal(resp.Data, &co); err != nil {
		return nil, fmt.Errorf("%s: unmarshal checkout: %w", op, err)
	}
	return &co, nil
}

// includeAvailableShippingOptions is the BigCommerce Checkout API's opt-in
// query param for returning consignments[].available_shipping_options.
// Without it, BC omits the field entirely (not an empty array) — the
// checkout otherwise looks correctly populated (address, totals, etc.) which
// makes the missing options easy to mistake for "no shipping methods
// configured" rather than "we didn't ask for them." Confirmed against
// BigCommerce's own headless-checkout tutorial, which appends this exact
// param to both the consignment-add and checkout-get calls.
const includeAvailableShippingOptions = "include=consignments.available_shipping_options"

// GetCheckout fetches the checkout for a cart UUID via GET /v3/checkouts/{id}.
func (c *Client) GetCheckout(ctx context.Context, checkoutID string) (*Checkout, error) {
	body, err := c.Get(ctx, fmt.Sprintf("checkouts/%s?%s", checkoutID, includeAvailableShippingOptions))
	if err != nil {
		return nil, fmt.Errorf("get checkout %s: %w", checkoutID, err)
	}
	return c.unmarshalCheckout(body, "get checkout")
}

// ApplyCoupon applies a coupon code to a checkout via POST /v3/checkouts/{id}/coupons.
func (c *Client) ApplyCoupon(ctx context.Context, checkoutID, code string) (*Checkout, error) {
	body, err := c.Post(ctx, fmt.Sprintf("checkouts/%s/coupons", checkoutID), map[string]string{"coupon_code": code})
	if err != nil {
		return nil, fmt.Errorf("apply coupon %q to checkout %s: %w", code, checkoutID, err)
	}
	return c.unmarshalCheckout(body, "apply coupon")
}

// RemoveCoupon removes a coupon code from a checkout via DELETE /v3/checkouts/{id}/coupons/{code}.
func (c *Client) RemoveCoupon(ctx context.Context, checkoutID, code string) (*Checkout, error) {
	body, err := c.Delete(ctx, fmt.Sprintf("checkouts/%s/coupons/%s", checkoutID, code))
	if err != nil {
		return nil, fmt.Errorf("remove coupon %q from checkout %s: %w", code, checkoutID, err)
	}
	if len(body) == 0 {
		return c.GetCheckout(ctx, checkoutID)
	}
	return c.unmarshalCheckout(body, "remove coupon")
}

// SetBillingAddress sets the billing address via POST /v3/checkouts/{id}/billing-address.
func (c *Client) SetBillingAddress(ctx context.Context, checkoutID string, addr CheckoutAddressInput) (*Checkout, error) {
	body, err := c.Post(ctx, fmt.Sprintf("checkouts/%s/billing-address", checkoutID), addr)
	if err != nil {
		return nil, fmt.Errorf("set billing address on checkout %s: %w", checkoutID, err)
	}
	return c.unmarshalCheckout(body, "set billing address")
}

// UpdateBillingAddress updates the billing address via
// PUT /v3/checkouts/{id}/billing-address/{addr_id}. Checkout address IDs are
// STRINGS in BigCommerce (e.g. "6a5828ce429e0"), not ints.
func (c *Client) UpdateBillingAddress(ctx context.Context, checkoutID, addrID string, addr CheckoutAddressInput) (*Checkout, error) {
	body, err := c.Put(ctx, fmt.Sprintf("checkouts/%s/billing-address/%s", checkoutID, addrID), addr)
	if err != nil {
		return nil, fmt.Errorf("update billing address on checkout %s: %w", checkoutID, err)
	}
	return c.unmarshalCheckout(body, "update billing address")
}

// AddConsignment adds a shipping consignment via POST /v3/checkouts/{id}/consignments.
// Appends include=consignments.available_shipping_options so the response
// carries the shipping options actually valid for the consignment's address
// — BigCommerce computes these dynamically per-request from the store's
// configured shipping zones/methods; nothing here is hardcoded.
func (c *Client) AddConsignment(ctx context.Context, checkoutID string, consignment CheckoutConsignmentInput) (*Checkout, error) {
	path := fmt.Sprintf("checkouts/%s/consignments?%s", checkoutID, includeAvailableShippingOptions)
	body, err := c.Post(ctx, path, []CheckoutConsignmentInput{consignment})
	if err != nil {
		return nil, fmt.Errorf("add consignment to checkout %s: %w", checkoutID, err)
	}
	return c.unmarshalCheckout(body, "add consignment")
}

// UpdateConsignment updates a consignment (e.g. select shipping option)
// via PUT /v3/checkouts/{id}/consignments/{consign_id}. Also requests
// available_shipping_options so a re-fetched address/line-item change still
// surfaces the current valid options in the same response.
func (c *Client) UpdateConsignment(ctx context.Context, checkoutID, consignID string, update CheckoutConsignmentUpdate) (*Checkout, error) {
	path := fmt.Sprintf("checkouts/%s/consignments/%s?%s", checkoutID, consignID, includeAvailableShippingOptions)
	body, err := c.Put(ctx, path, update)
	if err != nil {
		return nil, fmt.Errorf("update consignment %s on checkout %s: %w", consignID, checkoutID, err)
	}
	return c.unmarshalCheckout(body, "update consignment")
}

// ConvertCheckoutToOrder converts a completed checkout to an order
// via POST /v3/checkouts/{id}/orders.
func (c *Client) ConvertCheckoutToOrder(ctx context.Context, checkoutID string) (*CheckoutOrderResult, error) {
	body, err := c.Post(ctx, fmt.Sprintf("checkouts/%s/orders", checkoutID), struct{}{})
	if err != nil {
		return nil, fmt.Errorf("convert checkout %s to order: %w", checkoutID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse convert checkout response: %w", err)
	}
	var result CheckoutOrderResult
	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal order result: %w", err)
	}
	return &result, nil
}
