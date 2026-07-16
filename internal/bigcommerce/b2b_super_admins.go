package bigcommerce

import (
	"context"
	"fmt"
)

// B2BSuperAdminCreate is the request body for POST /super-admins and
// POST /super-admins/bulk. An existing BigCommerce customer account with this
// email is converted to a Super Admin; creation fails if that email is
// already a B2B company user (any role).
type B2BSuperAdminCreate struct {
	FirstName       string          `json:"firstName"`
	LastName        string          `json:"lastName"`
	Email           string          `json:"email"`
	Phone           string          `json:"phone,omitempty"`
	UUID            string          `json:"uuid,omitempty"`
	OriginChannelID int             `json:"originChannelId,omitempty"`
	ChannelIDs      []int           `json:"channelIds,omitempty"`
	ExtraFields     []B2BExtraField `json:"extraFields,omitempty"`
}

// B2BSuperAdminUpdate is the request body for PUT /super-admins/info/{id}.
// Email is read-only (identity key shared with the BigCommerce customer
// account) and is not included here.
type B2BSuperAdminUpdate struct {
	FirstName   string          `json:"firstName,omitempty"`
	LastName    string          `json:"lastName,omitempty"`
	Phone       string          `json:"phone,omitempty"`
	UUID        string          `json:"uuid,omitempty"`
	ExtraFields []B2BExtraField `json:"extraFields,omitempty"`
}

// B2BSuperAdminCompanyAssignment is one entry in the PUT /super-admins/{id}
// request body (assign/unassign companies for a super admin). Non-destructive
// — unlisted companies are unaffected.
type B2BSuperAdminCompanyAssignment struct {
	CompanyID  int  `json:"companyId"`
	IsAssigned bool `json:"isAssigned"`
}

// B2BCompanySuperAdminAssignment is one entry in the
// PUT /companies/{companyId}/super-admins request body (assign/unassign super
// admins for a company). Non-destructive.
type B2BCompanySuperAdminAssignment struct {
	SuperAdminID int  `json:"superAdminId"`
	IsAssigned   bool `json:"isAssigned"`
}

// Super admin response bodies are underdocumented in the OpenAPI spec (empty
// object schemas), so read endpoints are passed through as generic maps.

// ListB2BSuperAdmins returns Super Admins with a count of assigned companies.
// Optional params: limit, offset, orderBy.
func (c *B2BClient) ListB2BSuperAdmins(ctx context.Context, params string) ([]map[string]any, error) {
	path := "companies/super-admins"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B super admins: %w", err)
	}
	return unmarshalMapSlice(raw, "super admin")
}

// ListB2BSuperAdminCompaniesOverview returns companies with a count of
// assigned Super Admins (the inverse view of ListB2BSuperAdmins).
func (c *B2BClient) ListB2BSuperAdminCompaniesOverview(ctx context.Context, params string) ([]map[string]any, error) {
	path := "super-admins/companies"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B super admin companies overview: %w", err)
	}
	return unmarshalMapSlice(raw, "super admin company overview")
}

// GetB2BSuperAdmin returns detailed account info for one Super Admin.
func (c *B2BClient) GetB2BSuperAdmin(ctx context.Context, superAdminID int) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("super-admins/info/%d", superAdminID))
	if err != nil {
		return nil, fmt.Errorf("get B2B super admin %d: %w", superAdminID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B super admin"); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateB2BSuperAdmin creates (or converts an existing BC customer into) a
// Super Admin account.
func (c *B2BClient) CreateB2BSuperAdmin(ctx context.Context, payload B2BSuperAdminCreate) (map[string]any, error) {
	body, err := c.B2BPost(ctx, "super-admins", payload)
	if err != nil {
		return nil, fmt.Errorf("create B2B super admin: %w", err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "create B2B super admin"); err != nil {
		return nil, err
	}
	return out, nil
}

// BulkCreateB2BSuperAdmins creates up to 10 Super Admins in one call.
func (c *B2BClient) BulkCreateB2BSuperAdmins(ctx context.Context, payloads []B2BSuperAdminCreate) (map[string]any, error) {
	body, err := c.B2BPost(ctx, "super-admins/bulk", payloads)
	if err != nil {
		return nil, fmt.Errorf("bulk create B2B super admins: %w", err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "bulk create B2B super admins"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}

// UpdateB2BSuperAdmin updates a Super Admin's account details. Email cannot
// be changed here.
func (c *B2BClient) UpdateB2BSuperAdmin(ctx context.Context, superAdminID int, payload B2BSuperAdminUpdate) (map[string]any, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("super-admins/info/%d", superAdminID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B super admin %d: %w", superAdminID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "update B2B super admin"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}

// UpdateB2BSuperAdminCompanyAssignments assigns/unassigns companies for one
// Super Admin (from the super admin's perspective).
func (c *B2BClient) UpdateB2BSuperAdminCompanyAssignments(ctx context.Context, superAdminID int, assignments []B2BSuperAdminCompanyAssignment) (map[string]any, error) {
	body := map[string]any{"companies": assignments}
	respBody, err := c.B2BPut(ctx, fmt.Sprintf("super-admins/%d", superAdminID), body)
	if err != nil {
		return nil, fmt.Errorf("update B2B super admin %d company assignments: %w", superAdminID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "update B2B super admin company assignments"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}

// GetB2BSuperAdminCompanies returns the companies assigned to one Super
// Admin.
func (c *B2BClient) GetB2BSuperAdminCompanies(ctx context.Context, superAdminID int, params string) ([]map[string]any, error) {
	path := fmt.Sprintf("super-admins/%d/companies", superAdminID)
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get companies for B2B super admin %d: %w", superAdminID, err)
	}
	return unmarshalMapSlice(raw, "super admin's company")
}

// ListB2BCompanySuperAdmins returns the Super Admins assigned to one company
// (from the company's perspective; includes extended account data).
func (c *B2BClient) ListB2BCompanySuperAdmins(ctx context.Context, companyID int, params string) ([]map[string]any, error) {
	path := fmt.Sprintf("companies/%d/super-admins", companyID)
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list super admins for B2B company %d: %w", companyID, err)
	}
	return unmarshalMapSlice(raw, "company's super admin")
}

// UpdateB2BCompanySuperAdminAssignments assigns/unassigns Super Admins for
// one company (from the company's perspective).
func (c *B2BClient) UpdateB2BCompanySuperAdminAssignments(ctx context.Context, companyID int, assignments []B2BCompanySuperAdminAssignment) (map[string]any, error) {
	body := map[string]any{"superAdmins": assignments}
	respBody, err := c.B2BPut(ctx, fmt.Sprintf("companies/%d/super-admins", companyID), body)
	if err != nil {
		return nil, fmt.Errorf("update super admin assignments for B2B company %d: %w", companyID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "update B2B company super admin assignments"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}
