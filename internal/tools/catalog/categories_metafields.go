package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/mark3labs/mcp-go/mcp"
)

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

	ps, err := ParseOptionalPermissionSet(args)
	if err != nil {
		return nil, err
	}
	p.PermissionSet = ps

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

	return metafieldListJSON(resolvedID, "category_id", mfs)
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
	return metafieldUpsertCore(
		ctx, resolvedID,
		params.Namespace, params.Key, params.Value, params.Description, params.PermissionSet,
		categoryMetafieldOps(c.bc),
		"category_id", "write", confirmed, "category",
		nil,
	)
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
	return metafieldDeleteCore(
		ctx, resolvedID,
		params.MetafieldID, params.Namespace, params.Key,
		categoryMetafieldOps(c.bc),
		"category_id", confirmed, "category",
		nil,
	)
}
