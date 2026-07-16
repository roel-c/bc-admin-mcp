package bigcommerce

import (
	"context"
	"fmt"
)

// B2BSalesStaffAssignment is one entry in the PUT /sales-staffs/{id} request
// body: assigns (true) or unassigns (false) a company from a sales staff
// account. Unlisted companies are left unaffected (non-destructive).
type B2BSalesStaffAssignment struct {
	CompanyID    int  `json:"companyId"`
	AssignStatus bool `json:"assignStatus"`
}

// Sales staff list/detail response bodies are underdocumented in the OpenAPI
// spec (empty object schemas), so they are passed through as generic maps.

// ListB2BSalesStaff returns B2B Edition users assigned a Sales Staff role.
// Optional params: limit, offset, orderBy (ASC/DESC), sortBy (updated_at/email),
// companyId.
func (c *B2BClient) ListB2BSalesStaff(ctx context.Context, params string) ([]map[string]any, error) {
	path := "sales-staffs"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B sales staff: %w", err)
	}
	return unmarshalMapSlice(raw, "sales staff")
}

// GetB2BSalesStaff returns detail for one sales staff account, including
// company assignments and assignment timestamps.
func (c *B2BClient) GetB2BSalesStaff(ctx context.Context, salesStaffID int) (map[string]any, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("sales-staffs/%d", salesStaffID))
	if err != nil {
		return nil, fmt.Errorf("get B2B sales staff %d: %w", salesStaffID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B sales staff"); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateB2BSalesStaffAssignments assigns/unassigns companies for a sales staff
// account. Only the companies listed in assignments are affected.
func (c *B2BClient) UpdateB2BSalesStaffAssignments(ctx context.Context, salesStaffID int, assignments []B2BSalesStaffAssignment) (map[string]any, error) {
	respBody, err := c.B2BPut(ctx, fmt.Sprintf("sales-staffs/%d", salesStaffID), assignments)
	if err != nil {
		return nil, fmt.Errorf("update B2B sales staff %d assignments: %w", salesStaffID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(respBody, &out, "update B2B sales staff assignments"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}
