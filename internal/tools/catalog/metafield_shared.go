package catalog

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// metafieldUpsertExecute runs list → match namespace+key → update or create with the same rules as
// metafieldUpsertCore when confirmed. Used by MCP tools (via core) and bulk/catalog callers.
func metafieldUpsertExecute(
	ctx context.Context,
	resourceID int,
	namespace, key, value, description, permissionSet string,
	ops MetafieldResourceOps,
	createDefaultPermission string,
	opts *metafieldUpsertOptions,
) (action string, mf *bigcommerce.Metafield, err error) {
	var o metafieldUpsertOptions
	if opts != nil {
		o = *opts
	}

	existing, listErr := ops.List(ctx, resourceID)
	if listErr != nil {
		return "", nil, listErr
	}

	var existingMF *bigcommerce.Metafield
	for i := range existing {
		if existing[i].Namespace == namespace && existing[i].Key == key {
			existingMF = &existing[i]
			break
		}
	}

	payload := bigcommerce.Metafield{
		Namespace:     namespace,
		Key:           key,
		Value:         value,
		Description:   description,
		PermissionSet: permissionSet,
	}

	if existingMF != nil {
		if o.PreserveEmptyPermissionOnUpdate && payload.PermissionSet == "" {
			payload.PermissionSet = existingMF.PermissionSet
		}
		updated, updateErr := ops.Update(ctx, resourceID, existingMF.ID, payload)
		if updateErr != nil {
			return "", nil, fmt.Errorf("update failed: %w", updateErr)
		}
		return "updated", updated, nil
	}

	if payload.PermissionSet == "" {
		payload.PermissionSet = createDefaultPermission
	}
	created, createErr := ops.Create(ctx, resourceID, payload)
	if createErr != nil {
		return "", nil, fmt.Errorf("create failed: %w", createErr)
	}
	return "created", created, nil
}

// metafieldResolveIDByNamespaceKey returns the metafield id for namespace+key, or 0 if absent.
func metafieldResolveIDByNamespaceKey(
	ctx context.Context,
	resourceID int,
	namespace, key string,
	ops MetafieldResourceOps,
) (int, error) {
	existing, err := ops.List(ctx, resourceID)
	if err != nil {
		return 0, err
	}
	for i := range existing {
		if existing[i].Namespace == namespace && existing[i].Key == key {
			return existing[i].ID, nil
		}
	}
	return 0, nil
}

// MetafieldResourceOps binds List/Create/Update/Delete for one catalog resource
// (e.g. category or brand) so upsert/delete flows can be shared.
type MetafieldResourceOps struct {
	List   func(context.Context, int) ([]bigcommerce.Metafield, error)
	Create func(context.Context, int, bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	Update func(context.Context, int, int, bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	Delete func(context.Context, int, int) error
}

// metafieldListJSON builds the standard list response for category/brand metafields.
func metafieldListJSON(resolvedID int, idKey string, mfs []bigcommerce.Metafield) (*mcp.CallToolResult, error) {
	return toolJSON(map[string]any{
		idKey:        resolvedID,
		"total":      len(mfs),
		"metafields": mfs,
	})
}

// metafieldListVariantJSON builds list response for variant metafields (product + variant ids).
func metafieldListVariantJSON(productID, variantID int, mfs []bigcommerce.Metafield) (*mcp.CallToolResult, error) {
	return toolJSON(map[string]any{
		"product_id": productID,
		"variant_id": variantID,
		"total":      len(mfs),
		"metafields": mfs,
	})
}

func mergeMetafieldPreview(dst map[string]any, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

const appOnlyMetafieldPermissionNote = "New metafields default to app_only. Use read_and_sf_access or write_and_sf_access for Storefront-readable values."

// metafieldUpsertOptions tweaks shared upsert behavior for product/variant metafields
// (app_only defaults, permission preview, preserve permission on update). Nil opts = category/brand behavior.
type metafieldUpsertOptions struct {
	PreserveEmptyPermissionOnUpdate bool
	AppOnlyStylePreview             bool
	PreviewMerge                    map[string]any
	// MessageSuffix is appended before the final period in preview/success messages, e.g. " (product 42)" for variants.
	MessageSuffix string
}

// metafieldDeleteOptions tweaks delete preview/success JSON. Nil opts = category/brand behavior.
type metafieldDeleteOptions struct {
	PreviewMerge  map[string]any
	MessageSuffix string
}

// metafieldUpsertCore implements preview + create/update for catalog metafields
// where create defaults permission to createDefaultPermission when empty.
func metafieldUpsertCore(
	ctx context.Context,
	resolvedID int,
	namespace, key, value, description, permissionSet string,
	ops MetafieldResourceOps,
	resourceIDKey string,
	createDefaultPermission string,
	confirmed bool,
	resourceNoun string,
	opts *metafieldUpsertOptions,
) (*mcp.CallToolResult, error) {
	var o metafieldUpsertOptions
	if opts != nil {
		o = *opts
	}

	if !confirmed {
		existing, err := ops.List(ctx, resolvedID)
		if err != nil {
			return toolError("failed to list existing metafields: %v", err), nil
		}
		var existingMF *bigcommerce.Metafield
		for i := range existing {
			if existing[i].Namespace == namespace && existing[i].Key == key {
				existingMF = &existing[i]
				break
			}
		}

		action := "create"
		preview := map[string]any{
			"status":      "pending_confirmation",
			resourceIDKey: resolvedID,
			"namespace":   namespace,
			"key":         key,
			"value":       value,
		}
		if existingMF != nil {
			action = "update"
			preview["existing_value"] = existingMF.Value
			preview["metafield_id"] = existingMF.ID
		}
		preview["action"] = action
		if o.AppOnlyStylePreview {
			var effectivePerm string
			if existingMF != nil {
				if permissionSet != "" {
					effectivePerm = permissionSet
				} else {
					effectivePerm = existingMF.PermissionSet
				}
			} else {
				if permissionSet != "" {
					effectivePerm = permissionSet
				} else {
					effectivePerm = createDefaultPermission
				}
			}
			preview["permission_set"] = effectivePerm
			preview["permission_note"] = appOnlyMetafieldPermissionNote
			if existingMF != nil {
				preview["existing_permission_set"] = existingMF.PermissionSet
			}
			if description != "" {
				preview["description"] = description
			}
		}
		mergeMetafieldPreview(preview, o.PreviewMerge)
		preview["message"] = fmt.Sprintf(
			"Will %s metafield %s.%s on %s %d%s. Pass confirmed=true to execute.",
			action, namespace, key, resourceNoun, resolvedID, o.MessageSuffix,
		)
		return toolJSON(preview)
	}

	action, mf, execErr := metafieldUpsertExecute(
		ctx, resolvedID, namespace, key, value, description, permissionSet,
		ops, createDefaultPermission, opts,
	)
	if execErr != nil {
		return toolError("%s", execErr.Error()), nil
	}
	status := "created"
	if action == "updated" {
		status = "updated"
	}
	return toolJSON(map[string]any{
		"status":    status,
		"metafield": mf,
		"message": fmt.Sprintf(
			"Metafield %s.%s %s on %s %d%s.",
			namespace, key, action, resourceNoun, resolvedID, o.MessageSuffix,
		),
	})
}

// metafieldDeleteCore implements preview + delete by id or namespace+key.
func metafieldDeleteCore(
	ctx context.Context,
	resolvedID int,
	metafieldID int,
	namespace, key string,
	ops MetafieldResourceOps,
	resourceIDKey string,
	confirmed bool,
	resourceNoun string,
	opts *metafieldDeleteOptions,
) (*mcp.CallToolResult, error) {
	var o metafieldDeleteOptions
	if opts != nil {
		o = *opts
	}

	mfID := metafieldID
	if mfID == 0 {
		existing, listErr := ops.List(ctx, resolvedID)
		if listErr != nil {
			return toolError("failed to list metafields: %v", listErr), nil
		}
		for _, mf := range existing {
			if mf.Namespace == namespace && mf.Key == key {
				mfID = mf.ID
				break
			}
		}
		if mfID == 0 {
			return toolError("no metafield found with namespace %q key %q on %s %d", namespace, key, resourceNoun, resolvedID), nil
		}
	}

	if !confirmed {
		preview := map[string]any{
			"status":       "pending_confirmation",
			resourceIDKey:  resolvedID,
			"metafield_id": mfID,
			"message": fmt.Sprintf(
				"Will delete metafield %d from %s %d%s. Pass confirmed=true to execute.",
				mfID, resourceNoun, resolvedID, o.MessageSuffix,
			),
		}
		if namespace != "" {
			preview["namespace"] = namespace
			preview["key"] = key
		}
		mergeMetafieldPreview(preview, o.PreviewMerge)
		return toolJSON(preview)
	}

	if delErr := ops.Delete(ctx, resolvedID, mfID); delErr != nil {
		return toolError("delete failed: %v", delErr), nil
	}

	return toolJSON(map[string]any{
		"status": "deleted",
		"message": fmt.Sprintf(
			"Metafield %d deleted from %s %d%s.",
			mfID, resourceNoun, resolvedID, o.MessageSuffix,
		),
	})
}

func categoryMetafieldOps(bc BigCommerceAPI) MetafieldResourceOps {
	return MetafieldResourceOps{
		List: func(ctx context.Context, id int) ([]bigcommerce.Metafield, error) {
			return bc.ListCategoryMetafields(ctx, id)
		},
		Create: func(ctx context.Context, id int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			return bc.CreateCategoryMetafield(ctx, id, mf)
		},
		Update: func(ctx context.Context, id int, mfID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			return bc.UpdateCategoryMetafield(ctx, id, mfID, mf)
		},
		Delete: func(ctx context.Context, id int, mfID int) error {
			return bc.DeleteCategoryMetafield(ctx, id, mfID)
		},
	}
}

func brandMetafieldOps(bc BigCommerceAPI) MetafieldResourceOps {
	return MetafieldResourceOps{
		List: func(ctx context.Context, id int) ([]bigcommerce.Metafield, error) {
			return bc.ListBrandMetafields(ctx, id)
		},
		Create: func(ctx context.Context, id int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			return bc.CreateBrandMetafield(ctx, id, mf)
		},
		Update: func(ctx context.Context, id int, mfID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			return bc.UpdateBrandMetafield(ctx, id, mfID, mf)
		},
		Delete: func(ctx context.Context, id int, mfID int) error {
			return bc.DeleteBrandMetafield(ctx, id, mfID)
		},
	}
}

func productMetafieldOps(bc BigCommerceAPI, productID int) MetafieldResourceOps {
	return MetafieldResourceOps{
		List: func(ctx context.Context, id int) ([]bigcommerce.Metafield, error) {
			return bc.ListProductMetafields(ctx, productID)
		},
		Create: func(ctx context.Context, id int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			return bc.CreateProductMetafield(ctx, productID, mf)
		},
		Update: func(ctx context.Context, id int, mfID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			return bc.UpdateProductMetafield(ctx, productID, mfID, mf)
		},
		Delete: func(ctx context.Context, id int, mfID int) error {
			return bc.DeleteProductMetafield(ctx, productID, mfID)
		},
	}
}

func variantMetafieldOps(bc BigCommerceAPI, productID int) MetafieldResourceOps {
	return MetafieldResourceOps{
		List: func(ctx context.Context, variantID int) ([]bigcommerce.Metafield, error) {
			return bc.ListVariantMetafields(ctx, productID, variantID)
		},
		Create: func(ctx context.Context, variantID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			return bc.CreateVariantMetafield(ctx, productID, variantID, mf)
		},
		Update: func(ctx context.Context, variantID int, mfID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
			return bc.UpdateVariantMetafield(ctx, productID, variantID, mfID, mf)
		},
		Delete: func(ctx context.Context, variantID int, mfID int) error {
			return bc.DeleteVariantMetafield(ctx, productID, variantID, mfID)
		},
	}
}
