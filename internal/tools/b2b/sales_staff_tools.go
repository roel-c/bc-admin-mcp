package b2b

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ============================================================
// Sales staff tools
// ============================================================

func (ct *CompanyTools) registerSalesStaffTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/sales_staff/list",
		Tier:    middleware.TierR0,
		Summary: "List B2B users assigned a Sales Staff role",
		Tool: mcp.NewTool("b2b_sales_staff_list",
			mcp.WithDescription("List B2B Edition users assigned a Sales Staff role. Filter by company to see which sales staff are assigned to it."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
			mcp.WithString("sort_by", mcp.Description("updated_at or email (default updated_at).")),
			mcp.WithString("order_by", mcp.Description("ASC or DESC (default DESC).")),
			mcp.WithNumber("company_id", mcp.Description("Filter to sales staff assigned to this company.")),
		),
		Handler: ct.handleSalesStaffList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/sales_staff/get",
		Tier:    middleware.TierR0,
		Summary: "Get a sales staff account's company assignments",
		Tool: mcp.NewTool("b2b_sales_staff_get",
			mcp.WithDescription("Get detail for a sales staff account, including the companies it's assigned to and when each assignment was made."),
			mcp.WithNumber("sales_staff_id", mcp.Description("Sales staff ID (the B2B user ID)"), mcp.Required()),
		),
		Handler: ct.handleSalesStaffGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/sales_staff/update_assignments",
		Tier:    middleware.TierR1,
		Summary: "Assign or unassign companies for a sales staff account",
		Tool: mcp.NewTool("b2b_sales_staff_update_assignments",
			mcp.WithDescription("Assign or unassign companies for a sales staff account. Non-destructive: only the companies listed in assignments_json are affected, other existing assignments are left alone. Preview → confirm."),
			mcp.WithNumber("sales_staff_id", mcp.Description("Sales staff ID"), mcp.Required()),
			mcp.WithString("assignments_json", mcp.Description(`JSON array: [{"companyId":42,"assignStatus":true},{"companyId":43,"assignStatus":false}]`), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleSalesStaffUpdateAssignments,
	})
}

func (ct *CompanyTools) handleSalesStaffList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if v, ok := args["company_id"].(float64); ok && v > 0 {
		params.Set("companyId", fmt.Sprintf("%d", int(v)))
	}
	staff, err := ct.bc.ListB2BSalesStaff(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B sales staff: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(staff), "sales_staff": staff})
}

func (ct *CompanyTools) handleSalesStaffGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "sales_staff_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	staff, err := ct.bc.GetB2BSalesStaff(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B sales staff %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"sales_staff": staff})
}

func (ct *CompanyTools) handleSalesStaffUpdateAssignments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "sales_staff_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	assignments, err := parseSalesStaffAssignments(args, "assignments_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(assignments) == 0 {
		return shared.ToolError("assignments_json must contain at least one {companyId, assignStatus} entry"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":         "preview",
			"action":         "update_b2b_sales_staff_assignments",
			"sales_staff_id": id,
			"assignments":    assignments,
			"message":        fmt.Sprintf("Will apply %d company assignment change(s) for sales staff %d. Other existing assignments are unaffected. Pass confirmed=true.", len(assignments), id),
		})
	}

	result, err := ct.bc.UpdateB2BSalesStaffAssignments(ctx, id, assignments)
	if err != nil {
		return shared.ToolError("failed to update assignments for B2B sales staff %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "sales_staff_id": id, "result": result})
}

func parseSalesStaffAssignments(args map[string]any, key string) ([]bigcommerce.B2BSalesStaffAssignment, error) {
	raw, ok := args[key].(string)
	if !ok || raw == "" {
		return nil, fmt.Errorf("%s is required (a JSON array of {companyId, assignStatus} objects)", key)
	}
	var items []bigcommerce.B2BSalesStaffAssignment
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("invalid %s: %v", key, err)
	}
	return items, nil
}
