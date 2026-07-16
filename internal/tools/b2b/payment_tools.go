package b2b

import (
	"context"
	"fmt"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ============================================================
// Payment, credit, and net-terms tools (read-only)
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
