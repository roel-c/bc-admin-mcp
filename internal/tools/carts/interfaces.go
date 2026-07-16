package carts

import (
	"context"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.Client satisfies CartAPI.
var _ CartAPI = (*bigcommerce.Client)(nil)

// CartAPI defines the BigCommerce client methods used by cart tool handlers.
// Defined on the consumer side per Go convention so tests can provide a mock
// without depending on the full client implementation.
type CartAPI interface {
	// Cart CRUD
	CreateCart(ctx context.Context, payload bigcommerce.CartCreate) (*bigcommerce.Cart, error)
	GetCart(ctx context.Context, cartID string, includeRedirectURLs bool) (*bigcommerce.Cart, error)
	UpdateCart(ctx context.Context, cartID string, payload bigcommerce.CartUpdate) (*bigcommerce.Cart, error)
	DeleteCart(ctx context.Context, cartID string) error
	// Cart items
	AddCartItems(ctx context.Context, cartID string, payload bigcommerce.CartItemsAdd) (*bigcommerce.Cart, error)
	UpdateCartItem(ctx context.Context, cartID, itemID string, payload bigcommerce.CartItemUpdate) (*bigcommerce.Cart, error)
	DeleteCartItem(ctx context.Context, cartID, itemID string) (*bigcommerce.Cart, error)
	// Cart redirect URLs
	CreateCartRedirectURLs(ctx context.Context, cartID string) (*bigcommerce.CartRedirectURLs, error)
	// Cart metafields
	ListCartMetafields(ctx context.Context, cartID string) ([]bigcommerce.Metafield, error)
	CreateCartMetafield(ctx context.Context, cartID string, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	UpdateCartMetafield(ctx context.Context, cartID string, mfID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	DeleteCartMetafield(ctx context.Context, cartID string, mfID int) error
	// Checkout
	GetCheckout(ctx context.Context, checkoutID string) (*bigcommerce.Checkout, error)
	ApplyCoupon(ctx context.Context, checkoutID, code string) (*bigcommerce.Checkout, error)
	RemoveCoupon(ctx context.Context, checkoutID, code string) (*bigcommerce.Checkout, error)
	SetBillingAddress(ctx context.Context, checkoutID string, addr bigcommerce.CheckoutAddressInput) (*bigcommerce.Checkout, error)
	UpdateBillingAddress(ctx context.Context, checkoutID, addrID string, addr bigcommerce.CheckoutAddressInput) (*bigcommerce.Checkout, error)
	AddConsignment(ctx context.Context, checkoutID string, consignment bigcommerce.CheckoutConsignmentInput) (*bigcommerce.Checkout, error)
	UpdateConsignment(ctx context.Context, checkoutID, consignID string, update bigcommerce.CheckoutConsignmentUpdate) (*bigcommerce.Checkout, error)
	ConvertCheckoutToOrder(ctx context.Context, checkoutID string) (*bigcommerce.CheckoutOrderResult, error)
}
