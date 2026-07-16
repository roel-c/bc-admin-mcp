package carts_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/carts"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type CartToolsSuite struct {
	suite.Suite
	ctrl   *gomock.Controller
	mockBC *MockCartAPI
	ct     *carts.Carts
	reg    *discovery.Registry
}

func TestCartToolsSuite(t *testing.T) {
	suite.Run(t, new(CartToolsSuite))
}

func (s *CartToolsSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBC = NewMockCartAPI(s.ctrl)
	s.ct = carts.NewCarts(s.mockBC, session.NewStore(60*time.Second))
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("carts", "Carts")
	s.reg.RegisterCategory("carts/cart", "Cart CRUD")
	s.reg.RegisterCategory("carts/cart/items", "Cart item management")
	s.reg.RegisterCategory("carts/cart/metafields", "Cart metafields")
	s.reg.RegisterCategory("carts/checkout", "Checkout management")
	s.ct.RegisterTools(s.reg)
	s.ct.RegisterMetafieldTools(s.reg)
	s.ct.RegisterCheckoutTools(s.reg)
}

func (s *CartToolsSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *CartToolsSuite) callTool(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(toolPath)
	s.Require().NotNil(def, "tool %q not found in registry", toolPath)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: toolPath, Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *CartToolsSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func sampleCart() *bigcommerce.Cart {
	return &bigcommerce.Cart{
		ID:         "abc-123",
		CustomerID: 42,
		ChannelID:  1,
		Currency:   bigcommerce.CartCurrency{Code: "USD"},
		CartAmount: 19.99,
		LineItems: bigcommerce.CartLineItems{
			PhysicalItems: []bigcommerce.CartPhysicalItem{
				{ID: "item-uuid-1", Name: "Widget", ProductID: 100, VariantID: 200, SKU: "WGT-1", Quantity: 2, SalePrice: 9.99},
			},
		},
	}
}

// --- carts/cart/create ---

func (s *CartToolsSuite) TestCreatePreviewRequiresNoAPICall() {
	res, err := s.callTool("carts/cart/create", map[string]any{
		"line_items_json": `[{"product_id":100,"quantity":2}]`,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestCreateConfirmedCreatesCart() {
	created := sampleCart()
	s.mockBC.EXPECT().CreateCart(gomock.Any(), gomock.Any()).Return(created, nil)

	res, err := s.callTool("carts/cart/create", map[string]any{
		"line_items_json": `[{"product_id":100,"quantity":2}]`,
		"confirmed":       true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
	cart := data["cart"].(map[string]any)
	s.Equal("abc-123", cart["id"])
}

func (s *CartToolsSuite) TestCreateRejectsEmptyItems() {
	res, err := s.callTool("carts/cart/create", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CartToolsSuite) TestCreateRejectsInvalidLineItemsJSON() {
	res, err := s.callTool("carts/cart/create", map[string]any{
		"line_items_json": `not-json`,
	})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/cart/get ---

func (s *CartToolsSuite) TestGetReturnsCartView() {
	s.mockBC.EXPECT().GetCart(gomock.Any(), "abc-123", false).Return(sampleCart(), nil)

	res, err := s.callTool("carts/cart/get", map[string]any{"cart_id": "abc-123"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	cart := data["cart"].(map[string]any)
	s.Equal("abc-123", cart["id"])
	s.Equal("USD", cart["currency"])
}

func (s *CartToolsSuite) TestGetWithRedirectURLs() {
	cartWithURLs := sampleCart()
	cartWithURLs.RedirectURLs = &bigcommerce.CartRedirectURLs{
		CartURL:     "https://example.com/cart/abc-123",
		CheckoutURL: "https://example.com/checkout/abc-123",
	}
	s.mockBC.EXPECT().GetCart(gomock.Any(), "abc-123", true).Return(cartWithURLs, nil)

	res, err := s.callTool("carts/cart/get", map[string]any{"cart_id": "abc-123", "include_redirect_urls": true})
	s.NoError(err)
	data := s.parseJSON(res)
	cart := data["cart"].(map[string]any)
	s.NotNil(cart["redirect_urls"])
}

func (s *CartToolsSuite) TestGetRequiresCartID() {
	res, err := s.callTool("carts/cart/get", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/cart/update ---

func (s *CartToolsSuite) TestUpdatePreviewFetchesCurrentCart() {
	s.mockBC.EXPECT().GetCart(gomock.Any(), "abc-123", false).Return(sampleCart(), nil)

	res, err := s.callTool("carts/cart/update", map[string]any{
		"cart_id":     "abc-123",
		"customer_id": float64(99),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestUpdateConfirmedUpdatesCart() {
	updated := sampleCart()
	updated.CustomerID = 99
	s.mockBC.EXPECT().GetCart(gomock.Any(), "abc-123", false).Return(sampleCart(), nil)
	s.mockBC.EXPECT().UpdateCart(gomock.Any(), "abc-123", gomock.Any()).Return(updated, nil)

	res, err := s.callTool("carts/cart/update", map[string]any{
		"cart_id":     "abc-123",
		"customer_id": float64(99),
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *CartToolsSuite) TestUpdateRejectsNoFields() {
	res, err := s.callTool("carts/cart/update", map[string]any{"cart_id": "abc-123"})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/cart/delete ---

func (s *CartToolsSuite) TestDeletePreviewShowsCartSummary() {
	s.mockBC.EXPECT().GetCart(gomock.Any(), "abc-123", false).Return(sampleCart(), nil)

	res, err := s.callTool("carts/cart/delete", map[string]any{"cart_id": "abc-123"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.NotNil(data["would_delete"])
}

func (s *CartToolsSuite) TestDeleteConfirmedDeletesCart() {
	s.mockBC.EXPECT().GetCart(gomock.Any(), "abc-123", false).Return(sampleCart(), nil)
	s.mockBC.EXPECT().DeleteCart(gomock.Any(), "abc-123").Return(nil)

	res, err := s.callTool("carts/cart/delete", map[string]any{
		"cart_id":   "abc-123",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
}

// --- carts/cart/items/add ---

func (s *CartToolsSuite) TestItemsAddPreviewRequiresNoAPICall() {
	res, err := s.callTool("carts/cart/items/add", map[string]any{
		"cart_id":         "abc-123",
		"line_items_json": `[{"product_id":200,"quantity":1}]`,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestItemsAddConfirmedAddsItems() {
	updated := sampleCart()
	s.mockBC.EXPECT().AddCartItems(gomock.Any(), "abc-123", gomock.Any()).Return(updated, nil)

	res, err := s.callTool("carts/cart/items/add", map[string]any{
		"cart_id":         "abc-123",
		"line_items_json": `[{"product_id":200,"quantity":1}]`,
		"confirmed":       true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("items_added", data["status"])
}

func (s *CartToolsSuite) TestItemsAddRejectsZeroQuantity() {
	res, err := s.callTool("carts/cart/items/add", map[string]any{
		"cart_id":         "abc-123",
		"line_items_json": `[{"product_id":200,"quantity":0}]`,
	})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/cart/items/update ---

func (s *CartToolsSuite) TestItemsUpdatePreviewRequiresNoAPICall() {
	res, err := s.callTool("carts/cart/items/update", map[string]any{
		"cart_id":    "abc-123",
		"item_id":    "item-uuid-1",
		"quantity":   float64(3),
		"product_id": float64(100),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestItemsUpdateConfirmedUpdatesItem() {
	updated := sampleCart()
	s.mockBC.EXPECT().UpdateCartItem(gomock.Any(), "abc-123", "item-uuid-1", gomock.Any()).Return(updated, nil)

	res, err := s.callTool("carts/cart/items/update", map[string]any{
		"cart_id":    "abc-123",
		"item_id":    "item-uuid-1",
		"quantity":   float64(3),
		"product_id": float64(100),
		"confirmed":  true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("item_updated", data["status"])
}

func (s *CartToolsSuite) TestItemsUpdateRejectsQuantityZero() {
	res, err := s.callTool("carts/cart/items/update", map[string]any{
		"cart_id":    "abc-123",
		"item_id":    "item-uuid-1",
		"quantity":   float64(0),
		"product_id": float64(100),
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CartToolsSuite) TestItemsUpdateRequiresProductOrCustom() {
	res, err := s.callTool("carts/cart/items/update", map[string]any{
		"cart_id":  "abc-123",
		"item_id":  "item-uuid-1",
		"quantity": float64(2),
	})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/cart/items/remove ---

func (s *CartToolsSuite) TestItemsRemovePreviewFetchesCart() {
	s.mockBC.EXPECT().GetCart(gomock.Any(), "abc-123", false).Return(sampleCart(), nil)

	res, err := s.callTool("carts/cart/items/remove", map[string]any{
		"cart_id": "abc-123",
		"item_id": "item-uuid-1",
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.NotNil(data["would_remove"])
}

func (s *CartToolsSuite) TestItemsRemoveConfirmedRemovesItem() {
	updated := sampleCart()
	updated.LineItems.PhysicalItems = nil
	// No GetCart call expected — confirmed=true goes straight to DeleteCartItem.
	s.mockBC.EXPECT().DeleteCartItem(gomock.Any(), "abc-123", "item-uuid-1").Return(updated, nil)

	res, err := s.callTool("carts/cart/items/remove", map[string]any{
		"cart_id":   "abc-123",
		"item_id":   "item-uuid-1",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("item_removed", data["status"])
}

// --- carts/cart/checkout_url ---

func (s *CartToolsSuite) TestCheckoutURLReturnsURLs() {
	s.mockBC.EXPECT().CreateCartRedirectURLs(gomock.Any(), "abc-123").Return(&bigcommerce.CartRedirectURLs{
		CartURL:     "https://store.example.com/cart/abc-123",
		CheckoutURL: "https://store.example.com/checkout/abc-123",
	}, nil)

	res, err := s.callTool("carts/cart/checkout_url", map[string]any{"cart_id": "abc-123"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("abc-123", data["cart_id"])
	s.NotEmpty(data["checkout_url"])
}

func (s *CartToolsSuite) TestCheckoutURLRequiresCartID() {
	res, err := s.callTool("carts/cart/checkout_url", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/cart/metafields/list ---

func (s *CartToolsSuite) TestMetafieldListReturnsMetafields() {
	s.mockBC.EXPECT().ListCartMetafields(gomock.Any(), "abc-123").Return([]bigcommerce.Metafield{
		{ID: 1, Namespace: "my_app", Key: "ref", Value: "pim-42"},
	}, nil)

	res, err := s.callTool("carts/cart/metafields/list", map[string]any{"cart_id": "abc-123"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *CartToolsSuite) TestMetafieldListRequiresCartID() {
	res, err := s.callTool("carts/cart/metafields/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/cart/metafields/set ---

func (s *CartToolsSuite) TestMetafieldSetPreviewRequiresNoAPICall() {
	res, err := s.callTool("carts/cart/metafields/set", map[string]any{
		"cart_id":   "abc-123",
		"namespace": "my_app",
		"key":       "ref",
		"value":     "pim-42",
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestMetafieldSetConfirmedCreatesWhenNotExisting() {
	s.mockBC.EXPECT().ListCartMetafields(gomock.Any(), "abc-123").Return(nil, nil)
	s.mockBC.EXPECT().CreateCartMetafield(gomock.Any(), "abc-123", gomock.Any()).Return(
		&bigcommerce.Metafield{ID: 10, Namespace: "my_app", Key: "ref", Value: "pim-42"}, nil)

	res, err := s.callTool("carts/cart/metafields/set", map[string]any{
		"cart_id":   "abc-123",
		"namespace": "my_app",
		"key":       "ref",
		"value":     "pim-42",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("created", data["status"])
}

func (s *CartToolsSuite) TestMetafieldSetConfirmedUpdatesWhenExisting() {
	s.mockBC.EXPECT().ListCartMetafields(gomock.Any(), "abc-123").Return([]bigcommerce.Metafield{
		{ID: 10, Namespace: "my_app", Key: "ref", Value: "old-value"},
	}, nil)
	s.mockBC.EXPECT().UpdateCartMetafield(gomock.Any(), "abc-123", 10, gomock.Any()).Return(
		&bigcommerce.Metafield{ID: 10, Namespace: "my_app", Key: "ref", Value: "new-value"}, nil)

	res, err := s.callTool("carts/cart/metafields/set", map[string]any{
		"cart_id":   "abc-123",
		"namespace": "my_app",
		"key":       "ref",
		"value":     "new-value",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

// --- carts/cart/metafields/delete ---

func (s *CartToolsSuite) TestMetafieldDeletePreviewByID() {
	res, err := s.callTool("carts/cart/metafields/delete", map[string]any{
		"cart_id":     "abc-123",
		"metafield_id": float64(10),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestMetafieldDeleteConfirmedByID() {
	s.mockBC.EXPECT().DeleteCartMetafield(gomock.Any(), "abc-123", 10).Return(nil)

	res, err := s.callTool("carts/cart/metafields/delete", map[string]any{
		"cart_id":     "abc-123",
		"metafield_id": float64(10),
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("deleted", data["status"])
}

// --- carts/checkout/get ---

func (s *CartToolsSuite) TestCheckoutGetReturnsView() {
	co := &bigcommerce.Checkout{
		ID:          "abc-123",
		GrandTotal:  29.99,
		SubtotalExTax: 24.99,
		ChannelID:   1,
	}
	s.mockBC.EXPECT().GetCheckout(gomock.Any(), "abc-123").Return(co, nil)

	res, err := s.callTool("carts/checkout/get", map[string]any{"checkout_id": "abc-123"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	checkout := data["checkout"].(map[string]any)
	s.Equal("abc-123", checkout["id"])
	s.Equal(29.99, checkout["grand_total"])
}

// --- carts/checkout/coupon_apply ---

func (s *CartToolsSuite) TestCouponApplyPreview() {
	res, err := s.callTool("carts/checkout/coupon_apply", map[string]any{
		"checkout_id": "abc-123",
		"coupon_code": "SAVE10",
	})
	s.NoError(err)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestCouponApplyConfirmed() {
	co := &bigcommerce.Checkout{ID: "abc-123", GrandTotal: 19.99}
	s.mockBC.EXPECT().ApplyCoupon(gomock.Any(), "abc-123", "SAVE10").Return(co, nil)

	res, err := s.callTool("carts/checkout/coupon_apply", map[string]any{
		"checkout_id": "abc-123",
		"coupon_code": "SAVE10",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("coupon_applied", data["status"])
}

// --- carts/checkout/convert ---

func (s *CartToolsSuite) TestCheckoutConvertPreviewFetchesCheckout() {
	co := &bigcommerce.Checkout{
		ID:         "abc-123",
		GrandTotal: 29.99,
		Cart:       sampleCart(),
	}
	s.mockBC.EXPECT().GetCheckout(gomock.Any(), "abc-123").Return(co, nil)

	res, err := s.callTool("carts/checkout/convert", map[string]any{"checkout_id": "abc-123"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal(29.99, data["grand_total"])
}

// A checkout missing both a billing address and a consignment must surface
// BOTH warnings (previously the second overwrote the first).
func (s *CartToolsSuite) TestCheckoutConvertPreviewCollectsAllWarnings() {
	co := &bigcommerce.Checkout{ID: "abc-123", GrandTotal: 29.99, Cart: sampleCart()}
	s.mockBC.EXPECT().GetCheckout(gomock.Any(), "abc-123").Return(co, nil)

	res, err := s.callTool("carts/checkout/convert", map[string]any{"checkout_id": "abc-123"})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	warnings, ok := data["warnings"].([]any)
	s.Require().True(ok, "expected a warnings array")
	s.GreaterOrEqual(len(warnings), 2, "missing billing + consignment should both warn")
}

func (s *CartToolsSuite) TestCheckoutConvertConfirmedCreatesOrder() {
	co := &bigcommerce.Checkout{ID: "abc-123", GrandTotal: 29.99, Cart: sampleCart()}
	s.mockBC.EXPECT().GetCheckout(gomock.Any(), "abc-123").Return(co, nil)
	s.mockBC.EXPECT().ConvertCheckoutToOrder(gomock.Any(), "abc-123").Return(
		&bigcommerce.CheckoutOrderResult{ID: 456}, nil)

	res, err := s.callTool("carts/checkout/convert", map[string]any{
		"checkout_id": "abc-123",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("order_created", data["status"])
	s.Equal(float64(456), data["order_id"])
}

// --- carts/checkout/coupon_remove ---

func (s *CartToolsSuite) TestCouponRemovePreview() {
	res, err := s.callTool("carts/checkout/coupon_remove", map[string]any{
		"checkout_id": "abc-123",
		"coupon_code": "SAVE10",
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestCouponRemoveConfirmed() {
	co := &bigcommerce.Checkout{ID: "abc-123", GrandTotal: 29.99}
	s.mockBC.EXPECT().RemoveCoupon(gomock.Any(), "abc-123", "SAVE10").Return(co, nil)

	res, err := s.callTool("carts/checkout/coupon_remove", map[string]any{
		"checkout_id": "abc-123",
		"coupon_code": "SAVE10",
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("coupon_removed", data["status"])
}

func (s *CartToolsSuite) TestCouponRemoveRequiresCode() {
	res, err := s.callTool("carts/checkout/coupon_remove", map[string]any{"checkout_id": "abc-123"})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/checkout/billing_address ---

func (s *CartToolsSuite) TestBillingAddressPreview() {
	res, err := s.callTool("carts/checkout/billing_address", map[string]any{
		"checkout_id":  "abc-123",
		"address_json": `{"first_name":"Jane","last_name":"Doe","address1":"1 Main St","city":"NYC","country_code":"US"}`,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal("set_billing_address", data["action"])
}

func (s *CartToolsSuite) TestBillingAddressConfirmedCreates() {
	co := &bigcommerce.Checkout{ID: "abc-123", GrandTotal: 29.99}
	s.mockBC.EXPECT().SetBillingAddress(gomock.Any(), "abc-123", gomock.Any()).Return(co, nil)

	res, err := s.callTool("carts/checkout/billing_address", map[string]any{
		"checkout_id":  "abc-123",
		"address_json": `{"first_name":"Jane","last_name":"Doe","address1":"1 Main St","city":"NYC","country_code":"US"}`,
		"confirmed":    true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("billing_address_set", data["status"])
}

func (s *CartToolsSuite) TestBillingAddressConfirmedUpdatesWhenIDProvided() {
	co := &bigcommerce.Checkout{ID: "abc-123", GrandTotal: 29.99}
	s.mockBC.EXPECT().UpdateBillingAddress(gomock.Any(), "abc-123", "addr-77", gomock.Any()).Return(co, nil)

	res, err := s.callTool("carts/checkout/billing_address", map[string]any{
		"checkout_id":        "abc-123",
		"address_json":       `{"first_name":"Jane","last_name":"Doe","address1":"1 Main St","city":"NYC","country_code":"US"}`,
		"billing_address_id": "addr-77",
		"confirmed":          true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("billing_address_set", data["status"])
}

func (s *CartToolsSuite) TestBillingAddressRejectsMissingCountry() {
	res, err := s.callTool("carts/checkout/billing_address", map[string]any{
		"checkout_id":  "abc-123",
		"address_json": `{"first_name":"Jane","last_name":"Doe","address1":"1 Main St","city":"NYC"}`,
	})
	s.NoError(err)
	s.True(res.IsError)
}

func (s *CartToolsSuite) TestBillingAddressRejectsInvalidJSON() {
	res, err := s.callTool("carts/checkout/billing_address", map[string]any{
		"checkout_id":  "abc-123",
		"address_json": `not-json`,
	})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/checkout/consignment_add ---

func (s *CartToolsSuite) TestConsignmentAddPreview() {
	res, err := s.callTool("carts/checkout/consignment_add", map[string]any{
		"checkout_id":     "abc-123",
		"address_json":    `{"first_name":"Jane","last_name":"Doe","address1":"1 Main St","city":"NYC","country_code":"US"}`,
		"line_items_json": `[{"item_id":"item-uuid-1","quantity":2}]`,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
	s.Equal("add_consignment", data["action"])
}

func (s *CartToolsSuite) TestConsignmentAddConfirmed() {
	co := &bigcommerce.Checkout{ID: "abc-123", GrandTotal: 29.99}
	s.mockBC.EXPECT().AddConsignment(gomock.Any(), "abc-123", gomock.Any()).Return(co, nil)

	res, err := s.callTool("carts/checkout/consignment_add", map[string]any{
		"checkout_id":     "abc-123",
		"address_json":    `{"first_name":"Jane","last_name":"Doe","address1":"1 Main St","city":"NYC","country_code":"US"}`,
		"line_items_json": `[{"item_id":"item-uuid-1","quantity":2}]`,
		"confirmed":       true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("consignment_added", data["status"])
}

func (s *CartToolsSuite) TestConsignmentAddRejectsEmptyLineItems() {
	res, err := s.callTool("carts/checkout/consignment_add", map[string]any{
		"checkout_id":     "abc-123",
		"address_json":    `{"first_name":"Jane","last_name":"Doe","address1":"1 Main St","city":"NYC","country_code":"US"}`,
		"line_items_json": `[]`,
	})
	s.NoError(err)
	s.True(res.IsError)
}

// --- carts/checkout/consignment_update ---

func (s *CartToolsSuite) TestConsignmentUpdatePreview() {
	res, err := s.callTool("carts/checkout/consignment_update", map[string]any{
		"checkout_id":        "abc-123",
		"consignment_id":     "cons-1",
		"shipping_option_id": "opt-1",
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *CartToolsSuite) TestConsignmentUpdateConfirmedSelectsShippingOption() {
	co := &bigcommerce.Checkout{ID: "abc-123", GrandTotal: 34.99}
	s.mockBC.EXPECT().UpdateConsignment(gomock.Any(), "abc-123", "cons-1", gomock.Any()).Return(co, nil)

	res, err := s.callTool("carts/checkout/consignment_update", map[string]any{
		"checkout_id":        "abc-123",
		"consignment_id":     "cons-1",
		"shipping_option_id": "opt-1",
		"confirmed":          true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("consignment_updated", data["status"])
}

func (s *CartToolsSuite) TestConsignmentUpdateRejectsNoFields() {
	res, err := s.callTool("carts/checkout/consignment_update", map[string]any{
		"checkout_id":    "abc-123",
		"consignment_id": "cons-1",
	})
	s.NoError(err)
	s.True(res.IsError)
}
