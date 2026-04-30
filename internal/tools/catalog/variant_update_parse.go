package catalog

import "github.com/roel-c/bc-admin-mcp/internal/bigcommerce"

// ApplyProductVariantUpdateFromMap fills dst from src using the same field keys as
// catalog/products/variants/update and rows in catalog/variants/bulk_update.
// Ignores keys that are not variant fields (e.g. product_id, variant_id, confirmed).
// Returns true if at least one writable field was set.
func ApplyProductVariantUpdateFromMap(src map[string]any, dst *bigcommerce.ProductVariantUpdate) bool {
	has := false
	if v, ok := src["sku"].(string); ok {
		dst.SKU = &v
		has = true
	}
	setFloatChangeMap(src, "price", &dst.Price, &has)
	setFloatChangeMap(src, "cost_price", &dst.CostPrice, &has)
	setFloatChangeMap(src, "sale_price", &dst.SalePrice, &has)
	setFloatChangeMap(src, "retail_price", &dst.RetailPrice, &has)
	setFloatChangeMap(src, "map_price", &dst.MapPrice, &has)
	setFloatChangeMap(src, "weight", &dst.Weight, &has)
	setFloatChangeMap(src, "width", &dst.Width, &has)
	setFloatChangeMap(src, "height", &dst.Height, &has)
	setFloatChangeMap(src, "depth", &dst.Depth, &has)
	setIntChangeMap(src, "inventory_level", &dst.InventoryLevel, &has)
	setIntChangeMap(src, "inventory_warning_level", &dst.InventoryWarningLevel, &has)
	if v, ok := src["bin_picking_number"].(string); ok {
		dst.BinPickingNumber = &v
		has = true
	}
	if v, ok := src["upc"].(string); ok {
		dst.UPC = &v
		has = true
	}
	if v, ok := src["gtin"].(string); ok {
		dst.GTIN = &v
		has = true
	}
	if v, ok := src["mpn"].(string); ok {
		dst.MPN = &v
		has = true
	}
	if v, ok := src["image_url"].(string); ok {
		dst.ImageURL = &v
		has = true
	}
	if v, ok := src["purchasing_disabled"].(bool); ok {
		dst.PurchasingDisabled = &v
		has = true
	}
	if v, ok := src["purchasing_disabled_message"].(string); ok {
		dst.PurchasingDisabledMsg = &v
		has = true
	}
	return has
}

// HasProductVariantUpdateChanges reports whether dst has any field set.
func HasProductVariantUpdateChanges(dst *bigcommerce.ProductVariantUpdate) bool {
	if dst == nil {
		return false
	}
	p := *dst
	return p.SKU != nil || p.Price != nil || p.CostPrice != nil || p.SalePrice != nil ||
		p.RetailPrice != nil || p.MapPrice != nil || p.Weight != nil || p.Width != nil ||
		p.Height != nil || p.Depth != nil || p.InventoryLevel != nil || p.InventoryWarningLevel != nil ||
		p.BinPickingNumber != nil || p.UPC != nil || p.GTIN != nil || p.MPN != nil ||
		p.ImageURL != nil || p.PurchasingDisabled != nil || p.PurchasingDisabledMsg != nil
}

func setFloatChangeMap(src map[string]any, key string, dst **float64, changed *bool) {
	if v, ok := src[key].(float64); ok {
		x := v
		*dst = &x
		*changed = true
	}
}

func setIntChangeMap(src map[string]any, key string, dst **int, changed *bool) {
	if v, ok := src[key].(float64); ok {
		i := int(v)
		*dst = &i
		*changed = true
	}
}
