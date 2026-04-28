package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultSortStart     = 0
	defaultSortIncrement = 10
)

// ReorderParams holds parsed arguments for the reorder tool.
type ReorderParams struct {
	CategoryIDs    []int
	CategoryNames  []string
	StartSortOrder int
	Increment      int
}

// ParseReorderParams validates arguments for the reorder tool.
func ParseReorderParams(args map[string]any) (*ReorderParams, error) {
	p := &ReorderParams{
		StartSortOrder: defaultSortStart,
		Increment:      defaultSortIncrement,
	}

	if v, ok := args["category_ids"]; ok {
		ids, err := parseFloat64SliceToPositiveInts(v, "category_ids")
		if err != nil {
			return nil, err
		}
		p.CategoryIDs = ids
	}
	if v, ok := args["category_names"]; ok {
		names, err := parseStringSlice(v, "category_names")
		if err != nil {
			return nil, err
		}
		p.CategoryNames = names
	}
	if len(p.CategoryIDs) == 0 && len(p.CategoryNames) == 0 {
		return nil, fmt.Errorf("provide category_ids or category_names (ordered)")
	}
	if len(p.CategoryIDs) > 0 && len(p.CategoryNames) > 0 {
		return nil, fmt.Errorf("provide category_ids or category_names, not both")
	}

	if v, ok := args["start_sort_order"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("start_sort_order must be a number")
		}
		p.StartSortOrder = int(f)
	}
	if v, ok := args["increment"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("increment must be a number")
		}
		p.Increment = int(f)
		if p.Increment < 1 {
			return nil, fmt.Errorf("increment must be at least 1")
		}
	}

	return p, nil
}

func (c *Categories) handleReorder(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseReorderParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	orderedIDs := params.CategoryIDs
	if len(params.CategoryNames) > 0 {
		orderedIDs = make([]int, 0, len(params.CategoryNames))
		for _, name := range params.CategoryNames {
			cid, resolveErr := resolveCategoryByExactName(ctx, c.bc, name)
			if resolveErr != nil {
				return toolError("%s", resolveErr.Error()), nil
			}
			orderedIDs = append(orderedIDs, cid)
		}
	}

	cats, err := c.fetchCategoriesByIDs(ctx, orderedIDs)
	if err != nil {
		return toolError("failed to fetch categories: %v", err), nil
	}

	catMap := make(map[int]bigcommerce.Category, len(cats))
	for _, cat := range cats {
		catMap[cat.ID] = cat
	}

	parentID := catMap[orderedIDs[0]].ParentID
	var mismatchedSiblings []string
	for _, id := range orderedIDs {
		cat := catMap[id]
		if cat.ParentID != parentID {
			mismatchedSiblings = append(mismatchedSiblings,
				fmt.Sprintf("%s (ID %d, parent %d)", cat.Name, cat.ID, cat.ParentID))
		}
	}
	if len(mismatchedSiblings) > 0 {
		return toolError(
			"all categories must share the same parent. Mismatched: %s",
			strings.Join(mismatchedSiblings, "; "),
		), nil
	}

	type reorderRow struct {
		ID              int    `json:"id"`
		Name            string `json:"name"`
		CurrentSortOrder int   `json:"current_sort_order"`
		NewSortOrder    int    `json:"new_sort_order"`
	}

	rows := make([]reorderRow, len(orderedIDs))
	for i, id := range orderedIDs {
		cat := catMap[id]
		rows[i] = reorderRow{
			ID:               id,
			Name:             cat.Name,
			CurrentSortOrder: cat.SortOrder,
			NewSortOrder:     params.StartSortOrder + i*params.Increment,
		}
	}

	var collisions []string
	siblings, err := c.getDirectChildren(ctx, parentID)
	if err == nil {
		reorderedSet := make(map[int]bool, len(orderedIDs))
		for _, id := range orderedIDs {
			reorderedSet[id] = true
		}
		for _, sib := range siblings {
			if reorderedSet[sib.ID] {
				continue
			}
			for _, row := range rows {
				if sib.SortOrder == row.NewSortOrder {
					collisions = append(collisions,
						fmt.Sprintf("%s (ID %d) has sort_order %d", sib.Name, sib.ID, sib.SortOrder))
					break
				}
			}
		}
	}

	confirmed := middleware.IsConfirmed(request)
	if !confirmed {
		preview := map[string]any{
			"status":      "pending_confirmation",
			"parent_id":   parentID,
			"changes":     rows,
			"start":       params.StartSortOrder,
			"increment":   params.Increment,
			"message":     fmt.Sprintf("%d categories will be reordered. Pass confirmed=true to execute.", len(rows)),
		}
		if len(collisions) > 0 {
			preview["sort_order_collisions"] = collisions
			preview["collision_warning"] = "Some sibling categories (not being reordered) share sort_order values with the new assignments. Consider reordering them too."
		}
		return toolJSON(preview)
	}

	updates := make([]bigcommerce.CategoryUpdate, len(orderedIDs))
	for i, id := range orderedIDs {
		newSort := params.StartSortOrder + i*params.Increment
		updates[i] = bigcommerce.CategoryUpdate{
			CategoryID: id,
			SortOrder:  &newSort,
		}
	}

	result, err := c.bc.BatchUpdateCategories(ctx, updates)
	if err != nil {
		return toolError("batch update failed: %v", err), nil
	}

	return toolJSON(map[string]any{
		"status":    "completed",
		"succeeded": result.Succeeded,
		"failed":    result.Failed,
		"message":   fmt.Sprintf("Successfully reordered %d categories.", result.Succeeded),
	})
}
