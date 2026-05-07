package orders

import (
	"context"
	"encoding/json"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.Client satisfies BigCommerceOrdersAPI.
var _ BigCommerceOrdersAPI = (*bigcommerce.Client)(nil)

// BigCommerceOrdersAPI defines the BigCommerce client methods consumed by
// orders-domain tool handlers.
type BigCommerceOrdersAPI interface {
	ListOrders(ctx context.Context, params bigcommerce.OrderListParams) ([]bigcommerce.Order, error)
	CountOrders(ctx context.Context, params bigcommerce.OrderCountParams) (int, error)
	GetOrder(ctx context.Context, orderID int, params bigcommerce.OrderGetParams) (*bigcommerce.Order, error)
	CreateOrder(ctx context.Context, payload json.RawMessage) (*bigcommerce.Order, error)
	DeleteOrder(ctx context.Context, orderID int) error
	UpdateOrder(ctx context.Context, orderID int, payload json.RawMessage) (*bigcommerce.Order, error)
	UpdateOrderStatus(ctx context.Context, orderID, statusID int) (*bigcommerce.Order, error)
	ListOrderProducts(ctx context.Context, orderID int, params bigcommerce.OrderProductListParams) ([]bigcommerce.OrderProduct, error)
	GetOrderProduct(ctx context.Context, orderID, productID int) (json.RawMessage, error)
	ListOrderStatuses(ctx context.Context) ([]bigcommerce.OrderStatus, error)
	ListOrderCoupons(ctx context.Context, orderID int, params bigcommerce.OrderCouponListParams) ([]json.RawMessage, error)
	ListOrderShippingAddresses(ctx context.Context, orderID int, params bigcommerce.OrderShippingAddressListParams) ([]json.RawMessage, error)
	GetOrderShippingAddress(ctx context.Context, orderID, shippingAddressID int) (json.RawMessage, error)
	UpdateOrderShippingAddress(ctx context.Context, orderID, shippingAddressID int, payload json.RawMessage) (json.RawMessage, error)
	ListOrderMessages(ctx context.Context, orderID int, params bigcommerce.OrderMessageListParams) ([]json.RawMessage, error)
	ListOrderTaxes(ctx context.Context, orderID int, params bigcommerce.OrderTaxListParams) ([]json.RawMessage, error)
	ListOrderMetafields(ctx context.Context, orderID int, params bigcommerce.OrderMetafieldListParams) ([]bigcommerce.Metafield, error)
	CreateOrderMetafield(ctx context.Context, orderID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	UpdateOrderMetafield(ctx context.Context, orderID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	DeleteOrderMetafield(ctx context.Context, orderID, metafieldID int) error
	ListOrderShipments(ctx context.Context, orderID int, params bigcommerce.OrderShipmentListParams) ([]bigcommerce.OrderShipment, error)
	GetOrderShipment(ctx context.Context, orderID, shipmentID int) (*bigcommerce.OrderShipment, error)
	UpdateOrderShipment(ctx context.Context, orderID, shipmentID int, payload json.RawMessage) (*bigcommerce.OrderShipment, error)
	DeleteOrderShipment(ctx context.Context, orderID, shipmentID int) error
	CreateOrderShipment(ctx context.Context, orderID int, payload bigcommerce.OrderShipmentCreate) (*bigcommerce.OrderShipment, error)
	ListOrderPaymentActions(ctx context.Context, orderID int, params bigcommerce.OrderPaymentActionListParams) ([]json.RawMessage, error)
	ListOrderTransactions(ctx context.Context, orderID int, params bigcommerce.OrderTransactionListParams) ([]json.RawMessage, error)
	ListOrderRefunds(ctx context.Context, orderID int, params bigcommerce.OrderRefundListParams) ([]json.RawMessage, error)
	ListOrderLegacyRefunds(ctx context.Context, orderID int, params bigcommerce.OrderLegacyRefundListParams) ([]json.RawMessage, error)
	CreateOrderPaymentCapture(ctx context.Context, orderID int) (json.RawMessage, error)
	CreateOrderPaymentVoid(ctx context.Context, orderID int) (json.RawMessage, error)
	CreateOrderRefundQuote(ctx context.Context, orderID int, payload json.RawMessage) (json.RawMessage, error)
	CreateOrderRefund(ctx context.Context, orderID int, payload json.RawMessage, transactionID string) (json.RawMessage, error)
}
