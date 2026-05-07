package catalog

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// ProductCreateParams holds parsed arguments for the product create tool.
// ChannelIDs is an optional MSF additive side-effect: after the product is
// created, a PUT to /v3/catalog/products/channel-assignments associates the
// new product with each listed channel. Existing assignments are unaffected.
type ProductCreateParams struct {
	Payload    bigcommerce.ProductCreate
	ChannelIDs []int
	Confirmed  bool
}

func parseProductCreateParams(args map[string]any) (*ProductCreateParams, error) {
	p := &ProductCreateParams{}

	name, ok := args["name"]
	if !ok {
		return nil, fmt.Errorf("name is required")
	}
	s, sOk := name.(string)
	if !sOk || s == "" {
		return nil, fmt.Errorf("name must be a non-empty string")
	}
	p.Payload.Name = s

	if v, ok := args["type"]; ok {
		t, tOk := v.(string)
		if !tOk || t == "" {
			return nil, fmt.Errorf("type must be a non-empty string")
		}
		valid := map[string]bool{"physical": true, "digital": true}
		if !valid[t] {
			return nil, fmt.Errorf("type must be 'physical' or 'digital'")
		}
		p.Payload.Type = t
	} else {
		p.Payload.Type = "physical"
	}

	if v, ok := args["weight"].(float64); ok {
		p.Payload.Weight = v
	}
	if v, ok := args["price"].(float64); ok {
		p.Payload.Price = v
	}
	if v, ok := args["sku"].(string); ok {
		p.Payload.SKU = v
	}
	if v, ok := args["description"].(string); ok {
		p.Payload.Description = v
	}

	// Dimensions
	if v, ok := args["width"].(float64); ok {
		p.Payload.Width = v
	}
	if v, ok := args["height"].(float64); ok {
		p.Payload.Height = v
	}
	if v, ok := args["depth"].(float64); ok {
		p.Payload.Depth = v
	}

	// Pricing
	if v, ok := args["cost_price"].(float64); ok {
		p.Payload.CostPrice = v
	}
	if v, ok := args["retail_price"].(float64); ok {
		p.Payload.RetailPrice = v
	}
	if v, ok := args["sale_price"].(float64); ok {
		p.Payload.SalePrice = v
	}
	if v, ok := args["map_price"].(float64); ok {
		p.Payload.MapPrice = v
	}
	if v, ok := args["tax_class_id"].(float64); ok {
		p.Payload.TaxClassID = int(v)
	}

	// Categories and brand
	if v, ok := args["category_ids"]; ok {
		ids, err := parseFloat64SliceToPositiveInts(v, "category_ids")
		if err != nil {
			return nil, err
		}
		p.Payload.Categories = ids
	}
	if v, ok := args["brand_id"].(float64); ok {
		p.Payload.BrandID = int(v)
	}

	// Inventory
	if v, ok := args["inventory_tracking"].(string); ok {
		p.Payload.InventoryTracking = v
	}
	if v, ok := args["inventory_level"].(float64); ok {
		p.Payload.InventoryLevel = int(v)
	}
	if v, ok := args["inventory_warning_level"].(float64); ok {
		p.Payload.InventoryWarningLevel = int(v)
	}

	// Storefront
	if v, ok := args["is_visible"].(bool); ok {
		p.Payload.IsVisible = &v
	}
	if v, ok := args["is_featured"].(bool); ok {
		p.Payload.IsFeatured = &v
	}
	if v, ok := args["sort_order"].(float64); ok {
		p.Payload.SortOrder = int(v)
	}
	if v, ok := args["condition"].(string); ok {
		p.Payload.Condition = v
	}
	if v, ok := args["is_condition_shown"].(bool); ok {
		p.Payload.IsConditionShown = &v
	}

	// SEO
	if v, ok := args["page_title"].(string); ok {
		p.Payload.PageTitle = v
	}
	if v, ok := args["meta_description"].(string); ok {
		p.Payload.MetaDescription = v
	}
	if v, ok := args["search_keywords"].(string); ok {
		p.Payload.SearchKeywords = v
	}

	// Availability / preorder
	if v, ok := args["availability"].(string); ok {
		p.Payload.Availability = v
	}
	if v, ok := args["availability_description"].(string); ok {
		p.Payload.AvailabilityDescription = v
	}
	if v, ok := args["is_preorder_only"].(bool); ok {
		p.Payload.IsPreorderOnly = &v
	}
	if v, ok := args["preorder_message"].(string); ok {
		p.Payload.PreorderMessage = v
	}
	if v, ok := args["preorder_release_date"].(string); ok {
		p.Payload.PreorderReleaseDate = v
	}

	// Shipping
	if v, ok := args["is_free_shipping"].(bool); ok {
		p.Payload.IsFreeShipping = &v
	}
	if v, ok := args["fixed_cost_shipping_price"].(float64); ok {
		p.Payload.FixedCostShippingPrice = v
	}

	// Identifiers
	if v, ok := args["upc"].(string); ok {
		p.Payload.UPC = v
	}
	if v, ok := args["gtin"].(string); ok {
		p.Payload.GTIN = v
	}
	if v, ok := args["mpn"].(string); ok {
		p.Payload.MPN = v
	}
	if v, ok := args["bin_picking_number"].(string); ok {
		p.Payload.BinPickingNumber = v
	}

	// Misc
	if v, ok := args["warranty"].(string); ok {
		p.Payload.Warranty = v
	}
	if v, ok := args["order_quantity_minimum"].(float64); ok {
		p.Payload.OrderQuantityMinimum = int(v)
	}
	if v, ok := args["order_quantity_maximum"].(float64); ok {
		p.Payload.OrderQuantityMaximum = int(v)
	}
	if v, ok := args["gift_wrapping_options_type"].(string); ok {
		p.Payload.GiftWrappingOptionsType = v
	}
	if v, ok := args["gift_wrapping_options_list"]; ok {
		ids, err := parseFloat64SliceToNonNegativeInts(v, "gift_wrapping_options_list")
		if err != nil {
			return nil, err
		}
		p.Payload.GiftWrappingOptionsList = ids
	}
	if v, ok := args["related_products"]; ok {
		ids, err := parseFloat64SliceToNonNegativeInts(v, "related_products")
		if err != nil {
			return nil, err
		}
		p.Payload.RelatedProducts = ids
	}

	// Open Graph
	if v, ok := args["open_graph_type"].(string); ok {
		p.Payload.OpenGraphType = v
	}
	if v, ok := args["open_graph_title"].(string); ok {
		p.Payload.OpenGraphTitle = v
	}
	if v, ok := args["open_graph_description"].(string); ok {
		p.Payload.OpenGraphDescription = v
	}
	if v, ok := args["open_graph_use_meta_description"].(bool); ok {
		p.Payload.OpenGraphUseMetaDesc = &v
	}
	if v, ok := args["open_graph_use_product_name"].(bool); ok {
		p.Payload.OpenGraphUseProductName = &v
	}
	if v, ok := args["open_graph_use_image"].(bool); ok {
		p.Payload.OpenGraphUseImage = &v
	}

	if v, ok := args["layout_file"].(string); ok {
		p.Payload.LayoutFile = v
	}

	// Inline images (URL-based)
	if v, ok := args["images"]; ok {
		arr, aOk := v.([]any)
		if !aOk {
			return nil, fmt.Errorf("images must be an array")
		}
		for i, item := range arr {
			m, mOk := item.(map[string]any)
			if !mOk {
				return nil, fmt.Errorf("images[%d] must be an object with at least an image_url field", i)
			}
			url, _ := m["image_url"].(string)
			if url == "" {
				return nil, fmt.Errorf("images[%d].image_url is required and must be a non-empty string", i)
			}
			img := bigcommerce.ProductImageCreate{ImageURL: url}
			if t, ok := m["is_thumbnail"].(bool); ok {
				img.IsThumbnail = t
			}
			if d, ok := m["description"].(string); ok {
				img.Description = d
			}
			if so, ok := m["sort_order"].(float64); ok {
				img.SortOrder = int(so)
			}
			p.Payload.Images = append(p.Payload.Images, img)
		}
	}

	if v, ok := args["channel_ids"]; ok && v != nil {
		ids, err := parseFloat64SliceToPositiveInts(v, "channel_ids")
		if err != nil {
			return nil, err
		}
		if len(ids) > maxChannelAssignListChannels {
			return nil, fmt.Errorf("channel_ids: maximum %d channels per call", maxChannelAssignListChannels)
		}
		p.ChannelIDs = ids
	}

	p.Confirmed = middleware.IsConfirmedFromArgs(args)
	return p, nil
}

func (p *Products) handleCreate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	params, err := parseProductCreateParams(request.GetArguments())
	if err != nil {
		return toolError("%s", err.Error()), nil
	}

	if !params.Confirmed {
		return p.previewCreate(params)
	}
	return p.executeCreate(ctx, params)
}

func (p *Products) previewCreate(params *ProductCreateParams) (*mcp.CallToolResult, error) {
	preview := map[string]any{
		"status":  "pending_confirmation",
		"product": params.Payload,
		"message": fmt.Sprintf(
			"Product %q will be created. Pass confirmed=true to execute.",
			params.Payload.Name,
		),
	}
	if len(params.ChannelIDs) > 0 {
		preview["channel_assignments_preview"] = map[string]any{
			"channel_ids": params.ChannelIDs,
			"effect": "After successful creation, PUT /v3/catalog/products/channel-assignments " +
				"will additively assign the new product to these channel IDs (existing assignments preserved).",
		}
	}
	return toolJSON(preview)
}

func (p *Products) executeCreate(ctx context.Context, params *ProductCreateParams) (*mcp.CallToolResult, error) {
	product, err := p.bc.CreateProduct(ctx, params.Payload)
	if err != nil {
		return toolError("create product failed: %v", err), nil
	}

	resp := map[string]any{
		"status": "created",
		"product": map[string]any{
			"id":         product.ID,
			"name":       product.Name,
			"sku":        product.SKU,
			"price":      product.Price,
			"is_visible": product.IsVisible,
		},
		"message": fmt.Sprintf("Product %q created with ID %d.", product.Name, product.ID),
	}

	if len(params.ChannelIDs) > 0 {
		assignments := make([]bigcommerce.ProductChannelAssignment, 0, len(params.ChannelIDs))
		for _, cid := range params.ChannelIDs {
			assignments = append(assignments, bigcommerce.ProductChannelAssignment{
				ProductID: product.ID,
				ChannelID: cid,
			})
		}
		if err := p.bc.UpsertProductChannelAssignments(ctx, assignments); err != nil {
			resp["status"] = "partial_success"
			resp["channel_assignments"] = map[string]any{
				"status":      "failed",
				"channel_ids": params.ChannelIDs,
				"error":       err.Error(),
			}
			resp["message"] = fmt.Sprintf(
				"Product %q created (ID %d) but the additive channel assignment to %d channel(s) FAILED: %v. "+
					"Re-run catalog/products/channel_assignments/assign for product_id=%d to retry.",
				product.Name, product.ID, len(params.ChannelIDs), err, product.ID,
			)
			return toolJSON(resp)
		}
		resp["channel_assignments"] = map[string]any{
			"status":      "completed",
			"channel_ids": params.ChannelIDs,
		}
		resp["message"] = fmt.Sprintf(
			"Product %q created with ID %d and additively assigned to %d channel(s).",
			product.Name, product.ID, len(params.ChannelIDs),
		)
	}

	return toolJSON(resp)
}
