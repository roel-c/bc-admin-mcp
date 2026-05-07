package customers

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

var validConsentCategories = map[string]bool{
	"essential":  true,
	"functional": true,
	"analytics":  true,
	"targeting":  true,
}

// CustomerConsentTools provides GET/PUT /v3/customers/{id}/consent handlers.
type CustomerConsentTools struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerConsentTools constructs consent tool handlers.
func NewCustomerConsentTools(bc BigCommerceCustomersAPI) *CustomerConsentTools {
	return &CustomerConsentTools{bc: bc}
}

// RegisterTools registers customers/consent/get and customers/consent/update.
func (c *CustomerConsentTools) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "customers/consent/get",
		Tier:        middleware.TierR0,
		Summary:     "Get a customer cookie/consent preferences (V3)",
		Description: "GET /v3/customers/{customerId}/consent — allow/deny lists for essential, functional, analytics, targeting.",
		Tool: mcp.NewTool("customers_consent_get",
			mcp.WithDescription("Fetch consent for one customer."),
			mcp.WithNumber("customer_id", mcp.Description("Customer ID."), mcp.Required()),
		),
		Handler: c.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "customers/consent/update",
		Tier:        middleware.TierR1,
		Summary:     "Update a customer cookie/consent preferences (V3)",
		Description: "PUT /v3/customers/{customerId}/consent — supply allow and/or deny as category name arrays. Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("customers_consent_update",
			mcp.WithDescription("Update consent. At least one of allow or deny must be non-empty."),
			mcp.WithNumber("customer_id", mcp.Description("Customer ID."), mcp.Required()),
			mcp.WithArray("allow", mcp.Description("Categories to allow: essential, functional, analytics, targeting."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithArray("deny", mcp.Description("Categories to deny."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: c.handleUpdate,
	})
}

func (c *CustomerConsentTools) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "customer_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	data, err := c.bc.GetCustomerConsent(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get consent: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"customer_id": id, "consent": data})
}

func (c *CustomerConsentTools) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "customer_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	allow, err := parseConsentCategorySlice(args, "allow")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	deny, err := parseConsentCategorySlice(args, "deny")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(allow) == 0 && len(deny) == 0 {
		return shared.ToolError("provide at least one category in allow or deny"), nil
	}
	if err := validateConsentCategories(allow); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if err := validateConsentCategories(deny); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	req := bigcommerce.DeclareCustomerConsentRequest{Allow: allow, Deny: deny}

	if !middleware.IsConfirmedFromArgs(args) {
		cur, gerr := c.bc.GetCustomerConsent(ctx, id)
		if gerr != nil {
			return shared.ToolError("failed to read current consent for preview: %v", gerr), nil
		}
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "update_consent",
			"customer_id": id,
			"current":     cur,
			"would_apply": req,
			"message":     "Review would_apply then pass confirmed=true to execute.",
		})
	}

	out, err := c.bc.UpdateCustomerConsent(ctx, id, req)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "customer_id": id, "consent": out})
}

func parseConsentCategorySlice(args map[string]any, key string) ([]string, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
	out := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string", key, i)
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out, nil
}

func validateConsentCategories(vals []string) error {
	for i, v := range vals {
		v = strings.TrimSpace(v)
		if v == "" {
			return fmt.Errorf("consent category at index %d is empty", i)
		}
		if !validConsentCategories[v] {
			return fmt.Errorf("invalid consent category %q (allowed: essential, functional, analytics, targeting)", v)
		}
	}
	return nil
}
