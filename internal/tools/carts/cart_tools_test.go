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
	s.ct.RegisterTools(s.reg)
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
