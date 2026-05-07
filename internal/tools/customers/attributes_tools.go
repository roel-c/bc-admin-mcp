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
	maxAttributeWriteBatch = 10
	maxAttributeDeleteIDs  = 50
)

// validAttributeTypes per BigCommerce schema for /v3/customers/attributes.
var validAttributeTypes = map[string]bool{
	"string": true,
	"number": true,
	"date":   true,
}

// AttributeListSearchFilters maps tool params to /v3/customers/attributes query keys.
var AttributeListSearchFilters = []shared.SearchFilter{
	{ToolKey: "name_like", BCKey: "name:like", Kind: "string"},
	{ToolKey: "name", BCKey: "name", Kind: "string"},
}

var attributeListNonDataKeys = map[string]bool{
	"sort": true, "page": true, "limit": true,
}

// CustomerAttributes provides MCP handlers for /v3/customers/attributes.
type CustomerAttributes struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerAttributes constructs the customer attribute tool handlers.
func NewCustomerAttributes(bc BigCommerceCustomersAPI) *CustomerAttributes {
	return &CustomerAttributes{bc: bc}
}

// RegisterTools registers customers/attributes/{list,create,update,delete}.
func (a *CustomerAttributes) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/attributes/list",
		Tier:    middleware.TierR0,
		Summary: "List customer attribute definitions (V3)",
		Description: "GET /v3/customers/attributes — returns the per-store attribute definitions " +
			"(id, name, type). Supports name and name:like filters. Allow list_all=true to " +
			"page through every attribute when no filter is provided.",
		Tool: mcp.NewTool("customers_attributes_list",
			mcp.WithDescription("List customer attribute definitions. Provide a filter or list_all=true."),
			mcp.WithBoolean("list_all", mcp.Description("Return every attribute (paginated). Required if no filter is given.")),
			mcp.WithArray("attribute_ids", mcp.Description("Filter by attribute id (id:in)."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithString("name", mcp.Description("Exact name match.")),
			mcp.WithString("name_like", mcp.Description("Substring match (name:like).")),
			mcp.WithNumber("page", mcp.Description("Page number.")),
			mcp.WithNumber("limit", mcp.Description("Page size.")),
		),
		Handler: a.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/attributes/create",
		Tier:    middleware.TierR1,
		Summary: "Create customer attribute definitions (V3)",
		Description: fmt.Sprintf(
			"POST /v3/customers/attributes — up to %d rows per call. "+
				"Each row requires name and type (one of string, number, date). "+
				"Type is immutable after creation. Preview first; pass confirmed=true to execute.",
			maxAttributeWriteBatch),
		Tool: mcp.NewTool("customers_attributes_create",
			mcp.WithDescription("Create attribute definitions. Provide attribute_batch or name + type for a single row."),
			mcp.WithArray("attribute_batch", mcp.Description("Array of {name, type} objects."),
				mcp.Items(map[string]any{"type": "object"})),
			mcp.WithString("name", mcp.Description("Single create: attribute name.")),
			mcp.WithString("type", mcp.Description("Single create: one of string, number, date.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: a.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/attributes/update",
		Tier:    middleware.TierR1,
		Summary: "Rename customer attribute definitions (V3)",
		Description: "PUT /v3/customers/attributes — only `name` is mutable. `type` is fixed at creation; " +
			"changing it requires deleting and recreating the attribute (cascades all values). " +
			"Up to " + fmt.Sprintf("%d", maxAttributeWriteBatch) + " rows per call. Preview then confirmed=true.",
		Tool: mcp.NewTool("customers_attributes_update",
			mcp.WithDescription("Rename attribute definitions. Each row must include id and name."),
			mcp.WithArray("attribute_batch", mcp.Description("Array of {id, name} objects."),
				mcp.Items(map[string]any{"type": "object"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: a.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/attributes/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete customer attribute definitions (V3)",
		Description: "DELETE /v3/customers/attributes?id:in=… — irreversible. Deleting an attribute " +
			"removes every value of that attribute on every customer. Preview shows the " +
			"matched attribute names; max " + fmt.Sprintf("%d", maxAttributeDeleteIDs) + " ids per call.",
		Tool: mcp.NewTool("customers_attributes_delete",
			mcp.WithDescription("Delete attribute definitions by id list. Cascades to all stored values."),
			mcp.WithArray("attribute_ids", mcp.Description("Attribute IDs to delete."), mcp.Items(map[string]any{"type": "number"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: a.handleDelete,
	})
}

func (a *CustomerAttributes) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	listAll := shared.ReadBool(args, "list_all")

	params, err := shared.ExtractFilters(args, AttributeListSearchFilters)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if ids, ierr := intSliceFromArgs(args, "attribute_ids"); ierr == nil && len(ids) > 0 {
		params["id:in"] = shared.JoinInts(ids)
	} else if ierr != nil {
		return shared.ToolError("%s", ierr.Error()), nil
	}

	if v, ok := args["page"].(float64); ok && v > 0 {
		params["page"] = fmt.Sprintf("%.0f", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params["limit"] = fmt.Sprintf("%.0f", v)
	}

	hasData := shared.HasDataFilterBCParams(params, AttributeListSearchFilters, attributeListNonDataKeys) || params["id:in"] != ""
	if !listAll && !hasData {
		return shared.ToolError("provide a filter (attribute_ids, name, name_like) or list_all=true."), nil
	}

	attrs, err := a.bc.SearchCustomerAttributes(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list attributes: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(attrs), "attributes": attrs})
}

func (a *CustomerAttributes) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	creates, err := parseAttributeCreates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(creates) == 0 {
		return shared.ToolError("no attributes to create"), nil
	}
	if len(creates) > maxAttributeWriteBatch {
		return shared.ToolError("attribute_batch exceeds max of %d per call", maxAttributeWriteBatch), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create",
			"count":   len(creates),
			"payload": creates,
			"message": "Review payload then pass confirmed=true. type is immutable after create.",
		})
	}

	created, err := a.bc.CreateCustomerAttributes(ctx, creates)
	if err != nil {
		return shared.ToolError("create failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "count": len(created), "attributes": created})
}

func (a *CustomerAttributes) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	updates, err := parseAttributeUpdates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(updates) == 0 {
		return shared.ToolError("attribute_batch must contain at least one row"), nil
	}
	if len(updates) > maxAttributeWriteBatch {
		return shared.ToolError("attribute_batch exceeds max of %d per call", maxAttributeWriteBatch), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "update",
			"count":   len(updates),
			"payload": updates,
			"message": "Review payload then pass confirmed=true. Only `name` is mutable.",
		})
	}

	updated, err := a.bc.UpdateCustomerAttributes(ctx, updates)
	if err != nil {
		return shared.ToolError("update failed: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "count": len(updated), "attributes": updated})
}

func (a *CustomerAttributes) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := requiredPositiveIntIDs(args, "attribute_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > maxAttributeDeleteIDs {
		return shared.ToolError("attribute_ids exceeds max of %d per call", maxAttributeDeleteIDs), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		if err := a.bc.DeleteCustomerAttributes(ctx, ids); err != nil {
			return shared.ToolError("delete failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "deleted", "attribute_ids": ids})
	}

	matched, err := a.bc.GetCustomerAttributesByIDs(ctx, ids)
	if err != nil {
		return shared.ToolError("failed to pre-fetch attributes: %v", err), nil
	}
	type sum struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	sums := make([]sum, 0, len(matched))
	for _, attr := range matched {
		sums = append(sums, sum{ID: attr.ID, Name: attr.Name, Type: attr.Type})
	}
	return shared.ToolJSON(map[string]any{
		"status":             "preview",
		"action":             "delete",
		"would_delete":       len(ids),
		"matched_attributes": sums,
		"message":            "Pass confirmed=true to permanently delete. Cascades to every stored value.",
	})
}

func parseAttributeCreates(args map[string]any) ([]bigcommerce.CustomerAttributeCreate, error) {
	if v, ok := args["attribute_batch"]; ok && v != nil {
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("attribute_batch must be an array")
		}
		out := make([]bigcommerce.CustomerAttributeCreate, 0, len(arr))
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("attribute_batch[%d] must be an object", i)
			}
			name, _ := m["name"].(string)
			typ, _ := m["type"].(string)
			name = strings.TrimSpace(name)
			typ = strings.TrimSpace(typ)
			if name == "" {
				return nil, fmt.Errorf("attribute_batch[%d]: name is required", i)
			}
			if !validAttributeTypes[typ] {
				return nil, fmt.Errorf("attribute_batch[%d]: type must be one of string, number, date", i)
			}
			out = append(out, bigcommerce.CustomerAttributeCreate{Name: name, Type: typ})
		}
		return out, nil
	}

	name, _ := args["name"].(string)
	typ, _ := args["type"].(string)
	name = strings.TrimSpace(name)
	typ = strings.TrimSpace(typ)
	if name == "" || typ == "" {
		return nil, fmt.Errorf("provide attribute_batch or name and type for a single attribute")
	}
	if !validAttributeTypes[typ] {
		return nil, fmt.Errorf("type must be one of string, number, date")
	}
	return []bigcommerce.CustomerAttributeCreate{{Name: name, Type: typ}}, nil
}

func parseAttributeUpdates(args map[string]any) ([]bigcommerce.CustomerAttributeUpdate, error) {
	v, ok := args["attribute_batch"]
	if !ok || v == nil {
		return nil, fmt.Errorf("attribute_batch is required")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("attribute_batch must be an array")
	}
	out := make([]bigcommerce.CustomerAttributeUpdate, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("attribute_batch[%d] must be an object", i)
		}
		idF, idOk := m["id"].(float64)
		if !idOk || int(idF) <= 0 {
			return nil, fmt.Errorf("attribute_batch[%d]: id is required and must be positive", i)
		}
		name, _ := m["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("attribute_batch[%d]: name is required", i)
		}
		if _, hasType := m["type"]; hasType {
			return nil, fmt.Errorf("attribute_batch[%d]: type cannot be changed; delete and recreate the attribute instead", i)
		}
		out = append(out, bigcommerce.CustomerAttributeUpdate{ID: int(idF), Name: name})
	}
	return out, nil
}
