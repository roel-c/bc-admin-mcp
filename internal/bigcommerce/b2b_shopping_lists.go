package bigcommerce

import (
	"context"
	"fmt"
)

// B2BShoppingListItemOption is a product option selection on a shopping list
// item. Usually omitted — BigCommerce derives it from productId/variantId.
type B2BShoppingListItemOption struct {
	OptionID    string `json:"optionId,omitempty"`
	OptionValue string `json:"optionValue,omitempty"`
}

// B2BShoppingListItem is one line item on a shopping list, used in both
// create and update request bodies. On update: if ID is 0/omitted, the item
// is matched by productId+variantId+optionList instead; quantity=0 removes
// the item.
type B2BShoppingListItem struct {
	ID         int                         `json:"id,omitempty"`
	ProductID  int                         `json:"productId"`
	VariantID  int                         `json:"variantId,omitempty"`
	Quantity   int                         `json:"quantity"`
	SortOrder  int                         `json:"sortOrder,omitempty"`
	OptionList []B2BShoppingListItemOption `json:"optionList,omitempty"`
}

// B2BShoppingListCreate is the request body for POST /shopping-list. Exactly
// one of UserID or CustomerID must be set. Status: 0=approved, 20=deleted,
// 30=draft, 40=ready for approval.
type B2BShoppingListCreate struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Status      string                `json:"status,omitempty"`
	UserID      int                   `json:"userId,omitempty"`
	CustomerID  int                   `json:"customerId,omitempty"`
	ChannelID   int                   `json:"channelId,omitempty"`
	Items       []B2BShoppingListItem `json:"items,omitempty"`
}

// B2BShoppingListUpdate is the request body for PUT /shopping-list/{id}. All
// fields optional; UserID/CustomerID here are only used to check permissions,
// not to reassign ownership.
type B2BShoppingListUpdate struct {
	Name        string                `json:"name,omitempty"`
	Description string                `json:"description,omitempty"`
	Status      string                `json:"status,omitempty"`
	UserID      int                   `json:"userId,omitempty"`
	CustomerID  int                   `json:"customerId,omitempty"`
	Items       []B2BShoppingListItem `json:"items,omitempty"`
}

// Shopping list response bodies are underdocumented in the OpenAPI spec, so
// list/get/create/update reads are passed through as generic maps.

// ListB2BShoppingLists returns shopping lists visible to the given buyer user
// (userId) or BC customer (customerId) — exactly one of the two params must be
// set. A junior buyer sees only their own lists; senior buyer/admin see all of
// their company's approved/pending lists.
func (c *B2BClient) ListB2BShoppingLists(ctx context.Context, params string) ([]map[string]any, error) {
	path := "shopping-list"
	if params != "" {
		path += "?" + params
	}
	raw, err := c.B2BGetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list B2B shopping lists: %w", err)
	}
	return unmarshalMapSlice(raw, "shopping list")
}

// GetB2BShoppingList fetches one shopping list's detail, including items.
func (c *B2BClient) GetB2BShoppingList(ctx context.Context, shoppingListID int, params string) (map[string]any, error) {
	path := fmt.Sprintf("shopping-list/%d", shoppingListID)
	if params != "" {
		path += "?" + params
	}
	body, err := c.B2BGet(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get B2B shopping list %d: %w", shoppingListID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "get B2B shopping list"); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateB2BShoppingList creates a new shopping list, optionally with initial
// items.
func (c *B2BClient) CreateB2BShoppingList(ctx context.Context, payload B2BShoppingListCreate) (map[string]any, error) {
	body, err := c.B2BPost(ctx, "shopping-list", payload)
	if err != nil {
		return nil, fmt.Errorf("create B2B shopping list: %w", err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "create B2B shopping list"); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateB2BShoppingList updates a shopping list's fields and/or items. Per
// the API contract: to keep existing items include them in Items; setting an
// item's quantity to 0 removes it.
func (c *B2BClient) UpdateB2BShoppingList(ctx context.Context, shoppingListID int, payload B2BShoppingListUpdate) (map[string]any, error) {
	body, err := c.B2BPut(ctx, fmt.Sprintf("shopping-list/%d", shoppingListID), payload)
	if err != nil {
		return nil, fmt.Errorf("update B2B shopping list %d: %w", shoppingListID, err)
	}
	out := map[string]any{}
	if err := b2bUnmarshalSingle(body, &out, "update B2B shopping list"); err != nil {
		return map[string]any{}, nil //nolint:nilerr // write succeeded; response body shape varies
	}
	return out, nil
}

// DeleteB2BShoppingList permanently deletes a shopping list. userID, if
// non-zero, is passed to check that user's delete permission.
func (c *B2BClient) DeleteB2BShoppingList(ctx context.Context, shoppingListID, userID int) error {
	path := fmt.Sprintf("shopping-list/%d", shoppingListID)
	if userID > 0 {
		path += fmt.Sprintf("?userId=%d", userID)
	}
	_, err := c.B2BDelete(ctx, path)
	if err != nil {
		return fmt.Errorf("delete B2B shopping list %d: %w", shoppingListID, err)
	}
	return nil
}

// DeleteB2BShoppingListItem removes a single item from a shopping list.
func (c *B2BClient) DeleteB2BShoppingListItem(ctx context.Context, shoppingListID, itemID int) error {
	_, err := c.B2BDelete(ctx, fmt.Sprintf("shopping-list/%d/items/%d", shoppingListID, itemID))
	if err != nil {
		return fmt.Errorf("delete item %d from B2B shopping list %d: %w", itemID, shoppingListID, err)
	}
	return nil
}
