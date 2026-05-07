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
	maxCustomerWriteBatch   = 10
	maxCustomerDeleteIDs    = 50
	maxAssignGroupCustomers = 100
)

// CustomerListSearchFilters maps tool parameters to GET /v3/customers query keys.
var CustomerListSearchFilters = []shared.SearchFilter{
	{ToolKey: "company_in", BCKey: "company:in", Kind: "string"},
	{ToolKey: "customer_group_id_in", BCKey: "customer_group_id:in", Kind: "string"},
	{ToolKey: "date_created", BCKey: "date_created", Kind: "string"},
	{ToolKey: "date_created_min", BCKey: "date_created:min", Kind: "string"},
	{ToolKey: "date_created_max", BCKey: "date_created:max", Kind: "string"},
	{ToolKey: "date_modified", BCKey: "date_modified", Kind: "string"},
	{ToolKey: "date_modified_min", BCKey: "date_modified:min", Kind: "string"},
	{ToolKey: "date_modified_max", BCKey: "date_modified:max", Kind: "string"},
	{ToolKey: "email_in", BCKey: "email:in", Kind: "string"},
	{ToolKey: "name_in", BCKey: "name:in", Kind: "string"},
	{ToolKey: "name_like", BCKey: "name:like", Kind: "string"},
	{ToolKey: "phone_in", BCKey: "phone:in", Kind: "string"},
	{ToolKey: "registration_ip_in", BCKey: "registration_ip_address:in", Kind: "string"},
}

var customerListNonDataKeys = map[string]bool{
	"sort": true, "include": true, "page": true, "limit": true, "after": true, "before": true,
}

var validCustomerSort = map[string]bool{
	"date_created:asc": true, "date_created:desc": true,
	"last_name:asc": true, "last_name:desc": true,
	"date_modified:asc": true, "date_modified:desc": true,
}

// CustomerRecords provides MCP handlers for GET/POST/PUT/DELETE /v3/customers
// and assign_group (batch PUT for customer_group_id).
type CustomerRecords struct {
	bc BigCommerceCustomersAPI
}

// NewCustomerRecords constructs V3 customer record tool handlers.
func NewCustomerRecords(bc BigCommerceCustomersAPI) *CustomerRecords {
	return &CustomerRecords{bc: bc}
}

// RegisterTools registers customers/list, get, create, update, delete, assign_group.
func (c *CustomerRecords) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/list",
		Tier:    middleware.TierR0,
		Summary: "List or search store customers (V3)",
		Description: "GET /v3/customers with optional filters. Requires at least one filter " +
			"(e.g. email_in, customer_ids, name_like) or list_all=true. Supports include, sort, " +
			"page, limit, and cursor params after/before per BigCommerce.",
		Tool: mcp.NewTool("customers_list",
			mcp.WithDescription("List customers. Provide filters or list_all=true."),
			mcp.WithBoolean("list_all", mcp.Description("When true, lists customers without other filters (paginated; respects server max records).")),
			mcp.WithArray("customer_ids", mcp.Description("Customer IDs; sent as id:in (comma-separated)."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithString("company_in", mcp.Description("company:in filter (comma-separated companies).")),
			mcp.WithString("customer_group_id_in", mcp.Description("customer_group_id:in (comma-separated group IDs).")),
			mcp.WithString("email_in", mcp.Description("email:in filter (comma-separated emails).")),
			mcp.WithString("name_in", mcp.Description("name:in filter (comma-separated full names).")),
			mcp.WithString("name_like", mcp.Description("name:like filter (substring on first+last name).")),
			mcp.WithString("phone_in", mcp.Description("phone:in filter.")),
			mcp.WithString("registration_ip_in", mcp.Description("registration_ip_address:in filter.")),
			mcp.WithString("date_created", mcp.Description("Exact date_created.")),
			mcp.WithString("date_created_min", mcp.Description("date_created:min.")),
			mcp.WithString("date_created_max", mcp.Description("date_created:max.")),
			mcp.WithString("date_modified", mcp.Description("Exact date_modified.")),
			mcp.WithString("date_modified_min", mcp.Description("date_modified:min.")),
			mcp.WithString("date_modified_max", mcp.Description("date_modified:max.")),
			mcp.WithString("include", mcp.Description("Comma list: addresses, storecredit, attributes, formfields, shopper_profile_id, segment_ids.")),
			mcp.WithString("sort", mcp.Description("One of: date_created:asc|desc, last_name:asc|desc, date_modified:asc|desc.")),
			mcp.WithNumber("page", mcp.Description("Page number (offset pagination).")),
			mcp.WithNumber("limit", mcp.Description("Page size (max per BC).")),
			mcp.WithString("after", mcp.Description("Cursor pagination: end_cursor from prior response.")),
			mcp.WithString("before", mcp.Description("Cursor pagination: start_cursor from prior response.")),
		),
		Handler: c.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "customers/get",
		Tier:        middleware.TierR0,
		Summary:     "Get one customer by ID (V3)",
		Description: "GET /v3/customers?id:in={id} — BigCommerce has no GET /customers/{id}; this wraps a single-ID filter.",
		Tool: mcp.NewTool("customers_get",
			mcp.WithDescription("Fetch one customer by customer_id."),
			mcp.WithNumber("customer_id", mcp.Description("Customer ID."), mcp.Required()),
			mcp.WithString("include", mcp.Description("Optional comma include list (same as customers/list).")),
		),
		Handler: c.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/create",
		Tier:    middleware.TierR2,
		Summary: "Create one or more customers (V3)",
		Description: "POST /v3/customers — up to 10 customers per call. Preview first; pass confirmed=true to execute. " +
			"Setting a new password requires set_password=true AND confirmed=true (R2 double gate). " +
			"Either pass customer_batch (array of customer objects) or single-record fields email, first_name, last_name.",
		Tool: mcp.NewTool("customers_create",
			mcp.WithDescription("Create customers (max 10). Preview then confirmed=true."),
			mcp.WithArray("customer_batch", mcp.Description("Batch: array of {email, first_name, last_name, ...} per BigCommerce customer_Post."),
				mcp.Items(map[string]any{"type": "object"})),
			mcp.WithString("email", mcp.Description("Single create: email (required with first_name, last_name if batch omitted).")),
			mcp.WithString("first_name", mcp.Description("Single create: first name.")),
			mcp.WithString("last_name", mcp.Description("Single create: last name.")),
			mcp.WithString("company", mcp.Description("Single create: company.")),
			mcp.WithString("phone", mcp.Description("Single create: phone.")),
			mcp.WithNumber("customer_group_id", mcp.Description("Single create: customer_group_id.")),
			mcp.WithBoolean("force_password_reset", mcp.Description("Single create: force_password_reset inside authentication.")),
			mcp.WithString("new_password", mcp.Description("Single create: new password (requires set_password=true and confirmed=true).")),
			mcp.WithBoolean("set_password", mcp.Description("Must be true when supplying new_password — explicit acknowledgement of password write (R2).")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: c.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/update",
		Tier:    middleware.TierR2,
		Summary: "Update one or more customers (V3)",
		Description: "PUT /v3/customers — up to 10 per call; sub-resources (addresses, attribute values) are not updated here. " +
			"new_password requires set_password=true AND confirmed=true.",
		Tool: mcp.NewTool("customers_update",
			mcp.WithDescription("Update customers (max 10). Each batch row must include id."),
			mcp.WithArray("customer_batch", mcp.Description("Array of customer update objects (must include id)."),
				mcp.Items(map[string]any{"type": "object"}), mcp.Required()),
			mcp.WithBoolean("set_password", mcp.Description("Must be true when any row sets new_password.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: c.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete customers by ID (V3)",
		Description: "DELETE /v3/customers?id:in=… — irreversible; cascades related customer data per BigCommerce. " +
			"Max " + fmt.Sprintf("%d", maxCustomerDeleteIDs) + " IDs per call. Preview first; confirmed=true to execute.",
		Tool: mcp.NewTool("customers_delete",
			mcp.WithDescription("Delete customers by id list. Preview required."),
			mcp.WithArray("customer_ids", mcp.Description("Customer IDs to delete."), mcp.Items(map[string]any{"type": "number"}), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: c.handleDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "customers/assign_group",
		Tier:    middleware.TierR2,
		Summary: "Assign many customers to a customer group (V3)",
		Description: "PUT /v3/customers in batches of 10 with only customer_group_id changed. " +
			"Use group_id=0 to clear assignment. Max " + fmt.Sprintf("%d", maxAssignGroupCustomers) + " customers per tool call. " +
			"Preview shows current vs target group. Requires confirmed=true.",
		Tool: mcp.NewTool("customers_assign_group",
			mcp.WithDescription("Batch-assign customer_group_id. Preview then confirmed=true."),
			mcp.WithArray("customer_ids", mcp.Description("Customer IDs to move."), mcp.Items(map[string]any{"type": "number"}), mcp.Required()),
			mcp.WithNumber("group_id", mcp.Description("Target customer_group_id (0 to unassign)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true to execute after preview.")),
		),
		Handler: c.handleAssignGroup,
	})
}

func (c *CustomerRecords) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	listAll := shared.ReadBool(args, "list_all")

	params, err := shared.ExtractFilters(args, CustomerListSearchFilters)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if ids, ierr := intSliceFromArgs(args, "customer_ids"); ierr == nil && len(ids) > 0 {
		params["id:in"] = shared.JoinInts(ids)
	} else if ierr != nil {
		return shared.ToolError("%s", ierr.Error()), nil
	}

	if v, ok := args["include"].(string); ok && strings.TrimSpace(v) != "" {
		params["include"] = strings.TrimSpace(v)
	}
	if v, ok := args["sort"].(string); ok && strings.TrimSpace(v) != "" {
		s := strings.TrimSpace(v)
		if !validCustomerSort[s] {
			return shared.ToolError("invalid sort %q", s), nil
		}
		params["sort"] = s
	}
	if v, ok := args["page"].(float64); ok && v > 0 {
		params["page"] = fmt.Sprintf("%.0f", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params["limit"] = fmt.Sprintf("%.0f", v)
	}
	if v, ok := args["after"].(string); ok && v != "" {
		params["after"] = v
	}
	if v, ok := args["before"].(string); ok && v != "" {
		params["before"] = v
	}

	hasData := shared.HasDataFilterBCParams(params, CustomerListSearchFilters, customerListNonDataKeys) || params["id:in"] != ""
	if !listAll && !hasData {
		return shared.ToolError(
			"provide at least one filter (customer_ids, email_in, name_like, company_in, customer_group_id_in, dates, …) " +
				"or set list_all=true.",
		), nil
	}

	customers, err := c.bc.SearchCustomers(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list customers: %v", err), nil
	}

	type row struct {
		ID              int    `json:"id"`
		Email           string `json:"email"`
		FirstName       string `json:"first_name"`
		LastName        string `json:"last_name"`
		CustomerGroupID int    `json:"customer_group_id,omitempty"`
		DateModified    string `json:"date_modified,omitempty"`
	}
	out := make([]row, 0, len(customers))
	for _, cu := range customers {
		out = append(out, row{
			ID:              cu.ID,
			Email:           cu.Email,
			FirstName:       cu.FirstName,
			LastName:        cu.LastName,
			CustomerGroupID: cu.CustomerGroupID,
			DateModified:    cu.DateModified,
		})
	}
	return shared.ToolJSON(map[string]any{"total": len(out), "customers": out})
}

func (c *CustomerRecords) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "customer_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := map[string]string{"id:in": fmt.Sprintf("%d", id)}
	if v, ok := args["include"].(string); ok && strings.TrimSpace(v) != "" {
		params["include"] = strings.TrimSpace(v)
	}
	customers, err := c.bc.SearchCustomers(ctx, params)
	if err != nil {
		return shared.ToolError("failed to get customer: %v", err), nil
	}
	if len(customers) == 0 {
		return shared.ToolError("customer %d not found", id), nil
	}
	return shared.ToolJSON(customers[0])
}

func (c *CustomerRecords) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	creates, err := parseCustomerCreates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(creates) == 0 {
		return shared.ToolError("no customer records to create"), nil
	}
	if len(creates) > maxCustomerWriteBatch {
		return shared.ToolError("customer_batch exceeds max of %d per call", maxCustomerWriteBatch), nil
	}

	hasPW := customerCreatesHaveNewPassword(creates)
	if hasPW {
		if !shared.ReadBool(args, "set_password") {
			return shared.ToolError("new_password was supplied — set set_password=true to acknowledge this high-risk write (R2)."), nil
		}
		if !middleware.IsConfirmedFromArgs(args) {
			return shared.ToolError("new_password was supplied — pass confirmed=true after preview to execute."), nil
		}
	}

	if middleware.IsConfirmedFromArgs(args) {
		created, err := c.bc.CreateCustomers(ctx, creates)
		if err != nil {
			return shared.ToolError("create failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "created", "count": len(created), "customers": created})
	}

	preview := redactCustomerCreatesForPreview(creates)
	return shared.ToolJSON(map[string]any{
		"status":  "preview",
		"action":  "create",
		"count":   len(creates),
		"payload": preview,
		"message": "Review payload then pass confirmed=true to execute." + passwordGateHint(hasPW),
	})
}

func (c *CustomerRecords) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	updates, err := parseCustomerUpdates(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(updates) == 0 {
		return shared.ToolError("customer_batch is required with at least one row including id"), nil
	}
	if len(updates) > maxCustomerWriteBatch {
		return shared.ToolError("customer_batch exceeds max of %d per call", maxCustomerWriteBatch), nil
	}

	hasPW := customerUpdatesHaveNewPassword(updates)
	if hasPW {
		if !shared.ReadBool(args, "set_password") {
			return shared.ToolError("new_password was supplied — set set_password=true to acknowledge this high-risk write (R2)."), nil
		}
		if !middleware.IsConfirmedFromArgs(args) {
			return shared.ToolError("new_password was supplied — pass confirmed=true after preview to execute."), nil
		}
	}

	if middleware.IsConfirmedFromArgs(args) {
		updated, err := c.bc.UpdateCustomers(ctx, updates)
		if err != nil {
			return shared.ToolError("update failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "updated", "count": len(updated), "customers": updated})
	}

	preview := redactCustomerUpdatesForPreview(updates)
	return shared.ToolJSON(map[string]any{
		"status":  "preview",
		"action":  "update",
		"count":   len(updates),
		"payload": preview,
		"message": "Review payload then pass confirmed=true to execute." + passwordGateHint(hasPW),
	})
}

func (c *CustomerRecords) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := requiredPositiveIntIDs(args, "customer_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) == 0 {
		return shared.ToolError("customer_ids must contain at least one id"), nil
	}
	if len(ids) > maxCustomerDeleteIDs {
		return shared.ToolError("customer_ids exceeds max of %d per call", maxCustomerDeleteIDs), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		if err := c.bc.DeleteCustomers(ctx, ids); err != nil {
			return shared.ToolError("delete failed: %v", err), nil
		}
		return shared.ToolJSON(map[string]any{"status": "deleted", "customer_ids": ids})
	}

	existing, err := c.bc.GetCustomersByIDs(ctx, ids)
	if err != nil {
		return shared.ToolError("failed to pre-fetch customers: %v", err), nil
	}
	type sum struct {
		ID    int    `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	sums := make([]sum, 0, len(existing))
	for _, cu := range existing {
		sums = append(sums, sum{ID: cu.ID, Email: cu.Email, Name: strings.TrimSpace(cu.FirstName + " " + cu.LastName)})
	}
	return shared.ToolJSON(map[string]any{
		"status":            "preview",
		"action":            "delete",
		"would_delete":      len(ids),
		"matched_customers": sums,
		"message":           "Pass confirmed=true to permanently delete these customers.",
	})
}

func (c *CustomerRecords) handleAssignGroup(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	ids, err := requiredPositiveIntIDs(args, "customer_ids")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(ids) == 0 {
		return shared.ToolError("customer_ids must contain at least one id"), nil
	}
	if len(ids) > maxAssignGroupCustomers {
		return shared.ToolError("customer_ids exceeds max of %d per call", maxAssignGroupCustomers), nil
	}

	gidRaw, ok := args["group_id"]
	if !ok {
		return shared.ToolError("group_id is required"), nil
	}
	gf, ok := gidRaw.(float64)
	if !ok {
		return shared.ToolError("group_id must be a number"), nil
	}
	groupID := int(gf)
	if groupID < 0 {
		return shared.ToolError("group_id must be non-negative (use 0 to unassign)"), nil
	}

	if middleware.IsConfirmedFromArgs(args) {
		for i := 0; i < len(ids); i += maxCustomerWriteBatch {
			end := i + maxCustomerWriteBatch
			if end > len(ids) {
				end = len(ids)
			}
			chunk := ids[i:end]
			updates := make([]bigcommerce.CustomerUpdate, 0, len(chunk))
			g := groupID
			for _, id := range chunk {
				updates = append(updates, bigcommerce.CustomerUpdate{ID: id, CustomerGroupID: &g})
			}
			if _, err := c.bc.UpdateCustomers(ctx, updates); err != nil {
				return shared.ToolError("assign_group failed at offset %d: %v", i, err), nil
			}
		}
		return shared.ToolJSON(map[string]any{"status": "updated", "customer_count": len(ids), "group_id": groupID})
	}

	existing, err := c.bc.GetCustomersByIDs(ctx, ids)
	if err != nil {
		return shared.ToolError("failed to pre-fetch customers: %v", err), nil
	}
	type row struct {
		ID             int    `json:"id"`
		CurrentGroupID int    `json:"current_customer_group_id"`
		TargetGroupID  int    `json:"target_customer_group_id"`
		Email          string `json:"email"`
	}
	byID := make(map[int]bigcommerce.Customer, len(existing))
	for _, cu := range existing {
		byID[cu.ID] = cu
	}
	rows := make([]row, 0, len(ids))
	for _, id := range ids {
		cu, ok := byID[id]
		cur := 0
		if ok {
			cur = cu.CustomerGroupID
			rows = append(rows, row{ID: id, CurrentGroupID: cur, TargetGroupID: groupID, Email: cu.Email})
		} else {
			rows = append(rows, row{ID: id, CurrentGroupID: 0, TargetGroupID: groupID, Email: ""})
		}
	}
	return shared.ToolJSON(map[string]any{
		"status":    "preview",
		"action":    "assign_group",
		"customers": rows,
		"message":   "Pass confirmed=true to apply customer_group_id to all listed IDs.",
	})
}

func passwordGateHint(hasPassword bool) string {
	if !hasPassword {
		return ""
	}
	return " Password fields require set_password=true and confirmed=true."
}

func requiredPositiveIntIDs(args map[string]any, key string) ([]int, error) {
	raw, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("%s is required", key)
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	out := make([]int, 0, len(arr))
	for _, item := range arr {
		f, ok := item.(float64)
		if !ok {
			return nil, fmt.Errorf("each %s entry must be a number", key)
		}
		id := int(f)
		if id <= 0 {
			return nil, fmt.Errorf("each %s entry must be a positive integer", key)
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s must contain at least one id", key)
	}
	return out, nil
}

func intSliceFromArgs(args map[string]any, key string) ([]int, error) {
	raw, ok := args[key]
	if !ok {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}
	out := make([]int, 0, len(arr))
	for _, item := range arr {
		f, ok := item.(float64)
		if !ok {
			return nil, fmt.Errorf("each %s entry must be a number", key)
		}
		id := int(f)
		if id > 0 {
			out = append(out, id)
		}
	}
	return out, nil
}

func parseCustomerCreates(args map[string]any) ([]bigcommerce.CustomerCreate, error) {
	if v, ok := args["customer_batch"]; ok && v != nil {
		arr, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("customer_batch must be an array")
		}
		out := make([]bigcommerce.CustomerCreate, 0, len(arr))
		for i, item := range arr {
			m, ok := item.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("customer_batch[%d] must be an object", i)
			}
			var c bigcommerce.CustomerCreate
			b, err := json.Marshal(m)
			if err != nil {
				return nil, fmt.Errorf("customer_batch[%d]: %w", i, err)
			}
			if err := json.Unmarshal(b, &c); err != nil {
				return nil, fmt.Errorf("customer_batch[%d]: %w", i, err)
			}
			out = append(out, c)
		}
		return out, nil
	}

	email, _ := args["email"].(string)
	fn, _ := args["first_name"].(string)
	ln, _ := args["last_name"].(string)
	if email == "" || fn == "" || ln == "" {
		return nil, fmt.Errorf("provide customer_batch or email, first_name, and last_name")
	}
	c := bigcommerce.CustomerCreate{Email: email, FirstName: fn, LastName: ln}
	if v, ok := args["company"].(string); ok {
		c.Company = v
	}
	if v, ok := args["phone"].(string); ok {
		c.Phone = v
	}
	if v, ok := args["customer_group_id"].(float64); ok {
		c.CustomerGroupID = int(v)
	}
	var auth *bigcommerce.CustomerAuthentication
	if v, ok := args["force_password_reset"].(bool); ok {
		auth = &bigcommerce.CustomerAuthentication{ForcePasswordReset: &v}
	}
	if v, ok := args["new_password"].(string); ok && v != "" {
		if auth == nil {
			auth = &bigcommerce.CustomerAuthentication{}
		}
		p := v
		auth.NewPassword = &p
	}
	c.Authentication = auth
	return []bigcommerce.CustomerCreate{c}, nil
}

func parseCustomerUpdates(args map[string]any) ([]bigcommerce.CustomerUpdate, error) {
	v, ok := args["customer_batch"]
	if !ok || v == nil {
		return nil, fmt.Errorf("customer_batch is required")
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("customer_batch must be an array")
	}
	out := make([]bigcommerce.CustomerUpdate, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("customer_batch[%d] must be an object", i)
		}
		var u bigcommerce.CustomerUpdate
		b, err := json.Marshal(m)
		if err != nil {
			return nil, fmt.Errorf("customer_batch[%d]: %w", i, err)
		}
		if err := json.Unmarshal(b, &u); err != nil {
			return nil, fmt.Errorf("customer_batch[%d]: %w", i, err)
		}
		if u.ID == 0 {
			return nil, fmt.Errorf("customer_batch[%d]: id is required", i)
		}
		out = append(out, u)
	}
	return out, nil
}

func customerCreatesHaveNewPassword(cs []bigcommerce.CustomerCreate) bool {
	for _, c := range cs {
		if c.Authentication != nil && c.Authentication.NewPassword != nil && *c.Authentication.NewPassword != "" {
			return true
		}
	}
	return false
}

func customerUpdatesHaveNewPassword(us []bigcommerce.CustomerUpdate) bool {
	for _, u := range us {
		if u.Authentication != nil && u.Authentication.NewPassword != nil && *u.Authentication.NewPassword != "" {
			return true
		}
	}
	return false
}

func redactCustomerCreatesForPreview(cs []bigcommerce.CustomerCreate) []any {
	out := make([]any, 0, len(cs))
	for _, c := range cs {
		cp := c
		if cp.Authentication != nil && cp.Authentication.NewPassword != nil {
			auth := *cp.Authentication
			redacted := "(set)"
			auth.NewPassword = &redacted
			cp.Authentication = &auth
		}
		b, _ := json.Marshal(cp)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		out = append(out, m)
	}
	return out
}

func redactCustomerUpdatesForPreview(us []bigcommerce.CustomerUpdate) []any {
	out := make([]any, 0, len(us))
	for _, u := range us {
		cp := u
		if cp.Authentication != nil && cp.Authentication.NewPassword != nil {
			auth := *cp.Authentication
			redacted := "(set)"
			auth.NewPassword = &redacted
			cp.Authentication = &auth
		}
		b, _ := json.Marshal(cp)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		out = append(out, m)
	}
	return out
}
