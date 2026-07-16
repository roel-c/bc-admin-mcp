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

// ============================================================
// Payment, credit, and net-terms tools
// ============================================================

func (ct *CompanyTools) registerPaymentTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payments/list",
		Tier:    middleware.TierR0,
		Summary: "List the store's payment method definitions",
		Tool: mcp.NewTool("b2b_payments_list",
			mcp.WithDescription("List the store's B2B Edition payment method definitions (id, code, title). Use b2b/companies/payments/list to see which are enabled for a specific company."),
		),
		Handler: ct.handlePaymentsList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/payments/active_methods",
		Tier:    middleware.TierR0,
		Summary: "List currently-enabled payment methods across companies",
		Tool: mcp.NewTool("b2b_payments_active_methods",
			mcp.WithDescription("List currently-enabled payment methods across all companies (or one company via company_id). If a method is enabled on multiple companies, each appears as a separate row."),
			mcp.WithNumber("company_id", mcp.Description("Filter to one company's active payment methods.")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
			mcp.WithString("sort_by", mcp.Description("updatedAt or createdAt (default updatedAt).")),
			mcp.WithString("order_by", mcp.Description("ASC or DESC (default DESC).")),
			mcp.WithString("q", mcp.Description("Search term.")),
		),
		Handler: ct.handlePaymentsActiveMethods,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/payments/list",
		Tier:    middleware.TierR0,
		Summary: "List payment methods and their enabled state for a company",
		Tool: mcp.NewTool("b2b_companies_payments_list",
			mcp.WithDescription("List payment methods for a company, indicating which are currently enabled for that company's users."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
		),
		Handler: ct.handleCompanyPaymentsList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/credit/get",
		Tier:    middleware.TierR0,
		Summary: "Get a company's credit settings",
		Tool: mcp.NewTool("b2b_companies_credit_get",
			mcp.WithDescription("Get a company's credit settings: whether credit is enabled, available credit, purchase limit behavior, and credit-hold status. Fails if the store's Company Credit feature is disabled."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
		),
		Handler: ct.handleCompanyCreditGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/payment_terms/get",
		Tier:    middleware.TierR0,
		Summary: "Get a company's net-terms (payment-on-terms) settings",
		Tool: mcp.NewTool("b2b_companies_payment_terms_get",
			mcp.WithDescription("Get a company's net-terms configuration (e.g. Net 30/45/60). If disabled for the company, paymentTerms reflects the store-level default."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
		),
		Handler: ct.handleCompanyPaymentTermsGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/payments/update",
		Tier:    middleware.TierR2,
		Summary: "Enable or disable payment methods for a company",
		Tool: mcp.NewTool("b2b_companies_payments_update",
			mcp.WithDescription("Enable or disable payment methods for a company. Only the methods listed in updates_json are affected. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithString("updates_json", mcp.Description(`JSON array: [{"code":"cheque","isEnabled":true}]. Use b2b/companies/payments/list to discover codes.`), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleCompanyPaymentsUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/credit/update",
		Tier:    middleware.TierR2,
		Summary: "Update a company's credit settings",
		Tool: mcp.NewTool("b2b_companies_credit_update",
			mcp.WithDescription("Update a company's credit settings (enable/disable credit, available credit, purchase-limit behavior, credit hold). Fails if the store's Company Credit feature is disabled. All fields optional but an empty request causes unexpected behavior — send the full current state (from b2b/companies/credit/get) with your changes applied. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithBoolean("credit_enabled", mcp.Description("Whether credit is enabled for this company.")),
			mcp.WithString("credit_currency", mcp.Description("Currency code for the credit limit.")),
			mcp.WithNumber("available_credit", mcp.Description("Available credit amount.")),
			mcp.WithBoolean("limit_purchases", mcp.Description("Whether purchases are limited to available credit.")),
			mcp.WithBoolean("credit_hold", mcp.Description("Whether the company is on credit hold (blocks all purchases if true).")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleCompanyCreditUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/payment_terms/update",
		Tier:    middleware.TierR2,
		Summary: "Update a company's net-terms settings",
		Tool: mcp.NewTool("b2b_companies_payment_terms_update",
			mcp.WithDescription("Update a company's net-terms configuration. payment_terms is ignored (defaults to the store-level value) when is_enabled is false. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithBoolean("is_enabled", mcp.Description("Whether payment on terms is available for this company."), mcp.Required()),
			mcp.WithString("payment_terms", mcp.Description("One of: 0, 5, 15, 30, 45, 60 (days)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleCompanyPaymentTermsUpdate,
	})
}

func (ct *CompanyTools) handlePaymentsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	methods, err := ct.bc.ListB2BPaymentMethods(ctx)
	if err != nil {
		return shared.ToolError("failed to list B2B payment methods: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(methods), "payment_methods": methods})
}

func (ct *CompanyTools) handlePaymentsActiveMethods(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["company_id"].(float64); ok && v > 0 {
		params.Set("companyId", fmt.Sprintf("%d", int(v)))
	}
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
	methods, err := ct.bc.ListB2BActivePaymentMethods(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B active payment methods: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(methods), "active_methods": methods})
}

func (ct *CompanyTools) handleCompanyPaymentsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	methods, err := ct.bc.ListB2BCompanyPaymentMethods(ctx, id)
	if err != nil {
		return shared.ToolError("failed to list payment methods for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"company_id": id, "total": len(methods), "payment_methods": methods})
}

func (ct *CompanyTools) handleCompanyCreditGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	credit, err := ct.bc.GetB2BCompanyCredit(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get credit for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"company_id": id, "credit": credit})
}

func (ct *CompanyTools) handleCompanyPaymentTermsGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	terms, err := ct.bc.GetB2BCompanyPaymentTerms(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get payment terms for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"company_id": id, "payment_terms": terms})
}

func (ct *CompanyTools) handleCompanyPaymentsUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	raw, _ := args["updates_json"].(string)
	if strings.TrimSpace(raw) == "" {
		return shared.ToolError("updates_json is required (a JSON array of {code, isEnabled} objects)"), nil
	}
	var updates []bigcommerce.B2BCompanyPaymentMethodUpdate
	if uerr := json.Unmarshal([]byte(raw), &updates); uerr != nil {
		return shared.ToolError("invalid updates_json: %v", uerr), nil
	}
	if len(updates) == 0 {
		return shared.ToolError("updates_json must contain at least one {code, isEnabled} entry"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "update_b2b_company_payment_methods",
			"company_id": id,
			"updates":    updates,
			"message":    fmt.Sprintf("Will apply %d payment method change(s) for company %d. Pass confirmed=true.", len(updates), id),
		})
	}

	if err := ct.bc.UpdateB2BCompanyPaymentMethods(ctx, id, updates); err != nil {
		return shared.ToolError("failed to update payment methods for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "company_id": id})
}

func (ct *CompanyTools) handleCompanyCreditUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload := bigcommerce.B2BCompanyCredit{}
	if v, ok := args["credit_enabled"].(bool); ok {
		payload.CreditEnabled = v
	}
	if v, ok := args["credit_currency"].(string); ok {
		payload.CreditCurrency = v
	}
	if v, ok := args["available_credit"].(float64); ok {
		payload.AvailableCredit = &v
	}
	if v, ok := args["limit_purchases"].(bool); ok {
		payload.LimitPurchases = v
	}
	if v, ok := args["credit_hold"].(bool); ok {
		payload.CreditHold = v
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "update_b2b_company_credit",
			"company_id": id,
			"payload":    payload,
			"message":    fmt.Sprintf("Will apply these credit settings to company %d. Pass confirmed=true.", id),
		})
	}

	result, err := ct.bc.UpdateB2BCompanyCredit(ctx, id, payload)
	if err != nil {
		return shared.ToolError("failed to update credit for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "company_id": id, "credit": result})
}

func (ct *CompanyTools) handleCompanyPaymentTermsUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	isEnabled, ok := args["is_enabled"].(bool)
	if !ok {
		return shared.ToolError("is_enabled is required"), nil
	}
	paymentTerms, _ := args["payment_terms"].(string)
	if strings.TrimSpace(paymentTerms) == "" {
		return shared.ToolError("payment_terms is required (one of 0, 5, 15, 30, 45, 60)"), nil
	}
	validTerms := map[string]bool{"0": true, "5": true, "15": true, "30": true, "45": true, "60": true}
	if !validTerms[paymentTerms] {
		return shared.ToolError("payment_terms must be one of 0, 5, 15, 30, 45, 60 (got %q)", paymentTerms), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":        "preview",
			"action":        "update_b2b_company_payment_terms",
			"company_id":    id,
			"is_enabled":    isEnabled,
			"payment_terms": paymentTerms,
			"message":       fmt.Sprintf("Will set payment terms for company %d (enabled=%v, terms=%s days). Pass confirmed=true.", id, isEnabled, paymentTerms),
		})
	}

	result, err := ct.bc.UpdateB2BCompanyPaymentTerms(ctx, id, isEnabled, paymentTerms)
	if err != nil {
		return shared.ToolError("failed to update payment terms for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "company_id": id, "payment_terms": result})
}
