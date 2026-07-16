package b2b

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// CompanyTools provides MCP tool handlers for B2B Edition companies,
// users, and addresses via the B2B Edition Management REST API.
type CompanyTools struct {
	bc        B2BCompanyAPI
	customers BCCustomerManager
	cache     *session.Store
}

// NewCompanyTools constructs a CompanyTools handler. customers is used by the
// company delete flow to clean up linked BC customer accounts; it may be nil,
// in which case that cleanup is skipped.
func NewCompanyTools(bc B2BCompanyAPI, customers BCCustomerManager, cache *session.Store) *CompanyTools {
	return &CompanyTools{bc: bc, customers: customers, cache: cache}
}

// RegisterTools wires all B2B Phase B1 tools into the discovery registry.
func (ct *CompanyTools) RegisterTools(reg *discovery.Registry) {
	ct.registerCompanyTools(reg)
	ct.registerUserTools(reg)
	ct.registerAddressTools(reg)
}

// ============================================================
// Company tools
// ============================================================

func (ct *CompanyTools) registerCompanyTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/list",
		Tier:    middleware.TierR0,
		Summary: "List B2B company accounts with optional status/name filters",
		Tool: mcp.NewTool("b2b_companies_list",
			mcp.WithDescription("List B2B Edition company accounts. Filter by status (0=pending, 1=approved, 2=rejected, 3=inactive), name, email, or date range."),
			mcp.WithNumber("status", mcp.Description("Filter by status: 0=pending, 1=approved, 2=rejected, 3=inactive.")),
			mcp.WithString("company_name", mcp.Description("Filter by company name (partial match).")),
			mcp.WithString("email", mcp.Description("Filter by company email.")),
		),
		Handler: ct.handleCompanyList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/get",
		Tier:    middleware.TierR0,
		Summary: "Get full details for a single B2B company account by ID",
		Tool: mcp.NewTool("b2b_companies_get",
			mcp.WithDescription("Get a B2B Edition company account by its companyId."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
		),
		Handler: ct.handleCompanyGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/create",
		Tier:    middleware.TierR1,
		Summary: "Create a new B2B company account with an initial admin user",
		Tool: mcp.NewTool("b2b_companies_create",
			mcp.WithDescription("Create a B2B company account. Also creates the admin user unless bc_customer_id is provided to link an existing BC customer. Required by the B2B API: company_name, company_email, company_phone, company_country, admin_first_name, admin_last_name, admin_email. Preview → confirm."),
			mcp.WithString("company_name", mcp.Description("Company name"), mcp.Required()),
			mcp.WithString("company_email", mcp.Description("Company contact email."), mcp.Required()),
			mcp.WithString("company_phone", mcp.Description("Company phone number."), mcp.Required()),
			mcp.WithString("company_country", mcp.Description("Country: full name or ISO2 code (e.g. US)."), mcp.Required()),
			mcp.WithString("company_address1", mcp.Description("Address line 1.")),
			mcp.WithString("company_city", mcp.Description("City.")),
			mcp.WithString("company_state", mcp.Description("State or province.")),
			mcp.WithString("company_zip", mcp.Description("Zip/postal code.")),
			mcp.WithString("admin_email", mcp.Description("Admin user email. Required unless bc_customer_id is provided."), mcp.Required()),
			mcp.WithString("admin_first_name", mcp.Description("Admin first name."), mcp.Required()),
			mcp.WithString("admin_last_name", mcp.Description("Admin last name."), mcp.Required()),
			mcp.WithNumber("bc_customer_id", mcp.Description("Link existing BC customer as admin instead of creating a new user.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create the company.")),
		),
		Handler: ct.handleCompanyCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/update",
		Tier:    middleware.TierR1,
		Summary: "Update a B2B company's profile fields (preview then confirm)",
		Tool: mcp.NewTool("b2b_companies_update",
			mcp.WithDescription("Update a B2B company's name, contact info, address, or customer group. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithString("company_name", mcp.Description("New company name.")),
			mcp.WithString("company_email", mcp.Description("New contact email.")),
			mcp.WithString("company_phone", mcp.Description("New phone.")),
			mcp.WithString("company_address1", mcp.Description("New address line 1.")),
			mcp.WithString("company_city", mcp.Description("New city.")),
			mcp.WithString("company_state", mcp.Description("New state.")),
			mcp.WithString("company_country", mcp.Description("New country.")),
			mcp.WithString("company_zip", mcp.Description("New zip code.")),
			mcp.WithString("description", mcp.Description("Company description.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleCompanyUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/set_status",
		Tier:    middleware.TierR2,
		Summary: "Approve, reject, or deactivate a B2B company account",
		Tool: mcp.NewTool("b2b_companies_set_status",
			mcp.WithDescription("Change a B2B company's lifecycle status. Actions: approved, rejected, inactive, active. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithString("action",
				mcp.Description("Status action: approved, rejected, inactive, active."),
				mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleCompanySetStatus,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a B2B company account, its users, and linked BC customers",
		Tool: mcp.NewTool("b2b_companies_delete",
			mcp.WithDescription("Permanently delete a B2B company account. This also removes all associated buyer portal users, and by default deletes the linked BigCommerce customer accounts of those users (which BC otherwise leaves orphaned). Set delete_bc_customers=false to keep the BC customer records. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithBoolean("delete_bc_customers", mcp.Description("Also delete the linked BigCommerce customer accounts of this company's users. Defaults to true.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete permanently.")),
		),
		Handler: ct.handleCompanyDelete,
	})
}

// ---- company handlers ----

func (ct *CompanyTools) handleCompanyList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["status"].(float64); ok {
		params.Set("companyStatus", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["company_name"].(string); ok && v != "" {
		params.Set("companyName", v)
	}
	if v, ok := args["email"].(string); ok && v != "" {
		params.Set("companyEmail", v)
	}

	companies, err := ct.bc.ListB2BCompanies(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B companies: %v", err), nil
	}
	views := make([]map[string]any, len(companies))
	for i, co := range companies {
		views[i] = companyView(co)
	}
	return shared.ToolJSON(map[string]any{"total": len(companies), "companies": views})
}

func (ct *CompanyTools) handleCompanyGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	co, err := ct.bc.GetB2BCompany(ctx, id)
	if err != nil {
		return shared.ToolError("failed to get B2B company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"company": companyView(*co)})
}

func (ct *CompanyTools) handleCompanyCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	name, ok := args["company_name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return shared.ToolError("company_name is required"), nil
	}

	payload := bigcommerce.B2BCompanyCreate{CompanyName: name}
	if v, ok := args["company_email"].(string); ok { payload.CompanyEmail = v }
	if v, ok := args["company_phone"].(string); ok { payload.CompanyPhone = v }
	if v, ok := args["company_address1"].(string); ok { payload.AddressLine1 = v }
	if v, ok := args["company_city"].(string); ok { payload.City = v }
	if v, ok := args["company_state"].(string); ok { payload.State = v }
	if v, ok := args["company_country"].(string); ok { payload.Country = v }
	if v, ok := args["company_zip"].(string); ok { payload.ZipCode = v }
	if v, ok := args["admin_email"].(string); ok { payload.AdminEmail = v }
	if v, ok := args["admin_first_name"].(string); ok { payload.AdminFirstName = v }
	if v, ok := args["admin_last_name"].(string); ok { payload.AdminLastName = v }
	if v, ok := args["bc_customer_id"].(float64); ok && v > 0 { payload.BCCustomerID = int(v) }

	// Required fields per the BigCommerce B2B Edition API (POST /companies):
	// companyName, companyEmail, companyPhone, country, adminFirstName,
	// adminLastName, adminEmail. Enforcing them here yields a clear error
	// instead of an opaque BC 422.
	// https://docs.bigcommerce.com/developer/learn/courses/b2b-core/company/rest-company-management
	if payload.AdminEmail == "" {
		return shared.ToolError("admin_email is required (use the BC customer's email when providing bc_customer_id)"), nil
	}
	if payload.CompanyPhone == "" {
		return shared.ToolError("company_phone is required"), nil
	}
	if payload.CompanyEmail == "" {
		return shared.ToolError("company_email is required by the B2B API"), nil
	}
	if payload.Country == "" {
		return shared.ToolError("company_country is required by the B2B API (full country name or ISO2 code, e.g. \"US\")"), nil
	}
	if payload.AdminFirstName == "" || payload.AdminLastName == "" {
		return shared.ToolError("admin_first_name and admin_last_name are required by the B2B API"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_company",
			"payload": payload,
			"message": fmt.Sprintf("Will create B2B company %q. Pass confirmed=true.", name),
		})
	}

	co, err := ct.bc.CreateB2BCompany(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create B2B company: %v", err), nil
	}
	// The B2B create response is sparse (essentially just the id). Re-fetch the
	// full record so the caller gets a useful confirmation. Fall back to the
	// sparse view if the re-fetch fails.
	if co != nil && co.CompanyID > 0 {
		if full, gerr := ct.bc.GetB2BCompany(ctx, co.CompanyID); gerr == nil && full != nil {
			co = full
		}
	}
	return shared.ToolJSON(map[string]any{"status": "created", "company": companyView(*co)})
}

func (ct *CompanyTools) handleCompanyUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	patch := bigcommerce.B2BCompanyUpdate{}
	hasField := false
	if v, ok := args["company_name"].(string); ok && v != "" { patch.CompanyName = v; hasField = true }
	if v, ok := args["company_email"].(string); ok && v != "" { patch.CompanyEmail = v; hasField = true }
	if v, ok := args["company_phone"].(string); ok && v != "" { patch.CompanyPhone = v; hasField = true }
	if v, ok := args["company_address1"].(string); ok && v != "" { patch.AddressLine1 = v; hasField = true }
	if v, ok := args["company_city"].(string); ok && v != "" { patch.City = v; hasField = true }
	if v, ok := args["company_state"].(string); ok && v != "" { patch.State = v; hasField = true }
	if v, ok := args["company_country"].(string); ok && v != "" { patch.Country = v; hasField = true }
	if v, ok := args["company_zip"].(string); ok && v != "" { patch.ZipCode = v; hasField = true }
	if v, ok := args["description"].(string); ok && v != "" { patch.Description = v; hasField = true }
	if !hasField {
		return shared.ToolError("at least one field must be provided"), nil
	}

	cacheKey := fmt.Sprintf("b2b_company:%d", id)
	current, err := session.CacheOrFetch(ct.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.B2BCompany, error) {
		return ct.bc.GetB2BCompany(ctx, id)
	})
	if err != nil {
		return shared.ToolError("failed to fetch company %d: %v", id, err), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "update_b2b_company",
			"company_id":  id,
			"current_name": current.CompanyName,
			"patch":       patch,
			"message":     "Pass confirmed=true to apply.",
		})
	}

	ct.cache.ForContext(ctx).Delete(cacheKey)
	updated, err := ct.bc.UpdateB2BCompany(ctx, id, patch)
	if err != nil {
		return shared.ToolError("failed to update B2B company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "company": companyView(*updated)})
}

func (ct *CompanyTools) handleCompanySetStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	action, ok := args["action"].(string)
	if !ok || strings.TrimSpace(action) == "" {
		return shared.ToolError("action is required (approved/rejected/inactive/active)"), nil
	}

	validActions := map[string]bool{"approved": true, "rejected": true, "inactive": true, "active": true}
	if !validActions[strings.ToLower(action)] {
		return shared.ToolError("invalid action %q; must be one of: approved, rejected, inactive, active", action), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "set_b2b_company_status",
			"company_id": id,
			"new_status": action,
			"message":    fmt.Sprintf("Will set company %d status to %q. Pass confirmed=true.", id, action),
		})
	}

	updated, err := ct.bc.SetB2BCompanyStatus(ctx, id, action)
	if err != nil {
		return shared.ToolError("failed to set company %d status: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "company": companyView(*updated)})
}

func (ct *CompanyTools) handleCompanyDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	// Default to cleaning up the linked BC customer accounts. Deleting the B2B
	// company alone leaves them orphaned in the store.
	deleteBCCustomers := true
	if v, ok := args["delete_bc_customers"].(bool); ok {
		deleteBCCustomers = v
	}
	// Without a customer deleter wired in we cannot perform the cleanup.
	if ct.customers == nil {
		deleteBCCustomers = false
	}

	cacheKey := fmt.Sprintf("b2b_company:%d", id)
	co, err := session.CacheOrFetch(ct.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.B2BCompany, error) {
		return ct.bc.GetB2BCompany(ctx, id)
	})
	if err != nil {
		return shared.ToolError("failed to fetch company %d: %v", id, err), nil
	}

	// Resolve the linked BC customer accounts up front so both the preview and
	// the confirm step operate on the same set. Company users must be listed
	// before the company is deleted.
	var linkedCustomers []map[string]any
	var bcCustomerIDs []int
	var usersLookupErr string
	if deleteBCCustomers {
		params := url.Values{}
		params.Set("companyId", fmt.Sprintf("%d", id))
		users, uerr := ct.bc.ListB2BUsers(ctx, params.Encode())
		if uerr != nil {
			usersLookupErr = uerr.Error()
		}

		// The B2B user's bcCustomerId is frequently 0 (e.g. admins created via
		// company-create), so resolve the remaining links by matching each
		// user's email against the core customer store in a single query.
		var emails []string
		for _, u := range users {
			if u.BCCustomerID <= 0 && strings.TrimSpace(u.Email) != "" {
				emails = append(emails, u.Email)
			}
		}
		emailToID := map[string]int{}
		if len(emails) > 0 && ct.customers != nil {
			custs, serr := ct.customers.SearchCustomers(ctx, map[string]string{"email:in": strings.Join(emails, ",")})
			if serr != nil && usersLookupErr == "" {
				usersLookupErr = serr.Error()
			}
			for _, cu := range custs {
				if cu.ID > 0 {
					emailToID[strings.ToLower(strings.TrimSpace(cu.Email))] = cu.ID
				}
			}
		}

		seen := map[int]bool{}
		roleLabel := map[int]string{0: "admin", 1: "senior_buyer", 2: "junior_buyer"}
		for _, u := range users {
			cid := u.BCCustomerID
			if cid <= 0 {
				cid = emailToID[strings.ToLower(strings.TrimSpace(u.Email))]
			}
			if cid <= 0 || seen[cid] {
				continue
			}
			seen[cid] = true
			bcCustomerIDs = append(bcCustomerIDs, cid)
			linkedCustomers = append(linkedCustomers, map[string]any{
				"bc_customer_id": cid,
				"email":          u.Email,
				"name":           strings.TrimSpace(u.FirstName + " " + u.LastName),
				"role_label":     roleLabel[u.Role],
			})
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		preview := map[string]any{
			"status":              "preview",
			"action":              "delete_b2b_company",
			"company_id":          id,
			"company_name":        co.CompanyName,
			"delete_bc_customers": deleteBCCustomers,
			"linked_bc_customers": linkedCustomers,
		}
		msg := fmt.Sprintf("Will permanently delete company %q (ID %d) and all its buyer-portal users.", co.CompanyName, id)
		if deleteBCCustomers {
			if len(bcCustomerIDs) > 0 {
				msg += fmt.Sprintf(" It will ALSO permanently delete %d linked BigCommerce customer account(s): %s.", len(bcCustomerIDs), shared.JoinInts(bcCustomerIDs))
			} else {
				msg += " No linked BigCommerce customer accounts were found to delete."
			}
		} else {
			msg += " Linked BigCommerce customer accounts will be kept."
		}
		if usersLookupErr != "" {
			preview["users_lookup_error"] = usersLookupErr
			msg += " (Warning: could not list company users, so linked customers may be incomplete.)"
		}
		preview["message"] = msg + " Pass confirmed=true."
		return shared.ToolJSON(preview)
	}

	ct.cache.ForContext(ctx).Delete(cacheKey)
	if err := ct.bc.DeleteB2BCompany(ctx, id); err != nil {
		return shared.ToolError("failed to delete company %d: %v", id, err), nil
	}

	result := map[string]any{"status": "deleted", "company_id": id}
	if deleteBCCustomers && len(bcCustomerIDs) > 0 {
		result["bc_customer_ids"] = bcCustomerIDs
		if err := ct.customers.DeleteCustomers(ctx, bcCustomerIDs); err != nil {
			// The company is already gone; surface the customer-cleanup failure
			// rather than masking it as a full success.
			result["status"] = "partial_success"
			result["bc_customers_deleted"] = false
			result["bc_customer_delete_error"] = err.Error()
			result["message"] = fmt.Sprintf("Company %d deleted, but failed to delete %d linked BC customer(s): %v", id, len(bcCustomerIDs), err)
		} else {
			result["bc_customers_deleted"] = true
			result["message"] = fmt.Sprintf("Company %d and %d linked BC customer account(s) deleted.", id, len(bcCustomerIDs))
		}
	}
	return shared.ToolJSON(result)
}

// ============================================================
// User tools
// ============================================================

func (ct *CompanyTools) registerUserTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/users/list",
		Tier:    middleware.TierR0,
		Summary: "List buyer portal users for a company or across all companies",
		Tool: mcp.NewTool("b2b_companies_users_list",
			mcp.WithDescription("List B2B Edition buyer portal users. Filter by company, role, or email."),
			mcp.WithNumber("company_id", mcp.Description("Filter by company ID.")),
			mcp.WithNumber("role", mcp.Description("Filter by role: 0=admin, 1=senior buyer, 2=junior buyer.")),
			mcp.WithString("email", mcp.Description("Filter by email address.")),
		),
		Handler: ct.handleUserList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/users/create",
		Tier:    middleware.TierR1,
		Summary: "Create a buyer portal user and assign them to a company",
		Tool: mcp.NewTool("b2b_companies_users_create",
			mcp.WithDescription("Create a B2B buyer portal user. Roles: 0=admin, 1=senior buyer, 2=junior buyer. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID to assign the user to"), mcp.Required()),
			mcp.WithString("email", mcp.Description("User email address"), mcp.Required()),
			mcp.WithString("first_name", mcp.Description("First name"), mcp.Required()),
			mcp.WithString("last_name", mcp.Description("Last name"), mcp.Required()),
			mcp.WithNumber("role", mcp.Description("Role: 0=admin, 1=senior buyer, 2=junior buyer"), mcp.Required()),
			mcp.WithString("phone", mcp.Description("Phone number.")),
			mcp.WithNumber("bc_customer_id", mcp.Description("Link an existing BC customer ID instead of creating a new account.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create the user.")),
		),
		Handler: ct.handleUserCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/users/update",
		Tier:    middleware.TierR1,
		Summary: "Update a buyer portal user's name, phone, or role",
		Tool: mcp.NewTool("b2b_companies_users_update",
			mcp.WithDescription("Update a B2B buyer portal user's profile or role. Preview → confirm."),
			mcp.WithNumber("user_id", mcp.Description("User ID"), mcp.Required()),
			mcp.WithString("first_name", mcp.Description("New first name.")),
			mcp.WithString("last_name", mcp.Description("New last name.")),
			mcp.WithString("phone", mcp.Description("New phone.")),
			mcp.WithNumber("role", mcp.Description("New role: 0=admin, 1=senior buyer, 2=junior buyer.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleUserUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/users/delete",
		Tier:    middleware.TierR2,
		Summary: "Remove a buyer portal user from the B2B Edition portal",
		Tool: mcp.NewTool("b2b_companies_users_delete",
			mcp.WithDescription("Remove a user from the B2B Edition buyer portal. Their BC customer account is not deleted. Preview → confirm."),
			mcp.WithNumber("user_id", mcp.Description("User ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to remove the user.")),
		),
		Handler: ct.handleUserDelete,
	})
}

// ---- user handlers ----

func (ct *CompanyTools) handleUserList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["company_id"].(float64); ok && v > 0 { params.Set("companyId", fmt.Sprintf("%d", int(v))) }
	if v, ok := args["role"].(float64); ok { params.Set("roles", fmt.Sprintf("%d", int(v))) }
	if v, ok := args["email"].(string); ok && v != "" { params.Set("email", v) }

	users, err := ct.bc.ListB2BUsers(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B users: %v", err), nil
	}
	views := make([]map[string]any, len(users))
	for i, u := range users {
		views[i] = userView(u)
	}
	return shared.ToolJSON(map[string]any{"total": len(users), "users": views})
}

func (ct *CompanyTools) handleUserCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cid, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	email, _ := args["email"].(string)
	if strings.TrimSpace(email) == "" {
		return shared.ToolError("email is required"), nil
	}
	firstName, _ := args["first_name"].(string)
	lastName, _ := args["last_name"].(string)
	roleRaw, hasRole := args["role"].(float64)
	if !hasRole {
		return shared.ToolError("role is required (0=admin, 1=senior buyer, 2=junior buyer)"), nil
	}

	payload := bigcommerce.B2BUserCreate{
		CompanyID: cid,
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Role:      int(roleRaw),
	}
	if v, ok := args["phone"].(string); ok { payload.PhoneNumber = v }
	if v, ok := args["bc_customer_id"].(float64); ok && v > 0 { payload.BCCustomerID = int(v) }

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_user",
			"payload": payload,
			"message": fmt.Sprintf("Will create B2B user %s for company %d. Pass confirmed=true.", email, cid),
		})
	}

	u, err := ct.bc.CreateB2BUser(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create B2B user: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "user": userView(*u)})
}

func (ct *CompanyTools) handleUserUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	uid, err := shared.ReadPositiveInt(args, "user_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	patch := bigcommerce.B2BUserUpdate{}
	hasField := false
	if v, ok := args["first_name"].(string); ok && v != "" { patch.FirstName = v; hasField = true }
	if v, ok := args["last_name"].(string); ok && v != "" { patch.LastName = v; hasField = true }
	if v, ok := args["phone"].(string); ok && v != "" { patch.PhoneNumber = v; hasField = true }
	if v, ok := args["role"].(float64); ok {
		r := int(v)
		patch.Role = &r
		hasField = true
	}
	if !hasField {
		return shared.ToolError("at least one field must be provided"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "update_b2b_user",
			"user_id": uid,
			"patch":   patch,
			"message": "Pass confirmed=true to apply.",
		})
	}

	u, err := ct.bc.UpdateB2BUser(ctx, uid, patch)
	if err != nil {
		return shared.ToolError("failed to update B2B user %d: %v", uid, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "user": userView(*u)})
}

func (ct *CompanyTools) handleUserDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	uid, err := shared.ReadPositiveInt(args, "user_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "delete_b2b_user",
			"user_id": uid,
			"message": fmt.Sprintf("Will remove user %d from the B2B buyer portal. Their BC customer account is not deleted. Pass confirmed=true.", uid),
		})
	}

	if err := ct.bc.DeleteB2BUser(ctx, uid); err != nil {
		return shared.ToolError("failed to delete B2B user %d: %v", uid, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "user_id": uid})
}

// ============================================================
// Address tools
// ============================================================

func (ct *CompanyTools) registerAddressTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/addresses/list",
		Tier:    middleware.TierR0,
		Summary: "List company addresses with optional billing/shipping filters",
		Tool: mcp.NewTool("b2b_companies_addresses_list",
			mcp.WithDescription("List addresses for one or all companies. Filter by company, billing/shipping type, city, country."),
			mcp.WithNumber("company_id", mcp.Description("Filter by company ID.")),
			mcp.WithBoolean("is_billing", mcp.Description("Filter to billing addresses only.")),
			mcp.WithBoolean("is_shipping", mcp.Description("Filter to shipping addresses only.")),
			mcp.WithString("country", mcp.Description("Filter by country.")),
			mcp.WithString("city", mcp.Description("Filter by city.")),
		),
		Handler: ct.handleAddressList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/addresses/create",
		Tier:    middleware.TierR1,
		Summary: "Add a new address to a B2B company account",
		Tool: mcp.NewTool("b2b_companies_addresses_create",
			mcp.WithDescription("Create an address for a B2B company. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithString("address_line1", mcp.Description("Address line 1"), mcp.Required()),
			mcp.WithString("city", mcp.Description("City"), mcp.Required()),
			mcp.WithString("country", mcp.Description("Country"), mcp.Required()),
			mcp.WithString("address_line2", mcp.Description("Address line 2.")),
			mcp.WithString("state", mcp.Description("State or province.")),
			mcp.WithString("zip_code", mcp.Description("Zip/postal code.")),
			mcp.WithString("phone", mcp.Description("Phone.")),
			mcp.WithString("label", mcp.Description("Address label (e.g. HQ, Warehouse).")),
			mcp.WithString("first_name", mcp.Description("Contact first name.")),
			mcp.WithString("last_name", mcp.Description("Contact last name.")),
				mcp.WithString("state_code", mcp.Description("2-letter state/province code (e.g. CA, NY, TX). Required by B2B API.")),
			mcp.WithString("country_code", mcp.Description("ISO 2-letter country code (e.g. US, CA). Defaults to US.")),
				mcp.WithBoolean("is_billing", mcp.Description("Mark as billing address.")),
			mcp.WithBoolean("is_shipping", mcp.Description("Mark as shipping address.")),
			mcp.WithBoolean("is_default_billing", mcp.Description("Set as the company's default billing address.")),
			mcp.WithBoolean("is_default_shipping", mcp.Description("Set as the company's default shipping address.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create.")),
		),
		Handler: ct.handleAddressCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/addresses/update",
		Tier:    middleware.TierR1,
		Summary: "Update a B2B company address by ID",
		Tool: mcp.NewTool("b2b_companies_addresses_update",
			mcp.WithDescription("Update a B2B company address. Provide all fields — this is a full PUT, not a patch. Preview → confirm."),
			mcp.WithNumber("address_id", mcp.Description("Address ID"), mcp.Required()),
			mcp.WithNumber("company_id", mcp.Description("Company ID the address belongs to"), mcp.Required()),
			mcp.WithString("address_line1", mcp.Description("Address line 1"), mcp.Required()),
			mcp.WithString("city", mcp.Description("City"), mcp.Required()),
			mcp.WithString("country", mcp.Description("Country"), mcp.Required()),
			mcp.WithString("address_line2", mcp.Description("Address line 2.")),
			mcp.WithString("state", mcp.Description("State or province.")),
			mcp.WithString("state_code", mcp.Description("2-letter state/province code (e.g. CA, NY, TX). Required by B2B API.")),
			mcp.WithString("country_code", mcp.Description("ISO 2-letter country code (e.g. US, CA). Defaults to US.")),
			mcp.WithString("zip_code", mcp.Description("Zip/postal code.")),
			mcp.WithString("phone", mcp.Description("Phone.")),
			mcp.WithString("label", mcp.Description("Label.")),
			mcp.WithString("first_name", mcp.Description("Contact first name.")),
			mcp.WithString("last_name", mcp.Description("Contact last name.")),
			mcp.WithBoolean("is_billing", mcp.Description("Mark as billing address.")),
			mcp.WithBoolean("is_shipping", mcp.Description("Mark as shipping address.")),
			mcp.WithBoolean("is_default_billing", mcp.Description("Set as the default billing address.")),
			mcp.WithBoolean("is_default_shipping", mcp.Description("Set as the default shipping address.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleAddressUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/addresses/delete",
		Tier:    middleware.TierR2,
		Summary: "Remove an address from a B2B company account",
		Tool: mcp.NewTool("b2b_companies_addresses_delete",
			mcp.WithDescription("Delete a B2B company address. Existing quotes/invoices/orders referencing this address are not affected. Preview → confirm."),
			mcp.WithNumber("address_id", mcp.Description("Address ID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete.")),
		),
		Handler: ct.handleAddressDelete,
	})
}

// ---- address handlers ----

func (ct *CompanyTools) handleAddressList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params := url.Values{}
	if v, ok := args["company_id"].(float64); ok && v > 0 { params.Set("companyId", fmt.Sprintf("%d", int(v))) }
	if v, ok := args["is_billing"].(bool); ok { params.Set("isBilling", fmt.Sprintf("%v", v)) }
	if v, ok := args["is_shipping"].(bool); ok { params.Set("isShipping", fmt.Sprintf("%v", v)) }
	if v, ok := args["country"].(string); ok && v != "" { params.Set("country", v) }
	if v, ok := args["city"].(string); ok && v != "" { params.Set("city", v) }

	addrs, err := ct.bc.ListB2BAddresses(ctx, params.Encode())
	if err != nil {
		return shared.ToolError("failed to list B2B addresses: %v", err), nil
	}
	views := make([]map[string]any, len(addrs))
	for i, a := range addrs {
		views[i] = addressView(a)
	}
	return shared.ToolJSON(map[string]any{"total": len(addrs), "addresses": views})
}

func (ct *CompanyTools) handleAddressCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cid, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	addr1, _ := args["address_line1"].(string)
	city, _ := args["city"].(string)
	country, _ := args["country"].(string)
	if strings.TrimSpace(addr1) == "" || strings.TrimSpace(city) == "" || strings.TrimSpace(country) == "" {
		return shared.ToolError("address_line1, city, and country are required"), nil
	}

	payload := bigcommerce.B2BAddressCreate{
		CompanyID:    cid,
		AddressLine1: addr1,
		City:         city,
		Country:      country,
	}
	if v, ok := args["address_line2"].(string); ok { payload.AddressLine2 = v }
	if v, ok := args["state"].(string); ok { payload.State = v }
	if v, ok := args["state_code"].(string); ok { payload.StateCode = v }
	if v, ok := args["country_code"].(string); ok { payload.CountryCode = v } else { payload.CountryCode = "US" }
	if v, ok := args["zip_code"].(string); ok { payload.ZipCode = v }
	if v, ok := args["phone"].(string); ok { payload.PhoneNumber = v }
	if v, ok := args["label"].(string); ok { payload.Label = v }
	if v, ok := args["first_name"].(string); ok { payload.FirstName = v }
	if v, ok := args["last_name"].(string); ok { payload.LastName = v }
	if v, ok := args["is_billing"].(bool); ok { payload.IsBilling = v }
	if v, ok := args["is_shipping"].(bool); ok { payload.IsShipping = v }
	if v, ok := args["is_default_billing"].(bool); ok { payload.IsDefaultBilling = v }
	if v, ok := args["is_default_shipping"].(bool); ok { payload.IsDefaultShipping = v }

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_b2b_address",
			"payload": payload,
			"message": "Pass confirmed=true to create the address.",
		})
	}

	a, err := ct.bc.CreateB2BAddress(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create B2B address: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "address": addressView(*a)})
}

func (ct *CompanyTools) handleAddressUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	addrID, err := shared.ReadPositiveInt(args, "address_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	cid, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	addr1, _ := args["address_line1"].(string)
	city, _ := args["city"].(string)
	country, _ := args["country"].(string)
	if strings.TrimSpace(addr1) == "" || strings.TrimSpace(city) == "" || strings.TrimSpace(country) == "" {
		return shared.ToolError("address_line1, city, and country are required"), nil
	}

	payload := bigcommerce.B2BAddressCreate{CompanyID: cid, AddressLine1: addr1, City: city, Country: country}
	if v, ok := args["address_line2"].(string); ok { payload.AddressLine2 = v }
	if v, ok := args["state"].(string); ok { payload.State = v }
	if v, ok := args["state_code"].(string); ok { payload.StateCode = v }
	if v, ok := args["country_code"].(string); ok { payload.CountryCode = v } else { payload.CountryCode = "US" }
	if v, ok := args["zip_code"].(string); ok { payload.ZipCode = v }
	if v, ok := args["phone"].(string); ok { payload.PhoneNumber = v }
	if v, ok := args["label"].(string); ok { payload.Label = v }
	if v, ok := args["first_name"].(string); ok { payload.FirstName = v }
	if v, ok := args["last_name"].(string); ok { payload.LastName = v }
	if v, ok := args["is_billing"].(bool); ok { payload.IsBilling = v }
	if v, ok := args["is_shipping"].(bool); ok { payload.IsShipping = v }
	if v, ok := args["is_default_billing"].(bool); ok { payload.IsDefaultBilling = v }
	if v, ok := args["is_default_shipping"].(bool); ok { payload.IsDefaultShipping = v }

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "update_b2b_address",
			"address_id": addrID,
			"payload":    payload,
			"message":    "Pass confirmed=true to apply.",
		})
	}

	a, err := ct.bc.UpdateB2BAddress(ctx, addrID, payload)
	if err != nil {
		return shared.ToolError("failed to update B2B address %d: %v", addrID, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "updated", "address": addressView(*a)})
}

func (ct *CompanyTools) handleAddressDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	addrID, err := shared.ReadPositiveInt(args, "address_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":     "preview",
			"action":     "delete_b2b_address",
			"address_id": addrID,
			"message":    fmt.Sprintf("Will delete address %d. Existing orders/quotes/invoices are not affected. Pass confirmed=true.", addrID),
		})
	}

	if err := ct.bc.DeleteB2BAddress(ctx, addrID); err != nil {
		return shared.ToolError("failed to delete B2B address %d: %v", addrID, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "deleted", "address_id": addrID})
}

// ============================================================
// View helpers
// ============================================================

func companyView(co bigcommerce.B2BCompany) map[string]any {
	statusLabel := map[int]string{0: "pending", 1: "approved", 2: "rejected", 3: "inactive"}
	v := map[string]any{
		"company_id":     co.CompanyID,
		"company_name":   co.CompanyName,
		"company_email":  co.CompanyEmail,
		"company_phone":  co.CompanyPhone,
		"company_status": co.CompanyStatus,
		"status_label":   statusLabel[co.CompanyStatus],
		"city":           co.City,
		"state":          co.State,
		"country":        co.Country,
		"bc_group_id":    co.BCGroupID,
		"bc_group_name":  co.BCGroupName,
		"created_at":     co.CreatedAt,
		"updated_at":     co.UpdatedAt,
	}
	if co.AddressLine1 != "" {
		v["address_line1"] = co.AddressLine1
	}
	if co.ParentCompany != nil && co.ParentCompany.ID != nil {
		v["parent_company_id"] = *co.ParentCompany.ID
	}
	return v
}

func userView(u bigcommerce.B2BUser) map[string]any {
	roleLabel := map[int]string{0: "admin", 1: "senior_buyer", 2: "junior_buyer"}
	return map[string]any{
		"id":             u.ID,
		"company_id":     u.CompanyID,
		"email":          u.Email,
		"first_name":     u.FirstName,
		"last_name":      u.LastName,
		"phone":          u.PhoneNumber,
		"role":           u.Role,
		"role_label":     roleLabel[u.Role],
		"bc_customer_id": u.BCCustomerID,
		"created_at":     u.CreatedAt,
	}
}

func addressView(a bigcommerce.B2BAddress) map[string]any {
	return map[string]any{
		"address_id":          a.AddressID,
		"company_id":          a.CompanyID.String(),
		"label":               a.Label,
		"first_name":          a.FirstName,
		"last_name":           a.LastName,
		"address_line1":       a.AddressLine1,
		"address_line2":       a.AddressLine2,
		"city":                a.City,
		"state":               a.StateName,
		"state_code":          a.StateCode,
		"country":             a.CountryName,
		"country_code":        a.CountryCode,
		"zip_code":            a.ZipCode,
		"is_billing":          a.IsBilling,
		"is_shipping":         a.IsShipping,
		"is_default_billing":  a.IsDefaultBilling,
		"is_default_shipping": a.IsDefaultShipping,
	}
}
