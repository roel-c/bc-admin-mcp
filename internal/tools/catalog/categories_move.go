package catalog

import (
	"context"
	"fmt"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

// MoveParams holds parsed arguments for the category move tool.
type MoveParams struct {
	CategoryID    int
	CategoryName  string
	NewParentID   int
	NewParentName string
	MoveToRoot    bool
}

// ParseMoveParams validates arguments for the move tool.
func ParseMoveParams(args map[string]any) (*MoveParams, error) {
	p := &MoveParams{}

	_, hasID := args["category_id"]
	_, hasName := args["category_name"]
	if hasID && hasName {
		return nil, fmt.Errorf("category_id and category_name are mutually exclusive")
	}
	if !hasID && !hasName {
		return nil, fmt.Errorf("provide either category_id or category_name for the category to move")
	}

	if v, ok := args["category_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("category_id must be a number")
		}
		p.CategoryID = int(f)
	}
	if v, ok := args["category_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("category_name must be a non-empty string")
		}
		p.CategoryName = s
	}

	_, hasNewID := args["new_parent_id"]
	_, hasNewName := args["new_parent_name"]
	if hasNewID && hasNewName {
		return nil, fmt.Errorf("new_parent_id and new_parent_name are mutually exclusive")
	}
	if !hasNewID && !hasNewName {
		return nil, fmt.Errorf("provide new_parent_id or new_parent_name for the destination (use new_parent_id=0 for root)")
	}

	if v, ok := args["new_parent_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("new_parent_id must be a number")
		}
		p.NewParentID = int(f)
		if p.NewParentID == 0 {
			p.MoveToRoot = true
		}
	}
	if v, ok := args["new_parent_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("new_parent_name must be a non-empty string")
		}
		p.NewParentName = s
	}

	return p, nil
}

// IsDescendant checks if potentialAncestor is an ancestor of targetID by walking
// down the tree from potentialAncestor. This prevents creating cycles.
func IsDescendant(ctx context.Context, bc BigCommerceAPI, targetID, potentialAncestorID int) (bool, error) {
	if targetID == potentialAncestorID {
		return true, nil
	}
	return isDescendantRecursive(ctx, bc, targetID, potentialAncestorID, 0)
}

func isDescendantRecursive(ctx context.Context, bc BigCommerceAPI, targetID, currentID, depth int) (bool, error) {
	if depth > 10 {
		return false, fmt.Errorf("category tree depth exceeded 10 levels during cycle check")
	}
	children, err := bc.SearchCategories(ctx, map[string]string{
		"parent_id": fmt.Sprintf("%d", currentID),
	})
	if err != nil {
		return false, fmt.Errorf("failed to check children of category %d: %w", currentID, err)
	}
	for _, child := range children {
		if child.ID == targetID {
			return true, nil
		}
		found, err := isDescendantRecursive(ctx, bc, targetID, child.ID, depth+1)
		if err != nil {
			return false, err
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

func countDescendants(ctx context.Context, bc BigCommerceAPI, categoryID int) (int, error) {
	children, err := bc.SearchCategories(ctx, map[string]string{
		"parent_id": fmt.Sprintf("%d", categoryID),
	})
	if err != nil {
		return 0, err
	}
	count := len(children)
	for _, child := range children {
		subCount, subErr := countDescendants(ctx, bc, child.ID)
		if subErr != nil {
			return count, subErr
		}
		count += subCount
	}
	return count, nil
}

func (c *Categories) handleMove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseMoveParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	sourceID := params.CategoryID
	if params.CategoryName != "" {
		id, resolveErr := c.resolveParentName(ctx, params.CategoryName)
		if resolveErr != nil {
			return toolError("%s", resolveErr.Error()), nil
		}
		sourceID = id
	}

	newParentID := params.NewParentID
	if params.NewParentName != "" {
		id, resolveErr := c.resolveParentName(ctx, params.NewParentName)
		if resolveErr != nil {
			return toolError("%s", resolveErr.Error()), nil
		}
		newParentID = id
	}

	sourceCat, err := c.bc.GetCategory(ctx, sourceID)
	if err != nil {
		return toolError("failed to get source category: %v", err), nil
	}

	if sourceCat.ParentID == newParentID {
		return toolError("category %q is already under parent %d — no move needed", sourceCat.Name, newParentID), nil
	}

	if !params.MoveToRoot && newParentID > 0 {
		if newParentID == sourceID {
			return toolError("cannot move a category into itself"), nil
		}
		isDesc, cycleErr := IsDescendant(ctx, c.bc, newParentID, sourceID)
		if cycleErr != nil {
			return toolError("cycle detection failed: %v", cycleErr), nil
		}
		if isDesc {
			return toolError("cannot move category %q into its own descendant (would create a cycle)", sourceCat.Name), nil
		}
	}

	childCount, countErr := countDescendants(ctx, c.bc, sourceID)

	confirmed := middleware.IsConfirmed(request)
	if !confirmed {
		currentParent := map[string]any{"id": sourceCat.ParentID}
		if sourceCat.ParentID > 0 {
			if parent, pErr := c.bc.GetCategory(ctx, sourceCat.ParentID); pErr == nil {
				currentParent["name"] = parent.Name
			}
		} else {
			currentParent["name"] = "(root)"
		}

		newParent := map[string]any{"id": newParentID}
		if newParentID > 0 {
			if parent, pErr := c.bc.GetCategory(ctx, newParentID); pErr == nil {
				newParent["name"] = parent.Name
			}
		} else {
			newParent["name"] = "(root)"
		}

		preview := map[string]any{
			"status":                  "pending_confirmation",
			"category":               map[string]any{"id": sourceID, "name": sourceCat.Name},
			"current_parent":         currentParent,
			"new_parent":             newParent,
			"descendants_that_move":  childCount,
			"warning":                "This changes storefront navigation. All descendant categories move with the parent.",
			"message":                fmt.Sprintf("Category %q will be moved. Pass confirmed=true to execute.", sourceCat.Name),
		}
		if countErr != nil {
			preview["descendant_count_warning"] = "Could not determine exact descendant count; the actual number may be higher."
		}
		return toolJSON(preview)
	}

	update := bigcommerce.CategoryUpdate{
		CategoryID: sourceID,
		ParentID:   &newParentID,
	}
	_, updateErr := c.bc.BatchUpdateCategories(ctx, []bigcommerce.CategoryUpdate{update})
	if updateErr != nil {
		return toolError("move failed: %v", updateErr), nil
	}

	return toolJSON(map[string]any{
		"status":  "completed",
		"message": fmt.Sprintf("Category %q (ID %d) moved to parent %d.", sourceCat.Name, sourceID, newParentID),
	})
}
