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
// Super admin tools
// ============================================================

func (ct *CompanyTools) registerSuperAdminTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/super_admins/list",
		Tier:    middleware.TierR0,
		Summary: "List Super Admins with their assigned-company counts",
		Tool: mcp.NewTool("b2b_super_admins_list",
			mcp.WithDescription("List B2B Edition Super Admin accounts, each with a count of assigned companies."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
			mcp.WithString("order_by", mcp.Description("ASC or DESC (default DESC).")),
		),
		Handler: ct.handleSuperAdminList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/super_admins/companies_overview",
		Tier:    middleware.TierR0,
		Summary: "List companies with their assigned-Super-Admin counts",
		Tool: mcp.NewTool("b2b_super_admins_companies_overview",
			mcp.WithDescription("List companies, each with a count of assigned Super Admins (the inverse view of b2b/super_admins/list)."),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
			mcp.WithString("order_by", mcp.Description("ASC or DESC (default DESC).")),
		),
		Handler: ct.handleSuperAdminCompaniesOverview,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/super_admins/get",
		Tier:    middleware.TierR0,
		Summary: "Get a Super Admin's account details",
		Tool: mcp.NewTool("b2b_super_admins_get",
			mcp.WithDescription("Get detailed account info for one Super Admin."),
			mcp.WithNumber("super_admin_id", mcp.Description("Super Admin ID (not the BC customer ID)"), mcp.Required()),
		),
		Handler: ct.handleSuperAdminGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/super_admins/companies",
		Tier:    middleware.TierR0,
		Summary: "List the companies assigned to a Super Admin",
		Tool: mcp.NewTool("b2b_super_admins_companies",
			mcp.WithDescription("List the companies assigned to one Super Admin account."),
			mcp.WithNumber("super_admin_id", mcp.Description("Super Admin ID"), mcp.Required()),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleSuperAdminCompanies,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/super_admins/create",
		Tier:    middleware.TierR1,
		Summary: "Create a Super Admin account",
		Tool: mcp.NewTool("b2b_super_admins_create",
			mcp.WithDescription("Create a Super Admin account. An existing BigCommerce customer with this email is converted to a Super Admin. Fails if the email already belongs to a B2B company user (any role). Preview → confirm."),
			mcp.WithString("first_name", mcp.Description("First name"), mcp.Required()),
			mcp.WithString("last_name", mcp.Description("Last name"), mcp.Required()),
			mcp.WithString("email", mcp.Description("Email address"), mcp.Required()),
			mcp.WithString("phone", mcp.Description("Phone number.")),
			mcp.WithString("uuid", mcp.Description("External ID (not required to be unique).")),
			mcp.WithNumber("origin_channel_id", mcp.Description("Origin BigCommerce channel ID.")),
			mcp.WithArray("channel_ids", mcp.Description("BigCommerce channel IDs to assign."), mcp.Items(map[string]any{"type": "number"})),
			mcp.WithString("extra_fields_json", mcp.Description(`Optional JSON array: [{"fieldName":"...","fieldValue":"..."}]`)),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create.")),
		),
		Handler: ct.handleSuperAdminCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/super_admins/bulk_create",
		Tier:    middleware.TierR1,
		Summary: "Create up to 10 Super Admin accounts in one call",
		Tool: mcp.NewTool("b2b_super_admins_bulk_create",
			mcp.WithDescription("Create up to 10 Super Admin accounts at once. Preview → confirm."),
			mcp.WithString("super_admins_json", mcp.Description(`JSON array (max 10): [{"first_name":"A","last_name":"B","email":"a@b.com","phone":"","uuid":"","origin_channel_id":0,"channel_ids":[],"extra_fields":[]}]`), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create.")),
		),
		Handler: ct.handleSuperAdminBulkCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/super_admins/update",
		Tier:    middleware.TierR1,
		Summary: "Update a Super Admin's account details",
		Tool: mcp.NewTool("b2b_super_admins_update",
			mcp.WithDescription("Update a Super Admin's first/last name, phone, uuid, or extra fields. Email cannot be changed here (update the underlying BC customer instead). Preview → confirm."),
			mcp.WithNumber("super_admin_id", mcp.Description("Super Admin ID"), mcp.Required()),
			mcp.WithString("first_name", mcp.Description("New first name.")),
			mcp.WithString("last_name", mcp.Description("New last name.")),
			mcp.WithString("phone", mcp.Description("New phone.")),
			mcp.WithString("uuid", mcp.Description("New external ID.")),
			mcp.WithString("extra_fields_json", mcp.Description(`Optional JSON array: [{"fieldName":"...","fieldValue":"..."}]`)),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleSuperAdminUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/super_admins/update_assignments",
		Tier:    middleware.TierR1,
		Summary: "Assign or unassign companies for a Super Admin",
		Tool: mcp.NewTool("b2b_super_admins_update_assignments",
			mcp.WithDescription("Assign or unassign companies for a Super Admin. Non-destructive: only companies listed in assignments_json are affected. Preview → confirm."),
			mcp.WithNumber("super_admin_id", mcp.Description("Super Admin ID"), mcp.Required()),
			mcp.WithString("assignments_json", mcp.Description(`JSON array: [{"companyId":42,"isAssigned":true}]`), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleSuperAdminUpdateAssignments,
	})

	// ---- Company-perspective super admin tools ----

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/super_admins/list",
		Tier:    middleware.TierR0,
		Summary: "List Super Admins assigned to a company",
		Tool: mcp.NewTool("b2b_companies_super_admins_list",
			mcp.WithDescription("List the Super Admins assigned to a company, with extended account data."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleCompanySuperAdminsList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/super_admins/update_assignments",
		Tier:    middleware.TierR1,
		Summary: "Assign or unassign Super Admins for a company",
		Tool: mcp.NewTool("b2b_companies_super_admins_update_assignments",
			mcp.WithDescription("Assign or unassign Super Admins for a company. Non-destructive: only Super Admins listed in assignments_json are affected. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithString("assignments_json", mcp.Description(`JSON array: [{"superAdminId":7,"isAssigned":true}]`), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleCompanySuperAdminsUpdateAssignments,
	})
}

// ---- super admin handlers ----

func (ct *CompanyTools) handleSuperAdminList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["order_by"].(string); ok && v != "" {
		params.Set("orderBy", v)
	}
	admins, err := ct.bc.ListB2BSuperAdmins(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B super admins: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(admins), "super_admins": admins})
}

func (ct *CompanyTools) handleSuperAdminCompaniesOverview(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["order_by"].(string); ok && v != "" {
		params.Set("orderBy", v)
	}
	companies, err := ct.bc.ListB2BSuperAdminCompaniesOverview(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B super admin companies overview: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(companies), "companies": companies})
}

func (ct *CompanyTools) handleSuperAdminGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "super_admin_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	admin, err := ct.bc.GetB2BSuperAdmin(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B super admin %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"super_admin": admin})
}

func (ct *CompanyTools) handleSuperAdminCompanies(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "super_admin_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	companies, err := ct.bc.GetB2BSuperAdminCompanies(ctx, id, params.Encode())
	if err != nil {
		return shared.ToolError("failed to get companies for B2B super admin %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"super_admin_id": id, "total": len(companies), "companies": companies})
}

func superAdminCreatePayloadFromArgs(args map[string]any) (bigcommerce.B2BSuperAdminCreate, error) {
	firstName, _ := args["first_name"].(string)
	lastName, _ := args["last_name"].(string)
	email, _ := args["email"].(string)
	if strings.TrimSpace(firstName) == "" || strings.TrimSpace(lastName) == "" || strings.TrimSpace(email) == "" {
		return bigcommerce.B2BSuperAdminCreate{}, fmt.Errorf("first_name, last_name, and email are required")
	}
	payload := bigcommerce.B2BSuperAdminCreate{FirstName: firstName, LastName: lastName, Email: email}
	if v, ok := args["phone"].(string); ok {
		payload.Phone = v
	}
	if v, ok := args["uuid"].(string); ok {
		payload.UUID = v
	}
	if v, ok := args["origin_channel_id"].(float64); ok && v > 0 {
		payload.OriginChannelID = int(v)
	}
	if raw, ok := args["channel_ids"].([]any); ok {
		for _, v := range raw {
			if f, ok := v.(float64); ok {
				payload.ChannelIDs = append(payload.ChannelIDs, int(f))
			}
		}
	}
	ef, err := parseB2BExtraFieldsJSON(args, "extra_fields_json")
	if err != nil {
		return bigcommerce.B2BSuperAdminCreate{}, err
	}
	payload.ExtraFields = ef
	return payload, nil
}

func (ct *CompanyTools) handleSuperAdminCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	payload, err := superAdminCreatePayloadFromArgs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_super_admin",
			"payload": payload,
			"message": fmt.Sprintf("Will create Super Admin %q (%s). Pass confirmed=true.", payload.FirstName+" "+payload.LastName, payload.Email),
		})
	}

	result, err := ct.bc.CreateB2BSuperAdmin(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create B2B super admin: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "result": result})
}

// b2bSuperAdminBatchItem is the per-row shape accepted by super_admins_json in
// b2b/super_admins/bulk_create.
type b2bSuperAdminBatchItem struct {
	FirstName       string                      `json:"first_name"`
	LastName        string                      `json:"last_name"`
	Email           string                      `json:"email"`
	Phone           string                      `json:"phone"`
	UUID            string                      `json:"uuid"`
	OriginChannelID int                         `json:"origin_channel_id"`
	ChannelIDs      []int                       `json:"channel_ids"`
	ExtraFields     []bigcommerce.B2BExtraField `json:"extra_fields"`
}

func (ct *CompanyTools) handleSuperAdminBulkCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	raw, _ := args["super_admins_json"].(string)
	if strings.TrimSpace(raw) == "" {
		return shared.ToolError("super_admins_json is required (a JSON array of super admin objects)"), nil
	}
	var items []b2bSuperAdminBatchItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return shared.ToolError("invalid super_admins_json: %v", err), nil
	}
	if len(items) == 0 {
		return shared.ToolError("super_admins_json must contain at least one super admin"), nil
	}
	if len(items) > 10 {
		return shared.ToolError("super_admins_json exceeds the B2B API max of 10 per call (got %d)", len(items)), nil
	}
	payloads := make([]bigcommerce.B2BSuperAdminCreate, len(items))
	for i, it := range items {
		if strings.TrimSpace(it.FirstName) == "" || strings.TrimSpace(it.LastName) == "" || strings.TrimSpace(it.Email) == "" {
			return shared.ToolError("super_admins_json[%d]: first_name, last_name, and email are required", i), nil
		}
		payloads[i] = bigcommerce.B2BSuperAdminCreate{
			FirstName: it.FirstName, LastName: it.LastName, Email: it.Email,
			Phone: it.Phone, UUID: it.UUID, OriginChannelID: it.OriginChannelID,
			ChannelIDs: it.ChannelIDs, ExtraFields: it.ExtraFields,
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "bulk_create_b2b_super_admins",
			"count":   len(payloads),
			"payload": payloads,
			"message": fmt.Sprintf("Will create %d Super Admin(s). Pass confirmed=true.", len(payloads)),
		})
	}

	result, err := ct.bc.BulkCreateB2BSuperAdmins(ctx, payloads)
	if err != nil {
		return shared.ToolError("failed to bulk create B2B super admins: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "count": len(payloads), "result": result})
}

func (ct *CompanyTools) handleSuperAdminUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "super_admin_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload := bigcommerce.B2BSuperAdminUpdate{}
	hasField := false
	if v, ok := args["first_name"].(string); ok && v != "" {
		payload.FirstName = v
		hasField = true
	}
	if v, ok := args["last_name"].(string); ok && v != "" {
		payload.LastName = v
		hasField = true
	}
	if v, ok := args["phone"].(string); ok && v != "" {
		payload.Phone = v
		hasField = true
	}
	if v, ok := args["uuid"].(string); ok && v != "" {
		payload.UUID = v
		hasField = true
	}
	if ef, eerr := parseB2BExtraFieldsJSON(args, "extra_fields_json"); eerr != nil {
		return shared.ToolError("%s", eerr.Error()), nil
	} else if len(ef) > 0 {
		payload.ExtraFields = ef
		hasField = true
	}
	if !hasField {
		return shared.ToolError("at least one field must be provided"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":         "preview",
			"action":         "update_b2b_super_admin",
			"super_admin_id": id,
			"payload":        payload,
			"message":        fmt.Sprintf("Will update super admin %d. Pass confirmed=true.", id),
		})
	}

	result, err := ct.bc.UpdateB2BSuperAdmin(ctx, id, payload)
	if err != nil {
		return shared.ToolError("failed to update B2B super admin %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "super_admin_id": id, "result": result})
}

func (ct *CompanyTools) handleSuperAdminUpdateAssignments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "super_admin_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	raw, _ := args["assignments_json"].(string)
	if strings.TrimSpace(raw) == "" {
		return shared.ToolError("assignments_json is required (a JSON array of {companyId, isAssigned} objects)"), nil
	}
	var assignments []bigcommerce.B2BSuperAdminCompanyAssignment
	if err := json.Unmarshal([]byte(raw), &assignments); err != nil {
		return shared.ToolError("invalid assignments_json: %v", err), nil
	}
	if len(assignments) == 0 {
		return shared.ToolError("assignments_json must contain at least one {companyId, isAssigned} entry"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":         "preview",
			"action":         "update_b2b_super_admin_assignments",
			"super_admin_id": id,
			"assignments":    assignments,
			"message":        fmt.Sprintf("Will apply %d company assignment change(s) for super admin %d. Other existing assignments are unaffected. Pass confirmed=true.", len(assignments), id),
		})
	}

	result, err := ct.bc.UpdateB2BSuperAdminCompanyAssignments(ctx, id, assignments)
	if err != nil {
		return shared.ToolError("failed to update assignments for B2B super admin %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "super_admin_id": id, "result": result})
}

// ---- company-perspective super admin handlers ----

func (ct *CompanyTools) handleCompanySuperAdminsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	admins, err := ct.bc.ListB2BCompanySuperAdmins(ctx, id, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list super admins for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"company_id": id, "total": len(admins), "super_admins": admins})
}

func (ct *CompanyTools) handleCompanySuperAdminsUpdateAssignments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	raw, _ := args["assignments_json"].(string)
	if strings.TrimSpace(raw) == "" {
		return shared.ToolError("assignments_json is required (a JSON array of {superAdminId, isAssigned} objects)"), nil
	}
	var assignments []bigcommerce.B2BCompanySuperAdminAssignment
	if err := json.Unmarshal([]byte(raw), &assignments); err != nil {
		return shared.ToolError("invalid assignments_json: %v", err), nil
	}
	if len(assignments) == 0 {
		return shared.ToolError("assignments_json must contain at least one {superAdminId, isAssigned} entry"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "update_b2b_company_super_admin_assignments",
			"company_id":  id,
			"assignments": assignments,
			"message":     fmt.Sprintf("Will apply %d super admin assignment change(s) for company %d. Other existing assignments are unaffected. Pass confirmed=true.", len(assignments), id),
		})
	}

	result, err := ct.bc.UpdateB2BCompanySuperAdminAssignments(ctx, id, assignments)
	if err != nil {
		return shared.ToolError("failed to update super admin assignments for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "company_id": id, "result": result})
}
