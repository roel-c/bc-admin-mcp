package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// BrandMetafieldSetParams holds parsed arguments for catalog/brands/metafields/set.
type BrandMetafieldSetParams struct {
	BrandID       int
	BrandName     string
	Namespace     string
	Key           string
	Value         string
	Description   string
	PermissionSet string
}

// ParseBrandMetafieldSetParams validates set-tool arguments.
func ParseBrandMetafieldSetParams(args map[string]any) (*BrandMetafieldSetParams, error) {
	p := &BrandMetafieldSetParams{}

	_, hasID := args["brand_id"]
	_, hasName := args["brand_name"]
	if hasID && hasName {
		return nil, fmt.Errorf("brand_id and brand_name are mutually exclusive")
	}
	if !hasID && !hasName {
		return nil, fmt.Errorf("provide either brand_id or brand_name")
	}

	if v, ok := args["brand_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("brand_id must be a number")
		}
		p.BrandID = int(f)
		if p.BrandID <= 0 {
			return nil, fmt.Errorf("brand_id must be positive")
		}
	}
	if v, ok := args["brand_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("brand_name must be a non-empty string")
		}
		p.BrandName = s
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

// BrandMetafieldDeleteParams holds parsed arguments for catalog/brands/metafields/delete.
type BrandMetafieldDeleteParams struct {
	BrandID     int
	BrandName   string
	Namespace   string
	Key         string
	MetafieldID int
}

// ParseBrandMetafieldDeleteParams validates delete-tool arguments.
func ParseBrandMetafieldDeleteParams(args map[string]any) (*BrandMetafieldDeleteParams, error) {
	p := &BrandMetafieldDeleteParams{}

	_, hasID := args["brand_id"]
	_, hasName := args["brand_name"]
	if hasID && hasName {
		return nil, fmt.Errorf("brand_id and brand_name are mutually exclusive")
	}
	if !hasID && !hasName {
		return nil, fmt.Errorf("provide either brand_id or brand_name")
	}

	if v, ok := args["brand_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return nil, fmt.Errorf("brand_id must be a number")
		}
		p.BrandID = int(f)
	}
	if v, ok := args["brand_name"]; ok {
		s, sOk := v.(string)
		if !sOk || s == "" {
			return nil, fmt.Errorf("brand_name must be a non-empty string")
		}
		p.BrandName = s
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

func (b *Brands) resolveBrandForMetafield(ctx context.Context, brandID int, brandName string) (int, error) {
	if brandID > 0 {
		return brandID, nil
	}
	return resolveBrandByExactName(ctx, b.bc, brandName)
}

func (b *Brands) registerBrandMetafieldTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/brands/metafields/list",
		Tier:    middleware.TierR0,
		Summary: "List metafields on a brand",
		Description: "Returns all custom key-value metafields attached to a brand. " +
			"Accepts brand_id or brand_name (exact match).",
		Tool: mcp.NewTool("catalog_brands_metafields_list",
			mcp.WithDescription("List all metafields on a brand. Provide brand_id or brand_name."),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID. Mutually exclusive with brand_name.")),
			mcp.WithString("brand_name", mcp.Description("Exact brand name. Mutually exclusive with brand_id.")),
		),
		Handler: b.handleBrandMetafieldsList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/brands/metafields/set",
		Tier:    middleware.TierR1,
		Summary: "Create or update a metafield on a brand",
		Description: "Sets a metafield by namespace+key. If a metafield with the same namespace and key " +
			"exists, it is updated; otherwise a new one is created. Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_brands_metafields_set",
			mcp.WithDescription(
				"Create or update a brand metafield. Provide brand target, namespace, key, and value. "+
					"Preview first; pass confirmed=true to execute.",
			),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID. Mutually exclusive with brand_name.")),
			mcp.WithString("brand_name", mcp.Description("Exact brand name. Mutually exclusive with brand_id.")),
			mcp.WithString("namespace", mcp.Description("Metafield namespace (e.g. 'my_app')."), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key within the namespace."), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value."), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional human-readable description of this metafield.")),
			mcp.WithString("permission_set", mcp.Description("Access level: app_only, read, write, read_and_sf_access, write_and_sf_access. Default: write.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute after reviewing the preview.")),
		),
		Handler: b.handleBrandMetafieldsSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/brands/metafields/delete",
		Tier:    middleware.TierR1,
		Summary: "Delete a metafield from a brand",
		Description: "Deletes a metafield by metafield_id, or by namespace+key lookup. " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_brands_metafields_delete",
			mcp.WithDescription(
				"Delete a brand metafield. Provide brand target and either metafield_id or namespace+key. "+
					"Preview first; pass confirmed=true to execute.",
			),
			mcp.WithNumber("brand_id", mcp.Description("Brand ID. Mutually exclusive with brand_name.")),
			mcp.WithString("brand_name", mcp.Description("Exact brand name. Mutually exclusive with brand_id.")),
			mcp.WithNumber("metafield_id", mcp.Description("Metafield ID to delete. Mutually exclusive with namespace+key.")),
			mcp.WithString("namespace", mcp.Description("Metafield namespace (use with key). Mutually exclusive with metafield_id.")),
			mcp.WithString("key", mcp.Description("Metafield key (use with namespace). Mutually exclusive with metafield_id.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set to true to execute after reviewing the preview.")),
		),
		Handler: b.handleBrandMetafieldsDelete,
	})
}

func (b *Brands) handleBrandMetafieldsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	_, hasID := args["brand_id"]
	_, hasName := args["brand_name"]
	if hasID && hasName {
		return toolError("brand_id and brand_name are mutually exclusive"), nil
	}
	if !hasID && !hasName {
		return toolError("provide either brand_id or brand_name"), nil
	}

	var brandID int
	if v, ok := args["brand_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return toolError("brand_id must be a number"), nil
		}
		brandID = int(f)
	}
	var brandName string
	if v, ok := args["brand_name"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return toolError("brand_name must be a string"), nil
		}
		brandName = s
	}

	resolvedID, err := b.resolveBrandForMetafield(ctx, brandID, brandName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	mfs, err := b.bc.ListBrandMetafields(ctx, resolvedID)
	if err != nil {
		return toolError("failed to list metafields: %v", err), nil
	}

	return metafieldListJSON(resolvedID, "brand_id", mfs)
}

func (b *Brands) handleBrandMetafieldsSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseBrandMetafieldSetParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	resolvedID, err := b.resolveBrandForMetafield(ctx, params.BrandID, params.BrandName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)
	return metafieldUpsertCore(
		ctx, resolvedID,
		params.Namespace, params.Key, params.Value, params.Description, params.PermissionSet,
		brandMetafieldOps(b.bc),
		"brand_id", "write", confirmed, "brand",
		nil,
	)
}

func (b *Brands) handleBrandMetafieldsDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseBrandMetafieldDeleteParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	resolvedID, err := b.resolveBrandForMetafield(ctx, params.BrandID, params.BrandName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)
	return metafieldDeleteCore(
		ctx, resolvedID,
		params.MetafieldID, params.Namespace, params.Key,
		brandMetafieldOps(b.bc),
		"brand_id", confirmed, "brand",
		nil,
	)
}
