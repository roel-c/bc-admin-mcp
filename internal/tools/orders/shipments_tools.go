package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

const maxShipmentItemsPerCall = 250

// Fulfillment holds handlers for orders/fulfillment/*.
type Fulfillment struct {
	bc BigCommerceOrdersAPI
}

// NewFulfillment constructs fulfillment handlers.
func NewFulfillment(bc BigCommerceOrdersAPI) *Fulfillment {
	return &Fulfillment{bc: bc}
}

// RegisterTools wires shipment tools into discovery.
func (f *Fulfillment) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/fulfillment/shipments/list",
		Tier:        middleware.TierR0,
		Summary:     "List shipments for one order (V2)",
		Description: "GET /v2/orders/{id}/shipments with optional page/limit.",
		Tool: mcp.NewTool("orders_fulfillment_shipments_list",
			mcp.WithDescription("List shipments for a specific order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: f.handleListShipments,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/fulfillment/shipments/get",
		Tier:        middleware.TierR0,
		Summary:     "Get one shipment on an order (V2)",
		Description: "GET /v2/orders/{id}/shipments/{shipment_id}.",
		Tool: mcp.NewTool("orders_fulfillment_shipments_get",
			mcp.WithDescription("Get one shipment row for an order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("shipment_id", mcp.Description("Shipment id."), mcp.Required()),
		),
		Handler: f.handleGetShipment,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/fulfillment/shipments/create",
		Tier:    middleware.TierR1,
		Summary: "Create an order shipment (V2)",
		Description: "POST /v2/orders/{id}/shipments. Requires order_address_id and items[]. " +
			"Preview payload first; pass confirmed=true to execute.",
		Tool: mcp.NewTool("orders_fulfillment_shipments_create",
			mcp.WithDescription("Create shipment with tracking and order-product quantities."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("order_address_id", mcp.Description("Order address id from the order's shipping_addresses."), mcp.Required()),
			mcp.WithArray("items", mcp.Description("Shipment items: [{order_product_id, quantity}]"),
				mcp.Items(map[string]any{"type": "object"}), mcp.Required()),
			mcp.WithString("tracking_number", mcp.Description("Tracking number.")),
			mcp.WithString("shipping_provider", mcp.Description("Carrier/provider code (e.g. fedex).")),
			mcp.WithString("tracking_carrier", mcp.Description("AfterShip carrier value.")),
			mcp.WithString("tracking_link", mcp.Description("Custom tracking URL.")),
			mcp.WithString("comments", mcp.Description("Optional internal comments for shipment.")),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after reviewing preview.")),
		),
		Handler: f.handleCreateShipment,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/fulfillment/shipments/update",
		Tier:    middleware.TierR1,
		Summary: "Update one shipment on an order (V2)",
		Description: "PUT /v2/orders/{id}/shipments/{shipment_id}. " +
			"Preview before execution; pass confirmed=true to apply.",
		Tool: mcp.NewTool("orders_fulfillment_shipments_update",
			mcp.WithDescription("Update one shipment row."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("shipment_id", mcp.Description("Shipment id."), mcp.Required()),
			mcp.WithObject("patch", mcp.Description("Partial shipment update payload object."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: f.handleUpdateShipment,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/fulfillment/shipments/delete",
		Tier:    middleware.TierR3,
		Summary: "Delete one shipment on an order (V2)",
		Description: "DELETE /v2/orders/{id}/shipments/{shipment_id}. " +
			"Destructive fulfillment operation; preview and explicit confirmation required.",
		Tool: mcp.NewTool("orders_fulfillment_shipments_delete",
			mcp.WithDescription("Delete one shipment row. Preview first; confirmed=true to execute."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("shipment_id", mcp.Description("Shipment id."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: f.handleDeleteShipment,
	})
}

func (f *Fulfillment) handleListShipments(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.OrderShipmentListParams{}
	if page, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.Page = page
	}
	if limit, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		if limit > maxOrdersListLimit {
			return shared.ToolError("limit must be <= %d", maxOrdersListLimit), nil
		}
		params.Limit = limit
	}
	rows, err := f.bc.ListOrderShipments(ctx, orderID, params)
	if err != nil {
		return shared.ToolError("failed to list order %d shipments: %v", orderID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":  orderID,
		"total":     len(rows),
		"shipments": rows,
	})
}

func (f *Fulfillment) handleCreateShipment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, payload, err := parseCreateShipmentArgs(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":   "preview",
			"action":   "create_order_shipment",
			"order_id": orderID,
			"payload":  payload,
			"message": "Pass confirmed=true to create the shipment. " +
				"Ensure order_address_id and order_product_id values are from the target order.",
		})
	}
	created, err := f.bc.CreateOrderShipment(ctx, orderID, payload)
	if err != nil {
		return shared.ToolError("failed to create shipment for order %d: %v", orderID, err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":   "created",
		"order_id": orderID,
		"shipment": created,
	})
}

func (f *Fulfillment) handleGetShipment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	shipmentID, err := shared.ReadPositiveInt(args, "shipment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	row, err := f.bc.GetOrderShipment(ctx, orderID, shipmentID)
	if err != nil {
		return shared.ToolError("failed to get shipment: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":    orderID,
		"shipment_id": shipmentID,
		"shipment":    row,
	})
}

func (f *Fulfillment) handleUpdateShipment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	shipmentID, err := shared.ReadPositiveInt(args, "shipment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	patch, err := requiredObjectPayload(args, "patch")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	current, err := f.bc.GetOrderShipment(ctx, orderID, shipmentID)
	if err != nil {
		return shared.ToolError("failed to fetch current shipment: %v", err), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":      "preview",
			"action":      "update_shipment",
			"order_id":    orderID,
			"shipment_id": shipmentID,
			"current":     current,
			"patch":       patch,
			"message":     "Review patch then pass confirmed=true to execute.",
		})
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return shared.ToolError("failed to marshal patch payload: %v", err), nil
	}
	updated, err := f.bc.UpdateOrderShipment(ctx, orderID, shipmentID, raw)
	if err != nil {
		return shared.ToolError("failed to update shipment: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":      "updated",
		"order_id":    orderID,
		"shipment_id": shipmentID,
		"shipment":    updated,
	})
}

func (f *Fulfillment) handleDeleteShipment(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	shipmentID, err := shared.ReadPositiveInt(args, "shipment_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	current, err := f.bc.GetOrderShipment(ctx, orderID, shipmentID)
	if err != nil {
		return shared.ToolError("failed to fetch shipment for delete preview: %v", err), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":       "preview",
			"action":       "delete_shipment",
			"order_id":     orderID,
			"shipment_id":  shipmentID,
			"would_delete": current,
			"message":      "Pass confirmed=true to permanently delete this shipment.",
		})
	}
	if err := f.bc.DeleteOrderShipment(ctx, orderID, shipmentID); err != nil {
		return shared.ToolError("failed to delete shipment: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":      "deleted",
		"order_id":    orderID,
		"shipment_id": shipmentID,
	})
}

func parseCreateShipmentArgs(args map[string]any) (int, bigcommerce.OrderShipmentCreate, error) {
	var payload bigcommerce.OrderShipmentCreate

	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return 0, payload, err
	}
	orderAddressID, err := shared.ReadPositiveInt(args, "order_address_id")
	if err != nil {
		return 0, payload, err
	}
	payload.OrderAddressID = orderAddressID

	itemsRaw, ok := args["items"]
	if !ok || itemsRaw == nil {
		return 0, payload, fmt.Errorf("items is required")
	}
	itemsAny, ok := itemsRaw.([]any)
	if !ok {
		return 0, payload, fmt.Errorf("items must be an array")
	}
	if len(itemsAny) == 0 {
		return 0, payload, fmt.Errorf("items must contain at least one row")
	}
	if len(itemsAny) > maxShipmentItemsPerCall {
		return 0, payload, fmt.Errorf("items exceeds max of %d per call", maxShipmentItemsPerCall)
	}
	payload.Items = make([]bigcommerce.OrderShipmentItem, 0, len(itemsAny))
	for i, raw := range itemsAny {
		row, ok := raw.(map[string]any)
		if !ok {
			return 0, payload, fmt.Errorf("items[%d] must be an object", i)
		}
		orderProductRaw, ok := row["order_product_id"]
		if !ok {
			return 0, payload, fmt.Errorf("items[%d].order_product_id is required", i)
		}
		orderProductF, ok := orderProductRaw.(float64)
		if !ok || orderProductF != math.Trunc(orderProductF) || int(orderProductF) <= 0 {
			return 0, payload, fmt.Errorf("items[%d].order_product_id must be a positive integer", i)
		}
		quantityRaw, ok := row["quantity"]
		if !ok {
			return 0, payload, fmt.Errorf("items[%d].quantity is required", i)
		}
		quantityF, ok := quantityRaw.(float64)
		if !ok || quantityF != math.Trunc(quantityF) || int(quantityF) <= 0 {
			return 0, payload, fmt.Errorf("items[%d].quantity must be a positive integer", i)
		}
		item := bigcommerce.OrderShipmentItem{
			OrderProductID: int(orderProductF),
			Quantity:       int(quantityF),
		}
		if productRaw, ok := row["product_id"]; ok && productRaw != nil {
			productF, ok := productRaw.(float64)
			if !ok || productF != math.Trunc(productF) || int(productF) <= 0 {
				return 0, payload, fmt.Errorf("items[%d].product_id must be a positive integer when provided", i)
			}
			item.ProductID = int(productF)
		}
		payload.Items = append(payload.Items, item)
	}

	if s, ok, err := readOptionalTrimmedString(args, "tracking_number"); err != nil {
		return 0, payload, err
	} else if ok {
		payload.TrackingNumber = s
	}
	if s, ok, err := readOptionalTrimmedString(args, "shipping_provider"); err != nil {
		return 0, payload, err
	} else if ok {
		payload.ShippingProvider = s
	}
	if s, ok, err := readOptionalTrimmedString(args, "tracking_carrier"); err != nil {
		return 0, payload, err
	} else if ok {
		payload.TrackingCarrier = s
	}
	if s, ok, err := readOptionalTrimmedString(args, "tracking_link"); err != nil {
		return 0, payload, err
	} else if ok {
		payload.TrackingLink = s
	}
	if s, ok, err := readOptionalTrimmedString(args, "comments"); err != nil {
		return 0, payload, err
	} else if ok {
		payload.Comments = s
	}

	return orderID, payload, nil
}
