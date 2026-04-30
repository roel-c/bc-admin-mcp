package catalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

const defaultVariantMetafieldPermissionSet = "app_only"

// RegisterVariantMetafieldTools registers variant metafield list/set/delete tools.
func (p *Products) RegisterVariantMetafieldTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/metafields/list",
		Tier:    middleware.TierR0,
		Summary: "List all metafields on a product variant",
		Description: "Returns custom metafields for one variant. Identify the product by product_id, sku " +
			"(product SKU), or product_name; identify the variant by variant_id or variant_sku (variant SKU).",
		Tool: mcp.NewTool("catalog_products_variants_metafields_list",
			mcp.WithDescription(
				"List metafields on a variant. Product: exactly one of product_id, sku, or product_name. "+
					"Variant: exactly one of variant_id or variant_sku.",
			),
			mcp.WithNumber("product_id", mcp.Description("Numeric product ID")),
			mcp.WithString("sku", mcp.Description("Exact product SKU — resolves to a single product")),
			mcp.WithString("product_name", mcp.Description("Exact product name — resolves to a single product")),
			mcp.WithNumber("variant_id", mcp.Description("Variant ID on the resolved product")),
			mcp.WithString("variant_sku", mcp.Description("Exact variant SKU — must match a single variant on the product")),
		),
		Handler: p.handleVariantMetafieldsList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "catalog/products/variants/metafields/set",
		Tier:    middleware.TierR1,
		Summary: "Create or update a variant metafield (upsert by namespace+key)",
		Description: "Sets a metafield on a variant. If omitted, permission_set defaults to app_only (not Storefront-exposed). " +
			"Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_products_variants_metafields_set",
			mcp.WithDescription(
				"Create or update a variant metafield. Same product/variant targeting as list. "+
					"Required: namespace, key, value. Optional: description, permission_set (default app_only). "+
					"Preview; pass confirmed=true to execute.",
			),
			mcp.WithNumber("product_id", mcp.Description("Numeric product ID")),
			mcp.WithString("sku", mcp.Description("Exact product SKU")),
			mcp.WithString("product_name", mcp.Description("Exact product name")),
			mcp.WithNumber("variant_id", mcp.Description("Variant ID")),
			mcp.WithString("variant_sku", mcp.Description("Exact variant SKU")),
			mcp.WithString("namespace", mcp.Description("Metafield namespace (e.g. my_integration)"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key within the namespace"), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value (string; JSON may be stored as a string)"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional human-readable description")),
			mcp.WithString("permission_set", mcp.Description(
				"Optional. One of: app_only (default), read, write, read_and_sf_access, write_and_sf_access.",
			)),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleVariantMetafieldsSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/variants/metafields/delete",
		Tier:        middleware.TierR1,
		Summary:     "Delete a variant metafield",
		Description: "Deletes by metafield_id or by namespace+key. Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("catalog_products_variants_metafields_delete",
			mcp.WithDescription(
				"Delete a variant metafield. Same product/variant targeting as list. Use metafield_id or namespace+key.",
			),
			mcp.WithNumber("product_id", mcp.Description("Numeric product ID")),
			mcp.WithString("sku", mcp.Description("Exact product SKU")),
			mcp.WithString("product_name", mcp.Description("Exact product name")),
			mcp.WithNumber("variant_id", mcp.Description("Variant ID")),
			mcp.WithString("variant_sku", mcp.Description("Exact variant SKU")),
			mcp.WithNumber("metafield_id", mcp.Description("Metafield ID (mutually exclusive with namespace+key)")),
			mcp.WithString("namespace", mcp.Description("Namespace (with key)")),
			mcp.WithString("key", mcp.Description("Key (with namespace)")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview")),
		),
		Handler: p.handleVariantMetafieldsDelete,
	})
}

// VariantMetafieldSetParams holds parsed arguments for variant metafield set/list targeting.
type VariantMetafieldSetParams struct {
	ProductID     int
	SKU           string
	ProductName   string
	VariantID     int
	VariantSKU    string
	Namespace     string
	Key           string
	Value         string
	Description   string
	PermissionSet string
}

// ParseVariantMetafieldSetParams validates set-tool arguments.
func ParseVariantMetafieldSetParams(args map[string]any) (*VariantMetafieldSetParams, error) {
	p := &VariantMetafieldSetParams{}
	if err := parseVariantMetafieldLocatorArgs(args, &p.ProductID, &p.SKU, &p.ProductName, &p.VariantID, &p.VariantSKU); err != nil {
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

// VariantMetafieldDeleteParams holds parsed delete-tool arguments.
type VariantMetafieldDeleteParams struct {
	ProductID   int
	SKU         string
	ProductName string
	VariantID   int
	VariantSKU  string
	Namespace   string
	Key         string
	MetafieldID int
}

// ParseVariantMetafieldDeleteParams validates delete-tool arguments.
func ParseVariantMetafieldDeleteParams(args map[string]any) (*VariantMetafieldDeleteParams, error) {
	p := &VariantMetafieldDeleteParams{}
	if err := parseVariantMetafieldLocatorArgs(args, &p.ProductID, &p.SKU, &p.ProductName, &p.VariantID, &p.VariantSKU); err != nil {
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

func parseVariantMetafieldLocatorArgs(
	args map[string]any,
	productID *int,
	sku *string,
	productName *string,
	variantID *int,
	variantSKU *string,
) error {
	if err := parseProductMetafieldTargetArgs(args, productID, sku, productName); err != nil {
		return err
	}

	var vid int
	var vsku string
	hasVariantID := false

	if v, ok := args["variant_id"]; ok {
		f, fOk := v.(float64)
		if !fOk {
			return fmt.Errorf("variant_id must be a number")
		}
		vid = int(f)
		hasVariantID = true
		if vid <= 0 {
			return fmt.Errorf("variant_id must be positive")
		}
	}
	if v, ok := args["variant_sku"]; ok {
		s, sOk := v.(string)
		if !sOk {
			return fmt.Errorf("variant_sku must be a string")
		}
		vsku = strings.TrimSpace(s)
	}

	vmodes := 0
	if hasVariantID {
		vmodes++
	}
	if vsku != "" {
		vmodes++
	}
	if vmodes == 0 {
		return fmt.Errorf("provide exactly one of: variant_id or variant_sku")
	}
	if vmodes > 1 {
		return fmt.Errorf("use only one of: variant_id or variant_sku")
	}

	*variantID = vid
	*variantSKU = vsku
	return nil
}

func (p *Products) resolveVariantIDForMetafields(ctx context.Context, productID int, variantID int, variantSKU string) (int, error) {
	if variantID > 0 {
		v, err := p.bc.GetVariant(ctx, productID, variantID)
		if err != nil {
			return 0, fmt.Errorf("variant %d on product %d: %w", variantID, productID, err)
		}
		if v != nil && v.ProductID != 0 && v.ProductID != productID {
			return 0, fmt.Errorf("variant %d does not belong to product %d", variantID, productID)
		}
		return variantID, nil
	}

	vars, err := p.bc.ListVariantsForProduct(ctx, productID)
	if err != nil {
		return 0, err
	}
	var matches []int
	for i := range vars {
		if strings.TrimSpace(vars[i].SKU) == variantSKU {
			matches = append(matches, vars[i].ID)
		}
	}
	if len(matches) == 0 {
		return 0, fmt.Errorf("no variant with sku %q on product %d", variantSKU, productID)
	}
	if len(matches) > 1 {
		return 0, fmt.Errorf("multiple variants match sku %q on product %d; use variant_id", variantSKU, productID)
	}
	return matches[0], nil
}

func executeVariantMetafieldUpsert(
	ctx context.Context,
	bc BigCommerceAPI,
	productID, variantID int,
	namespace, key, value, description, permissionSet string,
) (action string, mf *bigcommerce.Metafield, err error) {
	return metafieldUpsertExecute(
		ctx, variantID, namespace, key, value, description, permissionSet,
		variantMetafieldOps(bc, productID),
		defaultVariantMetafieldPermissionSet,
		&metafieldUpsertOptions{PreserveEmptyPermissionOnUpdate: true},
	)
}

func resolveMetafieldIDForVariant(
	ctx context.Context,
	bc BigCommerceAPI,
	productID, variantID int,
	namespace, key string,
) (int, error) {
	return metafieldResolveIDByNamespaceKey(ctx, variantID, namespace, key, variantMetafieldOps(bc, productID))
}

func (p *Products) handleVariantMetafieldsList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	var loc VariantMetafieldSetParams
	if err := parseVariantMetafieldLocatorArgs(args, &loc.ProductID, &loc.SKU, &loc.ProductName, &loc.VariantID, &loc.VariantSKU); err != nil {
		return toolError("%s", err.Error()), nil
	}

	productID, err := p.resolveProductIDFromMetafieldParts(ctx, loc.ProductID, loc.SKU, loc.ProductName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	vid, err := p.resolveVariantIDForMetafields(ctx, productID, loc.VariantID, loc.VariantSKU)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	mfs, err := p.bc.ListVariantMetafields(ctx, productID, vid)
	if err != nil {
		return toolError("failed to list metafields: %v", err), nil
	}

	return metafieldListVariantJSON(productID, vid, mfs)
}

func (p *Products) handleVariantMetafieldsSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseVariantMetafieldSetParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	productID, err := p.resolveProductIDFromMetafieldParts(ctx, params.ProductID, params.SKU, params.ProductName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	vid, err := p.resolveVariantIDForMetafields(ctx, productID, params.VariantID, params.VariantSKU)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)
	msgSuffix := fmt.Sprintf(" (product %d)", productID)
	return metafieldUpsertCore(
		ctx, vid,
		params.Namespace, params.Key, params.Value, params.Description, params.PermissionSet,
		variantMetafieldOps(p.bc, productID),
		"variant_id",
		defaultVariantMetafieldPermissionSet,
		confirmed,
		"variant",
		&metafieldUpsertOptions{
			PreserveEmptyPermissionOnUpdate: true,
			AppOnlyStylePreview:             true,
			PreviewMerge:                    map[string]any{"product_id": productID},
			MessageSuffix:                   msgSuffix,
		},
	)
}

func (p *Products) handleVariantMetafieldsDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := ParseVariantMetafieldDeleteParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	productID, err := p.resolveProductIDFromMetafieldParts(ctx, params.ProductID, params.SKU, params.ProductName)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	vid, err := p.resolveVariantIDForMetafields(ctx, productID, params.VariantID, params.VariantSKU)
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	confirmed := middleware.IsConfirmed(request)
	msgSuffix := fmt.Sprintf(" (product %d)", productID)
	return metafieldDeleteCore(
		ctx, vid,
		params.MetafieldID, params.Namespace, params.Key,
		variantMetafieldOps(p.bc, productID),
		"variant_id", confirmed, "variant",
		&metafieldDeleteOptions{
			PreviewMerge:  map[string]any{"product_id": productID},
			MessageSuffix: msgSuffix,
		},
	)
}
