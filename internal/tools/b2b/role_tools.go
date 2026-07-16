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
// Company role tools
// ============================================================

func (ct *CompanyTools) registerRoleTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/roles/list",
		Tier:    middleware.TierR0,
		Summary: "List company user roles (predefined and custom)",
		Tool: mcp.NewTool("b2b_companies_roles_list",
			mcp.WithDescription("List B2B Edition company user roles. roleType: 1=predefined, 2=custom."),
			mcp.WithString("q", mcp.Description("Search term to filter roles.")),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleRoleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/roles/get",
		Tier:    middleware.TierR0,
		Summary: "Get a company user role by ID",
		Tool: mcp.NewTool("b2b_companies_roles_get",
			mcp.WithDescription("Get a B2B Edition company user role by roleId, including its permissions."),
			mcp.WithNumber("role_id", mcp.Description("Role ID"), mcp.Required()),
		),
		Handler: ct.handleRoleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/roles/create",
		Tier:    middleware.TierR1,
		Summary: "Create a custom company user role with permissions",
		Tool: mcp.NewTool("b2b_companies_roles_create",
			mcp.WithDescription(`Create a custom B2B Edition company user role. permissions_json is a JSON array of {"code","permissionLevel"} objects. permissionLevel: "1"=user, "2"=company, "3"=company and subsidiaries. Use b2b/companies/permissions/list for available codes. Preview → confirm.`),
			mcp.WithString("name", mcp.Description("Role name (visible to system and company users)."), mcp.Required()),
			mcp.WithString("permissions_json", mcp.Description(`JSON array: [{"code":"get_orders","permissionLevel":"2"},{"code":"get_quotes","permissionLevel":"1"}]`), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create the role.")),
		),
		Handler: ct.handleRoleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/roles/update",
		Tier:    middleware.TierR1,
		Summary: "Update a custom company user role (predefined roles are read-only)",
		Tool: mcp.NewTool("b2b_companies_roles_update",
			mcp.WithDescription("Update a custom B2B Edition company role's name and permissions. You cannot update predefined roles. The permissions array must include EVERY permission to keep (it replaces the existing set). Preview → confirm."),
			mcp.WithNumber("role_id", mcp.Description("Role ID"), mcp.Required()),
			mcp.WithString("name", mcp.Description("New role name."), mcp.Required()),
			mcp.WithString("permissions_json", mcp.Description(`Full JSON array of permissions to keep: [{"code":"get_orders","permissionLevel":"2"}]`), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleRoleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/roles/delete",
		Tier:    middleware.TierR2,
		Summary: "Delete a custom company user role",
		Tool: mcp.NewTool("b2b_companies_roles_delete",
			mcp.WithDescription("Delete a custom B2B Edition company role. Predefined roles cannot be deleted. Preview → confirm."),
			mcp.WithNumber("role_id", mcp.Description("Role ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete.")),
		),
		Handler: ct.handleRoleDelete,
	})
}

// ============================================================
// Company permission tools
// ============================================================

func (ct *CompanyTools) registerPermissionTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/permissions/list",
		Tier:    middleware.TierR0,
		Summary: "List company permission definitions",
		Tool: mcp.NewTool("b2b_companies_permissions_list",
			mcp.WithDescription("List B2B Edition company permission definitions (built-in and custom). Use the returned codes when building role permissions."),
			mcp.WithString("q", mcp.Description("Search term to filter permissions.")),
		),
		Handler: ct.handlePermissionList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/permissions/create",
		Tier:    middleware.TierR1,
		Summary: "Create a custom company permission",
		Tool: mcp.NewTool("b2b_companies_permissions_create",
			mcp.WithDescription("Create a custom B2B Edition company permission. name, description, and code are required; code must not match an existing permission. Preview → confirm."),
			mcp.WithString("name", mcp.Description("Permission name."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Permission description."), mcp.Required()),
			mcp.WithString("code", mcp.Description("Unique permission code string."), mcp.Required()),
			mcp.WithString("module_name", mcp.Description("Optional module/section name for control-panel grouping.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create.")),
		),
		Handler: ct.handlePermissionCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/permissions/update",
		Tier:    middleware.TierR1,
		Summary: "Update a custom company permission",
		Tool: mcp.NewTool("b2b_companies_permissions_update",
			mcp.WithDescription("Update a custom B2B Edition company permission. name, description, and code are required. Preview → confirm."),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID"), mcp.Required()),
			mcp.WithString("name", mcp.Description("Permission name."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Permission description."), mcp.Required()),
			mcp.WithString("code", mcp.Description("Permission code string."), mcp.Required()),
			mcp.WithString("module_name", mcp.Description("Optional module/section name.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handlePermissionUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/permissions/delete",
		Tier:    middleware.TierR2,
		Summary: "Delete a custom company permission",
		Tool: mcp.NewTool("b2b_companies_permissions_delete",
			mcp.WithDescription("Delete a custom B2B Edition company permission. Preview → confirm."),
			mcp.WithNumber("permission_id", mcp.Description("Permission ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete.")),
		),
		Handler: ct.handlePermissionDelete,
	})
}

// ---- role handlers ----

func (ct *CompanyTools) handleRoleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["q"].(string); ok && v != "" {
		params.Set("q", v)
	}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	roles, err := ct.bc.ListB2BRoles(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B roles: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(roles), "roles": roles})
}

func (ct *CompanyTools) handleRoleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "role_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	role, err := ct.bc.GetB2BRole(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B role %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"role": role})
}

func (ct *CompanyTools) handleRoleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	name, _ := args["name"].(string)
	if strings.TrimSpace(name) == "" {
		return shared.ToolError("name is required"), nil
	}
	perms, err := parseB2BPermissionsJSON(args, "permissions_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(perms) == 0 {
		return shared.ToolError("permissions_json must contain at least one {code, permissionLevel} entry"), nil
	}
	payload := bigcommerce.B2BRoleCreate{Name: name, Permissions: perms}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_role",
			"payload": payload,
			"message": fmt.Sprintf("Will create custom role %q with %d permission(s). Pass confirmed=true.", name, len(perms)),
		})
	}

	role, err := ct.bc.CreateB2BRole(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create B2B role: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "role": role})
}

func (ct *CompanyTools) handleRoleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "role_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	name, _ := args["name"].(string)
	if strings.TrimSpace(name) == "" {
		return shared.ToolError("name is required"), nil
	}
	perms, err := parseB2BPermissionsJSON(args, "permissions_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if len(perms) == 0 {
		return shared.ToolError("permissions_json must contain the full set of permissions to keep"), nil
	}
	payload := bigcommerce.B2BRoleCreate{Name: name, Permissions: perms}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "update_b2b_role",
			"role_id": id,
			"payload": payload,
			"message": fmt.Sprintf("Will replace role %d with name %q and %d permission(s). Predefined roles cannot be updated. Pass confirmed=true.", id, name, len(perms)),
		})
	}

	role, err := ct.bc.UpdateB2BRole(ctx, id, payload)
	if err != nil {
		return shared.ToolError("failed to update B2B role %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "role": role})
}

func (ct *CompanyTools) handleRoleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "role_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "delete_b2b_role",
			"role_id": id,
			"message": fmt.Sprintf("Will delete custom role %d. Predefined roles cannot be deleted. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.DeleteB2BRole(ctx, id); err != nil {
		return shared.ToolError("failed to delete B2B role %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "role_id": id})
}

// ---- permission handlers ----

func (ct *CompanyTools) handlePermissionList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["q"].(string); ok && v != "" {
		params.Set("q", v)
	}
	perms, err := ct.bc.ListB2BPermissions(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B permissions: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"total": len(perms), "permissions": perms})
}

func (ct *CompanyTools) handlePermissionCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	payload, verr := permissionPayloadFromArgs(args)
	if verr != "" {
		return shared.ToolError("%s", verr), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_permission",
			"payload": payload,
			"message": fmt.Sprintf("Will create custom permission %q (code %q). Pass confirmed=true.", payload.Name, payload.Code),
		})
	}

	p, err := ct.bc.CreateB2BPermission(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create B2B permission: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "permission": p})
}

func (ct *CompanyTools) handlePermissionUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "permission_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload, verr := permissionPayloadFromArgs(args)
	if verr != "" {
		return shared.ToolError("%s", verr), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":        "preview",
			"action":        "update_b2b_permission",
			"permission_id": id,
			"payload":       payload,
			"message":       fmt.Sprintf("Will update permission %d. Pass confirmed=true.", id),
		})
	}

	p, err := ct.bc.UpdateB2BPermission(ctx, id, payload)
	if err != nil {
		return shared.ToolError("failed to update B2B permission %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "permission": p})
}

func (ct *CompanyTools) handlePermissionDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "permission_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":        "preview",
			"action":        "delete_b2b_permission",
			"permission_id": id,
			"message":       fmt.Sprintf("Will delete custom permission %d. Pass confirmed=true.", id),
		})
	}

	if err := ct.bc.DeleteB2BPermission(ctx, id); err != nil {
		return shared.ToolError("failed to delete B2B permission %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "permission_id": id})
}

// ---- helpers ----

func parseB2BPermissionsJSON(args map[string]any, key string) ([]bigcommerce.B2BRolePermission, error) {
	raw, ok := args[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("%s is required (a JSON array of {code, permissionLevel} objects)", key)
	}
	var perms []bigcommerce.B2BRolePermission
	if err := json.Unmarshal([]byte(raw), &perms); err != nil {
		return nil, fmt.Errorf("invalid %s: %v", key, err)
	}
	for i, p := range perms {
		if strings.TrimSpace(p.Code) == "" {
			return nil, fmt.Errorf("%s[%d]: code is required", key, i)
		}
		if strings.TrimSpace(string(p.PermissionLevel)) == "" {
			return nil, fmt.Errorf("%s[%d]: permissionLevel is required", key, i)
		}
	}
	return perms, nil
}

func permissionPayloadFromArgs(args map[string]any) (bigcommerce.B2BPermissionCreate, string) {
	name, _ := args["name"].(string)
	description, _ := args["description"].(string)
	code, _ := args["code"].(string)
	if strings.TrimSpace(name) == "" {
		return bigcommerce.B2BPermissionCreate{}, "name is required"
	}
	if strings.TrimSpace(description) == "" {
		return bigcommerce.B2BPermissionCreate{}, "description is required"
	}
	if strings.TrimSpace(code) == "" {
		return bigcommerce.B2BPermissionCreate{}, "code is required"
	}
	payload := bigcommerce.B2BPermissionCreate{Name: name, Description: description, Code: code}
	if v, ok := args["module_name"].(string); ok {
		payload.ModuleName = v
	}
	return payload, ""
}
