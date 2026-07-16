package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
)

// Cart represents a BigCommerce server-side cart (GET/POST/PUT /v3/carts).
type Cart struct {
	ID             string       `json:"id"`
	CustomerID     int          `json:"customer_id"`
	ChannelID      int          `json:"channel_id"`
	Email          string       `json:"email,omitempty"`
	Currency       CartCurrency `json:"currency"`
	Locale         string       `json:"locale,omitempty"`
	TaxIncluded    bool         `json:"tax_included"`
	BaseAmount     float64      `json:"base_amount"`
	DiscountAmount float64      `json:"discount_amount"`
	CartAmount     float64      `json:"cart_amount"`
	Coupons        []CartCoupon `json:"coupons,omitempty"`
	LineItems      CartLineItems `json:"line_items"`
	CreatedTime    string       `json:"created_time,omitempty"`
	UpdatedTime    string       `json:"updated_time,omitempty"`
	RedirectURLs   *CartRedirectURLs `json:"redirect_urls,omitempty"`
}

// CartCurrency holds the cart's active currency code.
type CartCurrency struct {
	Code string `json:"code"`
}

// CartCoupon is a coupon applied to the cart.
type CartCoupon struct {
	Code          string  `json:"code"`
	ID            string  `json:"id,omitempty"`
	CouponType    int     `json:"coupon_type,omitempty"`
	DiscountedAmount float64 `json:"discounted_amount"`
}

// CartLineItems groups all line-item types for a cart.
type CartLineItems struct {
	PhysicalItems    []CartPhysicalItem    `json:"physical_items"`
	DigitalItems     []CartDigitalItem     `json:"digital_items"`
	CustomItems      []CartCustomItem      `json:"custom_items"`
	GiftCertificates []CartGiftCertificate `json:"gift_certificates"`
}

// CartPhysicalItem is a physical product line item in a cart.
type CartPhysicalItem struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ProductID      int     `json:"product_id"`
	VariantID      int     `json:"variant_id,omitempty"`
	SKU            string  `json:"sku,omitempty"`
	Quantity       int     `json:"quantity"`
	SalePrice      float64 `json:"sale_price"`
	ListPrice      float64 `json:"list_price"`
	OriginalPrice  float64 `json:"original_price"`
	DiscountAmount float64 `json:"discount_amount"`
	ImageURL       string  `json:"image_url,omitempty"`
}

// CartDigitalItem is a digital product line item in a cart.
type CartDigitalItem struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	ProductID      int     `json:"product_id"`
	VariantID      int     `json:"variant_id,omitempty"`
	SKU            string  `json:"sku,omitempty"`
	Quantity       int     `json:"quantity"`
	SalePrice      float64 `json:"sale_price"`
	ListPrice      float64 `json:"list_price"`
	DiscountAmount float64 `json:"discount_amount"`
}

// CartCustomItem is a custom (non-catalog) line item in a cart.
type CartCustomItem struct {
	ID        string  `json:"id,omitempty"`
	Name      string  `json:"name"`
	SKU       string  `json:"sku,omitempty"`
	Quantity  int     `json:"quantity"`
	ListPrice float64 `json:"list_price"`
}

// CartGiftCertificate is a gift certificate line item in a cart.
type CartGiftCertificate struct {
	ID     string  `json:"id,omitempty"`
	Name   string  `json:"name,omitempty"`
	Theme  string  `json:"theme,omitempty"`
	Amount float64 `json:"amount"`
}

// CartRedirectURLs holds the checkout redirect URLs for a cart.
type CartRedirectURLs struct {
	CartURL             string `json:"cart_url"`
	CheckoutURL         string `json:"checkout_url"`
	EmbeddedCheckoutURL string `json:"embedded_checkout_url"`
}

// CartCreate is the request body for POST /v3/carts.
type CartCreate struct {
	CustomerID       int                    `json:"customer_id,omitempty"`
	ChannelID        int                    `json:"channel_id,omitempty"`
	LineItems        []CartLineItemInput    `json:"line_items,omitempty"`
	CustomItems      []CartCustomItemInput  `json:"custom_items,omitempty"`
	GiftCertificates []CartGiftCertInput   `json:"gift_certificates,omitempty"`
	Locale           string                 `json:"locale,omitempty"`
}

// CartUpdate is the request body for PUT /v3/carts/{id}.
type CartUpdate struct {
	CustomerID int    `json:"customer_id,omitempty"`
	ChannelID  int    `json:"channel_id,omitempty"`
	Locale     string `json:"locale,omitempty"`
}

// CartItemsAdd is the request body for POST /v3/carts/{id}/items.
type CartItemsAdd struct {
	LineItems        []CartLineItemInput   `json:"line_items,omitempty"`
	CustomItems      []CartCustomItemInput `json:"custom_items,omitempty"`
	GiftCertificates []CartGiftCertInput  `json:"gift_certificates,omitempty"`
}

// CartLineItemInput is a catalog line item for cart create/add-items.
type CartLineItemInput struct {
	Quantity  int `json:"quantity"`
	ProductID int `json:"product_id"`
	VariantID int `json:"variant_id,omitempty"`
}

// CartCustomItemInput is a custom item for cart create/add-items.
type CartCustomItemInput struct {
	Name      string  `json:"name"`
	SKU       string  `json:"sku,omitempty"`
	Quantity  int     `json:"quantity"`
	ListPrice float64 `json:"list_price"`
}

// CartGiftCertInput is a gift certificate for cart create/add-items.
type CartGiftCertInput struct {
	Name     string  `json:"name"`
	Theme    string  `json:"theme,omitempty"`
	Amount   float64 `json:"amount"`
	Quantity int     `json:"quantity"`
	Sender   CartGiftCertContact `json:"sender"`
	Recipient CartGiftCertContact `json:"recipient"`
}

// CartGiftCertContact is a sender/recipient for a gift certificate.
type CartGiftCertContact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// CartItemUpdate is the request body for PUT /v3/carts/{id}/items/{item_id}.
// Exactly one of LineItem or CustomItem should be populated.
type CartItemUpdate struct {
	LineItem   *CartLineItemInput   `json:"line_item,omitempty"`
	CustomItem *CartCustomItemInput `json:"custom_item,omitempty"`
}

// ---- Client methods ----

func (c *Client) unmarshalCart(body []byte, op string) (*Cart, error) {
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("%s: parse response: %w", op, err)
	}
	if len(resp.Data) == 0 || string(resp.Data) == "null" {
		return nil, fmt.Errorf("%s: response missing data", op)
	}
	var cart Cart
	if err := json.Unmarshal(resp.Data, &cart); err != nil {
		return nil, fmt.Errorf("%s: unmarshal cart: %w", op, err)
	}
	return &cart, nil
}

// CreateCart creates a new server-side cart via POST /v3/carts.
func (c *Client) CreateCart(ctx context.Context, payload CartCreate) (*Cart, error) {
	body, err := c.Post(ctx, "carts", payload)
	if err != nil {
		return nil, fmt.Errorf("create cart: %w", err)
	}
	return c.unmarshalCart(body, "create cart")
}

// GetCart fetches a cart by ID via GET /v3/carts/{id}.
// includeRedirectURLs appends ?include=redirect_urls to the request.
func (c *Client) GetCart(ctx context.Context, cartID string, includeRedirectURLs bool) (*Cart, error) {
	path := fmt.Sprintf("carts/%s", cartID)
	if includeRedirectURLs {
		path += "?include=redirect_urls"
	}
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get cart %s: %w", cartID, err)
	}
	return c.unmarshalCart(body, "get cart")
}

// UpdateCart updates a cart's metadata via PUT /v3/carts/{id}.
func (c *Client) UpdateCart(ctx context.Context, cartID string, payload CartUpdate) (*Cart, error) {
	body, err := c.Put(ctx, fmt.Sprintf("carts/%s", cartID), payload)
	if err != nil {
		return nil, fmt.Errorf("update cart %s: %w", cartID, err)
	}
	return c.unmarshalCart(body, "update cart")
}

// DeleteCart deletes a cart via DELETE /v3/carts/{id} (returns 204 No Content).
func (c *Client) DeleteCart(ctx context.Context, cartID string) error {
	_, err := c.Delete(ctx, fmt.Sprintf("carts/%s", cartID))
	if err != nil {
		return fmt.Errorf("delete cart %s: %w", cartID, err)
	}
	return nil
}

// AddCartItems adds items to a cart via POST /v3/carts/{id}/items.
func (c *Client) AddCartItems(ctx context.Context, cartID string, payload CartItemsAdd) (*Cart, error) {
	body, err := c.Post(ctx, fmt.Sprintf("carts/%s/items", cartID), payload)
	if err != nil {
		return nil, fmt.Errorf("add cart %s items: %w", cartID, err)
	}
	return c.unmarshalCart(body, "add cart items")
}

// UpdateCartItem updates a cart item's quantity via PUT /v3/carts/{id}/items/{item_id}.
func (c *Client) UpdateCartItem(ctx context.Context, cartID, itemID string, payload CartItemUpdate) (*Cart, error) {
	body, err := c.Put(ctx, fmt.Sprintf("carts/%s/items/%s", cartID, itemID), payload)
	if err != nil {
		return nil, fmt.Errorf("update cart %s item %s: %w", cartID, itemID, err)
	}
	return c.unmarshalCart(body, "update cart item")
}

// DeleteCartItem removes an item from a cart via DELETE /v3/carts/{id}/items/{item_id}.
// BC returns the updated cart (without the removed item).
func (c *Client) DeleteCartItem(ctx context.Context, cartID, itemID string) (*Cart, error) {
	body, err := c.Delete(ctx, fmt.Sprintf("carts/%s/items/%s", cartID, itemID))
	if err != nil {
		return nil, fmt.Errorf("remove cart %s item %s: %w", cartID, itemID, err)
	}
	if len(body) == 0 {
		return nil, nil
	}
	return c.unmarshalCart(body, "remove cart item")
}

// CreateCartRedirectURLs generates checkout redirect URLs via POST /v3/carts/{id}/redirect_urls.
func (c *Client) CreateCartRedirectURLs(ctx context.Context, cartID string) (*CartRedirectURLs, error) {
	body, err := c.Post(ctx, fmt.Sprintf("carts/%s/redirect_urls", cartID), struct{}{})
	if err != nil {
		return nil, fmt.Errorf("create cart %s redirect URLs: %w", cartID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse redirect URL response: %w", err)
	}
	var urls CartRedirectURLs
	if err := json.Unmarshal(resp.Data, &urls); err != nil {
		return nil, fmt.Errorf("unmarshal redirect URLs: %w", err)
	}
	return &urls, nil
}

// ---- Cart metafields (/v3/carts/{id}/metafields) ----

// ListCartMetafields lists all metafields on a cart.
func (c *Client) ListCartMetafields(ctx context.Context, cartID string) ([]Metafield, error) {
	raw, err := c.GetAll(ctx, fmt.Sprintf("carts/%s/metafields", cartID))
	if err != nil {
		return nil, fmt.Errorf("list metafields for cart %s: %w", cartID, err)
	}
	mfs := make([]Metafield, 0, len(raw))
	for _, r := range raw {
		var mf Metafield
		if err := json.Unmarshal(r, &mf); err != nil {
			return nil, fmt.Errorf("unmarshal cart metafield: %w", err)
		}
		mfs = append(mfs, mf)
	}
	return mfs, nil
}

// CreateCartMetafield creates a metafield on a cart.
func (c *Client) CreateCartMetafield(ctx context.Context, cartID string, mf Metafield) (*Metafield, error) {
	body, err := c.Post(ctx, fmt.Sprintf("carts/%s/metafields", cartID), mf)
	if err != nil {
		return nil, fmt.Errorf("create metafield on cart %s: %w", cartID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse cart metafield response: %w", err)
	}
	var created Metafield
	if err := json.Unmarshal(resp.Data, &created); err != nil {
		return nil, fmt.Errorf("unmarshal created cart metafield: %w", err)
	}
	return &created, nil
}

// UpdateCartMetafield updates an existing metafield on a cart.
func (c *Client) UpdateCartMetafield(ctx context.Context, cartID string, mfID int, mf Metafield) (*Metafield, error) {
	body, err := c.Put(ctx, fmt.Sprintf("carts/%s/metafields/%d", cartID, mfID), mf)
	if err != nil {
		return nil, fmt.Errorf("update metafield %d on cart %s: %w", mfID, cartID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse cart metafield response: %w", err)
	}
	var updated Metafield
	if err := json.Unmarshal(resp.Data, &updated); err != nil {
		return nil, fmt.Errorf("unmarshal updated cart metafield: %w", err)
	}
	return &updated, nil
}

// DeleteCartMetafield removes a metafield from a cart.
func (c *Client) DeleteCartMetafield(ctx context.Context, cartID string, mfID int) error {
	_, err := c.Delete(ctx, fmt.Sprintf("carts/%s/metafields/%d", cartID, mfID))
	if err != nil {
		return fmt.Errorf("delete metafield %d from cart %s: %w", mfID, cartID, err)
	}
	return nil
}
