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
	CreateCart(ctx context.Context, payload bigcommerce.CartCreate) (*bigcommerce.Cart, error)
	GetCart(ctx context.Context, cartID string, includeRedirectURLs bool) (*bigcommerce.Cart, error)
	UpdateCart(ctx context.Context, cartID string, payload bigcommerce.CartUpdate) (*bigcommerce.Cart, error)
	DeleteCart(ctx context.Context, cartID string) error
	AddCartItems(ctx context.Context, cartID string, payload bigcommerce.CartItemsAdd) (*bigcommerce.Cart, error)
	UpdateCartItem(ctx context.Context, cartID, itemID string, payload bigcommerce.CartItemUpdate) (*bigcommerce.Cart, error)
	DeleteCartItem(ctx context.Context, cartID, itemID string) (*bigcommerce.Cart, error)
	CreateCartRedirectURLs(ctx context.Context, cartID string) (*bigcommerce.CartRedirectURLs, error)
}
