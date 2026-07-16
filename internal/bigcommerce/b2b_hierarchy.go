package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// B2BHierarchyNode is a company node in an Account Hierarchy tree. The
// subsidiaries array can nest recursively for multi-layer hierarchies.
type B2BHierarchyNode struct {
	CompanyID       int                `json:"companyId"`
	CompanyName     string             `json:"companyName"`
	CompanyStatus   int                `json:"companyStatus"`
	ParentCompanyID *int               `json:"parentCompanyId,omitempty"`
	Subsidiaries    []B2BHierarchyNode `json:"subsidiaries,omitempty"`
}

// b2bParentAttach is the request body for POST /companies/{companyId}/parent.
type b2bParentAttach struct {
	ParentCompanyID int `json:"parentCompanyId"`
}

// ListB2BCompanySubsidiaries returns the subsidiary accounts on lower hierarchy
// layers of the given company. Optional params support offset/limit.
func (c *B2BClient) ListB2BCompanySubsidiaries(ctx context.Context, companyID int, params string) ([]B2BHierarchyNode, error) {
	path := fmt.Sprintf("companies/%d/subsidiaries", companyID)
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B company %d subsidiaries: %w", companyID, err)
	}
	out := make([]B2BHierarchyNode, 0, len(raw))
	for _, r := range raw {
		var n B2BHierarchyNode
		if err := json.Unmarshal(r, &n); err != nil {
			return nil, fmt.Errorf("unmarshal B2B subsidiary: %w", err)
		}
		out = append(out, n)
	}
	return out, nil
}

// ListB2BCompanyHierarchy returns all parent and child accounts in the
// hierarchy of the given company. Optional params support offset/limit.
func (c *B2BClient) ListB2BCompanyHierarchy(ctx context.Context, companyID int, params string) ([]B2BHierarchyNode, error) {
	path := fmt.Sprintf("companies/%d/hierarchy", companyID)
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B company %d hierarchy: %w", companyID, err)
	}
	out := make([]B2BHierarchyNode, 0, len(raw))
	for _, r := range raw {
		var n B2BHierarchyNode
		if err := json.Unmarshal(r, &n); err != nil {
			return nil, fmt.Errorf("unmarshal B2B hierarchy node: %w", err)
		}
		out = append(out, n)
	}
	return out, nil
}

// AttachB2BCompanyParent assigns parentCompanyID as the parent of companyID via
// POST /companies/{companyId}/parent. A company already at a higher layer than
// the target cannot be assigned as its parent.
func (c *B2BClient) AttachB2BCompanyParent(ctx context.Context, companyID, parentCompanyID int) error {
	_, err := c.B2BPost(ctx, fmt.Sprintf("companies/%d/parent", companyID), b2bParentAttach{ParentCompanyID: parentCompanyID})
	if err != nil {
		return fmt.Errorf("attach parent %d to B2B company %d: %w", parentCompanyID, companyID, err)
	}
	return nil
}

// DeleteB2BCompanySubsidiary removes the parent-child relationship between a
// subsidiary (childCompanyID) and its parent (companyID). If the subsidiary has
// its own subsidiaries, they form a new top-level hierarchy.
func (c *B2BClient) DeleteB2BCompanySubsidiary(ctx context.Context, companyID, childCompanyID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("companies/%d/subsidiaries/%d", companyID, childCompanyID))
	if err != nil {
		return fmt.Errorf("delete subsidiary %d from B2B company %d: %w", childCompanyID, companyID, err)
	}
	return nil
}
