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

const (
	maxAttributeValueWriteBatch = 10
	maxAttributeValueDeleteIDs  = 50
)

// AttributeValueListSearchFilters maps tool params to /v3/customers/attribute-values keys.
var AttributeValueListSearchFilters = []shared.SearchFilter{
	{ToolKey: "customer_id_in", BCKey: "customer_id:in", Kind: "string"},
	{ToolKey: "attribute_id_in", BCKey: "attribute_id:in", Kind: "string"},
	{ToolKey: "attribute_value", BCKey: "attribute_value", Kind: "string"},
	{ToolKey: "attribute_value_in", BCKey: "attribute_value:in", Kind: "string"},
}

var attributeValueListNonDataKeys = map[string]bool{
	"sort": true, "page": true, "limit": true,
}

// CustomerAttributeValues provides MCP handlers for /v3/customers/attribute-values.
type CustomerAttributeValues struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerAttributeValues constructs the customer attribute value tool handlers.
func NewCustomerAttributeValues(bc BigCommerceCustomersAPI) *CustomerAttributeValues {
	return &CustomerAttributeValues{bc: bc}
}

// RegisterTools registers customers/attribute_values/{list,upsert,delete}.
func (a *CustomerAttributeValues) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/attribute_values/list",
		Tier:    middleware.TierR0,
		Summary: "List stored customer attribute values (V3)",
		Description: "GET /v3/customers/attribute-values — returns the per-customer values keyed by " +
			"(customer_id, attribute_id). Provide at least one of customer_ids, attribute_ids, " +
			"or attribute_value to scope the query (full-store scans should pass list_all=true).",
		Tool: mcp.NewTool("customers_attribute_values_list",
			mcp.WithDescription("List attribute values. Provide a filter or list_all=true."),
			mcp.WithBoolean("list_all", mcp.Description("Return every value (paginated). Required if no filter is given.")),
			mcp.WithArray("customer_ids", mcp.Description("customer_id:in filter."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithArray("attribute_ids", mcp.Description("attribute_id:in filter."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithString("attribute_value", mcp.Description("Exact attribute_value match.")),
			mcp.WithString("attribute_value_in", mcp.Description("attribute_value:in (comma list).")),
			mcp.WithNumber("page", mcp.Description("Page number.")),
			mcp.WithNumber("limit", mcp.Description("Page size.")),
		),
		Handler: a.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/attribute_values/upsert",
		Tier:    middleware.TierR1,
		Summary: "Set or replace customer attribute values (V3)",
		Description: fmt.Sprintf(
			"PUT /v3/customers/attribute-values — upsert by (customer_id, attribute_id). "+
				"Up to %d rows per call. Each row requires customer_id, attribute_id, and value. "+
				"BigCommerce auto-coerces value to the attribute's declared type. Preview then confirmed=true.",
			maxAttributeValueWriteBatch),
		Tool: mcp.NewTool("customers_attribute_values_upsert",
			mcp.WithDescription("Upsert attribute values. Provide value_batch (array of {customer_id, attribute_id, value})."),
			mcp.WithArray("value_batch", mcp.Description("Rows of {customer_id, attribute_id, value}."),
				mcp.Items(map[string]any{"type": "object"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: a.handleUpsert,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/attribute_values/delete",
		Tier:    middleware.TierR2,
		Summary: "Delete customer attribute values by id (V3)",
		Description: "DELETE /v3/customers/attribute-values?id:in=… — removes individual stored values " +
			"(does not affect the attribute definition). Max " +
			fmt.Sprintf("%d", maxAttributeValueDeleteIDs) + " ids per call. Preview then confirmed=true.",
		Tool: mcp.NewTool("customers_attribute_values_delete",
			mcp.WithDescription("Delete attribute values by id list."),
			mcp.WithArray("value_ids", mcp.Description("Attribute value IDs to delete."), mcp.Items(map[string]any{"type": "number"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: a.handleDelete,
	})
}

func (a *CustomerAttributeValues) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	listAll := shared.ReadBool(args, "list_all")

	params, err := shared.ExtractFilters(args, AttributeValueListSearchFilters)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if ids, ierr := intSliceFromArgs(args, "customer_ids"); ierr == nil && len(ids) > 0 {
		params["customer_id:in"] = shared.JoinInts(ids)
	} else if ierr != nil {
		return shared.ToolError("%s", ierr.Error()), nil
	}
	if ids, ierr := intSliceFromArgs(args, "attribute_ids"); ierr == nil && len(ids) > 0 {
		params["attribute_id:in"] = shared.JoinInts(ids)
	} else if ierr != nil {
		return shared.ToolError("%s", ierr.Error()), nil
	}

	if v, ok := args["page"].(float64); ok && v > 0 {
		params["page"] = fmt.Sprintf("%.0f", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params["limit"] = fmt.Sprintf("%.0f", v)
	}

	hasData := shared.HasDataFilterBCParams(params, AttributeValueListSearchFilters, attributeValueListNonDataKeys) ||
		params["customer_id:in"] != "" || params["attribute_id:in"] != ""
	if !listAll && !hasData {
		return shared.ToolError("provide a filter (customer_ids, attribute_ids, attribute_value, attribute_value_in) or list_all=true."), nil
	}

	values, err := a.bc.SearchCustomerAttributeValues(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list attribute values: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(values), "attribute_values": values})
}

func (a *CustomerAttributeValues) handleUpsert(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	upserts, err := parseAttributeValueUpserts(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(upserts) == 0 {
		return shared.ToolError("value_batch must contain at least one row"), nil
	}
	if len(upserts) > maxAttributeValueWriteBatch {
		return shared.ToolError("value_batch exceeds max of %d per call", maxAttributeValueWriteBatch), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "upsert",
			"count":   len(upserts),
			"payload": upserts,
			"message": "Review payload then pass confirmed=true.",
		})
	}

	saved, err := a.bc.UpsertCustomerAttributeValues(ctx, upserts)
	if err != nil {
		return shared.ToolError("upsert failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "upserted", "count": len(saved), "attribute_values": saved})
}

func (a *CustomerAttributeValues) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := requiredPositiveIntIDs(args, "value_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > maxAttributeValueDeleteIDs {
		return shared.ToolError("value_ids exceeds max of %d per call", maxAttributeValueDeleteIDs), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		if err := a.bc.DeleteCustomerAttributeValues(ctx, ids); err != nil {
			return shared.ToolError("delete failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "deleted", "value_ids": ids})
	}

	return shared.ToolJSON(map[string]any{
		"status":       "preview",
		"action":       "delete",
		"would_delete": len(ids),
		"value_ids":    ids,
		"message":      "Pass confirmed=true to permanently delete these attribute values.",
	})
}

func parseAttributeValueUpserts(args map[string]any) ([]bigcommerce.CustomerAttributeValueUpsert, error) {
	v, ok := args["value_batch"]
	if !ok || v == nil {
		return nil, fmt.Errorf("value_batch is required")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("value_batch must be an array")
	}
	out := make([]bigcommerce.CustomerAttributeValueUpsert, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("value_batch[%d] must be an object", i)
		}
		cidF, ok := m["customer_id"].(float64)
		if !ok || int(cidF) <= 0 {
			return nil, fmt.Errorf("value_batch[%d]: customer_id is required and must be positive", i)
		}
		aidF, ok := m["attribute_id"].(float64)
		if !ok || int(aidF) <= 0 {
			return nil, fmt.Errorf("value_batch[%d]: attribute_id is required and must be positive", i)
		}
		val, ok := m["value"].(string)
		if !ok {
			return nil, fmt.Errorf("value_batch[%d]: value must be a string", i)
		}
		val = strings.TrimSpace(val)
		out = append(out, bigcommerce.CustomerAttributeValueUpsert{
			CustomerID:  int(cidF),
			AttributeID: int(aidF),
			Value:       val,
		})
	}
	return out, nil
}
