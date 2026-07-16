package carts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// RegisterCheckoutTools wires carts/checkout/* into the registry.
func (c *Carts) RegisterCheckoutTools(reg *discovery.Registry) {
	// ---- carts/checkout/get ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/checkout/get",
		Tier:    middleware.TierR0,
		Summary: "Get checkout state: billing address, consignments, coupons, and totals",
		Tool: mcp.NewTool("carts_checkout_get",
			mcp.WithDescription("Get checkout details by cart UUID. Returns billing address, shipping consignments with available options, applied coupons, and totals. The checkout ID is the same UUID as the cart. Scope: store_checkouts."),
			mcp.WithString("checkout_id", mcp.Description("Cart/Checkout UUID"), mcp.Required()),
		),
		Handler: c.handleCheckoutGet,
	})

	// ---- carts/checkout/coupon_apply ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/checkout/coupon_apply",
		Tier:    middleware.TierR1,
		Summary: "Apply a coupon code to a checkout (preview then confirm)",
		Tool: mcp.NewTool("carts_checkout_coupon_apply",
			mcp.WithDescription("Apply a coupon code to a checkout. Returns updated checkout with discount amount. Scope: store_checkouts."),
			mcp.WithString("checkout_id", mcp.Description("Cart/Checkout UUID"), mcp.Required()),
			mcp.WithString("coupon_code", mcp.Description("Coupon code to apply"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: c.handleCouponApply,
	})

	// ---- carts/checkout/coupon_remove ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/checkout/coupon_remove",
		Tier:    middleware.TierR2,
		Summary: "Remove an applied coupon code from a checkout",
		Tool: mcp.NewTool("carts_checkout_coupon_remove",
			mcp.WithDescription("Remove a coupon code from a checkout. Scope: store_checkouts."),
			mcp.WithString("checkout_id", mcp.Description("Cart/Checkout UUID"), mcp.Required()),
			mcp.WithString("coupon_code", mcp.Description("Coupon code to remove"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to remove.")),
		),
		Handler: c.handleCouponRemove,
	})

	// ---- carts/checkout/billing_address ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/checkout/billing_address",
		Tier:    middleware.TierR1,
		Summary: "Set or update the billing address on a checkout",
		Tool: mcp.NewTool("carts_checkout_billing_address",
			mcp.WithDescription("Set the billing address on a checkout (POST if not set, PUT to update existing). Required fields: first_name, last_name, address1, city, country_code. Scope: store_checkouts."),
			mcp.WithString("checkout_id", mcp.Description("Cart/Checkout UUID"), mcp.Required()),
			mcp.WithString("address_json",
				mcp.Description(`JSON billing address object. Required keys: first_name, last_name, address1, city, country_code. Optional: last_name, company, address2, state_or_province_code, postal_code, phone, email.`),
				mcp.Required()),
			mcp.WithString("billing_address_id",
				mcp.Description("Existing billing address ID string (from a previous checkout/get). Provide to update; omit to create.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: c.handleBillingAddress,
	})

	// ---- carts/checkout/consignment_add ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/checkout/consignment_add",
		Tier:    middleware.TierR1,
		Summary: "Add a shipping consignment (address + line items) to a checkout",
		Tool: mcp.NewTool("carts_checkout_consignment_add",
			mcp.WithDescription("Add a shipping consignment to a checkout. A consignment assigns line items to a shipping address and reveals available shipping options. Call checkout/get afterwards to see the options, then checkout/consignment_update to select one. Scope: store_checkouts."),
			mcp.WithString("checkout_id", mcp.Description("Cart/Checkout UUID"), mcp.Required()),
			mcp.WithString("address_json",
				mcp.Description(`JSON shipping address: {"first_name":"...","last_name":"...","address1":"...","city":"...","country_code":"US"}`),
				mcp.Required()),
			mcp.WithString("line_items_json",
				mcp.Description(`JSON array of items to assign: [{"item_id":"<cart-line-item-uuid>","quantity":2}]`),
				mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to add the consignment.")),
		),
		Handler: c.handleConsignmentAdd,
	})

	// ---- carts/checkout/consignment_update ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/checkout/consignment_update",
		Tier:    middleware.TierR1,
		Summary: "Update a consignment: select a shipping option or change address",
		Tool: mcp.NewTool("carts_checkout_consignment_update",
			mcp.WithDescription("Update an existing consignment — typically to select a shipping_option_id (from the available_shipping_options list returned by checkout/get). Scope: store_checkouts."),
			mcp.WithString("checkout_id", mcp.Description("Cart/Checkout UUID"), mcp.Required()),
			mcp.WithString("consignment_id", mcp.Description("Consignment ID from checkout/get"), mcp.Required()),
			mcp.WithString("shipping_option_id",
				mcp.Description("ID of the shipping option to select (from available_shipping_options).")),
			mcp.WithString("address_json",
				mcp.Description("Updated shipping address JSON (optional).")),
			mcp.WithString("line_items_json",
				mcp.Description("Updated line items JSON (optional).")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: c.handleConsignmentUpdate,
	})

	// ---- carts/checkout/convert ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/checkout/convert",
		Tier:    middleware.TierR2,
		Summary: "Convert a completed checkout into an order (irreversible)",
		Tool: mcp.NewTool("carts_checkout_convert",
			mcp.WithDescription("Convert a checkout to an order. Prerequisites: billing address set, consignment with shipping option selected, cart has items. Returns the new order ID. The cart is consumed — this cannot be undone. Scope: store_checkouts."),
			mcp.WithString("checkout_id", mcp.Description("Cart/Checkout UUID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create the order.")),
		),
		Handler: c.handleCheckoutConvert,
	})
}

// ---- carts/checkout/get ----

func (c *Carts) handleCheckoutGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	checkoutID, ok := args["checkout_id"].(string)
	if !ok || strings.TrimSpace(checkoutID) == "" {
		return shared.ToolError("checkout_id is required"), nil
	}

	co, err := c.bc.GetCheckout(ctx, checkoutID)
	if err != nil {
		return shared.ToolError("failed to get checkout %s: %v", checkoutID, err), nil
	}
	return shared.ToolJSON(map[string]any{"checkout": checkoutView(co)})
}

// ---- carts/checkout/coupon_apply ----

func (c *Carts) handleCouponApply(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	checkoutID, ok := args["checkout_id"].(string)
	if !ok || strings.TrimSpace(checkoutID) == "" {
		return shared.ToolError("checkout_id is required"), nil
	}
	code, ok := args["coupon_code"].(string)
	if !ok || strings.TrimSpace(code) == "" {
		return shared.ToolError("coupon_code is required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "apply_coupon",
			"checkout_id": checkoutID,
			"coupon_code": code,
			"message":     fmt.Sprintf("Will apply coupon %q to checkout %s. Pass confirmed=true.", code, checkoutID),
		})
	}

	updated, err := c.bc.ApplyCoupon(ctx, checkoutID, code)
	if err != nil {
		return shared.ToolError("failed to apply coupon %q: %v", code, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "coupon_applied",
		"checkout": checkoutView(updated),
	})
}

// ---- carts/checkout/coupon_remove ----

func (c *Carts) handleCouponRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	checkoutID, ok := args["checkout_id"].(string)
	if !ok || strings.TrimSpace(checkoutID) == "" {
		return shared.ToolError("checkout_id is required"), nil
	}
	code, ok := args["coupon_code"].(string)
	if !ok || strings.TrimSpace(code) == "" {
		return shared.ToolError("coupon_code is required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "remove_coupon",
			"checkout_id": checkoutID,
			"coupon_code": code,
			"message":     fmt.Sprintf("Will remove coupon %q from checkout %s. Pass confirmed=true.", code, checkoutID),
		})
	}

	updated, err := c.bc.RemoveCoupon(ctx, checkoutID, code)
	if err != nil {
		return shared.ToolError("failed to remove coupon %q: %v", code, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "coupon_removed",
		"checkout": checkoutView(updated),
	})
}

// ---- carts/checkout/billing_address ----

func (c *Carts) handleBillingAddress(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	checkoutID, ok := args["checkout_id"].(string)
	if !ok || strings.TrimSpace(checkoutID) == "" {
		return shared.ToolError("checkout_id is required"), nil
	}
	rawJSON, ok := args["address_json"].(string)
	if !ok || strings.TrimSpace(rawJSON) == "" {
		return shared.ToolError("address_json is required"), nil
	}

	var addr bigcommerce.CheckoutAddressInput
	if err := json.Unmarshal([]byte(rawJSON), &addr); err != nil {
		return shared.ToolError("invalid address_json: %v", err), nil
	}
	if strings.TrimSpace(addr.FirstName) == "" || strings.TrimSpace(addr.LastName) == "" {
		return shared.ToolError("address_json must include first_name and last_name"), nil
	}
	if strings.TrimSpace(addr.CountryCode) == "" {
		return shared.ToolError("address_json must include country_code"), nil
	}

	// Checkout address IDs are strings in BigCommerce.
	addrID, _ := args["billing_address_id"].(string)
	addrID = strings.TrimSpace(addrID)

	if !middleware.IsConfirmedFromArgs(args) {
		action := "set_billing_address"
		if addrID != "" {
			action = "update_billing_address"
		}
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      action,
			"checkout_id": checkoutID,
			"address":     addr,
			"message":     "Pass confirmed=true to apply the billing address.",
		})
	}

	var (
		updated *bigcommerce.Checkout
		err     error
	)
	if addrID != "" {
		updated, err = c.bc.UpdateBillingAddress(ctx, checkoutID, addrID, addr)
	} else {
		updated, err = c.bc.SetBillingAddress(ctx, checkoutID, addr)
	}
	if err != nil {
		return shared.ToolError("failed to set billing address: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "billing_address_set",
		"checkout": checkoutView(updated),
	})
}

// ---- carts/checkout/consignment_add ----

func (c *Carts) handleConsignmentAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	checkoutID, ok := args["checkout_id"].(string)
	if !ok || strings.TrimSpace(checkoutID) == "" {
		return shared.ToolError("checkout_id is required"), nil
	}
	addrRaw, ok := args["address_json"].(string)
	if !ok || strings.TrimSpace(addrRaw) == "" {
		return shared.ToolError("address_json is required"), nil
	}
	itemsRaw, ok := args["line_items_json"].(string)
	if !ok || strings.TrimSpace(itemsRaw) == "" {
		return shared.ToolError("line_items_json is required"), nil
	}

	var addr bigcommerce.CheckoutAddressInput
	if err := json.Unmarshal([]byte(addrRaw), &addr); err != nil {
		return shared.ToolError("invalid address_json: %v", err), nil
	}
	var lineItems []bigcommerce.ConsignmentLineItem
	if err := json.Unmarshal([]byte(itemsRaw), &lineItems); err != nil {
		return shared.ToolError("invalid line_items_json: %v", err), nil
	}
	if len(lineItems) == 0 {
		return shared.ToolError("line_items_json must contain at least one item"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "add_consignment",
			"checkout_id": checkoutID,
			"address":     addr,
			"line_items":  lineItems,
			"message":     "Pass confirmed=true to add the consignment. Then call checkout/get to see available shipping options.",
		})
	}

	consignment := bigcommerce.CheckoutConsignmentInput{
		Address:   addr,
		LineItems: lineItems,
	}
	updated, err := c.bc.AddConsignment(ctx, checkoutID, consignment)
	if err != nil {
		return shared.ToolError("failed to add consignment: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "consignment_added",
		"checkout": checkoutView(updated),
		"tip":      "Call carts/checkout/get to see available_shipping_options, then carts/checkout/consignment_update to select one.",
	})
}

// ---- carts/checkout/consignment_update ----

func (c *Carts) handleConsignmentUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	checkoutID, ok := args["checkout_id"].(string)
	if !ok || strings.TrimSpace(checkoutID) == "" {
		return shared.ToolError("checkout_id is required"), nil
	}
	consignID, ok := args["consignment_id"].(string)
	if !ok || strings.TrimSpace(consignID) == "" {
		return shared.ToolError("consignment_id is required"), nil
	}

	update := bigcommerce.CheckoutConsignmentUpdate{}
	hasField := false

	if v, ok := args["shipping_option_id"].(string); ok && v != "" {
		update.ShippingOptionID = v
		hasField = true
	}
	if v, ok := args["address_json"].(string); ok && v != "" {
		var addr bigcommerce.CheckoutAddressInput
		if err := json.Unmarshal([]byte(v), &addr); err != nil {
			return shared.ToolError("invalid address_json: %v", err), nil
		}
		update.Address = &addr
		hasField = true
	}
	if v, ok := args["line_items_json"].(string); ok && v != "" {
		var items []bigcommerce.ConsignmentLineItem
		if err := json.Unmarshal([]byte(v), &items); err != nil {
			return shared.ToolError("invalid line_items_json: %v", err), nil
		}
		update.LineItems = items
		hasField = true
	}
	if !hasField {
		return shared.ToolError("provide at least one of: shipping_option_id, address_json, line_items_json"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":         "preview",
			"action":         "update_consignment",
			"checkout_id":    checkoutID,
			"consignment_id": consignID,
			"update":         update,
			"message":        "Pass confirmed=true to apply.",
		})
	}

	updated, err := c.bc.UpdateConsignment(ctx, checkoutID, consignID, update)
	if err != nil {
		return shared.ToolError("failed to update consignment: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "consignment_updated",
		"checkout": checkoutView(updated),
	})
}

// ---- carts/checkout/convert ----

func (c *Carts) handleCheckoutConvert(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	checkoutID, ok := args["checkout_id"].(string)
	if !ok || strings.TrimSpace(checkoutID) == "" {
		return shared.ToolError("checkout_id is required"), nil
	}

	cacheKey := fmt.Sprintf("checkout_convert:%s", checkoutID)
	co, err := session.CacheOrFetch(c.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Checkout, error) {
		return c.bc.GetCheckout(ctx, checkoutID)
	})
	if err != nil {
		return shared.ToolError("failed to fetch checkout %s: %v", checkoutID, err), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		itemCount := checkoutItemCount(co)
		// Collect every unmet prerequisite rather than overwriting a single
		// warning key — a checkout can be missing more than one.
		var warnings []string
		if co.BillingAddress == nil {
			warnings = append(warnings, "Billing address is not set — conversion may fail.")
		}
		if len(co.Consignments) == 0 {
			warnings = append(warnings, "No shipping consignment — conversion may fail for physical items.")
		} else if !anyConsignmentHasSelectedShipping(co) {
			warnings = append(warnings, "No shipping option selected on any consignment — select one via carts/checkout/consignment_update first.")
		}
		if itemCount == 0 {
			warnings = append(warnings, "Checkout has no line items — conversion will fail.")
		}
		preview := map[string]any{
			"status":      "preview",
			"action":      "convert_to_order",
			"checkout_id": checkoutID,
			"grand_total": co.GrandTotal,
			"item_count":  itemCount,
			"message":     "This will create an order from the checkout. The cart will be consumed. Pass confirmed=true to proceed.",
		}
		if len(warnings) > 0 {
			preview["warnings"] = warnings
		}
		return shared.ToolJSON(preview)
	}

	c.cache.ForContext(ctx).Delete(cacheKey)
	result, err := c.bc.ConvertCheckoutToOrder(ctx, checkoutID)
	if err != nil {
		return shared.ToolError("failed to convert checkout %s to order: %v", checkoutID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":      "order_created",
		"checkout_id": checkoutID,
		"order_id":    result.ID,
		"message":     fmt.Sprintf("Order %d created. Use orders/management/get to view it.", result.ID),
	})
}

// ---- View helpers ----

// checkoutView returns a compact summary of a Checkout for tool responses.
func checkoutView(co *bigcommerce.Checkout) map[string]any {
	if co == nil {
		return nil
	}
	v := map[string]any{
		"id":              co.ID,
		"channel_id":      co.ChannelID,
		"subtotal_ex_tax": co.SubtotalExTax,
		"subtotal_tax":    co.SubtotalTax,
		"grand_total":     co.GrandTotal,
		"created_time":    co.CreatedTime,
		"updated_time":    co.UpdatedTime,
	}
	if co.CustomerMessage != "" {
		v["customer_message"] = co.CustomerMessage
	}
	if co.OrderID != nil {
		v["order_id"] = *co.OrderID
	}
	if co.BillingAddress != nil {
		v["billing_address"] = map[string]any{
			"id":           co.BillingAddress.ID,
			"first_name":   co.BillingAddress.FirstName,
			"last_name":    co.BillingAddress.LastName,
			"address1":     co.BillingAddress.Address1,
			"city":         co.BillingAddress.City,
			"country_code": co.BillingAddress.CountryCode,
		}
	}
	if len(co.Coupons) > 0 {
		couponList := make([]map[string]any, len(co.Coupons))
		for i, cp := range co.Coupons {
			couponList[i] = map[string]any{
				"code":              cp.Code,
				"discounted_amount": cp.DiscountedAmount,
			}
		}
		v["coupons"] = couponList
	}
	if len(co.Consignments) > 0 {
		cons := make([]map[string]any, len(co.Consignments))
		for i, cn := range co.Consignments {
			cm := map[string]any{"id": cn.ID}
			if cn.ShippingAddress != nil {
				cm["city"] = cn.ShippingAddress.City
				cm["country_code"] = cn.ShippingAddress.CountryCode
			}
			if cn.SelectedShippingOption != nil {
				cm["selected_shipping"] = map[string]any{
					"id":          cn.SelectedShippingOption.ID,
					"description": cn.SelectedShippingOption.Description,
					"cost":        cn.SelectedShippingOption.Cost,
				}
			}
			cm["available_shipping_option_count"] = len(cn.AvailableShippingOptions)
			if len(cn.AvailableShippingOptions) > 0 {
				opts := make([]map[string]any, len(cn.AvailableShippingOptions))
				for j, opt := range cn.AvailableShippingOptions {
					opts[j] = map[string]any{
						"id": opt.ID, "description": opt.Description, "cost": opt.Cost,
					}
				}
				cm["available_shipping_options"] = opts
			}
			cons[i] = cm
		}
		v["consignments"] = cons
	}
	if co.Cart != nil {
		v["cart_id"] = co.Cart.ID
		v["cart_amount"] = co.Cart.CartAmount
	}
	return v
}

func checkoutItemCount(co *bigcommerce.Checkout) int {
	if co == nil || co.Cart == nil {
		return 0
	}
	return len(co.Cart.LineItems.PhysicalItems) +
		len(co.Cart.LineItems.DigitalItems) +
		len(co.Cart.LineItems.CustomItems) +
		len(co.Cart.LineItems.GiftCertificates)
}

// anyConsignmentHasSelectedShipping reports whether at least one consignment
// has a shipping option selected — a prerequisite for converting to an order.
func anyConsignmentHasSelectedShipping(co *bigcommerce.Checkout) bool {
	if co == nil {
		return false
	}
	for _, cn := range co.Consignments {
		if cn.SelectedShippingOption != nil {
			return true
		}
	}
	return false
}
