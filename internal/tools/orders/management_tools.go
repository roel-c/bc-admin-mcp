package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

const maxOrdersListLimit = 250

var validOrderSortFields = map[string]struct{}{
	"id":            {},
	"customer_id":   {},
	"date_created":  {},
	"date_modified": {},
	"status_id":     {},
	"channel_id":    {},
	"external_id":   {},
}

var validOrderIncludes = map[string]struct{}{
	"consignments":            {},
	"consignments.line_items": {},
	"fees":                    {},
}

// Management holds tool handlers for orders/management/*.
type Management struct {
	bc    BigCommerceOrdersAPI
	cache *session.Store
}

// NewManagement constructs orders management handlers.
func NewManagement(bc BigCommerceOrdersAPI, cache *session.Store) *Management {
	return &Management{bc: bc, cache: cache}
}

func orderCacheKey(orderID int) string {
	return fmt.Sprintf("order:%d", orderID)
}

// RegisterTools wires orders/management tools into the discovery registry.
func (m *Management) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/list",
		Tier:    middleware.TierR0,
		Summary: "List/search orders (V2)",
		Description: "GET /v2/orders with filters and optional single-page pagination (page/limit). " +
			"When page/limit are omitted the server auto-paginates up to configured MaxTotalRecords. " +
			"Provide filters or set list_all=true.",
		Tool: mcp.NewTool("orders_management_list",
			mcp.WithDescription("Search orders by status, customer, date range, channel, payment method, etc."),
			mcp.WithBoolean("list_all", mcp.Description("When true, allows listing without explicit filters.")),
			mcp.WithNumber("min_id", mcp.Description("Minimum order id.")),
			mcp.WithNumber("max_id", mcp.Description("Maximum order id.")),
			mcp.WithNumber("min_total", mcp.Description("Minimum order total.")),
			mcp.WithNumber("max_total", mcp.Description("Maximum order total.")),
			mcp.WithNumber("customer_id", mcp.Description("Filter by customer id.")),
			mcp.WithString("email", mcp.Description("Filter by customer email.")),
			mcp.WithNumber("status_id", mcp.Description("Filter by status id.")),
			mcp.WithString("cart_id", mcp.Description("Filter by cart id.")),
			mcp.WithString("payment_method", mcp.Description("Filter by payment method display name.")),
			mcp.WithString("min_date_created", mcp.Description("Minimum date created (RFC-2822 or ISO-8601).")),
			mcp.WithString("max_date_created", mcp.Description("Maximum date created (RFC-2822 or ISO-8601).")),
			mcp.WithString("min_date_modified", mcp.Description("Minimum date modified (RFC-2822 or ISO-8601).")),
			mcp.WithString("max_date_modified", mcp.Description("Maximum date modified (RFC-2822 or ISO-8601).")),
			mcp.WithNumber("channel_id", mcp.Description("Filter by channel id.")),
			mcp.WithString("external_order_id", mcp.Description("Filter by external order id.")),
			mcp.WithString("sort", mcp.Description("Sort field with optional direction suffix, e.g. date_created:desc.")),
			mcp.WithArray("include", mcp.Description("Optional includes: consignments, consignments.line_items, fees."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithString("consignment_structure", mcp.Description("Must be 'object' when include uses consignments.")),
			mcp.WithNumber("page", mcp.Description("Explicit page number (single-page mode).")),
			mcp.WithNumber("limit", mcp.Description("Explicit page size (single-page mode, max 250).")),
		),
		Handler: m.handleList,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/get",
		Tier:    middleware.TierR0,
		Summary: "Get order details and line items (V2)",
		Description: "GET /v2/orders/{id} and GET /v2/orders/{id}/products. " +
			"Optionally pass include and consignment_structure for additional order payloads.",
		Tool: mcp.NewTool("orders_management_get",
			mcp.WithDescription("Fetch one order and its order products."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithArray("include", mcp.Description("Optional include values: consignments, consignments.line_items, fees."), mcp.Items(map[string]any{"type": "string"})),
			mcp.WithString("consignment_structure", mcp.Description("Must be 'object' when include uses consignments.")),
		),
		Handler: m.handleGet,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/create",
		Tier:    middleware.TierR2,
		Summary: "Create one manual order (V2)",
		Description: "POST /v2/orders with a caller-supplied payload object. " +
			"Use for lower-frequency manual order creation workflows. Preview first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("orders_management_create",
			mcp.WithDescription("Create one order from a V2 payload object."),
			mcp.WithObject("order", mcp.Description("Manual order create payload object."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: m.handleCreate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/update",
		Tier:    middleware.TierR2,
		Summary: "Update selected order fields (V2)",
		Description: "PUT /v2/orders/{id} with a caller-supplied patch object. " +
			"Use for targeted updates beyond status changes. BigCommerce notes some order updates can clear discounts/promotions on affected line items; review preview carefully.",
		Tool: mcp.NewTool("orders_management_update",
			mcp.WithDescription("Update selected order fields. Preview first; confirmed=true to execute."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithObject("patch", mcp.Description("Partial V2 order update payload object."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: m.handleUpdate,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete one order (V2)",
		Description: "DELETE /v2/orders/{id}. Destructive operation; preview current order " +
			"first and pass confirmed=true to execute.",
		Tool: mcp.NewTool("orders_management_delete",
			mcp.WithDescription("Delete one order by id. Preview first; confirmed=true to execute."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: m.handleDelete,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/count",
		Tier:        middleware.TierR0,
		Summary:     "Count orders with filters (V2)",
		Description: "GET /v2/orders/count with the same filter family as list.",
		Tool: mcp.NewTool("orders_management_count",
			mcp.WithDescription("Return order count for optional filters."),
			mcp.WithNumber("min_id", mcp.Description("Minimum order id.")),
			mcp.WithNumber("max_id", mcp.Description("Maximum order id.")),
			mcp.WithNumber("min_total", mcp.Description("Minimum order total.")),
			mcp.WithNumber("max_total", mcp.Description("Maximum order total.")),
			mcp.WithNumber("customer_id", mcp.Description("Filter by customer id.")),
			mcp.WithString("email", mcp.Description("Filter by customer email.")),
			mcp.WithNumber("status_id", mcp.Description("Filter by status id.")),
			mcp.WithString("cart_id", mcp.Description("Filter by cart id.")),
			mcp.WithString("payment_method", mcp.Description("Filter by payment method.")),
			mcp.WithString("min_date_created", mcp.Description("Minimum date created (RFC-2822 or ISO-8601).")),
			mcp.WithString("max_date_created", mcp.Description("Maximum date created (RFC-2822 or ISO-8601).")),
			mcp.WithString("min_date_modified", mcp.Description("Minimum date modified (RFC-2822 or ISO-8601).")),
			mcp.WithString("max_date_modified", mcp.Description("Maximum date modified (RFC-2822 or ISO-8601).")),
			mcp.WithNumber("channel_id", mcp.Description("Filter by channel id.")),
			mcp.WithString("external_order_id", mcp.Description("Filter by external order id.")),
		),
		Handler: m.handleCount,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/statuses",
		Tier:        middleware.TierR0,
		Summary:     "List order statuses (V2)",
		Description: "GET /v2/order_statuses.",
		Tool: mcp.NewTool("orders_management_statuses",
			mcp.WithDescription("List order statuses and ids."),
		),
		Handler: m.handleStatuses,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/products/get",
		Tier:        middleware.TierR0,
		Summary:     "Get one order-product row (V2)",
		Description: "GET /v2/orders/{id}/products/{product_id}.",
		Tool: mcp.NewTool("orders_management_products_get",
			mcp.WithDescription("Fetch one order product row by product_id."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("product_id", mcp.Description("Order-product id from /orders/{id}/products."), mcp.Required()),
		),
		Handler: m.handleGetOrderProduct,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/update_status",
		Tier:    middleware.TierR1,
		Summary: "Update order status (V2)",
		Description: "PUT /v2/orders/{id} with status_id only. " +
			"Preview shows current vs target status; pass confirmed=true to apply.",
		Tool: mcp.NewTool("orders_management_update_status",
			mcp.WithDescription("Change one order status_id. Preview then confirmed=true."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("status_id", mcp.Description("Target status id (0+)."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: m.handleUpdateStatus,
	})
}

func (m *Management) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, hasDataFilter, err := parseOrderListParams(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !shared.ReadBool(args, "list_all") && !hasDataFilter {
		return shared.ToolError("provide at least one filter or set list_all=true"), nil
	}
	orders, err := m.bc.ListOrders(ctx, params)
	if err != nil {
		return shared.ToolError("failed to list orders: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"total":   len(orders),
		"orders":  orders,
		"filters": summarizeOrderFilters(params),
	})
}

func (m *Management) handleGet(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	include, consignmentStructure, err := parseOrderIncludeArgs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	orderRow, err := m.bc.GetOrder(ctx, orderID, bigcommerce.OrderGetParams{
		Include:           include,
		ConsignmentStruct: consignmentStructure,
	})
	if err != nil {
		return shared.ToolError("failed to get order %d: %v", orderID, err), nil
	}
	products, err := m.bc.ListOrderProducts(ctx, orderID, bigcommerce.OrderProductListParams{})
	if err != nil {
		return shared.ToolError("failed to list order %d products: %v", orderID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order":         orderRow,
		"products":      products,
		"product_count": len(products),
	})
}

func (m *Management) handleCount(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	params, err := parseOrderCountParams(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	count, err := m.bc.CountOrders(ctx, params)
	if err != nil {
		return shared.ToolError("failed to count orders: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"count":   count,
		"filters": summarizeOrderCountFilters(params),
	})
}

func (m *Management) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderPayload, err := requiredObjectPayload(args, "order")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":  "preview",
			"action":  "create_order",
			"order":   orderPayload,
			"message": "Review payload and pass confirmed=true to create the order.",
		})
	}
	raw, err := json.Marshal(orderPayload)
	if err != nil {
		return shared.ToolError("failed to marshal order payload: %v", err), nil
	}
	created, err := m.bc.CreateOrder(ctx, raw)
	if err != nil {
		return shared.ToolError("failed to create order: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status": "created",
		"order":  created,
	})
}

func (m *Management) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	patch, err := requiredObjectPayload(args, "patch")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	cacheKey := orderCacheKey(orderID)
	current, err := session.CacheOrFetch(m.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Order, error) {
		return m.bc.GetOrder(ctx, orderID, bigcommerce.OrderGetParams{})
	})
	if err != nil {
		return shared.ToolError("failed to fetch current order %d: %v", orderID, err), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "update_order",
			"order_id": orderID,
			"current": map[string]any{
				"id":         current.ID,
				"status_id":  current.StatusID,
				"status":     current.Status,
				"channel_id": current.ChannelID,
				"total":      current.TotalIncTax,
			},
			"patch": patch,
			"message": "Review patch carefully. BigCommerce order updates can affect discounts/promotions " +
				"on modified line items. Pass confirmed=true to execute.",
		})
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return shared.ToolError("failed to marshal patch payload: %v", err), nil
	}
	m.cache.ForContext(ctx).Delete(cacheKey)
	updated, err := m.bc.UpdateOrder(ctx, orderID, raw)
	if err != nil {
		return shared.ToolError("failed to update order %d: %v", orderID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status": "updated",
		"order":  updated,
	})
}

func (m *Management) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	cacheKey := orderCacheKey(orderID)
	current, err := session.CacheOrFetch(m.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Order, error) {
		return m.bc.GetOrder(ctx, orderID, bigcommerce.OrderGetParams{})
	})
	if err != nil {
		return shared.ToolError("failed to fetch current order %d: %v", orderID, err), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "delete_order",
			"order_id": orderID,
			"would_delete": map[string]any{
				"id":            current.ID,
				"status":        current.Status,
				"total_inc_tax": current.TotalIncTax,
				"date_created":  current.DateCreated,
				"customer_id":   current.CustomerID,
			},
			"message": "Destructive operation. Pass confirmed=true to permanently delete this order.",
		})
	}
	m.cache.ForContext(ctx).Delete(cacheKey)
	if err := m.bc.DeleteOrder(ctx, orderID); err != nil {
		return shared.ToolError("failed to delete order %d: %v", orderID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "deleted",
		"order_id": orderID,
	})
}

func (m *Management) handleStatuses(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	statuses, err := m.bc.ListOrderStatuses(ctx)
	if err != nil {
		return shared.ToolError("failed to list order statuses: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"total":    len(statuses),
		"statuses": statuses,
	})
}

func (m *Management) handleGetOrderProduct(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	productID, err := shared.ReadPositiveInt(args, "product_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	row, err := m.bc.GetOrderProduct(ctx, orderID, productID)
	if err != nil {
		return shared.ToolError("failed to get order product: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":      orderID,
		"product_id":    productID,
		"order_product": row,
	})
}

func (m *Management) handleUpdateStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	statusID, err := readNonNegativeInt(args, "status_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}

	cacheKey := orderCacheKey(orderID)
	current, err := session.CacheOrFetch(m.cache.ForContext(ctx), cacheKey, func() (*bigcommerce.Order, error) {
		return m.bc.GetOrder(ctx, orderID, bigcommerce.OrderGetParams{})
	})
	if err != nil {
		return shared.ToolError("failed to fetch order %d: %v", orderID, err), nil
	}
	if current.StatusID == statusID {
		return shared.ToolJSON(map[string]any{
			"status":            "noop",
			"order_id":          orderID,
			"current_status_id": current.StatusID,
			"message":           "order already has the requested status_id",
		})
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":            "preview",
			"action":            "update_order_status",
			"order_id":          orderID,
			"current_status_id": current.StatusID,
			"current_status":    current.Status,
			"target_status_id":  statusID,
			"message":           "Pass confirmed=true to apply.",
		})
	}
	m.cache.ForContext(ctx).Delete(cacheKey)
	updated, err := m.bc.UpdateOrderStatus(ctx, orderID, statusID)
	if err != nil {
		return shared.ToolError("failed to update order %d status: %v", orderID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status": "updated",
		"order":  updated,
	})
}

func parseOrderListParams(args map[string]any) (bigcommerce.OrderListParams, bool, error) {
	var out bigcommerce.OrderListParams
	hasData := false

	if v, ok, err := readOptionalPositiveInt(args, "min_id"); err != nil {
		return out, false, err
	} else if ok {
		out.MinID = v
		hasData = true
	}
	if v, ok, err := readOptionalPositiveInt(args, "max_id"); err != nil {
		return out, false, err
	} else if ok {
		out.MaxID = v
		hasData = true
	}
	if v, ok, err := readOptionalNonNegativeFloat(args, "min_total"); err != nil {
		return out, false, err
	} else if ok {
		out.MinTotal = v
		hasData = true
	}
	if v, ok, err := readOptionalNonNegativeFloat(args, "max_total"); err != nil {
		return out, false, err
	} else if ok {
		out.MaxTotal = v
		hasData = true
	}
	if v, ok, err := readOptionalPositiveInt(args, "customer_id"); err != nil {
		return out, false, err
	} else if ok {
		out.CustomerID = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "email"); err != nil {
		return out, false, err
	} else if ok {
		out.Email = v
		hasData = true
	}
	if v, ok, err := readOptionalNonNegativeInt(args, "status_id"); err != nil {
		return out, false, err
	} else if ok {
		out.StatusID = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "cart_id"); err != nil {
		return out, false, err
	} else if ok {
		out.CartID = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "payment_method"); err != nil {
		return out, false, err
	} else if ok {
		out.PaymentMethod = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "min_date_created"); err != nil {
		return out, false, err
	} else if ok {
		out.MinDateCreated = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "max_date_created"); err != nil {
		return out, false, err
	} else if ok {
		out.MaxDateCreated = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "min_date_modified"); err != nil {
		return out, false, err
	} else if ok {
		out.MinDateModified = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "max_date_modified"); err != nil {
		return out, false, err
	} else if ok {
		out.MaxDateModified = v
		hasData = true
	}
	if v, ok, err := readOptionalPositiveInt(args, "channel_id"); err != nil {
		return out, false, err
	} else if ok {
		out.ChannelID = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "external_order_id"); err != nil {
		return out, false, err
	} else if ok {
		out.ExternalOrderID = v
		hasData = true
	}
	if v, ok, err := readOptionalTrimmedString(args, "sort"); err != nil {
		return out, false, err
	} else if ok {
		normalized, err := normalizeSort(v)
		if err != nil {
			return out, false, err
		}
		out.Sort = normalized
	}
	include, consignmentStructure, err := parseOrderIncludeArgs(args)
	if err != nil {
		return out, false, err
	}
	out.Include = include
	out.ConsignmentStruct = consignmentStructure

	if v, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return out, false, err
	} else if ok {
		out.Page = v
	}
	if v, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return out, false, err
	} else if ok {
		if v > maxOrdersListLimit {
			return out, false, fmt.Errorf("limit must be <= %d", maxOrdersListLimit)
		}
		out.Limit = v
	}

	return out, hasData, nil
}

func parseOrderCountParams(args map[string]any) (bigcommerce.OrderCountParams, error) {
	var out bigcommerce.OrderCountParams
	if v, ok, err := readOptionalPositiveInt(args, "min_id"); err != nil {
		return out, err
	} else if ok {
		out.MinID = v
	}
	if v, ok, err := readOptionalPositiveInt(args, "max_id"); err != nil {
		return out, err
	} else if ok {
		out.MaxID = v
	}
	if v, ok, err := readOptionalNonNegativeFloat(args, "min_total"); err != nil {
		return out, err
	} else if ok {
		out.MinTotal = v
	}
	if v, ok, err := readOptionalNonNegativeFloat(args, "max_total"); err != nil {
		return out, err
	} else if ok {
		out.MaxTotal = v
	}
	if v, ok, err := readOptionalPositiveInt(args, "customer_id"); err != nil {
		return out, err
	} else if ok {
		out.CustomerID = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "email"); err != nil {
		return out, err
	} else if ok {
		out.Email = v
	}
	if v, ok, err := readOptionalNonNegativeInt(args, "status_id"); err != nil {
		return out, err
	} else if ok {
		out.StatusID = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "cart_id"); err != nil {
		return out, err
	} else if ok {
		out.CartID = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "payment_method"); err != nil {
		return out, err
	} else if ok {
		out.PaymentMethod = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "min_date_created"); err != nil {
		return out, err
	} else if ok {
		out.MinDateCreated = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "max_date_created"); err != nil {
		return out, err
	} else if ok {
		out.MaxDateCreated = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "min_date_modified"); err != nil {
		return out, err
	} else if ok {
		out.MinDateModified = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "max_date_modified"); err != nil {
		return out, err
	} else if ok {
		out.MaxDateModified = v
	}
	if v, ok, err := readOptionalPositiveInt(args, "channel_id"); err != nil {
		return out, err
	} else if ok {
		out.ChannelID = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "external_order_id"); err != nil {
		return out, err
	} else if ok {
		out.ExternalOrderID = v
	}
	return out, nil
}

func parseOrderIncludeArgs(args map[string]any) ([]string, string, error) {
	include := []string(nil)
	if raw, ok := args["include"]; ok && raw != nil {
		arr, ok := raw.([]any)
		if !ok {
			return nil, "", fmt.Errorf("include must be an array of strings")
		}
		include = make([]string, 0, len(arr))
		for i, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, "", fmt.Errorf("include[%d] must be a string", i)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				return nil, "", fmt.Errorf("include[%d] must be a non-empty string", i)
			}
			if _, allowed := validOrderIncludes[s]; !allowed {
				return nil, "", fmt.Errorf("include[%d] must be one of consignments, consignments.line_items, fees", i)
			}
			include = append(include, s)
		}
	}
	consignmentStructure := ""
	if v, ok, err := readOptionalTrimmedString(args, "consignment_structure"); err != nil {
		return nil, "", err
	} else if ok {
		if v != "object" {
			return nil, "", fmt.Errorf("consignment_structure must be 'object' when provided")
		}
		consignmentStructure = v
	}
	if includesConsignments(include) && consignmentStructure == "" {
		consignmentStructure = "object"
	}
	return include, consignmentStructure, nil
}

func includesConsignments(include []string) bool {
	for _, s := range include {
		if s == "consignments" || s == "consignments.line_items" {
			return true
		}
	}
	return false
}

func normalizeSort(raw string) (string, error) {
	parts := strings.Split(raw, ":")
	field := strings.TrimSpace(parts[0])
	if _, ok := validOrderSortFields[field]; !ok {
		return "", fmt.Errorf("sort field must be one of id, customer_id, date_created, date_modified, status_id, channel_id, external_id")
	}
	if len(parts) == 1 {
		return field, nil
	}
	if len(parts) != 2 {
		return "", fmt.Errorf("sort format must be field or field:asc|desc")
	}
	dir := strings.ToLower(strings.TrimSpace(parts[1]))
	if dir != "asc" && dir != "desc" {
		return "", fmt.Errorf("sort direction must be asc or desc")
	}
	return field + ":" + dir, nil
}

func summarizeOrderFilters(p bigcommerce.OrderListParams) map[string]any {
	out := map[string]any{}
	if p.MinID > 0 {
		out["min_id"] = p.MinID
	}
	if p.MaxID > 0 {
		out["max_id"] = p.MaxID
	}
	if p.MinTotal > 0 {
		out["min_total"] = p.MinTotal
	}
	if p.MaxTotal > 0 {
		out["max_total"] = p.MaxTotal
	}
	if p.CustomerID > 0 {
		out["customer_id"] = p.CustomerID
	}
	if p.Email != "" {
		out["email"] = p.Email
	}
	if p.StatusID > 0 {
		out["status_id"] = p.StatusID
	}
	if p.CartID != "" {
		out["cart_id"] = p.CartID
	}
	if p.PaymentMethod != "" {
		out["payment_method"] = p.PaymentMethod
	}
	if p.MinDateCreated != "" {
		out["min_date_created"] = p.MinDateCreated
	}
	if p.MaxDateCreated != "" {
		out["max_date_created"] = p.MaxDateCreated
	}
	if p.MinDateModified != "" {
		out["min_date_modified"] = p.MinDateModified
	}
	if p.MaxDateModified != "" {
		out["max_date_modified"] = p.MaxDateModified
	}
	if p.ChannelID > 0 {
		out["channel_id"] = p.ChannelID
	}
	if p.ExternalOrderID != "" {
		out["external_order_id"] = p.ExternalOrderID
	}
	if p.Sort != "" {
		out["sort"] = p.Sort
	}
	if len(p.Include) > 0 {
		out["include"] = p.Include
	}
	if p.ConsignmentStruct != "" {
		out["consignment_structure"] = p.ConsignmentStruct
	}
	if p.Page > 0 {
		out["page"] = p.Page
	}
	if p.Limit > 0 {
		out["limit"] = p.Limit
	}
	return out
}

func summarizeOrderCountFilters(p bigcommerce.OrderCountParams) map[string]any {
	out := map[string]any{}
	if p.MinID > 0 {
		out["min_id"] = p.MinID
	}
	if p.MaxID > 0 {
		out["max_id"] = p.MaxID
	}
	if p.MinTotal > 0 {
		out["min_total"] = p.MinTotal
	}
	if p.MaxTotal > 0 {
		out["max_total"] = p.MaxTotal
	}
	if p.CustomerID > 0 {
		out["customer_id"] = p.CustomerID
	}
	if p.Email != "" {
		out["email"] = p.Email
	}
	if p.StatusID > 0 {
		out["status_id"] = p.StatusID
	}
	if p.CartID != "" {
		out["cart_id"] = p.CartID
	}
	if p.PaymentMethod != "" {
		out["payment_method"] = p.PaymentMethod
	}
	if p.MinDateCreated != "" {
		out["min_date_created"] = p.MinDateCreated
	}
	if p.MaxDateCreated != "" {
		out["max_date_created"] = p.MaxDateCreated
	}
	if p.MinDateModified != "" {
		out["min_date_modified"] = p.MinDateModified
	}
	if p.MaxDateModified != "" {
		out["max_date_modified"] = p.MaxDateModified
	}
	if p.ChannelID > 0 {
		out["channel_id"] = p.ChannelID
	}
	if p.ExternalOrderID != "" {
		out["external_order_id"] = p.ExternalOrderID
	}
	return out
}

func readOptionalPositiveInt(args map[string]any, key string) (int, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
	n := int(f)
	if n <= 0 {
		return 0, false, fmt.Errorf("%s must be positive", key)
	}
	return n, true, nil
}

func readOptionalNonNegativeInt(args map[string]any, key string) (int, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return 0, false, fmt.Errorf("%s must be an integer", key)
	}
	n := int(f)
	if n < 0 {
		return 0, false, fmt.Errorf("%s must be non-negative", key)
	}
	return n, true, nil
}

func readNonNegativeInt(args map[string]any, key string) (int, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, fmt.Errorf("%s is required", key)
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("%s must be an integer", key)
	}
	n := int(f)
	if n < 0 {
		return 0, fmt.Errorf("%s must be non-negative", key)
	}
	return n, nil
}

func readOptionalNonNegativeFloat(args map[string]any, key string) (float64, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false, nil
	}
	f, ok := v.(float64)
	if !ok {
		return 0, false, fmt.Errorf("%s must be a number", key)
	}
	if f < 0 {
		return 0, false, fmt.Errorf("%s must be non-negative", key)
	}
	return f, true, nil
}

func readOptionalTrimmedString(args map[string]any, key string) (string, bool, error) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", false, fmt.Errorf("%s must be a string", key)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}
