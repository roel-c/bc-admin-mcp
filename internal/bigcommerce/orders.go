package bigcommerce

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const v2OrdersAutoPageLimit = 50

// ListOrders returns rows from GET /v2/orders.
// When page/limit are explicitly provided, it returns only that page.
// Otherwise it auto-paginates with conservative V2 defaults.
func (c *Client) ListOrders(ctx context.Context, params OrderListParams) ([]Order, error) {
	base := buildOrderListValues(params, false)

	if params.Page > 0 || params.Limit > 0 {
		path := "orders"
		q := buildOrderListValues(params, true).Encode()
		if q != "" {
			path += "?" + q
		}
		body, err := c.GetV2(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list orders (single page): %w", err)
		}
		rows, err := decodeV2Array[Order](body)
		if err != nil {
			return nil, fmt.Errorf("parse orders response: %w", err)
		}
		return rows, nil
	}

	limit := v2OrdersAutoPageLimit
	page := 1
	all := make([]Order, 0)
	for {
		vals := cloneURLValues(base)
		vals.Set("page", strconv.Itoa(page))
		vals.Set("limit", strconv.Itoa(limit))
		body, err := c.GetV2(ctx, "orders?"+vals.Encode())
		if err != nil {
			return nil, fmt.Errorf("list orders page %d: %w", page, err)
		}
		rows, err := decodeV2Array[Order](body)
		if err != nil {
			return nil, fmt.Errorf("parse orders page %d: %w", page, err)
		}
		all = append(all, rows...)

		if c.cfg.MaxTotalRecords > 0 && len(all) >= c.cfg.MaxTotalRecords {
			all = all[:c.cfg.MaxTotalRecords]
			break
		}
		if len(rows) < limit {
			break
		}
		page++
	}
	return all, nil
}

// CountOrders returns count from GET /v2/orders/count.
func (c *Client) CountOrders(ctx context.Context, params OrderCountParams) (int, error) {
	path := "orders/count"
	if q := buildOrderCountValues(params).Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.GetV2(ctx, path)
	if err != nil {
		return 0, fmt.Errorf("count orders: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return 0, nil
	}
	var resp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse order count response: %w", err)
	}
	return resp.Count, nil
}

// GetOrder fetches one order from GET /v2/orders/{id}.
func (c *Client) GetOrder(ctx context.Context, orderID int, params OrderGetParams) (*Order, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d", orderID)
	vals := url.Values{}
	if len(params.Include) > 0 {
		vals.Set("include", strings.Join(params.Include, ","))
	}
	if params.ConsignmentStruct != "" {
		vals.Set("consignment_structure", params.ConsignmentStruct)
	}
	if q := vals.Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.GetV2(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("get order %d: %w", orderID, err)
	}
	var out Order
	if err := decodeV2Object(body, &out); err != nil {
		return nil, fmt.Errorf("parse order %d response: %w", orderID, err)
	}
	return &out, nil
}

// UpdateOrder updates an order via PUT /v2/orders/{id} using a caller-supplied
// partial payload object.
func (c *Client) UpdateOrder(ctx context.Context, orderID int, payload json.RawMessage) (*Order, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("order update payload is required")
	}
	body, err := c.PutV2(ctx, fmt.Sprintf("orders/%d", orderID), json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("update order %d: %w", orderID, err)
	}
	var out Order
	if err := decodeV2Object(body, &out); err != nil {
		return nil, fmt.Errorf("parse update order %d response: %w", orderID, err)
	}
	return &out, nil
}

// CreateOrder creates one order via POST /v2/orders using a caller-supplied
// payload object.
func (c *Client) CreateOrder(ctx context.Context, payload json.RawMessage) (*Order, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("order create payload is required")
	}
	body, err := c.PostV2(ctx, "orders", json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}
	var out Order
	if err := decodeV2Object(body, &out); err != nil {
		return nil, fmt.Errorf("parse create order response: %w", err)
	}
	return &out, nil
}

// DeleteOrder deletes one order via DELETE /v2/orders/{id}.
func (c *Client) DeleteOrder(ctx context.Context, orderID int) error {
	if orderID <= 0 {
		return fmt.Errorf("order id must be positive")
	}
	if _, err := c.DeleteV2(ctx, fmt.Sprintf("orders/%d", orderID)); err != nil {
		return fmt.Errorf("delete order %d: %w", orderID, err)
	}
	return nil
}

// UpdateOrderStatus updates only status_id via PUT /v2/orders/{id}.
func (c *Client) UpdateOrderStatus(ctx context.Context, orderID, statusID int) (*Order, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if statusID < 0 {
		return nil, fmt.Errorf("status id must be non-negative")
	}
	payload := map[string]any{"status_id": statusID}
	body, err := c.PutV2(ctx, fmt.Sprintf("orders/%d", orderID), payload)
	if err != nil {
		return nil, fmt.Errorf("update order %d status: %w", orderID, err)
	}
	var out Order
	if err := decodeV2Object(body, &out); err != nil {
		return nil, fmt.Errorf("parse update order %d response: %w", orderID, err)
	}
	return &out, nil
}

// ListOrderProducts returns order products from GET /v2/orders/{id}/products.
// When page/limit are provided, returns only that page; otherwise auto-paginates.
func (c *Client) ListOrderProducts(ctx context.Context, orderID int, params OrderProductListParams) ([]OrderProduct, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	basePath := fmt.Sprintf("orders/%d/products", orderID)
	if params.Page > 0 || params.Limit > 0 {
		vals := url.Values{}
		if params.Page > 0 {
			vals.Set("page", strconv.Itoa(params.Page))
		}
		if params.Limit > 0 {
			vals.Set("limit", strconv.Itoa(params.Limit))
		}
		path := basePath
		if q := vals.Encode(); q != "" {
			path += "?" + q
		}
		body, err := c.GetV2(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list order %d products (single page): %w", orderID, err)
		}
		rows, err := decodeV2Array[OrderProduct](body)
		if err != nil {
			return nil, fmt.Errorf("parse order %d products response: %w", orderID, err)
		}
		return rows, nil
	}

	all := make([]OrderProduct, 0)
	page := 1
	for {
		path := fmt.Sprintf("%s?page=%d&limit=%d", basePath, page, v2OrdersAutoPageLimit)
		body, err := c.GetV2(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list order %d products page %d: %w", orderID, page, err)
		}
		rows, err := decodeV2Array[OrderProduct](body)
		if err != nil {
			return nil, fmt.Errorf("parse order %d products page %d: %w", orderID, page, err)
		}
		all = append(all, rows...)
		if len(rows) < v2OrdersAutoPageLimit {
			break
		}
		page++
	}
	return all, nil
}

// GetOrderProduct fetches one order-product row from
// GET /v2/orders/{id}/products/{product_id}.
func (c *Client) GetOrderProduct(ctx context.Context, orderID, productID int) (json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if productID <= 0 {
		return nil, fmt.Errorf("product id must be positive")
	}
	body, err := c.GetV2(ctx, fmt.Sprintf("orders/%d/products/%d", orderID, productID))
	if err != nil {
		return nil, fmt.Errorf("get order %d product %d: %w", orderID, productID, err)
	}
	raw, err := decodeV2RawObject(body)
	if err != nil {
		return nil, fmt.Errorf("parse order-product response: %w", err)
	}
	return raw, nil
}

// GetOrderShippingAddress fetches one address row from
// GET /v2/orders/{id}/shipping_addresses/{shipping_address_id}.
func (c *Client) GetOrderShippingAddress(ctx context.Context, orderID, shippingAddressID int) (json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if shippingAddressID <= 0 {
		return nil, fmt.Errorf("shipping address id must be positive")
	}
	body, err := c.GetV2(ctx, fmt.Sprintf("orders/%d/shipping_addresses/%d", orderID, shippingAddressID))
	if err != nil {
		return nil, fmt.Errorf("get order %d shipping address %d: %w", orderID, shippingAddressID, err)
	}
	raw, err := decodeV2RawObject(body)
	if err != nil {
		return nil, fmt.Errorf("parse order shipping-address response: %w", err)
	}
	return raw, nil
}

// UpdateOrderShippingAddress updates one order shipping address via
// PUT /v2/orders/{id}/shipping_addresses/{shipping_address_id}.
func (c *Client) UpdateOrderShippingAddress(ctx context.Context, orderID, shippingAddressID int, payload json.RawMessage) (json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if shippingAddressID <= 0 {
		return nil, fmt.Errorf("shipping address id must be positive")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("shipping address update payload is required")
	}
	body, err := c.PutV2(ctx, fmt.Sprintf("orders/%d/shipping_addresses/%d", orderID, shippingAddressID), json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("update order %d shipping address %d: %w", orderID, shippingAddressID, err)
	}
	raw, err := decodeV2RawObject(body)
	if err != nil {
		return nil, fmt.Errorf("parse update shipping-address response: %w", err)
	}
	return raw, nil
}

// ListOrderShipments returns shipments from GET /v2/orders/{id}/shipments.
// When page/limit are provided, returns only that page; otherwise auto-paginates.
func (c *Client) ListOrderShipments(ctx context.Context, orderID int, params OrderShipmentListParams) ([]OrderShipment, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	basePath := fmt.Sprintf("orders/%d/shipments", orderID)
	if params.Page > 0 || params.Limit > 0 {
		vals := url.Values{}
		if params.Page > 0 {
			vals.Set("page", strconv.Itoa(params.Page))
		}
		if params.Limit > 0 {
			vals.Set("limit", strconv.Itoa(params.Limit))
		}
		path := basePath
		if q := vals.Encode(); q != "" {
			path += "?" + q
		}
		body, err := c.GetV2(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list order %d shipments (single page): %w", orderID, err)
		}
		rows, err := decodeV2Array[OrderShipment](body)
		if err != nil {
			return nil, fmt.Errorf("parse order %d shipments response: %w", orderID, err)
		}
		return rows, nil
	}

	all := make([]OrderShipment, 0)
	page := 1
	for {
		path := fmt.Sprintf("%s?page=%d&limit=%d", basePath, page, v2OrdersAutoPageLimit)
		body, err := c.GetV2(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list order %d shipments page %d: %w", orderID, page, err)
		}
		rows, err := decodeV2Array[OrderShipment](body)
		if err != nil {
			return nil, fmt.Errorf("parse order %d shipments page %d: %w", orderID, page, err)
		}
		all = append(all, rows...)
		if len(rows) < v2OrdersAutoPageLimit {
			break
		}
		page++
	}
	return all, nil
}

// GetOrderShipment fetches one shipment from
// GET /v2/orders/{id}/shipments/{shipment_id}.
func (c *Client) GetOrderShipment(ctx context.Context, orderID, shipmentID int) (*OrderShipment, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if shipmentID <= 0 {
		return nil, fmt.Errorf("shipment id must be positive")
	}
	body, err := c.GetV2(ctx, fmt.Sprintf("orders/%d/shipments/%d", orderID, shipmentID))
	if err != nil {
		return nil, fmt.Errorf("get order %d shipment %d: %w", orderID, shipmentID, err)
	}
	var out OrderShipment
	if err := decodeV2Object(body, &out); err != nil {
		return nil, fmt.Errorf("parse order shipment response: %w", err)
	}
	return &out, nil
}

// CreateOrderShipment creates one shipment via POST /v2/orders/{id}/shipments.
func (c *Client) CreateOrderShipment(ctx context.Context, orderID int, payload OrderShipmentCreate) (*OrderShipment, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if payload.OrderAddressID <= 0 {
		return nil, fmt.Errorf("order_address_id must be positive")
	}
	if len(payload.Items) == 0 {
		return nil, fmt.Errorf("shipment items are required")
	}
	body, err := c.PostV2(ctx, fmt.Sprintf("orders/%d/shipments", orderID), payload)
	if err != nil {
		return nil, fmt.Errorf("create shipment for order %d: %w", orderID, err)
	}
	var out OrderShipment
	if err := decodeV2Object(body, &out); err != nil {
		return nil, fmt.Errorf("parse create shipment response: %w", err)
	}
	return &out, nil
}

// UpdateOrderShipment updates one shipment via
// PUT /v2/orders/{id}/shipments/{shipment_id}.
func (c *Client) UpdateOrderShipment(ctx context.Context, orderID, shipmentID int, payload json.RawMessage) (*OrderShipment, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if shipmentID <= 0 {
		return nil, fmt.Errorf("shipment id must be positive")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("shipment update payload is required")
	}
	body, err := c.PutV2(ctx, fmt.Sprintf("orders/%d/shipments/%d", orderID, shipmentID), json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("update shipment %d on order %d: %w", shipmentID, orderID, err)
	}
	var out OrderShipment
	if err := decodeV2Object(body, &out); err != nil {
		return nil, fmt.Errorf("parse update shipment response: %w", err)
	}
	return &out, nil
}

// DeleteOrderShipment deletes one shipment via
// DELETE /v2/orders/{id}/shipments/{shipment_id}.
func (c *Client) DeleteOrderShipment(ctx context.Context, orderID, shipmentID int) error {
	if orderID <= 0 {
		return fmt.Errorf("order id must be positive")
	}
	if shipmentID <= 0 {
		return fmt.Errorf("shipment id must be positive")
	}
	if _, err := c.DeleteV2(ctx, fmt.Sprintf("orders/%d/shipments/%d", orderID, shipmentID)); err != nil {
		return fmt.Errorf("delete shipment %d on order %d: %w", shipmentID, orderID, err)
	}
	return nil
}

// ListOrderStatuses returns rows from GET /v2/order_statuses.
func (c *Client) ListOrderStatuses(ctx context.Context) ([]OrderStatus, error) {
	body, err := c.GetV2(ctx, "order_statuses")
	if err != nil {
		return nil, fmt.Errorf("list order statuses: %w", err)
	}
	rows, err := decodeV2Array[OrderStatus](body)
	if err != nil {
		return nil, fmt.Errorf("parse order statuses response: %w", err)
	}
	return rows, nil
}

// ListOrderCoupons returns rows from GET /v2/orders/{id}/coupons.
func (c *Client) ListOrderCoupons(ctx context.Context, orderID int, params OrderCouponListParams) ([]json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/coupons", orderID)
	if q := buildV2PagingValues(params.Page, params.Limit).Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.GetV2(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list coupons for order %d: %w", orderID, err)
	}
	rows, err := decodeV2Array[json.RawMessage](body)
	if err != nil {
		return nil, fmt.Errorf("parse order coupons response: %w", err)
	}
	return rows, nil
}

// ListOrderShippingAddresses returns rows from GET /v2/orders/{id}/shipping_addresses.
func (c *Client) ListOrderShippingAddresses(ctx context.Context, orderID int, params OrderShippingAddressListParams) ([]json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/shipping_addresses", orderID)
	if q := buildV2PagingValues(params.Page, params.Limit).Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.GetV2(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list shipping addresses for order %d: %w", orderID, err)
	}
	rows, err := decodeV2Array[json.RawMessage](body)
	if err != nil {
		return nil, fmt.Errorf("parse order shipping addresses response: %w", err)
	}
	return rows, nil
}

// ListOrderMessages returns rows from GET /v2/orders/{id}/messages.
func (c *Client) ListOrderMessages(ctx context.Context, orderID int, params OrderMessageListParams) ([]json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/messages", orderID)
	if q := buildOrderMessageListValues(params).Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.GetV2(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list messages for order %d: %w", orderID, err)
	}
	rows, err := decodeV2Array[json.RawMessage](body)
	if err != nil {
		return nil, fmt.Errorf("parse order messages response: %w", err)
	}
	return rows, nil
}

// ListOrderTaxes returns rows from GET /v2/orders/{id}/taxes.
func (c *Client) ListOrderTaxes(ctx context.Context, orderID int, params OrderTaxListParams) ([]json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/taxes", orderID)
	if q := buildV2PagingValues(params.Page, params.Limit).Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.GetV2(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list taxes for order %d: %w", orderID, err)
	}
	rows, err := decodeV2Array[json.RawMessage](body)
	if err != nil {
		return nil, fmt.Errorf("parse order taxes response: %w", err)
	}
	return rows, nil
}

// ListOrderMetafields returns order metafields from
// GET /v3/orders/{id}/metafields.
// If page/limit is omitted, it auto-paginates all rows using GetAll.
func (c *Client) ListOrderMetafields(ctx context.Context, orderID int, params OrderMetafieldListParams) ([]Metafield, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/metafields", orderID)
	if params.Page > 0 || params.Limit > 0 {
		vals := url.Values{}
		if params.Page > 0 {
			vals.Set("page", strconv.Itoa(params.Page))
		}
		if params.Limit > 0 {
			vals.Set("limit", strconv.Itoa(params.Limit))
		}
		if q := vals.Encode(); q != "" {
			path += "?" + q
		}
		body, err := c.Get(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list metafields for order %d: %w", orderID, err)
		}
		var env metafieldsDataEnvelope
		if err := json.Unmarshal(body, &env); err != nil {
			return nil, fmt.Errorf("parse order metafields response: %w", err)
		}
		return env.Data, nil
	}

	raw, err := c.GetAll(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list metafields for order %d: %w", orderID, err)
	}
	out := make([]Metafield, 0, len(raw))
	for _, r := range raw {
		var mf Metafield
		if err := json.Unmarshal(r, &mf); err != nil {
			return nil, fmt.Errorf("unmarshal order metafield: %w", err)
		}
		out = append(out, mf)
	}
	return out, nil
}

// CreateOrderMetafield creates a metafield on an order via
// POST /v3/orders/{id}/metafields.
func (c *Client) CreateOrderMetafield(ctx context.Context, orderID int, mf Metafield) (*Metafield, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/metafields", orderID)
	body, err := c.Post(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("create metafield on order %d: %w", orderID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse metafield response: %w", err)
	}
	var created Metafield
	if err := json.Unmarshal(resp.Data, &created); err != nil {
		return nil, fmt.Errorf("unmarshal created order metafield: %w", err)
	}
	return &created, nil
}

// UpdateOrderMetafield updates one order metafield via
// PUT /v3/orders/{id}/metafields/{metafield_id}.
func (c *Client) UpdateOrderMetafield(ctx context.Context, orderID, metafieldID int, mf Metafield) (*Metafield, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if metafieldID <= 0 {
		return nil, fmt.Errorf("metafield id must be positive")
	}
	path := fmt.Sprintf("orders/%d/metafields/%d", orderID, metafieldID)
	body, err := c.Put(ctx, path, mf)
	if err != nil {
		return nil, fmt.Errorf("update metafield %d on order %d: %w", metafieldID, orderID, err)
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse metafield response: %w", err)
	}
	var updated Metafield
	if err := json.Unmarshal(resp.Data, &updated); err != nil {
		return nil, fmt.Errorf("unmarshal updated order metafield: %w", err)
	}
	return &updated, nil
}

// DeleteOrderMetafield deletes one order metafield via
// DELETE /v3/orders/{id}/metafields/{metafield_id}.
func (c *Client) DeleteOrderMetafield(ctx context.Context, orderID, metafieldID int) error {
	if orderID <= 0 {
		return fmt.Errorf("order id must be positive")
	}
	if metafieldID <= 0 {
		return fmt.Errorf("metafield id must be positive")
	}
	path := fmt.Sprintf("orders/%d/metafields/%d", orderID, metafieldID)
	if _, err := c.Delete(ctx, path); err != nil {
		return fmt.Errorf("delete metafield %d on order %d: %w", metafieldID, orderID, err)
	}
	return nil
}

// ListOrderPaymentActions returns V3 payment actions from
// GET /v3/orders/{id}/payment_actions.
func (c *Client) ListOrderPaymentActions(ctx context.Context, orderID int, params OrderPaymentActionListParams) ([]json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/payment_actions", orderID)
	vals := url.Values{}
	if params.Page > 0 {
		vals.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		vals.Set("limit", strconv.Itoa(params.Limit))
	}
	if q := vals.Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list payment actions for order %d: %w", orderID, err)
	}
	rows, err := decodeV3DataArray(body)
	if err != nil {
		return nil, fmt.Errorf("parse payment actions response: %w", err)
	}
	return rows, nil
}

// ListOrderRefunds returns V3 refunds from
// GET /v3/orders/{id}/payment_actions/refunds.
func (c *Client) ListOrderRefunds(ctx context.Context, orderID int, params OrderRefundListParams) ([]json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/payment_actions/refunds", orderID)
	vals := url.Values{}
	if params.TransactionID != "" {
		vals.Set("transaction_id", params.TransactionID)
	}
	if params.Page > 0 {
		vals.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		vals.Set("limit", strconv.Itoa(params.Limit))
	}
	if q := vals.Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list refunds for order %d: %w", orderID, err)
	}
	rows, err := decodeV3DataArray(body)
	if err != nil {
		return nil, fmt.Errorf("parse refunds response: %w", err)
	}
	return rows, nil
}

// ListOrderLegacyRefunds returns V2 refunds from
// GET /v2/orders/{id}/refunds.
func (c *Client) ListOrderLegacyRefunds(ctx context.Context, orderID int, params OrderLegacyRefundListParams) ([]json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/refunds", orderID)
	vals := buildV2PagingValues(params.Page, params.Limit)
	if q := vals.Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.GetV2(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list legacy refunds for order %d: %w", orderID, err)
	}
	rows, err := decodeV2Array[json.RawMessage](body)
	if err != nil {
		return nil, fmt.Errorf("parse legacy refunds response: %w", err)
	}
	return rows, nil
}

// ListOrderTransactions returns V3 order transactions from
// GET /v3/orders/{id}/transactions.
func (c *Client) ListOrderTransactions(ctx context.Context, orderID int, params OrderTransactionListParams) ([]json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	path := fmt.Sprintf("orders/%d/transactions", orderID)
	vals := url.Values{}
	if params.Page > 0 {
		vals.Set("page", strconv.Itoa(params.Page))
	}
	if params.Limit > 0 {
		vals.Set("limit", strconv.Itoa(params.Limit))
	}
	if q := vals.Encode(); q != "" {
		path += "?" + q
	}
	body, err := c.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("list transactions for order %d: %w", orderID, err)
	}
	rows, err := decodeV3DataArray(body)
	if err != nil {
		return nil, fmt.Errorf("parse transactions response: %w", err)
	}
	return rows, nil
}

// CreateOrderPaymentCapture triggers capture via
// POST /v3/orders/{id}/payment_actions/capture.
func (c *Client) CreateOrderPaymentCapture(ctx context.Context, orderID int) (json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	body, err := c.Post(ctx, fmt.Sprintf("orders/%d/payment_actions/capture", orderID), map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("capture payment for order %d: %w", orderID, err)
	}
	data, err := decodeV3DataObject(body)
	if err != nil {
		return nil, fmt.Errorf("parse capture response: %w", err)
	}
	return data, nil
}

// CreateOrderPaymentVoid triggers void via
// POST /v3/orders/{id}/payment_actions/void.
func (c *Client) CreateOrderPaymentVoid(ctx context.Context, orderID int) (json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	body, err := c.Post(ctx, fmt.Sprintf("orders/%d/payment_actions/void", orderID), map[string]any{})
	if err != nil {
		return nil, fmt.Errorf("void payment for order %d: %w", orderID, err)
	}
	data, err := decodeV3DataObject(body)
	if err != nil {
		return nil, fmt.Errorf("parse void response: %w", err)
	}
	return data, nil
}

// CreateOrderRefundQuote creates a refund quote via
// POST /v3/orders/{id}/payment_actions/refund_quotes.
func (c *Client) CreateOrderRefundQuote(ctx context.Context, orderID int, payload json.RawMessage) (json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("refund quote payload is required")
	}
	body, err := c.Post(ctx, fmt.Sprintf("orders/%d/payment_actions/refund_quotes", orderID), json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("create refund quote for order %d: %w", orderID, err)
	}
	data, err := decodeV3DataObject(body)
	if err != nil {
		return nil, fmt.Errorf("parse refund quote response: %w", err)
	}
	return data, nil
}

// CreateOrderRefund creates a refund via
// POST /v3/orders/{id}/payment_actions/refunds.
func (c *Client) CreateOrderRefund(ctx context.Context, orderID int, payload json.RawMessage, transactionID string) (json.RawMessage, error) {
	if orderID <= 0 {
		return nil, fmt.Errorf("order id must be positive")
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("refund payload is required")
	}
	path := fmt.Sprintf("orders/%d/payment_actions/refunds", orderID)
	if strings.TrimSpace(transactionID) != "" {
		vals := url.Values{}
		vals.Set("transaction_id", strings.TrimSpace(transactionID))
		path += "?" + vals.Encode()
	}
	body, err := c.Post(ctx, path, json.RawMessage(payload))
	if err != nil {
		return nil, fmt.Errorf("create refund for order %d: %w", orderID, err)
	}
	data, err := decodeV3DataObject(body)
	if err != nil {
		return nil, fmt.Errorf("parse refund response: %w", err)
	}
	return data, nil
}

func buildOrderListValues(p OrderListParams, includePaging bool) url.Values {
	vals := url.Values{}
	if p.MinID > 0 {
		vals.Set("min_id", strconv.Itoa(p.MinID))
	}
	if p.MaxID > 0 {
		vals.Set("max_id", strconv.Itoa(p.MaxID))
	}
	if p.MinTotal > 0 {
		vals.Set("min_total", strconv.FormatFloat(p.MinTotal, 'f', -1, 64))
	}
	if p.MaxTotal > 0 {
		vals.Set("max_total", strconv.FormatFloat(p.MaxTotal, 'f', -1, 64))
	}
	if p.CustomerID > 0 {
		vals.Set("customer_id", strconv.Itoa(p.CustomerID))
	}
	if p.Email != "" {
		vals.Set("email", p.Email)
	}
	if p.StatusID > 0 {
		vals.Set("status_id", strconv.Itoa(p.StatusID))
	}
	if p.CartID != "" {
		vals.Set("cart_id", p.CartID)
	}
	if p.PaymentMethod != "" {
		vals.Set("payment_method", p.PaymentMethod)
	}
	if p.MinDateCreated != "" {
		vals.Set("min_date_created", p.MinDateCreated)
	}
	if p.MaxDateCreated != "" {
		vals.Set("max_date_created", p.MaxDateCreated)
	}
	if p.MinDateModified != "" {
		vals.Set("min_date_modified", p.MinDateModified)
	}
	if p.MaxDateModified != "" {
		vals.Set("max_date_modified", p.MaxDateModified)
	}
	if p.ChannelID > 0 {
		vals.Set("channel_id", strconv.Itoa(p.ChannelID))
	}
	if p.ExternalOrderID != "" {
		vals.Set("external_order_id", p.ExternalOrderID)
	}
	if p.Sort != "" {
		vals.Set("sort", p.Sort)
	}
	if len(p.Include) > 0 {
		vals.Set("include", strings.Join(p.Include, ","))
	}
	if p.ConsignmentStruct != "" {
		vals.Set("consignment_structure", p.ConsignmentStruct)
	}
	if includePaging {
		if p.Page > 0 {
			vals.Set("page", strconv.Itoa(p.Page))
		}
		if p.Limit > 0 {
			vals.Set("limit", strconv.Itoa(p.Limit))
		}
	}
	return vals
}

func buildOrderCountValues(p OrderCountParams) url.Values {
	vals := url.Values{}
	if p.MinID > 0 {
		vals.Set("min_id", strconv.Itoa(p.MinID))
	}
	if p.MaxID > 0 {
		vals.Set("max_id", strconv.Itoa(p.MaxID))
	}
	if p.MinTotal > 0 {
		vals.Set("min_total", strconv.FormatFloat(p.MinTotal, 'f', -1, 64))
	}
	if p.MaxTotal > 0 {
		vals.Set("max_total", strconv.FormatFloat(p.MaxTotal, 'f', -1, 64))
	}
	if p.CustomerID > 0 {
		vals.Set("customer_id", strconv.Itoa(p.CustomerID))
	}
	if p.Email != "" {
		vals.Set("email", p.Email)
	}
	if p.StatusID > 0 {
		vals.Set("status_id", strconv.Itoa(p.StatusID))
	}
	if p.CartID != "" {
		vals.Set("cart_id", p.CartID)
	}
	if p.PaymentMethod != "" {
		vals.Set("payment_method", p.PaymentMethod)
	}
	if p.MinDateCreated != "" {
		vals.Set("min_date_created", p.MinDateCreated)
	}
	if p.MaxDateCreated != "" {
		vals.Set("max_date_created", p.MaxDateCreated)
	}
	if p.MinDateModified != "" {
		vals.Set("min_date_modified", p.MinDateModified)
	}
	if p.MaxDateModified != "" {
		vals.Set("max_date_modified", p.MaxDateModified)
	}
	if p.ChannelID > 0 {
		vals.Set("channel_id", strconv.Itoa(p.ChannelID))
	}
	if p.ExternalOrderID != "" {
		vals.Set("external_order_id", p.ExternalOrderID)
	}
	return vals
}

func buildV2PagingValues(page, limit int) url.Values {
	vals := url.Values{}
	if page > 0 {
		vals.Set("page", strconv.Itoa(page))
	}
	if limit > 0 {
		vals.Set("limit", strconv.Itoa(limit))
	}
	return vals
}

func buildOrderMessageListValues(p OrderMessageListParams) url.Values {
	vals := url.Values{}
	if p.MinID > 0 {
		vals.Set("min_id", strconv.Itoa(p.MinID))
	}
	if p.MaxID > 0 {
		vals.Set("max_id", strconv.Itoa(p.MaxID))
	}
	if p.CustomerID > 0 {
		vals.Set("customer_id", strconv.Itoa(p.CustomerID))
	}
	if p.MinDateCreated != "" {
		vals.Set("min_date_created", p.MinDateCreated)
	}
	if p.MaxDateCreated != "" {
		vals.Set("max_date_created", p.MaxDateCreated)
	}
	if p.Status != "" {
		vals.Set("status", p.Status)
	}
	if p.IsFlagged != nil {
		vals.Set("is_flagged", strconv.FormatBool(*p.IsFlagged))
	}
	if p.Page > 0 {
		vals.Set("page", strconv.Itoa(p.Page))
	}
	if p.Limit > 0 {
		vals.Set("limit", strconv.Itoa(p.Limit))
	}
	return vals
}

func cloneURLValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for k, vals := range in {
		cp := make([]string, len(vals))
		copy(cp, vals)
		out[k] = cp
	}
	return out
}

func decodeV2Array[T any](body []byte) ([]T, error) {
	trim := strings.TrimSpace(string(body))
	if trim == "" || trim == "null" {
		return []T{}, nil
	}
	var out []T
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeV2Object[T any](body []byte, out *T) error {
	trim := strings.TrimSpace(string(body))
	if trim == "" || trim == "null" {
		return fmt.Errorf("empty object response")
	}
	return json.Unmarshal(body, out)
}

func decodeV2RawObject(body []byte) (json.RawMessage, error) {
	trim := strings.TrimSpace(string(body))
	if trim == "" || trim == "null" {
		return nil, fmt.Errorf("empty object response")
	}
	var raw json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func decodeV3DataArray(body []byte) ([]json.RawMessage, error) {
	trim := strings.TrimSpace(string(body))
	if trim == "" || trim == "null" {
		return []json.RawMessage{}, nil
	}
	var resp struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func decodeV3DataObject(body []byte) (json.RawMessage, error) {
	trim := strings.TrimSpace(string(body))
	if trim == "" || trim == "null" {
		return nil, fmt.Errorf("empty object response")
	}
	var resp SingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}
