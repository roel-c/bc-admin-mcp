package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

const defaultProductMetafieldPermissionSet = "app_only"

// RegisterProductMetafieldTools registers product metafield list/set/delete tools.
func (p *Products) RegisterProductMetafieldTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/metafields/list",
		Tier:    middleware.TierR0,
		Summary: "List all metafields on a product",
		Description: "Returns custom metafields (namespace, key, value, permission_set) for one product. " +
			"Identify the product by product_id, sku, or product_name (exactly one).",
		Tool: mcp.NewTool("catalog_products_metafields_list",
			mcp.WithDescription("List metafields on a product. Provide exactly one of: product_id, sku, or product_name."),
			mcp.WithNumber("product_id", mcp.Description("Numeric product ID")),
			mcp.WithString("sku", mcp.Description("Exact SKU — resolves to a single product")),
			mcp.WithString("product_name", mcp.Description("Exact product name — resolves to a single product")),
		),
		Handler: p.handleProductMetafieldsList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/metafields/set",
		Tier:    middleware.TierR1,
		Summary: "Create or update a product metafield (upsert by namespace+key)",
		Description: "Sets a metafield on a product. If omitted, permission_set defaults to app_only (not Storefront-exposed). " +
			"Use read_and_sf_access or write_and_sf_access when the value must be readable via the Storefront API. " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_products_metafields_set",
			mcp.WithDescription(
				"Create or update a product metafield. Target product: exactly one of product_id, sku, or product_name. "+
					"Required: namespace, key, value. Optional: description, permission_set (default app_only). "+
					"Preview; pass confirmed=true to execute.",
			),
			mcp.WithNumber("product_id", mcp.Description("Numeric product ID")),
			mcp.WithString("sku", mcp.Description("Exact SKU")),
			mcp.WithString("product_name", mcp.Description("Exact product name")),
			mcp.WithString("namespace", mcp.Description("Metafield namespace (e.g. my_integration)"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key within the namespace"), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value (string; JSON may be stored as a string)"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional human-readable description")),
			mcp.WithString("permission_set", mcp.Description(
				"Optional. One of: app_only (default), read, write, read_and_sf_access, write_and_sf_access. "+
					"Storefront-readable values use read_and_sf_access or write_and_sf_access.",
			)),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleProductMetafieldsSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/metafields/delete",
		Tier:        middleware.TierR1,
		Summary:     "Delete a product metafield",
		Description: "Deletes by metafield_id or by namespace+key. Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_products_metafields_delete",
			mcp.WithDescription("Delete a product metafield. Target product: one of product_id, sku, or product_name. Use metafield_id or namespace+key."),
			mcp.WithNumber("product_id", mcp.Description("Numeric product ID")),
			mcp.WithString("sku", mcp.Description("Exact SKU")),
			mcp.WithString("product_name", mcp.Description("Exact product name")),
			mcp.WithNumber("metafield_id", mcp.Description("Metafield ID (mutually exclusive with namespace+key)")),
			mcp.WithString("namespace", mcp.Description("Namespace (with key)")),
			mcp.WithString("key", mcp.Description("Key (with namespace)")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleProductMetafieldsDelete,
	})
}

// ProductMetafieldSetParams holds parsed arguments for product metafield set.
type ProductMetafieldSetParams struct {
	ProductID     int
	SKU           string
	ProductName   string
	Namespace     string
	Key           string
	Value         string
	Description   string
	PermissionSet string
}

// ParseProductMetafieldSetParams validates set-tool arguments.
func ParseProductMetafieldSetParams(args map[string]any) (*ProductMetafieldSetParams, error) {
	p := &ProductMetafieldSetParams{}

	if err := parseProductMetafieldTargetArgs(args, &p.ProductID, &p.SKU, &p.ProductName); err != nil {
		return nil, err
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

// ProductMetafieldDeleteParams holds parsed delete-tool arguments.
type ProductMetafieldDeleteParams struct {
	ProductID   int
	SKU         string
	ProductName string
	Namespace   string
	Key         string
	MetafieldID int
}

// ParseProductMetafieldDeleteParams validates delete-tool arguments.
func ParseProductMetafieldDeleteParams(args map[string]any) (*ProductMetafieldDeleteParams, error) {
	p := &ProductMetafieldDeleteParams{}

	if err := parseProductMetafieldTargetArgs(args, &p.ProductID, &p.SKU, &p.ProductName); err != nil {
		return nil, err
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
		if p.MetafieldID <= 0 {
			return nil, fmt.Errorf("metafield_id must be positive")
		}
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

func parseProductMetafieldTargetArgs(args map[string]any, productID *int, sku *string, productName *string) error {
	var pid int
	var sk, pn string
	hasProductID := false

	if v, ok := args["product_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return fmt.Errorf("product_id must be a number")
		}
		pid = int(f)
		hasProductID = true
		if pid <= 0 {
			return fmt.Errorf("product_id must be positive")
		}
	}
	if v, ok := args["sku"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return fmt.Errorf("sku must be a string")
		}
		sk = strings.TrimSpace(s)
	}
	if v, ok := args["product_name"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return fmt.Errorf("product_name must be a string")
		}
		pn = strings.TrimSpace(s)
	}

	modes := 0
	if hasProductID {
		modes++
	}
	if sk != "" {
		modes++
	}
	if pn != "" {
		modes++
	}
	if modes == 0 {
		return fmt.Errorf("provide exactly one of: product_id, sku, or product_name")
	}
	if modes > 1 {
		return fmt.Errorf("use only one of: product_id, sku, or product_name")
	}

	*productID = pid
	*sku = sk
	*productName = pn
	return nil
}

func (p *Products) resolveProductIDForMetafields(ctx context.Context, params *ProductMetafieldSetParams) (int, error) {
	return p.resolveProductIDFromMetafieldParts(ctx, params.ProductID, params.SKU, params.ProductName)
}

func (p *Products) resolveProductIDFromMetafieldParts(ctx context.Context, productID int, sku, productName string) (int, error) {
	if productID > 0 {
		return productID, nil
	}
	prods, err := FetchProductsForWrite(ctx, p.bc, nil, sku, productName)
	if err != nil {
		return 0, err
	}
	if len(prods) != 1 {
		return 0, fmt.Errorf("expected exactly one product from resolution")
	}
	return prods[0].ID, nil
}

func (p *Products) handleProductMetafieldsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	var params ProductMetafieldSetParams
	if err := parseProductMetafieldTargetArgs(args, &params.ProductID, &params.SKU, &params.ProductName); err != nil {
		return toolError("%s", err.Error()), nil
	}

	resolvedID, err := p.resolveProductIDForMetafields(ctx, &params)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	mfs, err := p.bc.ListProductMetafields(ctx, resolvedID)
	if err != nil {
		return toolError("failed to list metafields: %v", err), nil
	}

	return metafieldListJSON(resolvedID, "product_id", mfs)
}

func (p *Products) handleProductMetafieldsSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseProductMetafieldSetParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	resolvedID, err := p.resolveProductIDForMetafields(ctx, params)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)
	return metafieldUpsertCore(
		ctx, resolvedID,
		params.Namespace, params.Key, params.Value, params.Description, params.PermissionSet,
		productMetafieldOps(p.bc, resolvedID),
		"product_id",
		defaultProductMetafieldPermissionSet,
		confirmed,
		"product",
		&metafieldUpsertOptions{
			PreserveEmptyPermissionOnUpdate: true,
			AppOnlyStylePreview:             true,
		},
	)
}

func (p *Products) handleProductMetafieldsDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseProductMetafieldDeleteParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	resolvedID, err := p.resolveProductIDFromMetafieldParts(ctx, params.ProductID, params.SKU, params.ProductName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)
	return metafieldDeleteCore(
		ctx, resolvedID,
		params.MetafieldID, params.Namespace, params.Key,
		productMetafieldOps(p.bc, resolvedID),
		"product_id", confirmed, "product",
		nil,
	)
}
