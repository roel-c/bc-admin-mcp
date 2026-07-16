package b2b

import (
	"context"
	"fmt"
	"net/url"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// ============================================================
// Account hierarchy tools
// ============================================================

func (ct *CompanyTools) registerHierarchyTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/hierarchy/get",
		Tier:    middleware.TierR0,
		Summary: "Get the full account hierarchy (parents + nested subsidiaries) for a company",
		Tool: mcp.NewTool("b2b_companies_hierarchy_get",
			mcp.WithDescription("Get all parent and child accounts in the Account Hierarchy of a company. Requires Account Hierarchy to be enabled on the store."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleHierarchyGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/hierarchy/subsidiaries",
		Tier:    middleware.TierR0,
		Summary: "List the subsidiary accounts beneath a company",
		Tool: mcp.NewTool("b2b_companies_hierarchy_subsidiaries",
			mcp.WithDescription("List the subsidiary accounts on lower hierarchy layers of a company."),
			mcp.WithNumber("company_id", mcp.Description("Company ID"), mcp.Required()),
			mcp.WithNumber("limit", mcp.Description("Max results (default 10).")),
			mcp.WithNumber("offset", mcp.Description("Results to skip (default 0).")),
		),
		Handler: ct.handleHierarchySubsidiaries,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/hierarchy/attach_parent",
		Tier:    middleware.TierR1,
		Summary: "Attach a parent company above a company in the hierarchy",
		Tool: mcp.NewTool("b2b_companies_hierarchy_attach_parent",
			mcp.WithDescription("Assign a parent company above the target company in the Account Hierarchy. You cannot assign a company that is already at a higher layer than the target. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("The company that will become the child."), mcp.Required()),
			mcp.WithNumber("parent_company_id", mcp.Description("The company to set as the parent."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: ct.handleHierarchyAttachParent,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "b2b/companies/hierarchy/detach_subsidiary",
		Tier:    middleware.TierR2,
		Summary: "Remove a subsidiary's parent relationship (splits it into its own hierarchy)",
		Tool: mcp.NewTool("b2b_companies_hierarchy_detach_subsidiary",
			mcp.WithDescription("Remove the parent-child relationship between a subsidiary and its parent company. If the subsidiary has its own subsidiaries, they become a new top-level hierarchy. Preview → confirm."),
			mcp.WithNumber("company_id", mcp.Description("The parent company ID."), mcp.Required()),
			mcp.WithNumber("child_company_id", mcp.Description("The subsidiary company ID to detach."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to detach.")),
		),
		Handler: ct.handleHierarchyDetachSubsidiary,
	})
}

func hierarchyPageParams(args map[string]any) string {
	params := url.Values{}
	if v, ok := args["limit"].(float64); ok && v > 0 {
		params.Set("limit", fmt.Sprintf("%d", int(v)))
	}
	if v, ok := args["offset"].(float64); ok && v >= 0 {
		params.Set("offset", fmt.Sprintf("%d", int(v)))
	}
	return params.Encode()
}

func (ct *CompanyTools) handleHierarchyGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	nodes, err := ct.bc.ListB2BCompanyHierarchy(ctx, id, hierarchyPageParams(args))
	if err != nil {
		return shared.ToolError("failed to get hierarchy for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"company_id": id, "total": len(nodes), "hierarchy": nodes})
}

func (ct *CompanyTools) handleHierarchySubsidiaries(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	nodes, err := ct.bc.ListB2BCompanySubsidiaries(ctx, id, hierarchyPageParams(args))
	if err != nil {
		return shared.ToolError("failed to list subsidiaries for company %d: %v", id, err), nil
	}
	return shared.ToolJSON(map[string]any{"company_id": id, "total": len(nodes), "subsidiaries": nodes})
}

func (ct *CompanyTools) handleHierarchyAttachParent(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	parentID, err := shared.ReadPositiveInt(args, "parent_company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if parentID == id {
		return shared.ToolError("parent_company_id must differ from company_id"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":            "preview",
			"action":            "attach_b2b_company_parent",
			"company_id":        id,
			"parent_company_id": parentID,
			"message":           fmt.Sprintf("Will set company %d as the parent of company %d. Pass confirmed=true.", parentID, id),
		})
	}

	if err := ct.bc.AttachB2BCompanyParent(ctx, id, parentID); err != nil {
		return shared.ToolError("failed to attach parent %d to company %d: %v", parentID, id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "attached", "company_id": id, "parent_company_id": parentID})
}

func (ct *CompanyTools) handleHierarchyDetachSubsidiary(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	id, err := shared.ReadPositiveInt(args, "company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	childID, err := shared.ReadPositiveInt(args, "child_company_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":           "preview",
			"action":           "detach_b2b_company_subsidiary",
			"company_id":       id,
			"child_company_id": childID,
			"message":          fmt.Sprintf("Will remove subsidiary %d from parent %d (subsidiary keeps any of its own children as a new hierarchy). Pass confirmed=true.", childID, id),
		})
	}

	if err := ct.bc.DeleteB2BCompanySubsidiary(ctx, id, childID); err != nil {
		return shared.ToolError("failed to detach subsidiary %d from company %d: %v", childID, id, err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "detached", "company_id": id, "child_company_id": childID})
}
