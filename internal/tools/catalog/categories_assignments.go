package catalog

import (
	"context"
	"fmt"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

// AssignmentParams holds parsed arguments for the category assignment tool.
type AssignmentParams struct {
	ProductIDs  []int
	CategoryIDs []int
}

// ParseAssignmentParams validates arguments for the category assignment tool.
func ParseAssignmentParams(args map[string]any) (*AssignmentParams, error) {
	p := &AssignmentParams{}

	if v, ok := args["product_ids"]; ok {
		ids, err := parseFloat64SliceToPositiveInts(v, "product_ids")
		if err != nil {
			return nil, err
		}
		p.ProductIDs = ids
	}
	if len(p.ProductIDs) == 0 {
		return nil, fmt.Errorf("product_ids is required and must not be empty")
	}

	if v, ok := args["category_ids"]; ok {
		ids, err := parseFloat64SliceToPositiveInts(v, "category_ids")
		if err != nil {
			return nil, err
		}
		p.CategoryIDs = ids
	}
	if len(p.CategoryIDs) == 0 {
		return nil, fmt.Errorf("category_ids is required and must not be empty")
	}

	return p, nil
}

func (p *Products) handleAssignCategories(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseAssignmentParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)

	assignments := make([]bigcommerce.CategoryAssignment, 0, len(params.ProductIDs)*len(params.CategoryIDs))
	for _, pid := range params.ProductIDs {
		for _, cid := range params.CategoryIDs {
			assignments = append(assignments, bigcommerce.CategoryAssignment{
				ProductID:  pid,
				CategoryID: cid,
			})
		}
	}

	if !confirmed {
		preview := map[string]any{
			"status":               "pending_confirmation",
			"total_assignments":    len(assignments),
			"product_count":        len(params.ProductIDs),
			"category_count":       len(params.CategoryIDs),
			"sample_assignments":   assignments[:min(5, len(assignments))],
			"message": fmt.Sprintf(
				"%d assignment(s) will be created (%d products x %d categories). "+
					"This is additive — existing category memberships are preserved. "+
					"Pass confirmed=true to execute.",
				len(assignments), len(params.ProductIDs), len(params.CategoryIDs),
			),
		}
		return toolJSON(preview)
	}

	if err := p.bc.UpsertCategoryAssignments(ctx, assignments); err != nil {
		return toolError("assignment failed: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":  "completed",
		"message": fmt.Sprintf("Successfully assigned %d products to %d categories (%d total assignments).", len(params.ProductIDs), len(params.CategoryIDs), len(assignments)),
	})
}
