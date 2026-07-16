package carts

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// RegisterMetafieldTools wires carts/cart/metafields/* into the registry.
func (c *Carts) RegisterMetafieldTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/metafields/list",
		Tier:    middleware.TierR0,
		Summary: "List all metafields on a cart",
		Tool: mcp.NewTool("carts_cart_metafields_list",
			mcp.WithDescription("List all metafields attached to a cart. Scope: store_cart."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
		),
		Handler: c.handleMetafieldList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/metafields/set",
		Tier:    middleware.TierR1,
		Summary: "Upsert a metafield on a cart by namespace+key (preview then confirm)",
		Tool: mcp.NewTool("carts_cart_metafields_set",
			mcp.WithDescription("Create or update a metafield on a cart. Upserts by namespace+key — if a metafield with the same namespace and key exists it is updated; otherwise a new one is created. Scope: store_cart."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
			mcp.WithString("namespace", mcp.Description("Metafield namespace (e.g. app name)"), mcp.Required()),
			mcp.WithString("key", mcp.Description("Metafield key"), mcp.Required()),
			mcp.WithString("value", mcp.Description("Metafield value (string)"), mcp.Required()),
			mcp.WithString("description", mcp.Description("Optional description.")),
			mcp.WithString("permission_set",
				mcp.Description("Access control: app_only (default), read, write, read_and_sf_access, write_and_sf_access.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply.")),
		),
		Handler: c.handleMetafieldSet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/metafields/delete",
		Tier:    middleware.TierR1,
		Summary: "Delete a metafield from a cart by ID or namespace+key",
		Tool: mcp.NewTool("carts_cart_metafields_delete",
			mcp.WithDescription("Delete a metafield from a cart. Provide metafield_id, or namespace+key to resolve the ID. Scope: store_cart."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
			mcp.WithNumber("metafield_id", mcp.Description("Metafield ID. Provide this or namespace+key.")),
			mcp.WithString("namespace", mcp.Description("Namespace for namespace+key resolution.")),
			mcp.WithString("key", mcp.Description("Key for namespace+key resolution.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to delete.")),
		),
		Handler: c.handleMetafieldDelete,
	})
}

// ---- carts/cart/metafields/list ----

func (c *Carts) handleMetafieldList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}

	mfs, err := c.bc.ListCartMetafields(ctx, cartID)
	if err != nil {
		return shared.ToolError("failed to list metafields for cart %s: %v", cartID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"cart_id":    cartID,
		"total":      len(mfs),
		"metafields": mfs,
	})
}

// ---- carts/cart/metafields/set ----

func (c *Carts) handleMetafieldSet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}
	ns, ok := args["namespace"].(string)
	if !ok || strings.TrimSpace(ns) == "" {
		return shared.ToolError("namespace is required"), nil
	}
	key, ok := args["key"].(string)
	if !ok || strings.TrimSpace(key) == "" {
		return shared.ToolError("key is required"), nil
	}
	val, ok := args["value"].(string)
	if !ok {
		return shared.ToolError("value is required"), nil
	}

	mf := bigcommerce.Metafield{
		Namespace:     ns,
		Key:           key,
		Value:         val,
		PermissionSet: "app_only",
	}
	if v, ok := args["description"].(string); ok {
		mf.Description = v
	}
	if v, ok := args["permission_set"].(string); ok && v != "" {
		mf.PermissionSet = v
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "upsert_cart_metafield",
			"cart_id": cartID,
			"metafield": map[string]any{
				"namespace":      mf.Namespace,
				"key":            mf.Key,
				"value":          mf.Value,
				"permission_set": mf.PermissionSet,
			},
			"message": fmt.Sprintf("Will upsert metafield %s/%s on cart %s. Pass confirmed=true.", ns, key, cartID),
		})
	}

	// Upsert: list to check if namespace+key exists, then create or update.
	cacheKey := fmt.Sprintf("cart_mf_list:%s", cartID)
	existing, err := session.CacheOrFetch(c.cache.ForContext(ctx), cacheKey, func() ([]bigcommerce.Metafield, error) {
		return c.bc.ListCartMetafields(ctx, cartID)
	})
	if err != nil {
		return shared.ToolError("failed to list cart metafields: %v", err), nil
	}
	c.cache.ForContext(ctx).Delete(cacheKey)

	for _, e := range existing {
		if e.Namespace == ns && e.Key == key {
			updated, err := c.bc.UpdateCartMetafield(ctx, cartID, e.ID, mf)
			if err != nil {
				return shared.ToolError("failed to update cart metafield: %v", err), nil
			}
			return shared.ToolJSON(map[string]any{"status": "updated", "metafield": updated})
		}
	}
	created, err := c.bc.CreateCartMetafield(ctx, cartID, mf)
	if err != nil {
		return shared.ToolError("failed to create cart metafield: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{"status": "created", "metafield": created})
}

// ---- carts/cart/metafields/delete ----

func (c *Carts) handleMetafieldDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}

	var mfID int
	if v, ok := args["metafield_id"].(float64); ok && v > 0 {
		mfID = int(v)
	} else {
		ns, _ := args["namespace"].(string)
		key, _ := args["key"].(string)
		if ns == "" || key == "" {
			return shared.ToolError("provide metafield_id or both namespace and key"), nil
		}
		mfs, err := c.bc.ListCartMetafields(ctx, cartID)
		if err != nil {
			return shared.ToolError("failed to list cart metafields: %v", err), nil
		}
		for _, m := range mfs {
			if m.Namespace == ns && m.Key == key {
				mfID = m.ID
				break
			}
		}
		if mfID == 0 {
			return shared.ToolError("metafield %s/%s not found on cart %s", ns, key, cartID), nil
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "delete_cart_metafield",
			"cart_id":     cartID,
			"metafield_id": mfID,
			"message":     fmt.Sprintf("Will delete metafield %d from cart %s. Pass confirmed=true.", mfID, cartID),
		})
	}

	if err := c.bc.DeleteCartMetafield(ctx, cartID, mfID); err != nil {
		return shared.ToolError("failed to delete cart metafield %d: %v", mfID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":      "deleted",
		"cart_id":     cartID,
		"metafield_id": mfID,
	})
}
