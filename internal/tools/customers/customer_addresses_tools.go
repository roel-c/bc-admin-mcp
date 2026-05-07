package customers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

const (
	maxAddressWriteBatch = 25
	maxAddressDeleteIDs  = 50
)

// AddressListSearchFilters maps tool params to GET /v3/customers/addresses.
var AddressListSearchFilters = []shared.SearchFilter{
	{ToolKey: "company_in", BCKey: "company:in", Kind: "string"},
	{ToolKey: "name_in", BCKey: "name:in", Kind: "string"},
}

var addressListNonDataKeys = map[string]bool{
	"page": true, "limit": true, "include": true,
}

// CustomerAddresses provides MCP handlers for /v3/customers/addresses.
type CustomerAddresses struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerAddresses constructs customer address tool handlers.
func NewCustomerAddresses(bc BigCommerceCustomersAPI) *CustomerAddresses {
	return &CustomerAddresses{bc: bc}
}

// RegisterTools registers customers/addresses/list, create, update, delete.
func (a *CustomerAddresses) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/addresses/list",
		Tier:    middleware.TierR0,
		Summary: "List customer addresses (V3)",
		Description: "GET /v3/customers/addresses. Requires at least one filter (customer_id, address_ids, " +
			"company_in, name_in) or list_all=true.",
		Tool: mcp.NewTool("customers_addresses_list",
			mcp.WithDescription("List addresses with filters or list_all=true."),
			mcp.WithBoolean("list_all", mcp.Description("When true, lists addresses without other filters (paginated).")),
			mcp.WithNumber("customer_id", mcp.Description("Filter by a single customer_id (sent as customer_id:in).")),
			mcp.WithArray("address_ids", mcp.Description("Address IDs; sent as id:in."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithString("company_in", mcp.Description("company:in filter.")),
			mcp.WithString("name_in", mcp.Description("name:in filter.")),
			mcp.WithString("include", mcp.Description("Optional: formfields.")),
			mcp.WithNumber("page", mcp.Description("Page number.")),
			mcp.WithNumber("limit", mcp.Description("Page size.")),
		),
		Handler: a.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/addresses/create",
		Tier:    middleware.TierR1,
		Summary: "Create customer address(es) (V3)",
		Description: "POST /v3/customers/addresses — up to " + fmt.Sprintf("%d", maxAddressWriteBatch) +
			" rows per call. Preview first; confirmed=true to execute.",
		Tool: mcp.NewTool("customers_addresses_create",
			mcp.WithDescription("Create one or more addresses (address_batch array)."),
			mcp.WithArray("address_batch", mcp.Description("Array of address objects (customer_id, first_name, last_name, address1, city, state_or_province, postal_code, country_code, …)."),
				mcp.Items(map[string]any{"type": "object"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: a.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/addresses/update",
		Tier:    middleware.TierR1,
		Summary: "Update customer address(es) (V3)",
		Description: "PUT /v3/customers/addresses — each row must include id. Max " +
			fmt.Sprintf("%d", maxAddressWriteBatch) + " per call.",
		Tool: mcp.NewTool("customers_addresses_update",
			mcp.WithDescription("Update addresses (address_batch with id on each row)."),
			mcp.WithArray("address_batch", mcp.Description("Array of address update objects."), mcp.Items(map[string]any{"type": "object"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: a.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/addresses/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete customer address(es) by ID (V3)",
		Description: "DELETE /v3/customers/addresses?id:in=… — max " + fmt.Sprintf("%d", maxAddressDeleteIDs) +
			" IDs per call. Preview first; confirmed=true to execute.",
		Tool: mcp.NewTool("customers_addresses_delete",
			mcp.WithDescription("Delete addresses by id list."),
			mcp.WithArray("address_ids", mcp.Description("Address IDs to delete."), mcp.Items(map[string]any{"type": "number"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: a.handleDelete,
	})
}

func (a *CustomerAddresses) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	listAll := shared.ReadBool(args, "list_all")

	params, err := shared.ExtractFilters(args, AddressListSearchFilters)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if v, ok := args["customer_id"].(float64); ok && v > 0 {
		params["customer_id:in"] = fmt.Sprintf("%.0f", v)
	}

	if ids, ierr := intSliceFromArgs(args, "address_ids"); ierr == nil && len(ids) > 0 {
		params["id:in"] = shared.JoinInts(ids)
	} else if ierr != nil {
		return shared.ToolError("%s", ierr.Error()), nil
	}

	if v, ok := args["include"].(string); ok && strings.TrimSpace(v) != "" {
		params["include"] = strings.TrimSpace(v)
	}
	if v, ok := args["page"].(float64); ok && v > 0 {
		params["page"] = fmt.Sprintf("%.0f", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params["limit"] = fmt.Sprintf("%.0f", v)
	}

	hasData := shared.HasDataFilterBCParams(params, AddressListSearchFilters, addressListNonDataKeys) ||
		params["customer_id:in"] != "" || params["id:in"] != ""
	if !listAll && !hasData {
		return shared.ToolError("provide customer_id, address_ids, company_in, name_in, or list_all=true"), nil
	}

	addrs, err := a.bc.SearchCustomerAddresses(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list addresses: %v", err), nil
	}

	type row struct {
		ID         int    `json:"id"`
		CustomerID int    `json:"customer_id"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		City       string `json:"city"`
		Country    string `json:"country_code"`
	}
	out := make([]row, 0, len(addrs))
	for _, ad := range addrs {
		out = append(out, row{
			ID: ad.ID, CustomerID: ad.CustomerID, FirstName: ad.FirstName, LastName: ad.LastName,
			City: ad.City, Country: ad.CountryCode,
		})
	}
	return shared.ToolJSON(map[string]any{"total": len(out), "addresses": out})
}

func (a *CustomerAddresses) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	batch, err := parseAddressCreates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(batch) > maxAddressWriteBatch {
		return shared.ToolError("address_batch exceeds max of %d", maxAddressWriteBatch), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		created, err := a.bc.CreateCustomerAddresses(ctx, batch)
		if err != nil {
			return shared.ToolError("create failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "created", "count": len(created), "addresses": created})
	}
	return shared.ToolJSON(map[string]any{
		"status":  "preview",
		"action":  "create",
		"count":   len(batch),
		"payload": batch,
		"message": "Pass confirmed=true to create these addresses.",
	})
}

func (a *CustomerAddresses) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	batch, err := parseAddressUpdates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(batch) > maxAddressWriteBatch {
		return shared.ToolError("address_batch exceeds max of %d", maxAddressWriteBatch), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		updated, err := a.bc.UpdateCustomerAddresses(ctx, batch)
		if err != nil {
			return shared.ToolError("update failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "updated", "count": len(updated), "addresses": updated})
	}
	return shared.ToolJSON(map[string]any{
		"status":  "preview",
		"action":  "update",
		"count":   len(batch),
		"payload": batch,
		"message": "Pass confirmed=true to apply address updates.",
	})
}

func (a *CustomerAddresses) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := requiredPositiveIntIDs(args, "address_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) > maxAddressDeleteIDs {
		return shared.ToolError("address_ids exceeds max of %d", maxAddressDeleteIDs), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		if err := a.bc.DeleteCustomerAddresses(ctx, ids); err != nil {
			return shared.ToolError("delete failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "deleted", "address_ids": ids})
	}

	params := map[string]string{"id:in": shared.JoinInts(ids)}
	addrs, err := a.bc.SearchCustomerAddresses(ctx, params)
	if err != nil {
		return shared.ToolError("failed to pre-fetch addresses: %v", err), nil
	}
	type sum struct {
		ID         int    `json:"id"`
		CustomerID int    `json:"customer_id"`
		Line       string `json:"line"`
	}
	sums := make([]sum, 0, len(addrs))
	for _, ad := range addrs {
		sums = append(sums, sum{
			ID: ad.ID, CustomerID: ad.CustomerID,
			Line: strings.TrimSpace(ad.Address1 + ", " + ad.City),
		})
	}
	return shared.ToolJSON(map[string]any{
		"status":            "preview",
		"action":            "delete",
		"would_delete":      len(ids),
		"matched_addresses": sums,
		"message":           "Pass confirmed=true to permanently delete these addresses.",
	})
}

func parseAddressCreates(args map[string]any) ([]bigcommerce.CustomerAddressCreate, error) {
	v, ok := args["address_batch"]
	if !ok || v == nil {
		return nil, fmt.Errorf("address_batch is required")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("address_batch must be an array")
	}
	out := make([]bigcommerce.CustomerAddressCreate, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("address_batch[%d] must be an object", i)
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("address_batch[%d]: %w", i, err)
		}
		var c bigcommerce.CustomerAddressCreate
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("address_batch[%d]: %w", i, err)
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("address_batch must contain at least one row")
	}
	return out, nil
}

func parseAddressUpdates(args map[string]any) ([]bigcommerce.CustomerAddressUpdate, error) {
	v, ok := args["address_batch"]
	if !ok || v == nil {
		return nil, fmt.Errorf("address_batch is required")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("address_batch must be an array")
	}
	out := make([]bigcommerce.CustomerAddressUpdate, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("address_batch[%d] must be an object", i)
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("address_batch[%d]: %w", i, err)
		}
		var u bigcommerce.CustomerAddressUpdate
		if err := json.Unmarshal(b, &u); err != nil {
			return nil, fmt.Errorf("address_batch[%d]: %w", i, err)
		}
		if u.ID == 0 {
			return nil, fmt.Errorf("address_batch[%d]: id is required", i)
		}
		out = append(out, u)
	}
	return out, nil
}
