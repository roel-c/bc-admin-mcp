package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

var validPermissionSets = map[string]bool{
	"app_only":          true,
	"read":              true,
	"write":             true,
	"read_and_sf_access": true,
	"write_and_sf_access": true,
}

// MetafieldSetParams holds parsed arguments for the metafield set tool.
type MetafieldSetParams struct {
	CategoryID    int
	CategoryName  string
	Namespace     string
	Key           string
	Value         string
	Description   string
	PermissionSet string
}

// ParseMetafieldSetParams validates arguments for the set tool.
func ParseMetafieldSetParams(args map[string]any) (*MetafieldSetParams, error) {
	p := &MetafieldSetParams{}

	_, hasID := args["category_id"]
	_, hasName := args["category_name"]
	if hasID && hasName {
		return nil, fmt.Errorf("category_id and category_name are mutually exclusive")
	}
	if !hasID && !hasName {
		return nil, fmt.Errorf("provide either category_id or category_name")
	}

	if v, ok := args["category_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("category_id must be a number")
		}
		p.CategoryID = int(f)
		if p.CategoryID <= 0 {
			return nil, fmt.Errorf("category_id must be positive")
		}
	}
	if v, ok := args["category_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("category_name must be a non-empty string")
		}
		p.CategoryName = s
	}

	nsRaw, ok := args["namespace"]
	if !ok {
		return nil, fmt.Errorf("namespace is required")
	}
	ns, sOk := nsRaw.(string)
	if !sOk || strings.TrimSpace(ns) == "" {
		return nil, fmt.Errorf("namespace must be a non-empty string")
	}
	p.Namespace = strings.TrimSpace(ns)

	kRaw, ok := args["key"]
	if !ok {
		return nil, fmt.Errorf("key is required")
	}
	k, sOk := kRaw.(string)
	if !sOk || strings.TrimSpace(k) == "" {
		return nil, fmt.Errorf("key must be a non-empty string")
	}
	p.Key = strings.TrimSpace(k)

	vRaw, ok := args["value"]
	if !ok {
		return nil, fmt.Errorf("value is required")
	}
	val, sOk := vRaw.(string)
	if !sOk {
		return nil, fmt.Errorf("value must be a string")
	}
	p.Value = val

	if v, ok := args["description"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("description must be a string")
		}
		p.Description = s
	}

	if v, ok := args["permission_set"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("permission_set must be a string")
		}
		if !validPermissionSets[s] {
			return nil, fmt.Errorf("permission_set must be one of: app_only, read, write, read_and_sf_access, write_and_sf_access")
		}
		p.PermissionSet = s
	}

	return p, nil
}

// MetafieldDeleteParams holds parsed arguments for the metafield delete tool.
type MetafieldDeleteParams struct {
	CategoryID   int
	CategoryName string
	Namespace    string
	Key          string
	MetafieldID  int
}

// ParseMetafieldDeleteParams validates arguments for the delete tool.
func ParseMetafieldDeleteParams(args map[string]any) (*MetafieldDeleteParams, error) {
	p := &MetafieldDeleteParams{}

	_, hasID := args["category_id"]
	_, hasName := args["category_name"]
	if hasID && hasName {
		return nil, fmt.Errorf("category_id and category_name are mutually exclusive")
	}
	if !hasID && !hasName {
		return nil, fmt.Errorf("provide either category_id or category_name")
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

	_, hasMFID := args["metafield_id"]
	_, hasNS := args["namespace"]
	_, hasKey := args["key"]

	if hasMFID && (hasNS || hasKey) {
		return nil, fmt.Errorf("use metafield_id alone, or namespace + key; do not combine")
	}
	if !hasMFID && (!hasNS || !hasKey) {
		return nil, fmt.Errorf("provide metafield_id, or both namespace and key")
	}

	if v, ok := args["metafield_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("metafield_id must be a number")
		}
		p.MetafieldID = int(f)
	}
	if v, ok := args["namespace"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("namespace must be a string")
		}
		p.Namespace = strings.TrimSpace(s)
	}
	if v, ok := args["key"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return nil, fmt.Errorf("key must be a string")
		}
		p.Key = strings.TrimSpace(s)
	}

	return p, nil
}

func (c *Categories) resolveCategoryForMetafield(ctx context.Context, catID int, catName string) (int, error) {
	if catID > 0 {
		return catID, nil
	}
	return resolveCategoryByExactName(ctx, c.bc, catName)
}

func (c *Categories) handleMetafieldsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	_, hasID := args["category_id"]
	_, hasName := args["category_name"]
	if hasID && hasName {
		return toolError("category_id and category_name are mutually exclusive"), nil
	}
	if !hasID && !hasName {
		return toolError("provide either category_id or category_name"), nil
	}

	var catID int
	if v, ok := args["category_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return toolError("category_id must be a number"), nil
		}
		catID = int(f)
	}
	var catName string
	if v, ok := args["category_name"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return toolError("category_name must be a string"), nil
		}
		catName = s
	}

	resolvedID, err := c.resolveCategoryForMetafield(ctx, catID, catName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	mfs, err := c.bc.ListCategoryMetafields(ctx, resolvedID)
	if err != nil {
		return toolError("failed to list metafields: %v", err), nil
	}

	result := map[string]any{
		"category_id": resolvedID,
		"total":       len(mfs),
		"metafields":  mfs,
	}
	return toolJSON(result)
}

func (c *Categories) handleMetafieldsSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseMetafieldSetParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	resolvedID, err := c.resolveCategoryForMetafield(ctx, params.CategoryID, params.CategoryName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)

	existing, err := c.bc.ListCategoryMetafields(ctx, resolvedID)
	if err != nil {
		return toolError("failed to list existing metafields: %v", err), nil
	}

	var existingMF *bigcommerce.Metafield
	for i := range existing {
		if existing[i].Namespace == params.Namespace && existing[i].Key == params.Key {
			existingMF = &existing[i]
			break
		}
	}

	if !confirmed {
		action := "create"
		preview := map[string]any{
			"status":      "pending_confirmation",
			"category_id": resolvedID,
			"namespace":   params.Namespace,
			"key":         params.Key,
			"value":       params.Value,
		}
		if existingMF != nil {
			action = "update"
			preview["existing_value"] = existingMF.Value
			preview["metafield_id"] = existingMF.ID
		}
		preview["action"] = action
		preview["message"] = fmt.Sprintf(
			"Will %s metafield %s.%s on category %d. Pass confirmed=true to execute.",
			action, params.Namespace, params.Key, resolvedID,
		)
		return toolJSON(preview)
	}

	mf := bigcommerce.Metafield{
		Namespace:     params.Namespace,
		Key:           params.Key,
		Value:         params.Value,
		Description:   params.Description,
		PermissionSet: params.PermissionSet,
	}

	if existingMF != nil {
		updated, updateErr := c.bc.UpdateCategoryMetafield(ctx, resolvedID, existingMF.ID, mf)
		if updateErr != nil {
			return toolError("update failed: %v", updateErr), nil
		}
		return toolJSON(map[string]any{
			"status":    "updated",
			"metafield": updated,
			"message":   fmt.Sprintf("Metafield %s.%s updated on category %d.", params.Namespace, params.Key, resolvedID),
		})
	}

	if mf.PermissionSet == "" {
		mf.PermissionSet = "write"
	}
	created, createErr := c.bc.CreateCategoryMetafield(ctx, resolvedID, mf)
	if createErr != nil {
		return toolError("create failed: %v", createErr), nil
	}
	return toolJSON(map[string]any{
		"status":    "created",
		"metafield": created,
		"message":   fmt.Sprintf("Metafield %s.%s created on category %d.", params.Namespace, params.Key, resolvedID),
	})
}

func (c *Categories) handleMetafieldsDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseMetafieldDeleteParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	resolvedID, err := c.resolveCategoryForMetafield(ctx, params.CategoryID, params.CategoryName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)

	mfID := params.MetafieldID
	if mfID == 0 {
		existing, listErr := c.bc.ListCategoryMetafields(ctx, resolvedID)
		if listErr != nil {
			return toolError("failed to list metafields: %v", listErr), nil
		}
		for _, mf := range existing {
			if mf.Namespace == params.Namespace && mf.Key == params.Key {
				mfID = mf.ID
				break
			}
		}
		if mfID == 0 {
			return toolError("no metafield found with namespace %q key %q on category %d", params.Namespace, params.Key, resolvedID), nil
		}
	}

	if !confirmed {
		preview := map[string]any{
			"status":       "pending_confirmation",
			"category_id":  resolvedID,
			"metafield_id": mfID,
			"message":      fmt.Sprintf("Will delete metafield %d from category %d. Pass confirmed=true to execute.", mfID, resolvedID),
		}
		if params.Namespace != "" {
			preview["namespace"] = params.Namespace
			preview["key"] = params.Key
		}
		return toolJSON(preview)
	}

	if delErr := c.bc.DeleteCategoryMetafield(ctx, resolvedID, mfID); delErr != nil {
		return toolError("delete failed: %v", delErr), nil
	}

	return toolJSON(map[string]any{
		"status":  "deleted",
		"message": fmt.Sprintf("Metafield %d deleted from category %d.", mfID, resolvedID),
	})
}
