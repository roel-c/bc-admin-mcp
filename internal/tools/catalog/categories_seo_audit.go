package catalog

import (
	"context"
	"fmt"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/mark3labs/mcp-go/mcp"
)

type seoIssue struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	ParentID      int      `json:"parent_id,omitempty"`
	MissingFields []string `json:"missing_fields"`
}

func (c *Categories) handleSEOAudit(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	params := make(map[string]string)
	if v, ok := args["parent_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return toolError("parent_id must be a number"), nil
		}
		params["parent_id"] = fmt.Sprintf("%.0f", f)
	}
	if v, ok := args["tree_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return toolError("tree_id must be a number"), nil
		}
		params["tree_id"] = fmt.Sprintf("%.0f", f)
	}

	cats, err := c.bc.SearchCategories(ctx, params)
	if err != nil {
		return toolError("failed to fetch categories: %v", err), nil
	}

	issues := AuditSEOFields(cats)

	result := map[string]any{
		"total_audited": len(cats),
		"issues_found":  len(issues),
		"categories_with_issues": issues,
	}
	if len(issues) == 0 {
		result["message"] = "All categories have page_title, meta_description, and search_keywords populated."
	} else {
		result["message"] = fmt.Sprintf(
			"%d of %d categories are missing one or more SEO fields. Use catalog/categories/bulk_update to fix them.",
			len(issues), len(cats),
		)
	}

	return toolJSON(result)
}

// AuditSEOFields returns categories that have at least one empty SEO field.
func AuditSEOFields(cats []bigcommerce.Category) []seoIssue {
	var issues []seoIssue
	for _, cat := range cats {
		var missing []string
		if cat.PageTitle == "" {
			missing = append(missing, "page_title")
		}
		if cat.MetaDescription == "" {
			missing = append(missing, "meta_description")
		}
		if cat.SearchKeywords == "" {
			missing = append(missing, "search_keywords")
		}
		if len(missing) > 0 {
			issues = append(issues, seoIssue{
				ID:            cat.ID,
				Name:          cat.Name,
				ParentID:      cat.ParentID,
				MissingFields: missing,
			})
		}
	}
	return issues
}
