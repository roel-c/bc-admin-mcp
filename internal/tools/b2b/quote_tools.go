package b2b

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ============================================================
// Quote tools
// ============================================================

func (ct *CompanyTools) registerQuoteTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/list",
		Tier:    middleware.TierR0,
		Summary: "List sales quotes with filters and sorting",
		Tool: mcp.NewTool("b2b_quotes_list",
			mcp.WithDescription("List B2B Edition sales quotes across the store. Filter by company/salesRep/status/date ranges; sort with sort_by + order_by."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
			mcp.WithString("sort_by", mcp.Description("Sort field: company, status, salesRep, createdAt, expiredAt, updatedAt (default updatedAt).")),
			mcp.WithString("order_by", mcp.Description("asc or desc (default desc).")),
			mcp.WithString("q", mcp.Description("Search quote number, quote title, company name, sales rep, or buyer name.")),
			mcp.WithString("quote_number", mcp.Description("Full or partial quote number.")),
			mcp.WithString("company", mcp.Description("Full or partial company name.")),
			mcp.WithString("sales_rep", mcp.Description("Sales rep name.")),
			mcp.WithString("status", mcp.Description("Backend quote status code (see BigCommerce Quote Statuses reference).")),
			mcp.WithString("quote_title", mcp.Description("External quote title.")),
			mcp.WithString("created_by", mcp.Description("Name of the sales rep or buyer who created the quote.")),
		),
		Handler: ct.handleQuoteList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/get",
		Tier:    middleware.TierR0,
		Summary: "Get full quote detail (line items, addresses, shipping, messages)",
		Tool: mcp.NewTool("b2b_quotes_get",
			mcp.WithDescription("Get full detail for a single B2B quote: line items, billing/shipping addresses, shipping method, discount, message/tracking history."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
		),
		Handler: ct.handleQuoteGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/create",
		Tier:    middleware.TierR1,
		Summary: "Create a new sales quote (visible to the buyer immediately)",
		Tool: mcp.NewTool("b2b_quotes_create",
			mcp.WithDescription("Create a new B2B sales quote. For Buyer Portal visibility you MUST include companyId (B2B company id) — contactInfo.email/companyName alone leave companyInfo empty and the quote stays Control-Panel-only. quote_json should match quoteData_POST (companyId, subtotal, channelId, quoteTitle, referenceNumber, currency, extraFields, notes, discount, grandTotal, legalTerms, displayDiscount, allowCheckout, productList with productId/variantId/basePrice/offeredPrice/discount as numbers, shippingAddress with state/stateCode, contactInfo object, userEmail of a Control Panel system user, expiredAt as MM/DD/YYYY). Preview → confirm."),
			mcp.WithString("quote_json", mcp.Description("JSON object matching the quote create body. Use b2b/quotes/get on an existing quote to see an example shape."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create the quote.")),
		),
		Handler: ct.handleQuoteCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/update",
		Tier:    middleware.TierR1,
		Summary: "Update an existing quote (partial update, except line items)",
		Tool: mcp.NewTool("b2b_quotes_update",
			mcp.WithDescription("Update a B2B quote. No field is required — send only what you want to change. IMPORTANT: if updating productList (line items), you must include every existing line item you want to keep; omitted items are removed. To hide a quote without deleting it, set status to archived here instead of using delete. Preview → confirm."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
			mcp.WithString("quote_json", mcp.Description("JSON object with the fields to update (subtotal, quoteTitle, notes, discount, grandTotal, productList, shippingAddress, contactInfo, message, status, expiredAt, etc.)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleQuoteUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a quote",
		Tool: mcp.NewTool("b2b_quotes_delete",
			mcp.WithDescription("Permanently delete a B2B quote. To hide a quote from the buyer without deleting it, use b2b/quotes/update with status set to archived instead. Preview → confirm."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete permanently.")),
		),
		Handler: ct.handleQuoteDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/checkout",
		Tier:    middleware.TierR1,
		Summary: "Generate cart and checkout URLs for a quote",
		Tool: mcp.NewTool("b2b_quotes_checkout",
			mcp.WithDescription("Generate a cart URL and checkout URL for a quote, so its line items can be purchased. Only valid for quotes in status New(0), In Process(2), or Updated by Customer(3). Preview → confirm."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to generate the URLs.")),
		),
		Handler: ct.handleQuoteCheckout,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/assign_to_order",
		Tier:    middleware.TierR2,
		Summary: "Associate an existing BigCommerce order with a quote",
		Tool: mcp.NewTool("b2b_quotes_assign_to_order",
			mcp.WithDescription("Associate an existing BigCommerce order with a quote via POST /rfq/{id}/ordered. Required after Management API carts/checkout/convert on a quote-generated cart (storefront/Buyer Portal checkout via the quote checkoutUrl links natively instead). Only valid for quotes in status New(0), In Process(2), or Updated by Customer(3). Pass the BigCommerce order ID (not the B2B Edition internal order id). Preview → confirm."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
			mcp.WithNumber("order_id", mcp.Description("BigCommerce order ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleQuoteAssignToOrder,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/pdf_export",
		Tier:    middleware.TierR0,
		Summary: "Get a download link for a backend-detail PDF of a quote",
		Tool: mcp.NewTool("b2b_quotes_pdf_export",
			mcp.WithDescription("Generate a download link for a PDF of the quote. Uses a non-configurable backend template (includes channel and line-item cost margins) — not one of the buyer-facing templates."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
			mcp.WithString("currency_json", mcp.Description(`Optional JSON object to override currency/exchange rate for the export, e.g. {"currencyCode":"USD","currencyExchangeRate":1.0}`)),
		),
		Handler: ct.handleQuotePDFExport,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/extra_fields",
		Tier:    middleware.TierR0,
		Summary: "List extra-field definitions configured for quotes",
		Tool: mcp.NewTool("b2b_quotes_extra_fields",
			mcp.WithDescription("List the extra-field (custom field) definitions configured for B2B Edition quotes."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleQuoteExtraFields,
	})

	// ---- Shipping ----

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/shipping/rates",
		Tier:    middleware.TierR0,
		Summary: "Get available static/real-time shipping rates for a quote",
		Tool: mcp.NewTool("b2b_quotes_shipping_rates",
			mcp.WithDescription("List the static and real-time shipping rates available to a quote, based on its products and shipping address. For custom shipping methods, use b2b/quotes/shipping/custom_methods instead."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
		),
		Handler: ct.handleQuoteShippingRates,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/shipping/select",
		Tier:    middleware.TierR1,
		Summary: "Assign a shipping method to a quote",
		Tool: mcp.NewTool("b2b_quotes_shipping_select",
			mcp.WithDescription("Assign a shipping method to a quote. Provide shipping_method_id (from shipping/rates or shipping/custom_methods) for a static/real-time/custom method, OR custom_name + custom_cost for an ad hoc custom method. Preview → confirm."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
			mcp.WithString("shipping_method_id", mcp.Description("Shipping method ID from shipping/rates or shipping/custom_methods.")),
			mcp.WithString("custom_name", mcp.Description("Ad hoc custom shipping method name (use with custom_cost instead of shipping_method_id).")),
			mcp.WithNumber("custom_cost", mcp.Description("Ad hoc custom shipping method cost.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleQuoteShippingSelect,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/shipping/remove",
		Tier:    middleware.TierR2,
		Summary: "Remove the shipping method currently assigned to a quote",
		Tool: mcp.NewTool("b2b_quotes_shipping_remove",
			mcp.WithDescription("Remove the shipping method currently assigned to a quote. Preview → confirm."),
			mcp.WithNumber("quote_id", mcp.Description("Quote ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to remove.")),
		),
		Handler: ct.handleQuoteShippingRemove,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/quotes/shipping/custom_methods",
		Tier:    middleware.TierR0,
		Summary: "List store-wide custom shipping methods enabled for quotes",
		Tool: mcp.NewTool("b2b_quotes_shipping_custom_methods",
			mcp.WithDescription("List the custom shipping methods enabled in the store's Quotes settings (store-wide, not scoped to one quote)."),
		),
		Handler: ct.handleQuoteShippingCustomMethods,
	})
}

// ---- quote handlers ----

func (ct *CompanyTools) handleQuoteList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["sort_by"].(string); ok && v != "" {
		params.Set("sortBy", v)
	}
	if v, ok := args["order_by"].(string); ok && v != "" {
		params.Set("orderBy", v)
	}
	if v, ok := args["q"].(string); ok && v != "" {
		params.Set("q", v)
	}
	if v, ok := args["quote_number"].(string); ok && v != "" {
		params.Set("quoteNumber", v)
	}
	if v, ok := args["company"].(string); ok && v != "" {
		params.Set("company", v)
	}
	if v, ok := args["sales_rep"].(string); ok && v != "" {
		params.Set("salesRep", v)
	}
	if v, ok := args["status"].(string); ok && v != "" {
		params.Set("status", v)
	}
	if v, ok := args["quote_title"].(string); ok && v != "" {
		params.Set("quoteTitle", v)
	}
	if v, ok := args["created_by"].(string); ok && v != "" {
		params.Set("createdBy", v)
	}

	quotes, err := ct.bc.ListB2BQuotes(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B quotes: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(quotes), "quotes": quotes})
}

func (ct *CompanyTools) handleQuoteGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	q, err := ct.bc.GetB2BQuote(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B quote %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"quote": q})
}

func parseQuoteJSONBody(args map[string]any, key string) (map[string]any, error) {
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

func (ct *CompanyTools) handleQuoteCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	body, err := parseQuoteJSONBody(args, "quote_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_quote",
			"payload": body,
			"message": "Will create this quote, immediately visible to the buyer unless allowCheckout=false. Pass confirmed=true.",
		})
	}

	q, err := ct.bc.CreateB2BQuote(ctx, body)
	if err != nil {
		return shared.ToolError("failed to create B2B quote: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "quote": q})
}

func (ct *CompanyTools) handleQuoteUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	body, err := parseQuoteJSONBody(args, "quote_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "update_b2b_quote",
			"quote_id": id,
			"payload":  body,
			"message":  fmt.Sprintf("Will apply these fields to quote %d. If productList is included, only the listed line items are kept. Pass confirmed=true.", id),
		})
	}

	q, err := ct.bc.UpdateB2BQuote(ctx, id, body)
	if err != nil {
		return shared.ToolError("failed to update B2B quote %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "quote": q})
}

func (ct *CompanyTools) handleQuoteDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "delete_b2b_quote",
			"quote_id": id,
			"message":  fmt.Sprintf("Will permanently delete quote %d. To hide it instead, use b2b/quotes/update with status=archived. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.DeleteB2BQuote(ctx, id); err != nil {
		return shared.ToolError("failed to delete B2B quote %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "quote_id": id})
}

func (ct *CompanyTools) handleQuoteCheckout(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "generate_b2b_quote_checkout",
			"quote_id": id,
			"message":  fmt.Sprintf("Will generate cart/checkout URLs for quote %d (only valid in status New/In Process/Updated by Customer). Pass confirmed=true.", id),
		})
	}

	result, err := ct.bc.GenerateB2BQuoteCheckout(ctx, id)
	if err != nil {
		return shared.ToolError("failed to generate checkout for B2B quote %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "generated", "quote_id": id, "urls": result})
}

func (ct *CompanyTools) handleQuoteAssignToOrder(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "assign_b2b_quote_to_order",
			"quote_id": id,
			"order_id": orderID,
			"message":  fmt.Sprintf("Will associate order %d with quote %d (only valid in status New/In Process/Updated by Customer). Pass confirmed=true.", orderID, id),
		})
	}

	if err := ct.bc.AssignB2BQuoteToOrder(ctx, id, orderID); err != nil {
		return shared.ToolError("failed to assign order %d to B2B quote %d: %v", orderID, id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "assigned", "quote_id": id, "order_id": orderID})
}

func (ct *CompanyTools) handleQuotePDFExport(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	var currency map[string]any
	if raw, ok := args["currency_json"].(string); ok && strings.TrimSpace(raw) != "" {
		if uerr := json.Unmarshal([]byte(raw), &currency); uerr != nil {
			return shared.ToolError("invalid currency_json: %v", uerr), nil
		}
	}
	result, err := ct.bc.ExportB2BQuotePDF(ctx, id, currency)
	if err != nil {
		return shared.ToolError("failed to export PDF for B2B quote %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"quote_id": id, "download": result})
}

func (ct *CompanyTools) handleQuoteExtraFields(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	defs, err := ct.bc.ListB2BQuoteExtraFields(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B quote extra fields: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(defs), "extra_fields": defs})
}

// ---- shipping handlers ----

func (ct *CompanyTools) handleQuoteShippingRates(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	rates, err := ct.bc.ListB2BQuoteShippingRates(ctx, id)
	if err != nil {
		return shared.ToolError("failed to list shipping rates for B2B quote %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"quote_id": id, "total": len(rates), "rates": rates})
}

func (ct *CompanyTools) handleQuoteShippingSelect(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	shippingMethodID, _ := args["shipping_method_id"].(string)
	customName, _ := args["custom_name"].(string)
	customCostVal, hasCustomCost := args["custom_cost"].(float64)
	if shippingMethodID == "" && (customName == "" || !hasCustomCost) {
		return shared.ToolError("provide shipping_method_id, or both custom_name and custom_cost"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":             "preview",
			"action":             "select_b2b_quote_shipping_rate",
			"quote_id":           id,
			"shipping_method_id": shippingMethodID,
			"custom_name":        customName,
			"custom_cost":        customCostVal,
			"message":            fmt.Sprintf("Will set the shipping method on quote %d. Pass confirmed=true.", id),
		})
	}

	result, err := ct.bc.SelectB2BQuoteShippingRate(ctx, id, shippingMethodID, customName, customCostVal, hasCustomCost)
	if err != nil {
		return shared.ToolError("failed to select shipping rate for B2B quote %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "selected", "quote_id": id, "quote": result})
}

func (ct *CompanyTools) handleQuoteShippingRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "quote_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "remove_b2b_quote_shipping_rate",
			"quote_id": id,
			"message":  fmt.Sprintf("Will remove the shipping method assigned to quote %d. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.RemoveB2BQuoteShippingRate(ctx, id); err != nil {
		return shared.ToolError("failed to remove shipping rate for B2B quote %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "removed", "quote_id": id})
}

func (ct *CompanyTools) handleQuoteShippingCustomMethods(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	methods, err := ct.bc.ListB2BQuoteCustomShippingMethods(ctx)
	if err != nil {
		return shared.ToolError("failed to list B2B quote custom shipping methods: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(methods), "methods": methods})
}
