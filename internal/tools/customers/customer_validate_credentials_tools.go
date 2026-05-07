package customers

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// CustomerValidateCredentials handles POST /v3/customers/validate-credentials.
// This endpoint is rate-limited by BigCommerce; use sparingly.
type CustomerValidateCredentials struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerValidateCredentials constructs the validate-credentials tool.
func NewCustomerValidateCredentials(bc BigCommerceCustomersAPI) *CustomerValidateCredentials {
	return &CustomerValidateCredentials{bc: bc}
}

// RegisterTools registers customers/credentials/validate.
func (v *CustomerValidateCredentials) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/credentials/validate",
		Tier:    middleware.TierR2,
		Summary: "Check whether an email/password pair is valid for login (V3)",
		Description: "POST /v3/customers/validate-credentials — verifies credentials against a channel (defaults to channel 1). " +
			"This endpoint has strict anti-abuse rate limits (429). Password is never echoed in responses. " +
			"Preview first; pass confirmed=true to execute (R2).",
		Tool: mcp.NewTool("customers_credentials_validate",
			mcp.WithDescription("Validate customer login credentials. Preview then confirmed=true."),
			mcp.WithString("email", mcp.Description("Customer email."), mcp.Required()),
			mcp.WithString("password", mcp.Description("Plain-text password to validate (never returned in tool output)."), mcp.Required()),
			mcp.WithNumber("channel_id", mcp.Description("Optional channel to validate against (BigCommerce defaults to 1 if omitted).")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: v.handleValidate,
	})
}

func (v *CustomerValidateCredentials) handleValidate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	emailRaw, ok := args["email"]
	if !ok {
		return shared.ToolError("email is required"), nil
	}
	email, ok := emailRaw.(string)
	if !ok || email == "" {
		return shared.ToolError("email must be a non-empty string"), nil
	}
	pwRaw, ok := args["password"]
	if !ok {
		return shared.ToolError("password is required"), nil
	}
	password, ok := pwRaw.(string)
	if !ok {
		return shared.ToolError("password must be a string"), nil
	}

	var chPtr *int
	if v, ok := args["channel_id"].(float64); ok && int(v) > 0 {
		cid := int(v)
		chPtr = &cid
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":       "preview",
			"action":       "validate_credentials",
			"email":        maskEmailForPreview(email),
			"channel_id":   chPtr,
			"message":      "Will POST validate-credentials to BigCommerce (rate limited). Pass confirmed=true to execute; password is never echoed.",
			"rate_warning": "Avoid repeated calls; BigCommerce may return 429 if abused.",
		})
	}

	req := bigcommerce.ValidateCustomerCredentialsRequest{Email: email, Password: password, ChannelID: chPtr}
	out, err := v.bc.ValidateCustomerCredentials(ctx, req)
	if err != nil {
		return shared.ToolError("validate credentials failed: %v", err), nil
	}
	resp := map[string]any{"is_valid": out.IsValid}
	if out.CustomerID != nil {
		resp["customer_id"] = *out.CustomerID
	}
	return shared.ToolJSON(map[string]any{
		"status":     "completed",
		"result":     resp,
		"email":      maskEmailForPreview(email),
		"channel_id": chPtr,
		"message":    "Password was not returned. Handle is_valid and customer_id per your security policy.",
	})
}
