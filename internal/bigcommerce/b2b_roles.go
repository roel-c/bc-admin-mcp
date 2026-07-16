package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// flexString unmarshals from either a JSON string or a JSON number into a Go
// string, and marshals back as a JSON string. The B2B Edition API is
// inconsistent about these enum-like fields (e.g. roleType/permissionLevel are
// documented as strings but returned as numbers).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" || s == "" {
		*f = ""
		return nil
	}
	s = strings.Trim(s, "\"")
	*f = flexString(s)
	return nil
}

// ---- Company role & permission types (B2B Edition RBAC) ----

// B2BRolePermission is one permission entry on a company role. Code is a
// permission code string (see the B2B "Permission Codes" reference);
// permissionLevel is the scope: "1" (user) or "2" (company), and some
// permissions also support "3" (company and subsidiaries). permissionLevel is
// sent as a JSON string on writes and tolerates number/string on reads.
type B2BRolePermission struct {
	Code            string     `json:"code"`
	PermissionLevel flexString `json:"permissionLevel"`
}

// B2BRole is a company user role. RoleType: 1=predefined, 2=custom. Predefined
// roles cannot be updated or deleted.
type B2BRole struct {
	ID          int                 `json:"id,omitempty"`
	Name        string              `json:"name"`
	RoleType    flexString          `json:"roleType,omitempty"`
	RoleLevel   flexString          `json:"roleLevel,omitempty"`
	Permissions []B2BRolePermission `json:"permissions,omitempty"`
}

// B2BRoleCreate is the request body for POST/PUT company roles.
type B2BRoleCreate struct {
	Name        string              `json:"name"`
	Permissions []B2BRolePermission `json:"permissions"`
}

// B2BPermission is a company permission definition.
type B2BPermission struct {
	ID          json.Number `json:"id,omitempty"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Code        string      `json:"code"`
	ModuleName  string      `json:"moduleName,omitempty"`
}

// B2BPermissionCreate is the request body for POST/PUT company permissions.
type B2BPermissionCreate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Code        string `json:"code"`
	ModuleName  string `json:"moduleName,omitempty"`
}

// ---- Role client methods ----

// ListB2BRoles returns company roles matching optional params (q/offset/limit).
func (c *B2BClient) ListB2BRoles(ctx context.Context, params string) ([]B2BRole, error) {
	path := "companies/roles"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B roles: %w", err)
	}
	out := make([]B2BRole, 0, len(raw))
	for _, r := range raw {
		var role B2BRole
		if err := json.Unmarshal(r, &role); err != nil {
			return nil, fmt.Errorf("unmarshal B2B role: %w", err)
		}
		out = append(out, role)
	}
	return out, nil
}

// GetB2BRole fetches a single company role by ID.
func (c *B2BClient) GetB2BRole(ctx context.Context, roleID int) (*B2BRole, error) {
	body, err := c.B2BGet(ctx, fmt.Sprintf("companies/roles/%d", roleID))
	if err != nil {
		return nil, fmt.Errorf("get B2B role %d: %w", roleID, err)
	}
	var role B2BRole
	if err := b2bUnmarshalSingle(body, &role, "get B2B role"); err != nil {
		return nil, err
	}
	return &role, nil
}

// CreateB2BRole creates a custom company user role.
func (c *B2BClient) CreateB2BRole(ctx context.Context, payload B2BRoleCreate) (*B2BRole, error) {
	body, err := c.B2BPost(ctx, "companies/roles", payload)
	if err != nil {
		return nil, fmt.Errorf("create B2B role: %w", err)
	}
	var role B2BRole
	if err := b2bUnmarshalSingle(body, &role, "create B2B role"); err != nil {
		return nil, err
	}
	return &role, nil
}

// UpdateB2BRole updates a custom company role. Predefined roles cannot be
// updated. The permissions array must include every permission to keep.
func (c *B2BClient) UpdateB2BRole(ctx context.Context, roleID int, payload B2BRoleCreate) (*B2BRole, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("companies/roles/%d", roleID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B role %d: %w", roleID, err)
	}
	var role B2BRole
	if err := b2bUnmarshalSingle(body, &role, "update B2B role"); err != nil {
		return nil, err
	}
	return &role, nil
}

// DeleteB2BRole deletes a custom company role.
func (c *B2BClient) DeleteB2BRole(ctx context.Context, roleID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("companies/roles/%d", roleID))
	if err != nil {
		return fmt.Errorf("delete B2B role %d: %w", roleID, err)
	}
	return nil
}

// ---- Permission client methods ----

// ListB2BPermissions returns company permission definitions (optional q param).
func (c *B2BClient) ListB2BPermissions(ctx context.Context, params string) ([]B2BPermission, error) {
	path := "companies/permissions"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B permissions: %w", err)
	}
	out := make([]B2BPermission, 0, len(raw))
	for _, r := range raw {
		var p B2BPermission
		if err := json.Unmarshal(r, &p); err != nil {
			return nil, fmt.Errorf("unmarshal B2B permission: %w", err)
		}
		out = append(out, p)
	}
	return out, nil
}

// CreateB2BPermission creates a custom company permission.
func (c *B2BClient) CreateB2BPermission(ctx context.Context, payload B2BPermissionCreate) (*B2BPermission, error) {
	body, err := c.B2BPost(ctx, "companies/permissions", payload)
	if err != nil {
		return nil, fmt.Errorf("create B2B permission: %w", err)
	}
	var p B2BPermission
	if err := b2bUnmarshalSingle(body, &p, "create B2B permission"); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateB2BPermission updates a custom company permission.
func (c *B2BClient) UpdateB2BPermission(ctx context.Context, permissionID int, payload B2BPermissionCreate) (*B2BPermission, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("companies/permissions/%d", permissionID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B permission %d: %w", permissionID, err)
	}
	var p B2BPermission
	if err := b2bUnmarshalSingle(body, &p, "update B2B permission"); err != nil {
		return nil, err
	}
	return &p, nil
}

// DeleteB2BPermission deletes a custom company permission.
func (c *B2BClient) DeleteB2BPermission(ctx context.Context, permissionID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("companies/permissions/%d", permissionID))
	if err != nil {
		return fmt.Errorf("delete B2B permission %d: %w", permissionID, err)
	}
	return nil
}
