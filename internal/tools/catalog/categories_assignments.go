package catalog

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// Per-call caps for the additive product↔category assignment tool.
// Mirrors the channel_assignments/assign cap of 500 pairs to keep PUT bodies
// small enough that BigCommerce won't 422 on body size and so a misbehaving
// agent can't dump tens of thousands of assignment rows in a single call.
const (
	maxAssignCategoriesProducts   = 100
	maxAssignCategoriesCategories = 50
	maxAssignCategoriesPairs      = 500
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
	if len(p.ProductIDs) > maxAssignCategoriesProducts {
		return nil, fmt.Errorf("product_ids: maximum %d per call", maxAssignCategoriesProducts)
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
	if len(p.CategoryIDs) > maxAssignCategoriesCategories {
		return nil, fmt.Errorf("category_ids: maximum %d per call", maxAssignCategoriesCategories)
	}

	if pairs := len(p.ProductIDs) * len(p.CategoryIDs); pairs > maxAssignCategoriesPairs {
		return nil, fmt.Errorf(
			"product_ids × category_ids would create %d (product, category) pairs; "+
				"maximum %d per call. Split the call into smaller batches.",
			pairs, maxAssignCategoriesPairs,
		)
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
			"status":             "pending_confirmation",
			"total_assignments":  len(assignments),
			"product_count":      len(params.ProductIDs),
			"category_count":     len(params.CategoryIDs),
			"sample_assignments": assignments[:min(5, len(assignments))],
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

const (
	maxUnassignProducts   = 100
	maxUnassignCategories = 50
)

func (p *Products) handleUnassignCategories(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	productIDs, err := parseFloat64SliceToPositiveInts(args["product_ids"], "product_ids")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(productIDs) == 0 {
		return toolError("product_ids is required and must not be empty"), nil
	}
	if len(productIDs) > maxUnassignProducts {
		return toolError("product_ids: maximum %d per call", maxUnassignProducts), nil
	}

	categoryIDs, err := parseFloat64SliceToPositiveInts(args["category_ids"], "category_ids")
	if err != nil {
		return toolError("%s", err.Error()), nil
	}
	if len(categoryIDs) == 0 {
		return toolError(
			"category_ids is required and must not be empty. " +
				"Removing a product from ALL category assignments is not exposed by this tool — " +
				"use catalog/products/update with categories=[] for that intent (full replacement).",
		), nil
	}
	if len(categoryIDs) > maxUnassignCategories {
		return toolError("category_ids: maximum %d per call", maxUnassignCategories), nil
	}

	confirmed := middleware.IsConfirmed(request)
	if !confirmed {
		return toolJSON(map[string]any{
			"status":         "pending_confirmation",
			"product_count":  len(productIDs),
			"category_count": len(categoryIDs),
			"product_ids":    productIDs,
			"category_ids":   categoryIDs,
			"api":            "DELETE /v3/catalog/products/category-assignments",
			"message": fmt.Sprintf(
				"Will remove %d product(s) from %d category(ies). "+
					"Other category memberships are preserved (filter-based delete). "+
					"Pass confirmed=true to execute.",
				len(productIDs), len(categoryIDs),
			),
		})
	}

	if err := p.bc.DeleteCategoryAssignmentsByFilter(ctx, productIDs, categoryIDs); err != nil {
		return toolError("unassign failed: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status": "completed",
		"message": fmt.Sprintf(
			"Removed assignments matching %d product(s) × %d category(ies).",
			len(productIDs), len(categoryIDs),
		),
	})
}
