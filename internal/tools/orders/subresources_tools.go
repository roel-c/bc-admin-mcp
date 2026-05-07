package orders

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

// Subresources holds handlers for order sub-resource read tools.
type Subresources struct {
	bc BigCommerceOrdersAPI
}

// NewSubresources constructs order sub-resource handlers.
func NewSubresources(bc BigCommerceOrdersAPI) *Subresources {
	return &Subresources{bc: bc}
}

// RegisterTools wires order sub-resource read tools into discovery.
func (s *Subresources) RegisterTools(reg *discovery.Registry) {
	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/coupons/list",
		Tier:        middleware.TierR0,
		Summary:     "List coupons for one order (V2)",
		Description: "GET /v2/orders/{id}/coupons.",
		Tool: mcp.NewTool("orders_management_coupons_list",
			mcp.WithDescription("List coupon rows attached to one order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: s.handleListCoupons,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/shipping_addresses/list",
		Tier:        middleware.TierR0,
		Summary:     "List shipping addresses for one order (V2)",
		Description: "GET /v2/orders/{id}/shipping_addresses.",
		Tool: mcp.NewTool("orders_management_shipping_addresses_list",
			mcp.WithDescription("List shipping-address rows on one order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: s.handleListShippingAddresses,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/shipping_addresses/get",
		Tier:        middleware.TierR0,
		Summary:     "Get one shipping address row on an order (V2)",
		Description: "GET /v2/orders/{id}/shipping_addresses/{shipping_address_id}.",
		Tool: mcp.NewTool("orders_management_shipping_addresses_get",
			mcp.WithDescription("Fetch one shipping-address row on an order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("shipping_address_id", mcp.Description("Shipping address id."), mcp.Required()),
		),
		Handler: s.handleGetShippingAddress,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/shipping_addresses/update",
		Tier:    middleware.TierR1,
		Summary: "Update one shipping address row on an order (V2)",
		Description: "PUT /v2/orders/{id}/shipping_addresses/{shipping_address_id}. " +
			"Preview before execution; pass confirmed=true to apply.",
		Tool: mcp.NewTool("orders_management_shipping_addresses_update",
			mcp.WithDescription("Update one order shipping-address row."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("shipping_address_id", mcp.Description("Shipping address id."), mcp.Required()),
			mcp.WithObject("patch", mcp.Description("Partial shipping-address update payload."), mcp.Required()),
			mcp.WithBoolean("confirmed", mcp.Description("Set true after preview to execute.")),
		),
		Handler: s.handleUpdateShippingAddress,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:    "orders/management/messages/list",
		Tier:    middleware.TierR0,
		Summary: "List messages for one order (V2)",
		Description: "GET /v2/orders/{id}/messages. Supports message filters " +
			"min_id, max_id, customer_id, min/max_date_created, status(read|unread), is_flagged, page, limit.",
		Tool: mcp.NewTool("orders_management_messages_list",
			mcp.WithDescription("List order messages with optional filters."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("min_id", mcp.Description("Minimum message id.")),
			mcp.WithNumber("max_id", mcp.Description("Maximum message id.")),
			mcp.WithNumber("customer_id", mcp.Description("Filter by customer id.")),
			mcp.WithString("min_date_created", mcp.Description("Minimum message date created.")),
			mcp.WithString("max_date_created", mcp.Description("Maximum message date created.")),
			mcp.WithString("status", mcp.Description("Message status: read|unread.")),
			mcp.WithBoolean("is_flagged", mcp.Description("Filter flagged status.")),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: s.handleListMessages,
	})

	reg.RegisterTool(&discovery.ToolDef{
		Path:        "orders/management/taxes/list",
		Tier:        middleware.TierR0,
		Summary:     "List taxes for one order (V2)",
		Description: "GET /v2/orders/{id}/taxes.",
		Tool: mcp.NewTool("orders_management_taxes_list",
			mcp.WithDescription("List tax rows on one order."),
			mcp.WithNumber("order_id", mcp.Description("Order id."), mcp.Required()),
			mcp.WithNumber("page", mcp.Description("Optional page number.")),
			mcp.WithNumber("limit", mcp.Description("Optional page size (max 250).")),
		),
		Handler: s.handleListTaxes,
	})
}

func (s *Subresources) handleListCoupons(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params, err := readPaging(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	rows, err := s.bc.ListOrderCoupons(ctx, orderID, bigcommerce.OrderCouponListParams{
		Page:  params.Page,
		Limit: params.Limit,
	})
	if err != nil {
		return shared.ToolError("failed to list coupons: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id": orderID,
		"total":    len(rows),
		"coupons":  rows,
	})
}

func (s *Subresources) handleListShippingAddresses(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params, err := readPaging(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	rows, err := s.bc.ListOrderShippingAddresses(ctx, orderID, bigcommerce.OrderShippingAddressListParams{
		Page:  params.Page,
		Limit: params.Limit,
	})
	if err != nil {
		return shared.ToolError("failed to list shipping addresses: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":           orderID,
		"total":              len(rows),
		"shipping_addresses": rows,
	})
}

func (s *Subresources) handleGetShippingAddress(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	shippingAddressID, err := shared.ReadPositiveInt(args, "shipping_address_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	row, err := s.bc.GetOrderShippingAddress(ctx, orderID, shippingAddressID)
	if err != nil {
		return shared.ToolError("failed to get shipping address: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id":            orderID,
		"shipping_address_id": shippingAddressID,
		"shipping_address":    row,
	})
}

func (s *Subresources) handleUpdateShippingAddress(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	shippingAddressID, err := shared.ReadPositiveInt(args, "shipping_address_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	patch, err := requiredObjectPayload(args, "patch")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	current, err := s.bc.GetOrderShippingAddress(ctx, orderID, shippingAddressID)
	if err != nil {
		return shared.ToolError("failed to fetch current shipping address: %v", err), nil
	}
	if !middleware.IsConfirmedFromArgs(args) {
		return shared.ToolJSON(map[string]any{
			"status":              "preview",
			"action":              "update_shipping_address",
			"order_id":            orderID,
			"shipping_address_id": shippingAddressID,
			"current":             current,
			"patch":               patch,
			"message":             "Review patch then pass confirmed=true to execute.",
		})
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return shared.ToolError("failed to marshal patch payload: %v", err), nil
	}
	updated, err := s.bc.UpdateOrderShippingAddress(ctx, orderID, shippingAddressID, raw)
	if err != nil {
		return shared.ToolError("failed to update shipping address: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"status":              "updated",
		"order_id":            orderID,
		"shipping_address_id": shippingAddressID,
		"shipping_address":    updated,
	})
}

func (s *Subresources) handleListMessages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params := bigcommerce.OrderMessageListParams{}
	if v, ok, err := readOptionalPositiveInt(args, "min_id"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.MinID = v
	}
	if v, ok, err := readOptionalPositiveInt(args, "max_id"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.MaxID = v
	}
	if v, ok, err := readOptionalPositiveInt(args, "customer_id"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.CustomerID = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "min_date_created"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.MinDateCreated = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "max_date_created"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		params.MaxDateCreated = v
	}
	if v, ok, err := readOptionalTrimmedString(args, "status"); err != nil {
		return shared.ToolError("%s", err.Error()), nil
	} else if ok {
		normalized := strings.ToLower(v)
		if normalized != "read" && normalized != "unread" {
			return shared.ToolError("status must be read or unread"), nil
		}
		params.Status = normalized
	}
	if v, ok := args["is_flagged"]; ok && v != nil {
		b, ok := v.(bool)
		if !ok {
			return shared.ToolError("is_flagged must be a boolean"), nil
		}
		params.IsFlagged = &b
	}
	paging, err := readPaging(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	params.Page = paging.Page
	params.Limit = paging.Limit

	rows, err := s.bc.ListOrderMessages(ctx, orderID, params)
	if err != nil {
		return shared.ToolError("failed to list order messages: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id": orderID,
		"total":    len(rows),
		"messages": rows,
		"filters": map[string]any{
			"min_id":           params.MinID,
			"max_id":           params.MaxID,
			"customer_id":      params.CustomerID,
			"min_date_created": params.MinDateCreated,
			"max_date_created": params.MaxDateCreated,
			"status":           params.Status,
			"is_flagged":       params.IsFlagged,
			"page":             params.Page,
			"limit":            params.Limit,
		},
	})
}

func (s *Subresources) handleListTaxes(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	orderID, err := shared.ReadPositiveInt(args, "order_id")
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	paging, err := readPaging(args)
	if err != nil {
		return shared.ToolError("%s", err.Error()), nil
	}
	rows, err := s.bc.ListOrderTaxes(ctx, orderID, bigcommerce.OrderTaxListParams{
		Page:  paging.Page,
		Limit: paging.Limit,
	})
	if err != nil {
		return shared.ToolError("failed to list order taxes: %v", err), nil
	}
	return shared.ToolJSON(map[string]any{
		"order_id": orderID,
		"total":    len(rows),
		"taxes":    rows,
	})
}

type paging struct {
	Page  int
	Limit int
}

func readPaging(args map[string]any) (paging, error) {
	out := paging{}
	if v, ok, err := readOptionalPositiveInt(args, "page"); err != nil {
		return out, err
	} else if ok {
		out.Page = v
	}
	if v, ok, err := readOptionalPositiveInt(args, "limit"); err != nil {
		return out, err
	} else if ok {
		if v > maxOrdersListLimit {
			return out, fmt.Errorf("limit must be <= %d", maxOrdersListLimit)
		}
		out.Limit = v
	}
	return out, nil
}
