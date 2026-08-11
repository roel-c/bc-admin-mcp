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

const (
	maxQuoteConvertToOrders = 10
	defaultQuoteOrderStatus = 7 // Awaiting Payment
)

func (ct *CompanyTools) registerQuoteConvertTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/convert_to_order",
		Tier:    middleware.TierR2,
		Summary: "Convert up to 10 quotes into BigCommerce orders (stops at Ordered; no invoice/payment)",
		Tool: mcp.NewTool("b2b_quotes_convert_to_order",
			mcp.WithDescription("Convert up to 10 B2B quotes into BigCommerce orders in one preview→confirm. Each quote runs a multi-call checkout pipeline (~8–12 BC API round-trips); prefer ≤3–5 quote_ids per confirmed call — larger batches may hit MCP client timeouts while work continues server-side. Already-ordered quotes (status 4 / bcOrderId set) are returned as already_converted without re-checkout (safe to retry after a timeout). Server-side: ensure quote shipping → generate checkout cart → resolve buyer customer_id → billing/consignment → convert → update_status (default 7 Awaiting Payment) → assign quote to order. Optional payment_method stamps the order after convert. Stops at orders/Ordered quotes — does NOT create invoices or payment records (use b2b/invoices/create_from_orders and b2b/payment_records/create_offline separately when asked). Partial failures report partial_success."),
			mcp.WithArray("quote_ids", mcp.Description("BigCommerce B2B quote IDs (positive integers, max 10). Prefer ≤3–5 per call to avoid MCP client timeouts."), mcp.Required(), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithString("shipping_method_id", mcp.Description("Optional quote shipping method id. When omitted, uses the quote's existing method or picks Free Shipping / first available rate.")),
			mcp.WithNumber("status_id", mcp.Description("Order status after convert (default 7 Awaiting Payment). Required to leave Incomplete.")),
			mcp.WithString("payment_method", mcp.Description("Optional payment_method string written on the order after convert (e.g. \"Check\").")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to convert.")),
		),
		Handler: ct.handleQuoteConvertToOrder,
	})
}

func (ct *CompanyTools) handleQuoteConvertToOrder(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	quoteIDs, err := parsePositiveIntIDs(args, "quote_ids", maxQuoteConvertToOrders)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	shippingMethodID, _ := args["shipping_method_id"].(string)
	shippingMethodID = strings.TrimSpace(shippingMethodID)
	statusID := defaultQuoteOrderStatus
	if v, ok := args["status_id"].(float64); ok {
		if v != float64(int(v)) || int(v) <= 0 {
			return shared.ToolError("status_id must be a positive integer"), nil
		}
		statusID = int(v)
	}
	paymentMethod, _ := args["payment_method"].(string)
	paymentMethod = strings.TrimSpace(paymentMethod)

	if !middleware.IsConfirmedFromArgs(args) {
		msg := fmt.Sprintf(
			"Will convert %d quote(s) to BigCommerce orders (shipping→checkout→customer→consignment→convert→status %d→assign). Stops at orders — no invoices or payments. Prefer ≤3–5 quote_ids per call; already-ordered quotes are skipped as already_converted. Pass confirmed=true.",
			len(quoteIDs), statusID,
		)
		if len(quoteIDs) > 5 {
			msg += fmt.Sprintf(" Note: %d ids may exceed typical MCP client timeouts.", len(quoteIDs))
		}
		preview := map[string]any{
			"status":    "preview",
			"action":    "convert_b2b_quotes_to_orders",
			"quote_ids": quoteIDs,
			"count":     len(quoteIDs),
			"status_id": statusID,
			"message":   msg,
		}
		if shippingMethodID != "" {
			preview["shipping_method_id"] = shippingMethodID
		}
		if paymentMethod != "" {
			preview["payment_method"] = paymentMethod
		}
		return shared.ToolJSON(preview)
	}

	if ct.checkout == nil {
		return shared.ToolError("quote convert requires checkout API (carts/orders client not wired)"), nil
	}
	if ct.customers == nil {
		return shared.ToolError("quote convert requires customer lookup (customers client not wired)"), nil
	}

	opts := quoteConvertOpts{
		shippingMethodID: shippingMethodID,
		statusID:         statusID,
		paymentMethod:    paymentMethod,
	}
	created := make([]map[string]any, 0, len(quoteIDs))
	failures := make([]map[string]any, 0)
	for _, quoteID := range quoteIDs {
		orderID, already, err := ct.convertQuoteToOrder(ctx, quoteID, opts)
		if err != nil {
			failures = append(failures, map[string]any{"quote_id": quoteID, "error": err.Error()})
			continue
		}
		row := map[string]any{
			"quote_id": quoteID,
			"order_id": orderID,
			"status":   "converted",
		}
		if already {
			row["status"] = "already_converted"
		}
		created = append(created, row)
	}
	status := "converted"
	if len(failures) > 0 && len(created) > 0 {
		status = "partial_success"
	} else if len(failures) > 0 {
		status = "failed"
	}
	out := map[string]any{
		"status":          status,
		"converted_count": len(created),
		"failed_count":    len(failures),
		"converted":       created,
	}
	if len(failures) > 0 {
		out["failures"] = failures
	}
	return shared.ToolJSON(out)
}

type quoteConvertOpts struct {
	shippingMethodID string
	statusID         int
	paymentMethod    string
}

// convertQuoteToOrder converts one quote. The bool return is true when the quote
// was already Ordered (status 4 / bcOrderId set) and no checkout was performed.
func (ct *CompanyTools) convertQuoteToOrder(ctx context.Context, quoteID int, opts quoteConvertOpts) (int, bool, error) {
	quote, err := ct.bc.GetB2BQuote(ctx, quoteID)
	if err != nil {
		return 0, false, fmt.Errorf("get quote: %w", err)
	}
	if orderID, ok := quoteAlreadyConvertedOrderID(quote); ok {
		return orderID, true, nil
	}
	if err := ct.ensureQuoteShipping(ctx, quoteID, quote, opts.shippingMethodID); err != nil {
		return 0, false, err
	}

	checkoutURLs, err := ct.bc.GenerateB2BQuoteCheckout(ctx, quoteID)
	if err != nil {
		return 0, false, fmt.Errorf("generate checkout: %w", err)
	}
	cartID := extractQuoteCartID(checkoutURLs)
	if cartID == "" {
		return 0, false, fmt.Errorf("checkout response missing cartId")
	}

	email := quoteContactEmail(quote)
	if email == "" {
		return 0, false, fmt.Errorf("quote contactInfo.email is required to resolve customer_id")
	}
	customerID, err := ct.resolveQuoteCustomerID(ctx, quote, email)
	if err != nil {
		return 0, false, err
	}

	if _, err := ct.checkout.UpdateCart(ctx, cartID, bigcommerce.CartUpdate{CustomerID: customerID}); err != nil {
		return 0, false, fmt.Errorf("set cart customer_id: %w", err)
	}
	cart, err := ct.checkout.GetCart(ctx, cartID, false)
	if err != nil {
		return 0, false, fmt.Errorf("get cart: %w", err)
	}
	lineItems := cartConsignmentLineItems(cart)
	if len(lineItems) == 0 {
		return 0, false, fmt.Errorf("cart %s has no shippable line items", cartID)
	}

	shipAddr := quoteCheckoutAddress(quote, "shippingAddress", email)
	billAddr := quoteCheckoutAddress(quote, "billingAddress", email)
	if billAddr.Address1 == "" {
		billAddr = shipAddr
	}
	if shipAddr.Address1 == "" {
		return 0, false, fmt.Errorf("quote shippingAddress is required")
	}

	if _, err := ct.checkout.SetBillingAddress(ctx, cartID, billAddr); err != nil {
		return 0, false, fmt.Errorf("set billing address: %w", err)
	}
	co, err := ct.checkout.AddConsignment(ctx, cartID, bigcommerce.CheckoutConsignmentInput{
		Address:   shipAddr,
		LineItems: lineItems,
	})
	if err != nil {
		return 0, false, fmt.Errorf("add consignment: %w", err)
	}
	if len(co.Consignments) == 0 || co.Consignments[0].ID == "" {
		return 0, false, fmt.Errorf("consignment add returned no consignment id")
	}
	consign := co.Consignments[0]
	optionID := pickCheckoutShippingOptionID(consign.AvailableShippingOptions)
	if optionID == "" {
		return 0, false, fmt.Errorf("no available shipping options on checkout consignment")
	}
	if _, err := ct.checkout.UpdateConsignment(ctx, cartID, consign.ID, bigcommerce.CheckoutConsignmentUpdate{
		ShippingOptionID: optionID,
	}); err != nil {
		return 0, false, fmt.Errorf("select shipping option: %w", err)
	}

	orderResult, err := ct.checkout.ConvertCheckoutToOrder(ctx, cartID)
	if err != nil {
		return 0, false, fmt.Errorf("convert checkout: %w", err)
	}
	if orderResult == nil || orderResult.ID <= 0 {
		return 0, false, fmt.Errorf("convert checkout returned no order id")
	}
	orderID := orderResult.ID

	if _, err := ct.checkout.UpdateOrderStatus(ctx, orderID, opts.statusID); err != nil {
		return 0, false, fmt.Errorf("update order status: %w", err)
	}
	if opts.paymentMethod != "" {
		payload, _ := json.Marshal(map[string]string{"payment_method": opts.paymentMethod})
		if _, err := ct.checkout.UpdateOrder(ctx, orderID, payload); err != nil {
			return 0, false, fmt.Errorf("set payment_method: %w", err)
		}
	}
	if err := ct.bc.AssignB2BQuoteToOrder(ctx, quoteID, orderID); err != nil {
		return 0, false, fmt.Errorf("assign quote to order: %w", err)
	}
	return orderID, false, nil
}

// quoteAlreadyConvertedOrderID returns the BigCommerce order ID when the quote
// already has bcOrderId set (Ordered). Safe for retries after MCP client timeouts.
func quoteAlreadyConvertedOrderID(quote map[string]any) (int, bool) {
	if quote == nil {
		return 0, false
	}
	orderID := anyToInt(quote["bcOrderId"])
	if orderID <= 0 {
		orderID = anyToInt(quote["bc_order_id"])
	}
	if orderID <= 0 {
		return 0, false
	}
	return orderID, true
}

func (ct *CompanyTools) ensureQuoteShipping(ctx context.Context, quoteID int, quote map[string]any, preferredMethodID string) error {
	if preferredMethodID != "" {
		if _, err := ct.bc.SelectB2BQuoteShippingRate(ctx, quoteID, preferredMethodID, "", 0, false); err != nil {
			return fmt.Errorf("select shipping method %q: %w", preferredMethodID, err)
		}
		return nil
	}
	if quoteHasShippingMethod(quote) {
		return nil
	}
	rates, err := ct.bc.ListB2BQuoteShippingRates(ctx, quoteID)
	if err != nil {
		return fmt.Errorf("list shipping rates: %w", err)
	}
	methodID := pickQuoteShippingMethodID(rates)
	if methodID == "" {
		return fmt.Errorf("no shipping rates available for quote")
	}
	if _, err := ct.bc.SelectB2BQuoteShippingRate(ctx, quoteID, methodID, "", 0, false); err != nil {
		return fmt.Errorf("select shipping method %q: %w", methodID, err)
	}
	return nil
}

func (ct *CompanyTools) resolveQuoteCustomerID(ctx context.Context, quote map[string]any, email string) (int, error) {
	custs, err := ct.customers.SearchCustomers(ctx, map[string]string{"email:in": email})
	if err != nil {
		return 0, fmt.Errorf("search customers for %s: %w", email, err)
	}
	for _, c := range custs {
		if strings.EqualFold(strings.TrimSpace(c.Email), email) && c.ID > 0 {
			return c.ID, nil
		}
	}

	companyID := quoteCompanyID(quote)
	if companyID <= 0 {
		return 0, fmt.Errorf("no BC customer for email %s and quote has no companyId for user lookup", email)
	}
	params := url.Values{}
	params.Set("companyId", fmt.Sprintf("%d", companyID))
	users, err := ct.bc.ListB2BUsers(ctx, params.Encode())
	if err != nil {
		return 0, fmt.Errorf("list company users: %w", err)
	}
	users = ct.enrichUsersWithBCCustomerIDs(ctx, users)
	for _, u := range users {
		if strings.EqualFold(strings.TrimSpace(u.Email), email) && u.BCCustomerID > 0 {
			return u.BCCustomerID, nil
		}
	}
	// Fall back to company admin with a resolved BC customer id.
	for _, u := range users {
		if u.Role == 0 && u.BCCustomerID > 0 {
			return u.BCCustomerID, nil
		}
	}
	for _, u := range users {
		if u.BCCustomerID > 0 {
			return u.BCCustomerID, nil
		}
	}
	return 0, fmt.Errorf("could not resolve bc_customer_id for quote contact %s (company %d)", email, companyID)
}

func parsePositiveIntIDs(args map[string]any, key string, max int) ([]int, error) {
	raw, ok := args[key].([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("%s is required (non-empty array of positive integers)", key)
	}
	if len(raw) > max {
		return nil, fmt.Errorf("%s: maximum %d per call", key, max)
	}
	ids := make([]int, 0, len(raw))
	seen := map[int]bool{}
	for i, v := range raw {
		f, ok := v.(float64)
		if !ok || f != float64(int(f)) || int(f) <= 0 {
			return nil, fmt.Errorf("%s[%d] must be a positive integer", key, i)
		}
		id := int(f)
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s must contain at least one positive integer", key)
	}
	return ids, nil
}

func extractQuoteCartID(urls map[string]any) string {
	if urls == nil {
		return ""
	}
	for _, key := range []string{"cartId", "cart_id", "checkoutId", "checkout_id"} {
		if s := anyToString(urls[key]); s != "" {
			return s
		}
	}
	return ""
}

func quoteContactEmail(quote map[string]any) string {
	info := asStringMap(quote["contactInfo"])
	return strings.ToLower(strings.TrimSpace(anyToString(info["email"])))
}

func quoteCompanyID(quote map[string]any) int {
	if id := anyToInt(quote["companyId"]); id > 0 {
		return id
	}
	info := asStringMap(quote["companyInfo"])
	return anyToInt(info["companyId"])
}

func quoteHasShippingMethod(quote map[string]any) bool {
	sm := asStringMap(quote["shippingMethod"])
	if sm == nil {
		return false
	}
	if anyToString(sm["id"]) != "" || anyToString(sm["shippingMethodId"]) != "" {
		return true
	}
	// Some payloads only echo description/type after select.
	return anyToString(sm["description"]) != "" || anyToString(sm["type"]) != ""
}

func pickQuoteShippingMethodID(rates []map[string]any) string {
	var first string
	for _, rate := range rates {
		id := anyToString(rate["id"])
		if id == "" {
			id = anyToString(rate["shippingMethodId"])
		}
		if id == "" {
			continue
		}
		if first == "" {
			first = id
		}
		desc := strings.ToLower(anyToString(rate["description"]) + " " + anyToString(rate["type"]) + " " + anyToString(rate["name"]))
		if strings.Contains(desc, "free") {
			return id
		}
	}
	return first
}

func pickCheckoutShippingOptionID(options []bigcommerce.CheckoutShippingOption) string {
	var first string
	for _, opt := range options {
		if opt.ID == "" {
			continue
		}
		if first == "" {
			first = opt.ID
		}
		desc := strings.ToLower(opt.Description + " " + opt.Type + " " + opt.AdditionalDescription)
		if strings.Contains(desc, "free") || opt.Cost == 0 {
			return opt.ID
		}
	}
	return first
}

func quoteCheckoutAddress(quote map[string]any, field, email string) bigcommerce.CheckoutAddressInput {
	addr := asStringMap(quote[field])
	companyInfo := asStringMap(quote["companyInfo"])
	contact := asStringMap(quote["contactInfo"])
	first := anyToString(addr["firstName"])
	last := anyToString(addr["lastName"])
	if first == "" && last == "" {
		first, last = splitPersonName(anyToString(contact["name"]))
	}
	company := anyToString(addr["company"])
	if company == "" {
		company = anyToString(companyInfo["companyName"])
	}
	if company == "" {
		company = anyToString(contact["companyName"])
	}
	return bigcommerce.CheckoutAddressInput{
		FirstName:           first,
		LastName:            last,
		Email:               email,
		Company:             company,
		Address1:            anyToString(addr["address"]),
		Address2:            anyToString(addr["apartment"]),
		City:                anyToString(addr["city"]),
		StateOrProvince:     anyToString(addr["state"]),
		StateOrProvinceCode: anyToString(addr["stateCode"]),
		PostalCode:          anyToString(addr["zipCode"]),
		CountryCode:         anyToString(addr["countryCode"]),
		Phone:               anyToString(addr["phoneNumber"]),
	}
}

func cartConsignmentLineItems(cart *bigcommerce.Cart) []bigcommerce.ConsignmentLineItem {
	if cart == nil {
		return nil
	}
	out := make([]bigcommerce.ConsignmentLineItem, 0, len(cart.LineItems.PhysicalItems)+len(cart.LineItems.DigitalItems))
	for _, item := range cart.LineItems.PhysicalItems {
		if item.ID == "" || item.Quantity <= 0 {
			continue
		}
		out = append(out, bigcommerce.ConsignmentLineItem{ItemID: item.ID, Quantity: item.Quantity})
	}
	for _, item := range cart.LineItems.DigitalItems {
		if item.ID == "" || item.Quantity <= 0 {
			continue
		}
		out = append(out, bigcommerce.ConsignmentLineItem{ItemID: item.ID, Quantity: item.Quantity})
	}
	return out
}

func asStringMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	case json.Number:
		return strings.TrimSpace(t.String())
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	default:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func anyToInt(v any) int {
	switch t := v.(type) {
	case float64:
		if t == float64(int(t)) {
			return int(t)
		}
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, err := t.Int64()
		if err == nil {
			return int(n)
		}
	case string:
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(t), "%d", &n); err == nil {
			return n
		}
	}
	return 0
}

func splitPersonName(name string) (string, string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ""
	}
	parts := strings.Fields(name)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}
