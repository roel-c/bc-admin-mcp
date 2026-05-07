package customers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// CustomerStoredInstruments lists GET /v3/customers/{id}/stored-instruments with
// double acknowledgement before returning raw payment token identifiers.
type CustomerStoredInstruments struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerStoredInstruments constructs stored-instruments tool handlers.
func NewCustomerStoredInstruments(bc BigCommerceCustomersAPI) *CustomerStoredInstruments {
	return &CustomerStoredInstruments{bc: bc}
}

// RegisterTools registers customers/stored_instruments/list.
func (s *CustomerStoredInstruments) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/stored_instruments/list",
		Tier:    middleware.TierR0,
		Summary: "List stored payment instruments for a customer (V3)",
		Description: "GET /v3/customers/{customerId}/stored-instruments — requires OAuth scope store_stored_payment_instruments*. " +
			"Gate 1: pass acknowledge_stored_instruments=true to fetch the list. " +
			"By default sensitive `token` fields are redacted. Gate 2: to return raw tokens pass include_sensitive_token_data=true " +
			"AND confirmed=true after reviewing the redacted preview (payment-adjacent data).",
		Tool: mcp.NewTool("customers_stored_instruments_list",
			mcp.WithDescription("List stored instruments with acknowledgement gates."),
			mcp.WithNumber("customer_id", mcp.Description("Customer ID."), mcp.Required()),
			mcp.WithBoolean("acknowledge_stored_instruments", mcp.Description("Must be true to call the API (confirms intent to access payment instrument metadata).")),
			mcp.WithBoolean("include_sensitive_token_data", mcp.Description("When true with confirmed=true, include raw token fields from BigCommerce (high sensitivity).")),
			mcp.WithBoolean("confirmed", mcp.Description("Required together with include_sensitive_token_data=true to return raw tokens.")),
		),
		Handler: s.handleList,
	})
}

func (s *CustomerStoredInstruments) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "customer_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !shared.ReadBool(args, "acknowledge_stored_instruments") {
		return shared.ToolError(
			"set acknowledge_stored_instruments=true to confirm you intend to access this customer's stored payment instruments (gate 1).",
		), nil
	}

	raw, err := s.bc.ListCustomerStoredInstruments(ctx, id)
	if err != nil {
		return shared.ToolError("failed to list stored instruments: %v", err), nil
	}

	wantTokens := shared.ReadBool(args, "include_sensitive_token_data")
	confirmed := middleware.IsConfirmedFromArgs(args)

	if wantTokens && !confirmed {
		redacted, rerr := redactInstrumentRows(raw)
		if rerr != nil {
			return shared.ToolError("%s", rerr.Error()), nil
		}
		return shared.ToolJSON(map[string]any{
			"status":                    "pending_confirmation",
			"customer_id":               id,
			"total":                     len(redacted),
			"stored_instruments":        redacted,
			"message":                   "Redacted preview shown. Pass include_sensitive_token_data=true and confirmed=true to return raw token fields (gate 2).",
			"sensitive_fields_redacted": true,
		})
	}

	if wantTokens && confirmed {
		parsed := make([]any, 0, len(raw))
		for _, row := range raw {
			var m map[string]any
			if err := json.Unmarshal(row, &m); err != nil {
				return shared.ToolError("parse instrument: %v", err), nil
			}
			parsed = append(parsed, m)
		}
		return shared.ToolJSON(map[string]any{
			"status":             "ok",
			"customer_id":        id,
			"total":              len(parsed),
			"stored_instruments": parsed,
			"warning":            "Raw token data included — handle as PCI-sensitive per your policy.",
		})
	}

	redacted, rerr := redactInstrumentRows(raw)
	if rerr != nil {
		return shared.ToolError("%s", rerr.Error()), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":                    "ok",
		"customer_id":               id,
		"total":                     len(redacted),
		"stored_instruments":        redacted,
		"sensitive_fields_redacted": true,
	})
}

func redactInstrumentRows(raw []json.RawMessage) ([]any, error) {
	out := make([]any, 0, len(raw))
	for _, row := range raw {
		var m map[string]any
		if err := json.Unmarshal(row, &m); err != nil {
			return nil, fmt.Errorf("parse instrument row: %w", err)
		}
		cp := make(map[string]any, len(m)+1)
		for k, v := range m {
			if k == "token" {
				cp[k] = "(redacted)"
				continue
			}
			cp[k] = v
		}
		out = append(out, cp)
	}
	return out, nil
}
