package carts

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// Carts provides MCP tool handlers for the BigCommerce Cart Management API
// (/v3/carts): full cart lifecycle — create, read, update, delete, item
// management, and checkout URL generation.
type Carts struct {
	bc    CartAPI
	cache *session.Store
}

// NewCarts constructs a Carts handler.
func NewCarts(bc CartAPI, cache *session.Store) *Carts {
	return &Carts{bc: bc, cache: cache}
}

func cartCacheKey(cartID string) string {
	return fmt.Sprintf("cart:%s", cartID)
}

// RegisterTools wires all cart tools into the discovery registry.
func (c *Carts) RegisterTools(reg *discovery.Registry) {
	// ---- carts/cart/create ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/create",
		Tier:    middleware.TierR1,
		Summary: "Create a server-side cart with optional items and customer assignment",
		Tool: mcp.NewTool("carts_cart_create",
			mcp.WithDescription("Create a cart. Preview shows proposed contents; pass confirmed=true to create."),
			mcp.WithNumber("customer_id", mcp.Description("Customer ID to assign. Omit for a guest cart.")),
			mcp.WithNumber("channel_id", mcp.Description("Channel ID (defaults to 1 — default storefront).")),
			mcp.WithString("line_items_json",
				mcp.Description(`JSON array of catalog items: [{"product_id":1,"quantity":2,"variant_id":10}]. variant_id optional.`)),
			mcp.WithString("custom_items_json",
				mcp.Description(`JSON array of custom items: [{"name":"Item","sku":"X","quantity":1,"list_price":9.99}].`)),
			mcp.WithString("locale", mcp.Description("BCP 47 locale, e.g. en-US.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to create the cart after reviewing the preview.")),
		),
		Handler: c.handleCreate,
	})

	// ---- carts/cart/get ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/get",
		Tier:    middleware.TierR0,
		Summary: "Get cart details by ID including line items and totals",
		Tool: mcp.NewTool("carts_cart_get",
			mcp.WithDescription("Get a cart by its UUID. Returns line items, totals, and currency."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
			mcp.WithBoolean("include_redirect_urls", mcp.Description("Include checkout redirect URLs in the response.")),
		),
		Handler: c.handleGet,
	})

	// ---- carts/cart/update ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/update",
		Tier:    middleware.TierR1,
		Summary: "Update cart metadata (customer ID, channel, locale)",
		Tool: mcp.NewTool("carts_cart_update",
			mcp.WithDescription("Update a cart's customer assignment, channel, or locale. Preview first."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
			mcp.WithNumber("customer_id", mcp.Description("Reassign to a different customer. Use 0 to convert to guest.")),
			mcp.WithNumber("channel_id", mcp.Description("Move cart to a different channel.")),
			mcp.WithString("locale", mcp.Description("BCP 47 locale.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply the update.")),
		),
		Handler: c.handleUpdate,
	})

	// ---- carts/cart/delete ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/delete",
		Tier:    middleware.TierR3,
		Summary: "Permanently delete a cart and all its contents",
		Tool: mcp.NewTool("carts_cart_delete",
			mcp.WithDescription("Delete a cart. Preview shows cart summary. Pass confirmed=true to delete permanently."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to permanently delete the cart.")),
		),
		Handler: c.handleDelete,
	})

	// ---- carts/cart/items/add ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/items/add",
		Tier:    middleware.TierR1,
		Summary: "Add catalog or custom items to an existing cart",
		Tool: mcp.NewTool("carts_cart_items_add",
			mcp.WithDescription("Add items to an existing cart. Preview shows items to be added."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
			mcp.WithString("line_items_json",
				mcp.Description(`JSON array: [{"product_id":1,"quantity":2,"variant_id":10}].`)),
			mcp.WithString("custom_items_json",
				mcp.Description(`JSON array: [{"name":"Custom","sku":"X","quantity":1,"list_price":5.00}].`)),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to add the items.")),
		),
		Handler: c.handleItemsAdd,
	})

	// ---- carts/cart/items/update ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/items/update",
		Tier:    middleware.TierR1,
		Summary: "Update the quantity of a line item in a cart",
		Tool: mcp.NewTool("carts_cart_items_update",
			mcp.WithDescription("Update a cart item's quantity. For catalog items provide product_id (and variant_id if applicable). For custom items provide name, sku, and list_price."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
			mcp.WithString("item_id", mcp.Description("Line item UUID from the cart"), mcp.Required()),
			mcp.WithNumber("quantity", mcp.Description("New quantity (must be ≥ 1)"), mcp.Required()),
			mcp.WithNumber("product_id", mcp.Description("Required for catalog (physical/digital) items.")),
			mcp.WithNumber("variant_id", mcp.Description("Required if the catalog item has variants.")),
			mcp.WithString("custom_item_name", mcp.Description("For custom items: item name.")),
			mcp.WithString("custom_item_sku", mcp.Description("For custom items: SKU.")),
			mcp.WithNumber("custom_item_list_price", mcp.Description("For custom items: list price.")),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to apply the quantity change.")),
		),
		Handler: c.handleItemsUpdate,
	})

	// ---- carts/cart/items/remove ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/items/remove",
		Tier:    middleware.TierR2,
		Summary: "Remove a line item from a cart",
		Tool: mcp.NewTool("carts_cart_items_remove",
			mcp.WithDescription("Remove a line item from a cart. Preview shows which item will be removed."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
			mcp.WithString("item_id", mcp.Description("Line item UUID to remove"), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Pass true to remove the item.")),
		),
		Handler: c.handleItemsRemove,
	})

	// ---- carts/cart/checkout_url ----
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "carts/cart/checkout_url",
		Tier:    middleware.TierR0,
		Summary: "Generate checkout and cart redirect URLs for a cart",
		Tool: mcp.NewTool("carts_cart_checkout_url",
			mcp.WithDescription("Generate cart_url, checkout_url, and embedded_checkout_url for a cart. Use checkout_url to send a customer directly to checkout."),
			mcp.WithString("cart_id", mcp.Description("Cart UUID"), mcp.Required()),
		),
		Handler: c.handleCheckoutURL,
	})
}

// ---- carts/cart/create ----

func (c *Carts) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()

	payload := bigcommerce.CartCreate{}
	if v, ok := args["customer_id"].(float64); ok && v > 0 {
		payload.CustomerID = int(v)
	}
	if v, ok := args["channel_id"].(float64); ok && v > 0 {
		payload.ChannelID = int(v)
	}
	if v, ok := args["locale"].(string); ok && v != "" {
		payload.Locale = v
	}

	lineItems, err := parseLineItemsJSON(args, "line_items_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload.LineItems = lineItems

	customItems, err := parseCustomItemsJSON(args, "custom_items_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	payload.CustomItems = customItems

	if len(payload.LineItems) == 0 && len(payload.CustomItems) == 0 {
		return shared.ToolError("at least one line item or custom item is required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":       "preview",
			"action":       "create_cart",
			"customer_id":  payload.CustomerID,
			"channel_id":   payload.ChannelID,
			"line_items":   payload.LineItems,
			"custom_items": payload.CustomItems,
			"message":      fmt.Sprintf("Will create a cart with %d catalog item(s) and %d custom item(s). Pass confirmed=true to create.", len(payload.LineItems), len(payload.CustomItems)),
		})
	}

	cart, err := c.bc.CreateCart(ctx, payload)
	if err != nil {
		return shared.ToolError("failed to create cart: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "created",
		"cart":     cartView(cart),
	})
}

// ---- carts/cart/get ----

func (c *Carts) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}

	includeRedirectURLs, _ := args["include_redirect_urls"].(bool)

	cart, err := c.bc.GetCart(ctx, cartID, includeRedirectURLs)
	if err != nil {
		return shared.ToolError("failed to get cart %s: %v", cartID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"cart": cartView(cart),
	})
}

// ---- carts/cart/update ----

func (c *Carts) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}

	patch := bigcommerce.CartUpdate{}
	hasAnyField := false
	if v, ok := args["customer_id"].(float64); ok {
		patch.CustomerID = int(v)
		hasAnyField = true
	}
	if v, ok := args["channel_id"].(float64); ok && v > 0 {
		patch.ChannelID = int(v)
		hasAnyField = true
	}
	if v, ok := args["locale"].(string); ok && v != "" {
		patch.Locale = v
		hasAnyField = true
	}
	if !hasAnyField {
		return shared.ToolError("at least one of customer_id, channel_id, or locale must be provided"), nil
	}

	cacheKey := cartCacheKey(cartID)
	current, err := session.CacheOrFetch(c.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Cart, error) {
		return c.bc.GetCart(ctx, cartID, false)
	})
	if err != nil {
		return shared.ToolError("failed to fetch cart %s: %v", cartID, err), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "update_cart",
			"cart_id": cartID,
			"current": map[string]any{
				"customer_id": current.CustomerID,
				"channel_id":  current.ChannelID,
				"locale":      current.Locale,
			},
			"patch":   patch,
			"message": "Pass confirmed=true to apply the update.",
		})
	}

	c.cache.ForContext(ctx).Delete(cacheKey)
	updated, err := c.bc.UpdateCart(ctx, cartID, patch)
	if err != nil {
		return shared.ToolError("failed to update cart %s: %v", cartID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status": "updated",
		"cart":   cartView(updated),
	})
}

// ---- carts/cart/delete ----

func (c *Carts) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}

	cacheKey := cartCacheKey(cartID)
	cart, err := session.CacheOrFetch(c.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Cart, error) {
		return c.bc.GetCart(ctx, cartID, false)
	})
	if err != nil {
		return shared.ToolError("failed to fetch cart %s: %v", cartID, err), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "delete_cart",
			"cart_id":     cartID,
			"would_delete": cartSummary(cart),
			"message":     "This will permanently delete the cart and all its items. Pass confirmed=true to delete.",
		})
	}

	c.cache.ForContext(ctx).Delete(cacheKey)
	if err := c.bc.DeleteCart(ctx, cartID); err != nil {
		return shared.ToolError("failed to delete cart %s: %v", cartID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":  "deleted",
		"cart_id": cartID,
	})
}

// ---- carts/cart/items/add ----

func (c *Carts) handleItemsAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}

	lineItems, err := parseLineItemsJSON(args, "line_items_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	customItems, err := parseCustomItemsJSON(args, "custom_items_json")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	if len(lineItems) == 0 && len(customItems) == 0 {
		return shared.ToolError("at least one line item or custom item is required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":       "preview",
			"action":       "add_cart_items",
			"cart_id":      cartID,
			"line_items":   lineItems,
			"custom_items": customItems,
			"message":      fmt.Sprintf("Will add %d catalog item(s) and %d custom item(s) to cart %s. Pass confirmed=true.", len(lineItems), len(customItems), cartID),
		})
	}

	payload := bigcommerce.CartItemsAdd{
		LineItems:   lineItems,
		CustomItems: customItems,
	}
	updated, err := c.bc.AddCartItems(ctx, cartID, payload)
	if err != nil {
		return shared.ToolError("failed to add items to cart %s: %v", cartID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status": "items_added",
		"cart":   cartView(updated),
	})
}

// ---- carts/cart/items/update ----

func (c *Carts) handleItemsUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}
	itemID, ok := args["item_id"].(string)
	if !ok || strings.TrimSpace(itemID) == "" {
		return shared.ToolError("item_id is required"), nil
	}
	qtyRaw, ok := args["quantity"].(float64)
	if !ok {
		return shared.ToolError("quantity is required"), nil
	}
	qty := int(qtyRaw)
	if qty < 1 {
		return shared.ToolError("quantity must be ≥ 1"), nil
	}

	// Build the update payload — catalog item or custom item.
	var payload bigcommerce.CartItemUpdate
	if pid, ok := args["product_id"].(float64); ok && pid > 0 {
		li := &bigcommerce.CartLineItemInput{
			Quantity:  qty,
			ProductID: int(pid),
		}
		if vid, ok := args["variant_id"].(float64); ok && vid > 0 {
			li.VariantID = int(vid)
		}
		payload.LineItem = li
	} else {
		name, _ := args["custom_item_name"].(string)
		if name == "" {
			return shared.ToolError("either product_id (for catalog items) or custom_item_name (for custom items) is required"), nil
		}
		sku, _ := args["custom_item_sku"].(string)
		price, _ := args["custom_item_list_price"].(float64)
		payload.CustomItem = &bigcommerce.CartCustomItemInput{
			Name:      name,
			SKU:       sku,
			Quantity:  qty,
			ListPrice: price,
		}
	}

	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "update_cart_item",
			"cart_id":  cartID,
			"item_id":  itemID,
			"payload":  payload,
			"message":  fmt.Sprintf("Will update item %s in cart %s to quantity %d. Pass confirmed=true.", itemID, cartID, qty),
		})
	}

	updated, err := c.bc.UpdateCartItem(ctx, cartID, itemID, payload)
	if err != nil {
		return shared.ToolError("failed to update item %s in cart %s: %v", itemID, cartID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status": "item_updated",
		"cart":   cartView(updated),
	})
}

// ---- carts/cart/items/remove ----

func (c *Carts) handleItemsRemove(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}
	itemID, ok := args["item_id"].(string)
	if !ok || strings.TrimSpace(itemID) == "" {
		return shared.ToolError("item_id is required"), nil
	}

	if !middleware.IsConfirmedFromArgs(args) {
		// Fetch cart to show which item will be removed.
		cacheKey := fmt.Sprintf("cart_item_remove:%s:%s", cartID, itemID)
		cart, err := session.CacheOrFetch(c.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Cart, error) {
			return c.bc.GetCart(ctx, cartID, false)
		})
		if err != nil {
			return shared.ToolError("failed to fetch cart %s: %v", cartID, err), nil
		}

		itemInfo := findLineItem(cart, itemID)
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "remove_cart_item",
			"cart_id":     cartID,
			"item_id":     itemID,
			"would_remove": itemInfo,
			"message":     fmt.Sprintf("Will remove item %s from cart %s. Pass confirmed=true.", itemID, cartID),
		})
	}

	c.cache.ForContext(ctx).Delete(fmt.Sprintf("cart_item_remove:%s:%s", cartID, itemID))
	updated, err := c.bc.DeleteCartItem(ctx, cartID, itemID)
	if err != nil {
		return shared.ToolError("failed to remove item %s from cart %s: %v", itemID, cartID, err), nil
	}
	result := map[string]any{
		"status":  "item_removed",
		"cart_id": cartID,
		"item_id": itemID,
	}
	if updated != nil {
		result["cart"] = cartView(updated)
	}
	return shared.ToolJSON(result)
}

// ---- carts/cart/checkout_url ----

func (c *Carts) handleCheckoutURL(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	cartID, ok := args["cart_id"].(string)
	if !ok || strings.TrimSpace(cartID) == "" {
		return shared.ToolError("cart_id is required"), nil
	}

	urls, err := c.bc.CreateCartRedirectURLs(ctx, cartID)
	if err != nil {
		return shared.ToolError("failed to generate redirect URLs for cart %s: %v", cartID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"cart_id":               cartID,
		"cart_url":              urls.CartURL,
		"checkout_url":          urls.CheckoutURL,
		"embedded_checkout_url": urls.EmbeddedCheckoutURL,
	})
}

// ---- View helpers ----

// cartView returns a token-efficient representation of a cart for tool responses.
func cartView(cart *bigcommerce.Cart) map[string]any {
	if cart == nil {
		return nil
	}
	physItems := make([]map[string]any, len(cart.LineItems.PhysicalItems))
	for i, it := range cart.LineItems.PhysicalItems {
		physItems[i] = map[string]any{
			"id": it.ID, "name": it.Name, "product_id": it.ProductID,
			"variant_id": it.VariantID, "sku": it.SKU,
			"quantity": it.Quantity, "sale_price": it.SalePrice,
		}
	}
	digItems := make([]map[string]any, len(cart.LineItems.DigitalItems))
	for i, it := range cart.LineItems.DigitalItems {
		digItems[i] = map[string]any{
			"id": it.ID, "name": it.Name, "product_id": it.ProductID,
			"quantity": it.Quantity, "sale_price": it.SalePrice,
		}
	}
	custItems := make([]map[string]any, len(cart.LineItems.CustomItems))
	for i, it := range cart.LineItems.CustomItems {
		custItems[i] = map[string]any{
			"id": it.ID, "name": it.Name, "sku": it.SKU,
			"quantity": it.Quantity, "list_price": it.ListPrice,
		}
	}

	v := map[string]any{
		"id":              cart.ID,
		"customer_id":     cart.CustomerID,
		"channel_id":      cart.ChannelID,
		"currency":        cart.Currency.Code,
		"locale":          cart.Locale,
		"base_amount":     cart.BaseAmount,
		"discount_amount": cart.DiscountAmount,
		"cart_amount":     cart.CartAmount,
		"line_items": map[string]any{
			"physical_items": physItems,
			"digital_items":  digItems,
			"custom_items":   custItems,
			"gift_certificate_count": len(cart.LineItems.GiftCertificates),
		},
		"created_time": cart.CreatedTime,
		"updated_time": cart.UpdatedTime,
	}
	if cart.RedirectURLs != nil {
		v["redirect_urls"] = map[string]string{
			"cart_url":              cart.RedirectURLs.CartURL,
			"checkout_url":          cart.RedirectURLs.CheckoutURL,
			"embedded_checkout_url": cart.RedirectURLs.EmbeddedCheckoutURL,
		}
	}
	if cart.Email != "" {
		v["email"] = cart.Email
	}
	return v
}

// cartSummary returns a minimal cart summary for delete previews.
func cartSummary(cart *bigcommerce.Cart) map[string]any {
	if cart == nil {
		return nil
	}
	totalItems := len(cart.LineItems.PhysicalItems) +
		len(cart.LineItems.DigitalItems) +
		len(cart.LineItems.CustomItems) +
		len(cart.LineItems.GiftCertificates)
	return map[string]any{
		"id":          cart.ID,
		"customer_id": cart.CustomerID,
		"channel_id":  cart.ChannelID,
		"currency":    cart.Currency.Code,
		"cart_amount": cart.CartAmount,
		"total_items": totalItems,
	}
}

// findLineItem locates a line item by UUID across all item types.
func findLineItem(cart *bigcommerce.Cart, itemID string) map[string]any {
	if cart == nil {
		return map[string]any{"item_id": itemID}
	}
	for _, it := range cart.LineItems.PhysicalItems {
		if it.ID == itemID {
			return map[string]any{"id": it.ID, "name": it.Name, "quantity": it.Quantity, "type": "physical"}
		}
	}
	for _, it := range cart.LineItems.DigitalItems {
		if it.ID == itemID {
			return map[string]any{"id": it.ID, "name": it.Name, "quantity": it.Quantity, "type": "digital"}
		}
	}
	for _, it := range cart.LineItems.CustomItems {
		if it.ID == itemID {
			return map[string]any{"id": it.ID, "name": it.Name, "quantity": it.Quantity, "type": "custom"}
		}
	}
	return map[string]any{"item_id": itemID, "note": "item not found in current cart"}
}

// ---- Parse helpers ----

func parseLineItemsJSON(args map[string]any, key string) ([]bigcommerce.CartLineItemInput, error) {
	raw, ok := args[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var items []bigcommerce.CartLineItemInput
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("invalid %s JSON: %v", key, err)
	}
	for i, item := range items {
		if item.ProductID == 0 {
			return nil, fmt.Errorf("%s[%d]: product_id is required", key, i)
		}
		if item.Quantity < 1 {
			return nil, fmt.Errorf("%s[%d]: quantity must be ≥ 1", key, i)
		}
	}
	return items, nil
}

func parseCustomItemsJSON(args map[string]any, key string) ([]bigcommerce.CartCustomItemInput, error) {
	raw, ok := args[key].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var items []bigcommerce.CartCustomItemInput
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("invalid %s JSON: %v", key, err)
	}
	for i, item := range items {
		if strings.TrimSpace(item.Name) == "" {
			return nil, fmt.Errorf("%s[%d]: name is required", key, i)
		}
		if item.Quantity < 1 {
			return nil, fmt.Errorf("%s[%d]: quantity must be ≥ 1", key, i)
		}
		if item.ListPrice <= 0 {
			return nil, fmt.Errorf("%s[%d]: list_price must be > 0", key, i)
		}
	}
	return items, nil
}
