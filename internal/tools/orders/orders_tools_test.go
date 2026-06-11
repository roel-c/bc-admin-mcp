package orders_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	orders "github.com/roel-c/bc-admin-mcp/internal/tools/orders"
	"github.com/stretchr/testify/suite"
)

type fakeOrdersBC struct {
	listOrdersFn           func(context.Context, bigcommerce.OrderListParams) ([]bigcommerce.Order, error)
	countOrdersFn          func(context.Context, bigcommerce.OrderCountParams) (int, error)
	getOrderFn             func(context.Context, int, bigcommerce.OrderGetParams) (*bigcommerce.Order, error)
	createOrderFn          func(context.Context, json.RawMessage) (*bigcommerce.Order, error)
	deleteOrderFn          func(context.Context, int) error
	updateOrderFn          func(context.Context, int, json.RawMessage) (*bigcommerce.Order, error)
	updateOrderStatusFn    func(context.Context, int, int) (*bigcommerce.Order, error)
	listOrderProductsFn    func(context.Context, int, bigcommerce.OrderProductListParams) ([]bigcommerce.OrderProduct, error)
	getOrderProductFn      func(context.Context, int, int) (json.RawMessage, error)
	listOrderStatusesFn    func(context.Context) ([]bigcommerce.OrderStatus, error)
	listOrderCouponsFn     func(context.Context, int, bigcommerce.OrderCouponListParams) ([]json.RawMessage, error)
	listOrderAddressesFn   func(context.Context, int, bigcommerce.OrderShippingAddressListParams) ([]json.RawMessage, error)
	getOrderAddressFn      func(context.Context, int, int) (json.RawMessage, error)
	updateOrderAddressFn   func(context.Context, int, int, json.RawMessage) (json.RawMessage, error)
	listOrderMessagesFn    func(context.Context, int, bigcommerce.OrderMessageListParams) ([]json.RawMessage, error)
	listOrderTaxesFn       func(context.Context, int, bigcommerce.OrderTaxListParams) ([]json.RawMessage, error)
	listOrderMetafieldsFn  func(context.Context, int, bigcommerce.OrderMetafieldListParams) ([]bigcommerce.Metafield, error)
	createOrderMetafieldFn func(context.Context, int, bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	updateOrderMetafieldFn func(context.Context, int, int, bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	deleteOrderMetafieldFn func(context.Context, int, int) error
	listOrderShipmentsFn   func(context.Context, int, bigcommerce.OrderShipmentListParams) ([]bigcommerce.OrderShipment, error)
	getShipmentFn          func(context.Context, int, int) (*bigcommerce.OrderShipment, error)
	updateShipmentFn       func(context.Context, int, int, json.RawMessage) (*bigcommerce.OrderShipment, error)
	deleteShipmentFn       func(context.Context, int, int) error
	createShipmentFn       func(context.Context, int, bigcommerce.OrderShipmentCreate) (*bigcommerce.OrderShipment, error)
	listPaymentActionsFn   func(context.Context, int, bigcommerce.OrderPaymentActionListParams) ([]json.RawMessage, error)
	listTransactionsFn     func(context.Context, int, bigcommerce.OrderTransactionListParams) ([]json.RawMessage, error)
	listOrderRefundsFn     func(context.Context, int, bigcommerce.OrderRefundListParams) ([]json.RawMessage, error)
	listLegacyRefundsFn    func(context.Context, int, bigcommerce.OrderLegacyRefundListParams) ([]json.RawMessage, error)
	captureFn              func(context.Context, int) (json.RawMessage, error)
	voidFn                 func(context.Context, int) (json.RawMessage, error)
	refundQuoteFn          func(context.Context, int, json.RawMessage) (json.RawMessage, error)
	refundCreateFn         func(context.Context, int, json.RawMessage, string) (json.RawMessage, error)
}

func (f *fakeOrdersBC) ListOrders(ctx context.Context, params bigcommerce.OrderListParams) ([]bigcommerce.Order, error) {
	if f.listOrdersFn != nil {
		return f.listOrdersFn(ctx, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) CountOrders(ctx context.Context, params bigcommerce.OrderCountParams) (int, error) {
	if f.countOrdersFn != nil {
		return f.countOrdersFn(ctx, params)
	}
	return 0, nil
}

func (f *fakeOrdersBC) GetOrder(ctx context.Context, orderID int, params bigcommerce.OrderGetParams) (*bigcommerce.Order, error) {
	if f.getOrderFn != nil {
		return f.getOrderFn(ctx, orderID, params)
	}
	return &bigcommerce.Order{}, nil
}

func (f *fakeOrdersBC) CreateOrder(ctx context.Context, payload json.RawMessage) (*bigcommerce.Order, error) {
	if f.createOrderFn != nil {
		return f.createOrderFn(ctx, payload)
	}
	return &bigcommerce.Order{}, nil
}

func (f *fakeOrdersBC) DeleteOrder(ctx context.Context, orderID int) error {
	if f.deleteOrderFn != nil {
		return f.deleteOrderFn(ctx, orderID)
	}
	return nil
}

func (f *fakeOrdersBC) UpdateOrder(ctx context.Context, orderID int, payload json.RawMessage) (*bigcommerce.Order, error) {
	if f.updateOrderFn != nil {
		return f.updateOrderFn(ctx, orderID, payload)
	}
	return &bigcommerce.Order{}, nil
}

func (f *fakeOrdersBC) UpdateOrderStatus(ctx context.Context, orderID, statusID int) (*bigcommerce.Order, error) {
	if f.updateOrderStatusFn != nil {
		return f.updateOrderStatusFn(ctx, orderID, statusID)
	}
	return &bigcommerce.Order{}, nil
}

func (f *fakeOrdersBC) ListOrderProducts(ctx context.Context, orderID int, params bigcommerce.OrderProductListParams) ([]bigcommerce.OrderProduct, error) {
	if f.listOrderProductsFn != nil {
		return f.listOrderProductsFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) GetOrderProduct(ctx context.Context, orderID, productID int) (json.RawMessage, error) {
	if f.getOrderProductFn != nil {
		return f.getOrderProductFn(ctx, orderID, productID)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderStatuses(ctx context.Context) ([]bigcommerce.OrderStatus, error) {
	if f.listOrderStatusesFn != nil {
		return f.listOrderStatusesFn(ctx)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderCoupons(ctx context.Context, orderID int, params bigcommerce.OrderCouponListParams) ([]json.RawMessage, error) {
	if f.listOrderCouponsFn != nil {
		return f.listOrderCouponsFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderShippingAddresses(ctx context.Context, orderID int, params bigcommerce.OrderShippingAddressListParams) ([]json.RawMessage, error) {
	if f.listOrderAddressesFn != nil {
		return f.listOrderAddressesFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) GetOrderShippingAddress(ctx context.Context, orderID, shippingAddressID int) (json.RawMessage, error) {
	if f.getOrderAddressFn != nil {
		return f.getOrderAddressFn(ctx, orderID, shippingAddressID)
	}
	return nil, nil
}

func (f *fakeOrdersBC) UpdateOrderShippingAddress(ctx context.Context, orderID, shippingAddressID int, payload json.RawMessage) (json.RawMessage, error) {
	if f.updateOrderAddressFn != nil {
		return f.updateOrderAddressFn(ctx, orderID, shippingAddressID, payload)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderMessages(ctx context.Context, orderID int, params bigcommerce.OrderMessageListParams) ([]json.RawMessage, error) {
	if f.listOrderMessagesFn != nil {
		return f.listOrderMessagesFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderTaxes(ctx context.Context, orderID int, params bigcommerce.OrderTaxListParams) ([]json.RawMessage, error) {
	if f.listOrderTaxesFn != nil {
		return f.listOrderTaxesFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderMetafields(ctx context.Context, orderID int, params bigcommerce.OrderMetafieldListParams) ([]bigcommerce.Metafield, error) {
	if f.listOrderMetafieldsFn != nil {
		return f.listOrderMetafieldsFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) CreateOrderMetafield(ctx context.Context, orderID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
	if f.createOrderMetafieldFn != nil {
		return f.createOrderMetafieldFn(ctx, orderID, mf)
	}
	return &bigcommerce.Metafield{}, nil
}

func (f *fakeOrdersBC) UpdateOrderMetafield(ctx context.Context, orderID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
	if f.updateOrderMetafieldFn != nil {
		return f.updateOrderMetafieldFn(ctx, orderID, metafieldID, mf)
	}
	return &bigcommerce.Metafield{}, nil
}

func (f *fakeOrdersBC) DeleteOrderMetafield(ctx context.Context, orderID, metafieldID int) error {
	if f.deleteOrderMetafieldFn != nil {
		return f.deleteOrderMetafieldFn(ctx, orderID, metafieldID)
	}
	return nil
}

func (f *fakeOrdersBC) ListOrderShipments(ctx context.Context, orderID int, params bigcommerce.OrderShipmentListParams) ([]bigcommerce.OrderShipment, error) {
	if f.listOrderShipmentsFn != nil {
		return f.listOrderShipmentsFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) GetOrderShipment(ctx context.Context, orderID, shipmentID int) (*bigcommerce.OrderShipment, error) {
	if f.getShipmentFn != nil {
		return f.getShipmentFn(ctx, orderID, shipmentID)
	}
	return &bigcommerce.OrderShipment{}, nil
}

func (f *fakeOrdersBC) UpdateOrderShipment(ctx context.Context, orderID, shipmentID int, payload json.RawMessage) (*bigcommerce.OrderShipment, error) {
	if f.updateShipmentFn != nil {
		return f.updateShipmentFn(ctx, orderID, shipmentID, payload)
	}
	return &bigcommerce.OrderShipment{}, nil
}

func (f *fakeOrdersBC) DeleteOrderShipment(ctx context.Context, orderID, shipmentID int) error {
	if f.deleteShipmentFn != nil {
		return f.deleteShipmentFn(ctx, orderID, shipmentID)
	}
	return nil
}

func (f *fakeOrdersBC) CreateOrderShipment(ctx context.Context, orderID int, payload bigcommerce.OrderShipmentCreate) (*bigcommerce.OrderShipment, error) {
	if f.createShipmentFn != nil {
		return f.createShipmentFn(ctx, orderID, payload)
	}
	return &bigcommerce.OrderShipment{}, nil
}

func (f *fakeOrdersBC) ListOrderPaymentActions(ctx context.Context, orderID int, params bigcommerce.OrderPaymentActionListParams) ([]json.RawMessage, error) {
	if f.listPaymentActionsFn != nil {
		return f.listPaymentActionsFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderTransactions(ctx context.Context, orderID int, params bigcommerce.OrderTransactionListParams) ([]json.RawMessage, error) {
	if f.listTransactionsFn != nil {
		return f.listTransactionsFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderRefunds(ctx context.Context, orderID int, params bigcommerce.OrderRefundListParams) ([]json.RawMessage, error) {
	if f.listOrderRefundsFn != nil {
		return f.listOrderRefundsFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) ListOrderLegacyRefunds(ctx context.Context, orderID int, params bigcommerce.OrderLegacyRefundListParams) ([]json.RawMessage, error) {
	if f.listLegacyRefundsFn != nil {
		return f.listLegacyRefundsFn(ctx, orderID, params)
	}
	return nil, nil
}

func (f *fakeOrdersBC) CreateOrderPaymentCapture(ctx context.Context, orderID int) (json.RawMessage, error) {
	if f.captureFn != nil {
		return f.captureFn(ctx, orderID)
	}
	return nil, nil
}

func (f *fakeOrdersBC) CreateOrderPaymentVoid(ctx context.Context, orderID int) (json.RawMessage, error) {
	if f.voidFn != nil {
		return f.voidFn(ctx, orderID)
	}
	return nil, nil
}

func (f *fakeOrdersBC) CreateOrderRefundQuote(ctx context.Context, orderID int, payload json.RawMessage) (json.RawMessage, error) {
	if f.refundQuoteFn != nil {
		return f.refundQuoteFn(ctx, orderID, payload)
	}
	return nil, nil
}

func (f *fakeOrdersBC) CreateOrderRefund(ctx context.Context, orderID int, payload json.RawMessage, transactionID string) (json.RawMessage, error) {
	if f.refundCreateFn != nil {
		return f.refundCreateFn(ctx, orderID, payload, transactionID)
	}
	return nil, nil
}

type OrdersToolsSuite struct {
	suite.Suite
	mock *fakeOrdersBC
	reg  *discovery.Registry
}

func TestOrdersToolsSuite(t *testing.T) {
	suite.Run(t, new(OrdersToolsSuite))
}

func (s *OrdersToolsSuite) SetupTest() {
	s.mock = &fakeOrdersBC{}
	s.reg = discovery.NewRegistry()
	s.reg.RegisterCategory("orders", "Orders")
	s.reg.RegisterCategory("orders/management", "Management")
	s.reg.RegisterCategory("orders/management/products", "Products")
	s.reg.RegisterCategory("orders/management/metafields", "Metafields")
	s.reg.RegisterCategory("orders/management/coupons", "Coupons")
	s.reg.RegisterCategory("orders/management/shipping_addresses", "Shipping addresses")
	s.reg.RegisterCategory("orders/management/messages", "Messages")
	s.reg.RegisterCategory("orders/management/taxes", "Taxes")
	s.reg.RegisterCategory("orders/fulfillment", "Fulfillment")
	s.reg.RegisterCategory("orders/fulfillment/shipments", "Shipments")
	s.reg.RegisterCategory("orders/payments", "Payments")
	s.reg.RegisterCategory("orders/payments/actions", "Payment Actions")
	s.reg.RegisterCategory("orders/payments/transactions", "Transactions")
	s.reg.RegisterCategory("orders/refunds", "Refunds")
	orders.NewManagement(s.mock, session.NewStore(60*time.Second)).RegisterTools(s.reg)
	orders.NewOrderMetafields(s.mock).RegisterTools(s.reg)
	orders.NewSubresources(s.mock).RegisterTools(s.reg)
	orders.NewFulfillment(s.mock).RegisterTools(s.reg)
	orders.NewPayments(s.mock).RegisterTools(s.reg)
}

func (s *OrdersToolsSuite) callTool(path string, args map[string]any) (*mcp.CallToolResult, error) {
	def := s.reg.GetTool(path)
	s.Require().NotNil(def)
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: path, Arguments: args},
	}
	return def.Handler(context.Background(), req)
}

func (s *OrdersToolsSuite) parseJSON(result *mcp.CallToolResult) map[string]any {
	s.Require().NotNil(result)
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data map[string]any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

func (s *OrdersToolsSuite) TestListOrdersRequiresFilterOrListAll() {
	res, err := s.callTool("orders/management/list", map[string]any{})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(res.Content[0].(mcp.TextContent).Text, "provide at least one filter or set list_all=true")
}

func (s *OrdersToolsSuite) TestListOrdersPassesFilters() {
	s.mock.listOrdersFn = func(_ context.Context, params bigcommerce.OrderListParams) ([]bigcommerce.Order, error) {
		s.Equal(7, params.StatusID)
		s.Equal(2, params.Page)
		s.Equal(25, params.Limit)
		s.Equal("date_created:desc", params.Sort)
		s.Equal([]string{"fees"}, params.Include)
		return []bigcommerce.Order{{ID: 41, StatusID: 7}}, nil
	}

	res, err := s.callTool("orders/management/list", map[string]any{
		"status_id": float64(7),
		"page":      float64(2),
		"limit":     float64(25),
		"sort":      "date_created:desc",
		"include":   []any{"fees"},
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *OrdersToolsSuite) TestGetOrderReturnsOrderAndProducts() {
	s.mock.getOrderFn = func(_ context.Context, orderID int, _ bigcommerce.OrderGetParams) (*bigcommerce.Order, error) {
		s.Equal(99, orderID)
		return &bigcommerce.Order{ID: 99, StatusID: 1}, nil
	}
	s.mock.listOrderProductsFn = func(_ context.Context, orderID int, _ bigcommerce.OrderProductListParams) ([]bigcommerce.OrderProduct, error) {
		s.Equal(99, orderID)
		return []bigcommerce.OrderProduct{{ID: 1001, OrderID: 99}}, nil
	}

	res, err := s.callTool("orders/management/get", map[string]any{"order_id": float64(99)})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["product_count"])
}

func (s *OrdersToolsSuite) TestUpdateStatusPreview() {
	s.mock.getOrderFn = func(_ context.Context, orderID int, _ bigcommerce.OrderGetParams) (*bigcommerce.Order, error) {
		s.Equal(55, orderID)
		return &bigcommerce.Order{ID: 55, StatusID: 1, Status: "Pending"}, nil
	}

	res, err := s.callTool("orders/management/update_status", map[string]any{
		"order_id":  float64(55),
		"status_id": float64(10),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("preview", data["status"])
}

func (s *OrdersToolsSuite) TestUpdateStatusConfirmed() {
	s.mock.getOrderFn = func(_ context.Context, _ int, _ bigcommerce.OrderGetParams) (*bigcommerce.Order, error) {
		return &bigcommerce.Order{ID: 55, StatusID: 1}, nil
	}
	s.mock.updateOrderStatusFn = func(_ context.Context, orderID, statusID int) (*bigcommerce.Order, error) {
		s.Equal(55, orderID)
		s.Equal(10, statusID)
		return &bigcommerce.Order{ID: 55, StatusID: 10}, nil
	}

	res, err := s.callTool("orders/management/update_status", map[string]any{
		"order_id":  float64(55),
		"status_id": float64(10),
		"confirmed": true,
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal("updated", data["status"])
}

func (s *OrdersToolsSuite) TestCreateOrderPreviewAndConfirmed() {
	createCalls := 0
	s.mock.createOrderFn = func(_ context.Context, payload json.RawMessage) (*bigcommerce.Order, error) {
		createCalls++
		s.Contains(string(payload), `"billing_address"`)
		return &bigcommerce.Order{ID: 1201, StatusID: 0}, nil
	}

	preview, err := s.callTool("orders/management/create", map[string]any{
		"order": map[string]any{
			"billing_address": map[string]any{"first_name": "Ada"},
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, createCalls)

	confirmed, err := s.callTool("orders/management/create", map[string]any{
		"order": map[string]any{
			"billing_address": map[string]any{"first_name": "Ada"},
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, createCalls)
}

func (s *OrdersToolsSuite) TestDeleteOrderPreviewAndConfirmed() {
	deleteCalls := 0
	s.mock.getOrderFn = func(_ context.Context, orderID int, _ bigcommerce.OrderGetParams) (*bigcommerce.Order, error) {
		s.Equal(1201, orderID)
		return &bigcommerce.Order{ID: 1201, StatusID: 0}, nil
	}
	s.mock.deleteOrderFn = func(_ context.Context, orderID int) error {
		deleteCalls++
		s.Equal(1201, orderID)
		return nil
	}

	preview, err := s.callTool("orders/management/delete", map[string]any{
		"order_id": float64(1201),
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, deleteCalls)

	confirmed, err := s.callTool("orders/management/delete", map[string]any{
		"order_id":  float64(1201),
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, deleteCalls)
}

func (s *OrdersToolsSuite) TestCreateShipmentPreviewAndConfirmed() {
	createCalls := 0
	s.mock.createShipmentFn = func(_ context.Context, orderID int, payload bigcommerce.OrderShipmentCreate) (*bigcommerce.OrderShipment, error) {
		createCalls++
		s.Equal(81, orderID)
		s.Equal(3, payload.OrderAddressID)
		s.Len(payload.Items, 1)
		return &bigcommerce.OrderShipment{ID: 5001, OrderID: orderID}, nil
	}

	preview, err := s.callTool("orders/fulfillment/shipments/create", map[string]any{
		"order_id":         float64(81),
		"order_address_id": float64(3),
		"items": []any{
			map[string]any{"order_product_id": float64(44), "quantity": float64(1)},
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, createCalls)
	previewData := s.parseJSON(preview)
	s.Equal("preview", previewData["status"])

	confirmed, err := s.callTool("orders/fulfillment/shipments/create", map[string]any{
		"order_id":         float64(81),
		"order_address_id": float64(3),
		"items": []any{
			map[string]any{"order_product_id": float64(44), "quantity": float64(1)},
		},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, createCalls)
	confirmedData := s.parseJSON(confirmed)
	s.Equal("created", confirmedData["status"])
}

func (s *OrdersToolsSuite) TestCreateShipmentRejectsFractionalQuantity() {
	res, err := s.callTool("orders/fulfillment/shipments/create", map[string]any{
		"order_id":         float64(81),
		"order_address_id": float64(3),
		"items": []any{
			map[string]any{"order_product_id": float64(44), "quantity": float64(1.2)},
		},
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(res.Content[0].(mcp.TextContent).Text, "quantity must be a positive integer")
}

func (s *OrdersToolsSuite) TestListCouponsPassesPaging() {
	s.mock.listOrderCouponsFn = func(_ context.Context, orderID int, params bigcommerce.OrderCouponListParams) ([]json.RawMessage, error) {
		s.Equal(101, orderID)
		s.Equal(2, params.Page)
		s.Equal(25, params.Limit)
		return []json.RawMessage{json.RawMessage(`{"id":1}`)}, nil
	}

	res, err := s.callTool("orders/management/coupons/list", map[string]any{
		"order_id": float64(101),
		"page":     float64(2),
		"limit":    float64(25),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *OrdersToolsSuite) TestListMessagesValidatesStatus() {
	res, err := s.callTool("orders/management/messages/list", map[string]any{
		"order_id": float64(102),
		"status":   "bad-status",
	})
	s.NoError(err)
	s.True(res.IsError)
	s.Contains(res.Content[0].(mcp.TextContent).Text, "status must be read or unread")
}

func (s *OrdersToolsSuite) TestListPaymentActions() {
	s.mock.listPaymentActionsFn = func(_ context.Context, orderID int, params bigcommerce.OrderPaymentActionListParams) ([]json.RawMessage, error) {
		s.Equal(90, orderID)
		s.Equal(2, params.Page)
		s.Equal(25, params.Limit)
		return []json.RawMessage{json.RawMessage(`{"id":"pa_1"}`)}, nil
	}

	res, err := s.callTool("orders/payments/actions/list", map[string]any{
		"order_id": float64(90),
		"page":     float64(2),
		"limit":    float64(25),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *OrdersToolsSuite) TestListTransactions() {
	s.mock.listTransactionsFn = func(_ context.Context, orderID int, params bigcommerce.OrderTransactionListParams) ([]json.RawMessage, error) {
		s.Equal(94, orderID)
		s.Equal(2, params.Page)
		s.Equal(15, params.Limit)
		return []json.RawMessage{json.RawMessage(`{"id":"txn_1"}`)}, nil
	}

	res, err := s.callTool("orders/payments/transactions/list", map[string]any{
		"order_id": float64(94),
		"page":     float64(2),
		"limit":    float64(15),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *OrdersToolsSuite) TestListLegacyRefunds() {
	s.mock.listLegacyRefundsFn = func(_ context.Context, orderID int, params bigcommerce.OrderLegacyRefundListParams) ([]json.RawMessage, error) {
		s.Equal(95, orderID)
		s.Equal(1, params.Page)
		s.Equal(20, params.Limit)
		return []json.RawMessage{json.RawMessage(`{"id":77}`)}, nil
	}

	res, err := s.callTool("orders/refunds/legacy_list", map[string]any{
		"order_id": float64(95),
		"page":     float64(1),
		"limit":    float64(20),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *OrdersToolsSuite) TestCapturePreviewAndConfirmed() {
	captureCalls := 0
	s.mock.getOrderFn = func(_ context.Context, orderID int, _ bigcommerce.OrderGetParams) (*bigcommerce.Order, error) {
		s.Equal(91, orderID)
		return &bigcommerce.Order{ID: 91, StatusID: 1, Status: "Pending"}, nil
	}
	s.mock.captureFn = func(_ context.Context, orderID int) (json.RawMessage, error) {
		captureCalls++
		s.Equal(91, orderID)
		return json.RawMessage(`{"id":"cap_1"}`), nil
	}

	preview, err := s.callTool("orders/payments/capture", map[string]any{
		"order_id": float64(91),
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, captureCalls)
	previewData := s.parseJSON(preview)
	s.Equal("preview", previewData["status"])

	confirmed, err := s.callTool("orders/payments/capture", map[string]any{
		"order_id":  float64(91),
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, captureCalls)
	confirmedData := s.parseJSON(confirmed)
	s.Equal("completed", confirmedData["status"])
}

func (s *OrdersToolsSuite) TestCreateRefundQuotePreviewAndConfirmed() {
	quoteCalls := 0
	s.mock.refundQuoteFn = func(_ context.Context, orderID int, payload json.RawMessage) (json.RawMessage, error) {
		quoteCalls++
		s.Equal(92, orderID)
		s.Contains(string(payload), `"items"`)
		return json.RawMessage(`{"id":"rq_1"}`), nil
	}

	preview, err := s.callTool("orders/refunds/quote", map[string]any{
		"order_id": float64(92),
		"quote": map[string]any{
			"items": []any{map[string]any{"item_type": "PRODUCT", "item_id": float64(11), "quantity": float64(1)}},
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, quoteCalls)

	confirmed, err := s.callTool("orders/refunds/quote", map[string]any{
		"order_id":  float64(92),
		"confirmed": true,
		"quote": map[string]any{
			"items": []any{map[string]any{"item_type": "PRODUCT", "item_id": float64(11), "quantity": float64(1)}},
		},
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, quoteCalls)
}

func (s *OrdersToolsSuite) TestCreateRefundPreviewAndConfirmed() {
	refundCalls := 0
	s.mock.getOrderFn = func(_ context.Context, orderID int, _ bigcommerce.OrderGetParams) (*bigcommerce.Order, error) {
		s.Equal(93, orderID)
		return &bigcommerce.Order{ID: 93, StatusID: 2, Status: "Shipped"}, nil
	}
	s.mock.refundCreateFn = func(_ context.Context, orderID int, payload json.RawMessage, transactionID string) (json.RawMessage, error) {
		refundCalls++
		s.Equal(93, orderID)
		s.Equal("txn_9", transactionID)
		s.Contains(string(payload), `"payments"`)
		return json.RawMessage(`{"id":"ref_1"}`), nil
	}

	preview, err := s.callTool("orders/refunds/create", map[string]any{
		"order_id":       float64(93),
		"transaction_id": "txn_9",
		"refund": map[string]any{
			"items":    []any{map[string]any{"item_type": "PRODUCT", "item_id": float64(11), "quantity": float64(1)}},
			"payments": []any{map[string]any{"provider_id": "bc", "amount": float64(10)}},
		},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, refundCalls)
	previewData := s.parseJSON(preview)
	s.Equal("preview", previewData["status"])

	confirmed, err := s.callTool("orders/refunds/create", map[string]any{
		"order_id":       float64(93),
		"transaction_id": "txn_9",
		"confirmed":      true,
		"refund": map[string]any{
			"items":    []any{map[string]any{"item_type": "PRODUCT", "item_id": float64(11), "quantity": float64(1)}},
			"payments": []any{map[string]any{"provider_id": "bc", "amount": float64(10)}},
		},
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, refundCalls)
	confirmedData := s.parseJSON(confirmed)
	s.Equal("completed", confirmedData["status"])
}

func (s *OrdersToolsSuite) TestUpdateOrderPreviewAndConfirmed() {
	updateCalls := 0
	s.mock.getOrderFn = func(_ context.Context, orderID int, _ bigcommerce.OrderGetParams) (*bigcommerce.Order, error) {
		s.Equal(104, orderID)
		return &bigcommerce.Order{ID: 104, StatusID: 1, Status: "Pending"}, nil
	}
	s.mock.updateOrderFn = func(_ context.Context, orderID int, payload json.RawMessage) (*bigcommerce.Order, error) {
		updateCalls++
		s.Equal(104, orderID)
		s.Contains(string(payload), `"staff_notes":"packed"`)
		return &bigcommerce.Order{ID: 104, StatusID: 1, Status: "Pending"}, nil
	}

	preview, err := s.callTool("orders/management/update", map[string]any{
		"order_id": float64(104),
		"patch":    map[string]any{"staff_notes": "packed"},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, updateCalls)

	confirmed, err := s.callTool("orders/management/update", map[string]any{
		"order_id":  float64(104),
		"patch":     map[string]any{"staff_notes": "packed"},
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, updateCalls)
}

func (s *OrdersToolsSuite) TestGetOrderProduct() {
	s.mock.getOrderProductFn = func(_ context.Context, orderID, productID int) (json.RawMessage, error) {
		s.Equal(105, orderID)
		s.Equal(77, productID)
		return json.RawMessage(`{"id":77}`), nil
	}

	res, err := s.callTool("orders/management/products/get", map[string]any{
		"order_id":   float64(105),
		"product_id": float64(77),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(105), data["order_id"])
}

func (s *OrdersToolsSuite) TestGetShippingAddress() {
	s.mock.getOrderAddressFn = func(_ context.Context, orderID, shippingAddressID int) (json.RawMessage, error) {
		s.Equal(106, orderID)
		s.Equal(12, shippingAddressID)
		return json.RawMessage(`{"id":12}`), nil
	}

	res, err := s.callTool("orders/management/shipping_addresses/get", map[string]any{
		"order_id":            float64(106),
		"shipping_address_id": float64(12),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(12), data["shipping_address_id"])
}

func (s *OrdersToolsSuite) TestUpdateShippingAddressPreviewAndConfirmed() {
	updateCalls := 0
	s.mock.getOrderAddressFn = func(_ context.Context, orderID, shippingAddressID int) (json.RawMessage, error) {
		s.Equal(107, orderID)
		s.Equal(15, shippingAddressID)
		return json.RawMessage(`{"id":15,"city":"Austin"}`), nil
	}
	s.mock.updateOrderAddressFn = func(_ context.Context, orderID, shippingAddressID int, payload json.RawMessage) (json.RawMessage, error) {
		updateCalls++
		s.Equal(107, orderID)
		s.Equal(15, shippingAddressID)
		s.Contains(string(payload), `"city":"Dallas"`)
		return json.RawMessage(`{"id":15,"city":"Dallas"}`), nil
	}

	preview, err := s.callTool("orders/management/shipping_addresses/update", map[string]any{
		"order_id":            float64(107),
		"shipping_address_id": float64(15),
		"patch":               map[string]any{"city": "Dallas"},
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, updateCalls)

	confirmed, err := s.callTool("orders/management/shipping_addresses/update", map[string]any{
		"order_id":            float64(107),
		"shipping_address_id": float64(15),
		"patch":               map[string]any{"city": "Dallas"},
		"confirmed":           true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, updateCalls)
}

func (s *OrdersToolsSuite) TestShipmentUpdateAndDeletePreviewFlow() {
	updateCalls := 0
	deleteCalls := 0
	s.mock.getShipmentFn = func(_ context.Context, orderID, shipmentID int) (*bigcommerce.OrderShipment, error) {
		s.Equal(108, orderID)
		s.Equal(3, shipmentID)
		return &bigcommerce.OrderShipment{ID: 3, OrderID: 108, TrackingNumber: "old"}, nil
	}
	s.mock.updateShipmentFn = func(_ context.Context, orderID, shipmentID int, payload json.RawMessage) (*bigcommerce.OrderShipment, error) {
		updateCalls++
		s.Equal(108, orderID)
		s.Equal(3, shipmentID)
		s.Contains(string(payload), `"tracking_number":"new"`)
		return &bigcommerce.OrderShipment{ID: 3, OrderID: 108, TrackingNumber: "new"}, nil
	}
	s.mock.deleteShipmentFn = func(_ context.Context, orderID, shipmentID int) error {
		deleteCalls++
		s.Equal(108, orderID)
		s.Equal(3, shipmentID)
		return nil
	}

	updatePreview, err := s.callTool("orders/fulfillment/shipments/update", map[string]any{
		"order_id":    float64(108),
		"shipment_id": float64(3),
		"patch":       map[string]any{"tracking_number": "new"},
	})
	s.NoError(err)
	s.False(updatePreview.IsError)
	s.Equal(0, updateCalls)

	updateConfirmed, err := s.callTool("orders/fulfillment/shipments/update", map[string]any{
		"order_id":    float64(108),
		"shipment_id": float64(3),
		"patch":       map[string]any{"tracking_number": "new"},
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(updateConfirmed.IsError)
	s.Equal(1, updateCalls)

	deletePreview, err := s.callTool("orders/fulfillment/shipments/delete", map[string]any{
		"order_id":    float64(108),
		"shipment_id": float64(3),
	})
	s.NoError(err)
	s.False(deletePreview.IsError)
	s.Equal(0, deleteCalls)

	deleteConfirmed, err := s.callTool("orders/fulfillment/shipments/delete", map[string]any{
		"order_id":    float64(108),
		"shipment_id": float64(3),
		"confirmed":   true,
	})
	s.NoError(err)
	s.False(deleteConfirmed.IsError)
	s.Equal(1, deleteCalls)
}

func (s *OrdersToolsSuite) TestListOrderMetafields() {
	s.mock.listOrderMetafieldsFn = func(_ context.Context, orderID int, params bigcommerce.OrderMetafieldListParams) ([]bigcommerce.Metafield, error) {
		s.Equal(109, orderID)
		s.Equal(2, params.Page)
		s.Equal(20, params.Limit)
		return []bigcommerce.Metafield{{ID: 5, Namespace: "ops", Key: "priority", Value: "high"}}, nil
	}

	res, err := s.callTool("orders/management/metafields/list", map[string]any{
		"order_id": float64(109),
		"page":     float64(2),
		"limit":    float64(20),
	})
	s.NoError(err)
	s.False(res.IsError)
	data := s.parseJSON(res)
	s.Equal(float64(1), data["total"])
}

func (s *OrdersToolsSuite) TestSetOrderMetafieldPreviewAndConfirmed() {
	createCalls := 0
	s.mock.listOrderMetafieldsFn = func(_ context.Context, orderID int, _ bigcommerce.OrderMetafieldListParams) ([]bigcommerce.Metafield, error) {
		s.Equal(110, orderID)
		return []bigcommerce.Metafield{}, nil
	}
	s.mock.createOrderMetafieldFn = func(_ context.Context, orderID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error) {
		createCalls++
		s.Equal(110, orderID)
		s.Equal("ops", mf.Namespace)
		s.Equal("flag", mf.Key)
		s.Equal("yes", mf.Value)
		s.Equal("app_only", mf.PermissionSet)
		return &bigcommerce.Metafield{ID: 7, Namespace: mf.Namespace, Key: mf.Key, Value: mf.Value}, nil
	}

	preview, err := s.callTool("orders/management/metafields/set", map[string]any{
		"order_id":  float64(110),
		"namespace": "ops",
		"key":       "flag",
		"value":     "yes",
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, createCalls)

	confirmed, err := s.callTool("orders/management/metafields/set", map[string]any{
		"order_id":  float64(110),
		"namespace": "ops",
		"key":       "flag",
		"value":     "yes",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, createCalls)
}

func (s *OrdersToolsSuite) TestDeleteOrderMetafieldByNamespaceKeyPreviewAndConfirmed() {
	deleteCalls := 0
	s.mock.listOrderMetafieldsFn = func(_ context.Context, orderID int, _ bigcommerce.OrderMetafieldListParams) ([]bigcommerce.Metafield, error) {
		s.Equal(111, orderID)
		return []bigcommerce.Metafield{{ID: 12, Namespace: "ops", Key: "flag", Value: "yes"}}, nil
	}
	s.mock.deleteOrderMetafieldFn = func(_ context.Context, orderID, metafieldID int) error {
		deleteCalls++
		s.Equal(111, orderID)
		s.Equal(12, metafieldID)
		return nil
	}

	preview, err := s.callTool("orders/management/metafields/delete", map[string]any{
		"order_id":  float64(111),
		"namespace": "ops",
		"key":       "flag",
	})
	s.NoError(err)
	s.False(preview.IsError)
	s.Equal(0, deleteCalls)

	confirmed, err := s.callTool("orders/management/metafields/delete", map[string]any{
		"order_id":  float64(111),
		"namespace": "ops",
		"key":       "flag",
		"confirmed": true,
	})
	s.NoError(err)
	s.False(confirmed.IsError)
	s.Equal(1, deleteCalls)
}
