package bigcommerce

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// APIError represents a non-retryable BigCommerce API error (4xx).
// Path / Method are populated by the client when available so that the
// error message can include scope hints relevant to the failing endpoint.
type APIError struct {
	StatusCode int
	Body       []byte
	Path       string // e.g. "catalog/products/channel-assignments"
	Method     string // GET / PUT / POST / DELETE
}

func (e *APIError) Error() string {
	return e.SafeError()
}

// SafeError returns a message suitable for returning to external callers
// (LLM / end-user) without leaking internal response details.
func (e *APIError) SafeError() string {
	hint := scopeHint(e.StatusCode, e.Method, e.Path)
	if hint != "" {
		return fmt.Sprintf("BigCommerce API returned status %d (%s)", e.StatusCode, hint)
	}
	return fmt.Sprintf("BigCommerce API returned status %d", e.StatusCode)
}

// scopeHint returns a short OAuth-scope or routing hint for common 4xx errors.
// Hints are derived from BigCommerce's documented scopes per endpoint family.
// Returns "" when the path/status combination has no actionable hint.
func scopeHint(status int, method, path string) string {
	if path == "" {
		return ""
	}
	p := strings.TrimPrefix(path, "/")
	read := method == "" || method == http.MethodGet
	switch {
	case status == http.StatusUnauthorized:
		return "401 — the X-Auth-Token is missing, invalid, or revoked. Re-issue an API account token."
	case status == http.StatusForbidden:
		return forbiddenScopeHint(p, read)
	case status == http.StatusNotFound && strings.HasPrefix(p, "channels/") && strings.Contains(p, "/listings"):
		return "404 — the channel_id may not exist, or the store has no listings on this channel. Verify with catalog/channels/list."
	case status == http.StatusNotFound && strings.HasPrefix(p, "catalog/trees"):
		return "404 — tree not found. Verify with catalog/channels/category_trees (and the channel's tree_id)."
	case status == http.StatusNotFound && strings.HasPrefix(p, "customer_groups/"):
		return "404 — customer group not found. List groups via customers/groups/list to confirm the ID."
	}
	return ""
}

func forbiddenScopeHint(path string, read bool) string {
	switch {
	case strings.HasPrefix(path, "channels") && strings.Contains(path, "/listings"):
		if read {
			return "403 — token likely missing 'store_channel_listings_read_only' (or 'store_channel_listings'). Update the API account scope."
		}
		return "403 — write requires 'store_channel_listings'. Update the API account scope."
	case path == "channels" || strings.HasPrefix(path, "channels"):
		if read {
			return "403 — token likely missing 'store_channel_settings_read_only' (or 'store_channel_settings'). Update the API account scope."
		}
		return "403 — write requires 'store_channel_settings'. Update the API account scope."
	case strings.HasPrefix(path, "catalog/products/channel-assignments"):
		if read {
			return "403 — token likely missing 'store_v2_products_read_only' (or 'store_v2_products'). Update the API account scope."
		}
		return "403 — write requires 'store_v2_products'. Update the API account scope."
	case strings.HasPrefix(path, "catalog/products") || strings.HasPrefix(path, "catalog/categories") || strings.HasPrefix(path, "catalog/brands") || strings.HasPrefix(path, "catalog/trees") || strings.HasPrefix(path, "catalog/variants"):
		if read {
			return "403 — token likely missing 'store_v2_products_read_only' (or 'store_v2_products'). Update the API account scope."
		}
		return "403 — write requires 'store_v2_products'. Update the API account scope."
	case strings.HasPrefix(path, "orders/") && (strings.Contains(path, "/payment_actions") || strings.Contains(path, "/transactions")):
		if read {
			return "403 — token likely missing order transactions read scopes ('store_v2_orders_read_only' or 'store_v2_transactions_read_only')."
		}
		return "403 — write requires order transaction scope ('store_v2_orders' or 'store_v2_transactions')."
	case strings.HasPrefix(path, "orders"):
		if read {
			return "403 — token likely missing 'store_v2_orders_read_only' (or 'store_v2_orders')."
		}
		return "403 — write requires 'store_v2_orders'."
	case strings.HasPrefix(path, "segments") || strings.HasPrefix(path, "shopper-profiles"):
		if read {
			return "403 — token likely missing 'store_v2_customers_read_only' (or 'store_v2_customers'). Customer Segmentation is also an Enterprise-only feature; verify the store plan supports /v3/segments."
		}
		return "403 — write requires 'store_v2_customers'. Customer Segmentation is an Enterprise-only feature; verify the store plan supports /v3/segments."
	case strings.HasPrefix(path, "promotions"):
		if read {
			return "403 — token likely missing 'store_v2_marketing_read_only' (or 'store_v2_marketing'). Promotions live on the V2 marketing scope (separate from customers/catalog)."
		}
		return "403 — write requires 'store_v2_marketing'. Promotions live on the V2 marketing scope (separate from customers/catalog)."
	case strings.HasPrefix(path, "pricelists"):
		return "403 — token likely missing 'store_price_lists'. Update the API account scope."
	case strings.HasPrefix(path, "inventory"):
		if read {
			return "403 — token likely missing 'store_inventory'. Update the API account scope."
		}
		return "403 — write requires 'store_inventory'. Update the API account scope."
	case strings.HasPrefix(path, "customers"):
		if read {
			return "403 — token likely missing 'store_v2_customers_read_only' (or 'store_v2_customers')."
		}
		return "403 — write requires 'store_v2_customers'."
	case strings.HasPrefix(path, "customer_groups"):
		if read {
			return "403 — token likely missing 'store_v2_customers_read_only' (or 'store_v2_customers'). Customer Groups live on the V2 customers scope."
		}
		return "403 — write requires 'store_v2_customers'. Customer Groups live on the V2 customers scope."
	}
	return "403 — the API token is missing the OAuth scope required for this endpoint."
}

// PaginatedResponse wraps the standard BC V3 paginated envelope.
type PaginatedResponse struct {
	Data []json.RawMessage `json:"data"`
	Meta PaginationMeta    `json:"meta"`
}

type PaginationMeta struct {
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	Total       int `json:"total"`
	Count       int `json:"count"`
	PerPage     int `json:"per_page"`
	CurrentPage int `json:"current_page"`
	TotalPages  int `json:"total_pages"`
}

// SingleResponse wraps a non-paginated BC V3 response.
type SingleResponse struct {
	Data json.RawMessage `json:"data"`
	Meta json.RawMessage `json:"meta,omitempty"`
}

// BatchResult tracks outcomes of a chunked batch operation.
type BatchResult struct {
	Succeeded int
	Failed    int
	Errors    []BatchError
	Responses [][]byte
}

type BatchError struct {
	Offset int
	Count  int
	Err    string
}

// Product represents a BigCommerce catalog product with all fields needed
// for reads, previews, and diff generation.
type Product struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	Type            string  `json:"type,omitempty"`
	SKU             string  `json:"sku,omitempty"`
	Description     string  `json:"description,omitempty"`
	Weight          float64 `json:"weight,omitempty"`
	Width           float64 `json:"width,omitempty"`
	Height          float64 `json:"height,omitempty"`
	Depth           float64 `json:"depth,omitempty"`
	Price           float64 `json:"price"`
	CalculatedPrice float64 `json:"calculated_price,omitempty"`
	CostPrice       float64 `json:"cost_price,omitempty"`
	RetailPrice     float64 `json:"retail_price,omitempty"`
	SalePrice       float64 `json:"sale_price,omitempty"`
	MapPrice        float64 `json:"map_price,omitempty"`
	TaxClassID      int     `json:"tax_class_id,omitempty"`
	Categories      []int   `json:"categories,omitempty"`
	BrandID         int     `json:"brand_id,omitempty"`

	InventoryTracking     string `json:"inventory_tracking,omitempty"`
	InventoryLevel        int    `json:"inventory_level,omitempty"`
	InventoryWarningLevel int    `json:"inventory_warning_level,omitempty"`

	IsVisible        bool   `json:"is_visible,omitempty"`
	IsFeatured       bool   `json:"is_featured,omitempty"`
	SortOrder        int    `json:"sort_order,omitempty"`
	Condition        string `json:"condition,omitempty"`
	IsConditionShown bool   `json:"is_condition_shown,omitempty"`

	PageTitle       string     `json:"page_title,omitempty"`
	MetaDescription string     `json:"meta_description,omitempty"`
	SearchKeywords  string     `json:"search_keywords,omitempty"`
	CustomURL       *CustomURL `json:"custom_url,omitempty"`

	Availability            string `json:"availability,omitempty"`
	AvailabilityDescription string `json:"availability_description,omitempty"`
	IsPreorderOnly          bool   `json:"is_preorder_only,omitempty"`
	PreorderMessage         string `json:"preorder_message,omitempty"`
	PreorderReleaseDate     string `json:"preorder_release_date,omitempty"`

	IsFreeShipping         bool    `json:"is_free_shipping,omitempty"`
	FixedCostShippingPrice float64 `json:"fixed_cost_shipping_price,omitempty"`

	UPC              string `json:"upc,omitempty"`
	GTIN             string `json:"gtin,omitempty"`
	MPN              string `json:"mpn,omitempty"`
	BinPickingNumber string `json:"bin_picking_number,omitempty"`

	Warranty             string `json:"warranty,omitempty"`
	OrderQuantityMinimum int    `json:"order_quantity_minimum,omitempty"`
	OrderQuantityMaximum int    `json:"order_quantity_maximum,omitempty"`

	GiftWrappingOptionsType string `json:"gift_wrapping_options_type,omitempty"`
	GiftWrappingOptionsList []int  `json:"gift_wrapping_options_list,omitempty"`
	RelatedProducts         []int  `json:"related_products,omitempty"`

	OpenGraphType           string `json:"open_graph_type,omitempty"`
	OpenGraphTitle          string `json:"open_graph_title,omitempty"`
	OpenGraphDescription    string `json:"open_graph_description,omitempty"`
	OpenGraphUseMetaDesc    bool   `json:"open_graph_use_meta_description,omitempty"`
	OpenGraphUseProductName bool   `json:"open_graph_use_product_name,omitempty"`
	OpenGraphUseImage       bool   `json:"open_graph_use_image,omitempty"`

	LayoutFile string `json:"layout_file,omitempty"`
}

// CustomURL handles the URL object returned by both product and category
// endpoints. Products use an inner "url" field, while the Category Tree
// endpoint uses "path". Both are deserialized; GetPath() returns whichever
// is populated.
type CustomURL struct {
	URL          string `json:"url"`
	Path         string `json:"path"`
	IsCustomized bool   `json:"is_customized"`
}

// GetPath returns the URL path regardless of which API shape populated it.
func (u *CustomURL) GetPath() string {
	if u.Path != "" {
		return u.Path
	}
	return u.URL
}

// ProductUpdate is the payload for a batch product update via
// PUT /v3/catalog/products. Pointer fields allow distinguishing "not
// included" (nil) from "set to zero/empty" (&0 / &"").
type ProductUpdate struct {
	ID          int     `json:"id"`
	Name        *string `json:"name,omitempty"`
	Type        *string `json:"type,omitempty"`
	SKU         *string `json:"sku,omitempty"`
	Description *string `json:"description,omitempty"`

	Weight *float64 `json:"weight,omitempty"`
	Width  *float64 `json:"width,omitempty"`
	Height *float64 `json:"height,omitempty"`
	Depth  *float64 `json:"depth,omitempty"`

	Price       *float64 `json:"price,omitempty"`
	CostPrice   *float64 `json:"cost_price,omitempty"`
	RetailPrice *float64 `json:"retail_price,omitempty"`
	SalePrice   *float64 `json:"sale_price,omitempty"`
	MapPrice    *float64 `json:"map_price,omitempty"`
	TaxClassID  *int     `json:"tax_class_id,omitempty"`

	Categories []int `json:"categories,omitempty"`
	BrandID    *int  `json:"brand_id,omitempty"`

	InventoryTracking     *string `json:"inventory_tracking,omitempty"`
	InventoryLevel        *int    `json:"inventory_level,omitempty"`
	InventoryWarningLevel *int    `json:"inventory_warning_level,omitempty"`

	IsVisible        *bool   `json:"is_visible,omitempty"`
	IsFeatured       *bool   `json:"is_featured,omitempty"`
	SortOrder        *int    `json:"sort_order,omitempty"`
	Condition        *string `json:"condition,omitempty"`
	IsConditionShown *bool   `json:"is_condition_shown,omitempty"`

	PageTitle       *string    `json:"page_title,omitempty"`
	MetaDescription *string    `json:"meta_description,omitempty"`
	SearchKeywords  *string    `json:"search_keywords,omitempty"`
	CustomURL       *CustomURL `json:"custom_url,omitempty"`

	Availability            *string `json:"availability,omitempty"`
	AvailabilityDescription *string `json:"availability_description,omitempty"`
	IsPreorderOnly          *bool   `json:"is_preorder_only,omitempty"`
	PreorderMessage         *string `json:"preorder_message,omitempty"`
	PreorderReleaseDate     *string `json:"preorder_release_date,omitempty"`

	IsFreeShipping         *bool    `json:"is_free_shipping,omitempty"`
	FixedCostShippingPrice *float64 `json:"fixed_cost_shipping_price,omitempty"`

	UPC              *string `json:"upc,omitempty"`
	GTIN             *string `json:"gtin,omitempty"`
	MPN              *string `json:"mpn,omitempty"`
	BinPickingNumber *string `json:"bin_picking_number,omitempty"`

	Warranty             *string `json:"warranty,omitempty"`
	OrderQuantityMinimum *int    `json:"order_quantity_minimum,omitempty"`
	OrderQuantityMaximum *int    `json:"order_quantity_maximum,omitempty"`

	GiftWrappingOptionsType *string `json:"gift_wrapping_options_type,omitempty"`
	GiftWrappingOptionsList []int   `json:"gift_wrapping_options_list,omitempty"`
	RelatedProducts         []int   `json:"related_products,omitempty"`

	OpenGraphType           *string `json:"open_graph_type,omitempty"`
	OpenGraphTitle          *string `json:"open_graph_title,omitempty"`
	OpenGraphDescription    *string `json:"open_graph_description,omitempty"`
	OpenGraphUseMetaDesc    *bool   `json:"open_graph_use_meta_description,omitempty"`
	OpenGraphUseProductName *bool   `json:"open_graph_use_product_name,omitempty"`
	OpenGraphUseImage       *bool   `json:"open_graph_use_image,omitempty"`

	LayoutFile *string `json:"layout_file,omitempty"`
}

// ProductCreate is the payload for POST /v3/catalog/products.
// Required fields: Name, Type, Weight.
type ProductCreate struct {
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Weight      float64 `json:"weight"`
	SKU         string  `json:"sku,omitempty"`
	Description string  `json:"description,omitempty"`

	Width  float64 `json:"width,omitempty"`
	Height float64 `json:"height,omitempty"`
	Depth  float64 `json:"depth,omitempty"`

	Price       float64 `json:"price,omitempty"`
	CostPrice   float64 `json:"cost_price,omitempty"`
	RetailPrice float64 `json:"retail_price,omitempty"`
	SalePrice   float64 `json:"sale_price,omitempty"`
	MapPrice    float64 `json:"map_price,omitempty"`
	TaxClassID  int     `json:"tax_class_id,omitempty"`

	Categories []int `json:"categories,omitempty"`
	BrandID    int   `json:"brand_id,omitempty"`

	InventoryTracking     string `json:"inventory_tracking,omitempty"`
	InventoryLevel        int    `json:"inventory_level,omitempty"`
	InventoryWarningLevel int    `json:"inventory_warning_level,omitempty"`

	IsVisible        *bool  `json:"is_visible,omitempty"`
	IsFeatured       *bool  `json:"is_featured,omitempty"`
	SortOrder        int    `json:"sort_order,omitempty"`
	Condition        string `json:"condition,omitempty"`
	IsConditionShown *bool  `json:"is_condition_shown,omitempty"`

	PageTitle       string `json:"page_title,omitempty"`
	MetaDescription string `json:"meta_description,omitempty"`
	SearchKeywords  string `json:"search_keywords,omitempty"`

	Availability            string `json:"availability,omitempty"`
	AvailabilityDescription string `json:"availability_description,omitempty"`
	IsPreorderOnly          *bool  `json:"is_preorder_only,omitempty"`
	PreorderMessage         string `json:"preorder_message,omitempty"`
	PreorderReleaseDate     string `json:"preorder_release_date,omitempty"`

	IsFreeShipping         *bool   `json:"is_free_shipping,omitempty"`
	FixedCostShippingPrice float64 `json:"fixed_cost_shipping_price,omitempty"`

	UPC              string `json:"upc,omitempty"`
	GTIN             string `json:"gtin,omitempty"`
	MPN              string `json:"mpn,omitempty"`
	BinPickingNumber string `json:"bin_picking_number,omitempty"`

	Warranty             string `json:"warranty,omitempty"`
	OrderQuantityMinimum int    `json:"order_quantity_minimum,omitempty"`
	OrderQuantityMaximum int    `json:"order_quantity_maximum,omitempty"`

	GiftWrappingOptionsType string `json:"gift_wrapping_options_type,omitempty"`
	GiftWrappingOptionsList []int  `json:"gift_wrapping_options_list,omitempty"`
	RelatedProducts         []int  `json:"related_products,omitempty"`

	OpenGraphType           string `json:"open_graph_type,omitempty"`
	OpenGraphTitle          string `json:"open_graph_title,omitempty"`
	OpenGraphDescription    string `json:"open_graph_description,omitempty"`
	OpenGraphUseMetaDesc    *bool  `json:"open_graph_use_meta_description,omitempty"`
	OpenGraphUseProductName *bool  `json:"open_graph_use_product_name,omitempty"`
	OpenGraphUseImage       *bool  `json:"open_graph_use_image,omitempty"`

	LayoutFile string `json:"layout_file,omitempty"`

	Images []ProductImageCreate `json:"images,omitempty"`
}

// Variant represents a product variant (compact form used by batch listing).
type Variant struct {
	ID        int     `json:"id"`
	ProductID int     `json:"product_id"`
	SKU       string  `json:"sku,omitempty"`
	Price     float64 `json:"price"`
}

// ProductVariantFull is the expanded variant representation including all
// fields returned by GET /v3/catalog/products/{id}/variants/{vid}.
type ProductVariantFull struct {
	ID                    int                `json:"id"`
	ProductID             int                `json:"product_id"`
	SKU                   string             `json:"sku,omitempty"`
	Price                 *float64           `json:"price,omitempty"`
	CalculatedPrice       float64            `json:"calculated_price,omitempty"`
	CostPrice             float64            `json:"cost_price,omitempty"`
	RetailPrice           float64            `json:"retail_price,omitempty"`
	SalePrice             float64            `json:"sale_price,omitempty"`
	MapPrice              float64            `json:"map_price,omitempty"`
	Weight                *float64           `json:"weight,omitempty"`
	Width                 *float64           `json:"width,omitempty"`
	Height                *float64           `json:"height,omitempty"`
	Depth                 *float64           `json:"depth,omitempty"`
	InventoryLevel        int                `json:"inventory_level,omitempty"`
	InventoryWarningLevel int                `json:"inventory_warning_level,omitempty"`
	BinPickingNumber      string             `json:"bin_picking_number,omitempty"`
	UPC                   string             `json:"upc,omitempty"`
	GTIN                  string             `json:"gtin,omitempty"`
	MPN                   string             `json:"mpn,omitempty"`
	PurchasingDisabled    bool               `json:"purchasing_disabled,omitempty"`
	PurchasingDisabledMsg string             `json:"purchasing_disabled_message,omitempty"`
	ImageURL              string             `json:"image_url,omitempty"`
	OptionValues          []VariantOptionVal `json:"option_values,omitempty"`
}

// VariantOptionVal represents a single option-value pair on a variant.
type VariantOptionVal struct {
	ID                int    `json:"id,omitempty"`
	OptionID          int    `json:"option_id,omitempty"`
	OptionDisplayName string `json:"option_display_name,omitempty"`
	Label             string `json:"label,omitempty"`
}

// ProductVariantCreate is the payload for POST /v3/catalog/products/{id}/variants.
type ProductVariantCreate struct {
	SKU                   string             `json:"sku,omitempty"`
	Price                 *float64           `json:"price,omitempty"`
	CostPrice             *float64           `json:"cost_price,omitempty"`
	SalePrice             *float64           `json:"sale_price,omitempty"`
	RetailPrice           *float64           `json:"retail_price,omitempty"`
	MapPrice              *float64           `json:"map_price,omitempty"`
	Weight                *float64           `json:"weight,omitempty"`
	Width                 *float64           `json:"width,omitempty"`
	Height                *float64           `json:"height,omitempty"`
	Depth                 *float64           `json:"depth,omitempty"`
	InventoryLevel        *int               `json:"inventory_level,omitempty"`
	InventoryWarningLevel *int               `json:"inventory_warning_level,omitempty"`
	BinPickingNumber      string             `json:"bin_picking_number,omitempty"`
	UPC                   string             `json:"upc,omitempty"`
	GTIN                  string             `json:"gtin,omitempty"`
	MPN                   string             `json:"mpn,omitempty"`
	PurchasingDisabled    *bool              `json:"purchasing_disabled,omitempty"`
	PurchasingDisabledMsg string             `json:"purchasing_disabled_message,omitempty"`
	ImageURL              string             `json:"image_url,omitempty"`
	OptionValues          []VariantOptionVal `json:"option_values"`
}

// CatalogVariantUpdate is one element of the JSON body for batch
// PUT /v3/catalog/variants (global variant update). ID is required per row.
type CatalogVariantUpdate struct {
	ID int `json:"id"`
	ProductVariantUpdate
}

// ProductVariantUpdate is the payload for PUT /v3/catalog/products/{id}/variants/{vid}.
type ProductVariantUpdate struct {
	SKU                   *string  `json:"sku,omitempty"`
	Price                 *float64 `json:"price,omitempty"`
	CostPrice             *float64 `json:"cost_price,omitempty"`
	SalePrice             *float64 `json:"sale_price,omitempty"`
	RetailPrice           *float64 `json:"retail_price,omitempty"`
	MapPrice              *float64 `json:"map_price,omitempty"`
	Weight                *float64 `json:"weight,omitempty"`
	Width                 *float64 `json:"width,omitempty"`
	Height                *float64 `json:"height,omitempty"`
	Depth                 *float64 `json:"depth,omitempty"`
	InventoryLevel        *int     `json:"inventory_level,omitempty"`
	InventoryWarningLevel *int     `json:"inventory_warning_level,omitempty"`
	BinPickingNumber      *string  `json:"bin_picking_number,omitempty"`
	UPC                   *string  `json:"upc,omitempty"`
	GTIN                  *string  `json:"gtin,omitempty"`
	MPN                   *string  `json:"mpn,omitempty"`
	PurchasingDisabled    *bool    `json:"purchasing_disabled,omitempty"`
	PurchasingDisabledMsg *string  `json:"purchasing_disabled_message,omitempty"`
	ImageURL              *string  `json:"image_url,omitempty"`
}

// ---------------------------------------------------------------------------
// Product Images
// ---------------------------------------------------------------------------

// ProductImage represents a product image returned by the BC API.
type ProductImage struct {
	ID           int    `json:"id"`
	ProductID    int    `json:"product_id"`
	IsThumbnail  bool   `json:"is_thumbnail"`
	SortOrder    int    `json:"sort_order"`
	Description  string `json:"description,omitempty"`
	ImageFile    string `json:"image_file,omitempty"`
	URLZoom      string `json:"url_zoom,omitempty"`
	URLStandard  string `json:"url_standard,omitempty"`
	URLThumbnail string `json:"url_thumbnail,omitempty"`
	URLTiny      string `json:"url_tiny,omitempty"`
	DateModified string `json:"date_modified,omitempty"`
}

// ProductImageCreate is the payload for POST /v3/catalog/products/{id}/images
// using the JSON (URL-based) content type.
type ProductImageCreate struct {
	ImageURL    string `json:"image_url"`
	IsThumbnail bool   `json:"is_thumbnail,omitempty"`
	SortOrder   int    `json:"sort_order,omitempty"`
	Description string `json:"description,omitempty"`
}

// ---------------------------------------------------------------------------
// Product Custom Fields
// ---------------------------------------------------------------------------

type ProductCustomField struct {
	ID    int    `json:"id,omitempty"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type ProductCustomFieldCreate struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ---------------------------------------------------------------------------
// Product Options (variant-generating options)
// ---------------------------------------------------------------------------

type ProductOption struct {
	ID           int                  `json:"id"`
	ProductID    int                  `json:"product_id"`
	DisplayName  string               `json:"display_name"`
	Type         string               `json:"type"`
	SortOrder    int                  `json:"sort_order"`
	OptionValues []ProductOptionValue `json:"option_values,omitempty"`
}

type ProductOptionValue struct {
	ID        int    `json:"id,omitempty"`
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

type ProductOptionCreate struct {
	DisplayName  string               `json:"display_name"`
	Type         string               `json:"type"`
	SortOrder    int                  `json:"sort_order,omitempty"`
	OptionValues []ProductOptionValue `json:"option_values,omitempty"`
}

type ProductOptionUpdate struct {
	DisplayName  *string              `json:"display_name,omitempty"`
	SortOrder    *int                 `json:"sort_order,omitempty"`
	OptionValues []ProductOptionValue `json:"option_values,omitempty"`
}

// ---------------------------------------------------------------------------
// Product Modifiers
// ---------------------------------------------------------------------------

type ProductModifier struct {
	ID           int                    `json:"id"`
	ProductID    int                    `json:"product_id"`
	DisplayName  string                 `json:"display_name"`
	Type         string                 `json:"type"`
	Required     bool                   `json:"required"`
	SortOrder    int                    `json:"sort_order,omitempty"`
	Config       json.RawMessage        `json:"config,omitempty"`
	OptionValues []ProductModifierValue `json:"option_values,omitempty"`
}

type ProductModifierValue struct {
	ID        int    `json:"id,omitempty"`
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

type ProductModifierCreate struct {
	DisplayName  string                 `json:"display_name"`
	Type         string                 `json:"type"`
	Required     bool                   `json:"required,omitempty"`
	SortOrder    int                    `json:"sort_order,omitempty"`
	Config       json.RawMessage        `json:"config,omitempty"`
	OptionValues []ProductModifierValue `json:"option_values,omitempty"`
}

// Category represents a BigCommerce product category.
// The Category Tree endpoint (/v3/catalog/trees/categories) returns
// "category_id" while other endpoints use "id". We use a custom
// UnmarshalJSON to handle both shapes.
type Category struct {
	ID                 int        `json:"category_id,omitempty"`
	ParentID           int        `json:"parent_id,omitempty"`
	TreeID             int        `json:"tree_id,omitempty"`
	Name               string     `json:"name"`
	Description        string     `json:"description,omitempty"`
	PageTitle          string     `json:"page_title,omitempty"`
	MetaDescription    string     `json:"meta_description,omitempty"`
	SearchKeywords     string     `json:"search_keywords,omitempty"`
	IsVisible          bool       `json:"is_visible,omitempty"`
	SortOrder          int        `json:"sort_order,omitempty"`
	DefaultProductSort string     `json:"default_product_sort,omitempty"`
	ImageURL           string     `json:"image_url,omitempty"`
	URL                *CustomURL `json:"url,omitempty"`
}

// CategoryCreate is the payload for creating a new category via
// PUT /v3/catalog/trees/categories. Required: name, tree_id, parent_id.
type CategoryCreate struct {
	Name               string `json:"name"`
	TreeID             int    `json:"tree_id,omitempty"`
	ParentID           int    `json:"parent_id,omitempty"`
	Description        string `json:"description,omitempty"`
	PageTitle          string `json:"page_title,omitempty"`
	MetaDescription    string `json:"meta_description,omitempty"`
	SearchKeywords     string `json:"search_keywords,omitempty"`
	IsVisible          *bool  `json:"is_visible,omitempty"`
	SortOrder          int    `json:"sort_order,omitempty"`
	DefaultProductSort string `json:"default_product_sort,omitempty"`
	ImageURL           string `json:"image_url,omitempty"`
}

// CategoryUpdate is the payload for updating an existing category via
// PUT /v3/catalog/trees/categories. Pointer fields distinguish "not included"
// (nil) from "set to empty/zero".
type CategoryUpdate struct {
	CategoryID         int     `json:"category_id"`
	ParentID           *int    `json:"parent_id,omitempty"`
	Name               *string `json:"name,omitempty"`
	Description        *string `json:"description,omitempty"`
	PageTitle          *string `json:"page_title,omitempty"`
	MetaDescription    *string `json:"meta_description,omitempty"`
	SearchKeywords     *string `json:"search_keywords,omitempty"`
	IsVisible          *bool   `json:"is_visible,omitempty"`
	SortOrder          *int    `json:"sort_order,omitempty"`
	DefaultProductSort *string `json:"default_product_sort,omitempty"`
}

func (c *Category) UnmarshalJSON(data []byte) error {
	type alias Category
	aux := &struct {
		*alias
		AltID int `json:"id,omitempty"`
	}{alias: (*alias)(c)}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if c.ID == 0 && aux.AltID != 0 {
		c.ID = aux.AltID
	}
	return nil
}

// Brand represents a BigCommerce catalog brand (GET /v3/catalog/brands).
type Brand struct {
	ID              int        `json:"id"`
	Name            string     `json:"name"`
	PageTitle       string     `json:"page_title,omitempty"`
	MetaDescription string     `json:"meta_description,omitempty"`
	SearchKeywords  string     `json:"search_keywords,omitempty"`
	ImageURL        string     `json:"image_url,omitempty"`
	CustomURL       *CustomURL `json:"custom_url,omitempty"`
	LayoutFile      string     `json:"layout_file,omitempty"`
}

// BrandCreate is the payload for POST /v3/catalog/brands.
type BrandCreate struct {
	Name            string     `json:"name"`
	PageTitle       string     `json:"page_title,omitempty"`
	MetaDescription string     `json:"meta_description,omitempty"`
	SearchKeywords  string     `json:"search_keywords,omitempty"`
	ImageURL        string     `json:"image_url,omitempty"`
	CustomURL       *CustomURL `json:"custom_url,omitempty"`
	LayoutFile      string     `json:"layout_file,omitempty"`
}

// BrandUpdate is the payload for PUT /v3/catalog/brands/{id}.
// Pointer fields distinguish omitted fields from explicit clears where supported.
type BrandUpdate struct {
	Name            *string    `json:"name,omitempty"`
	PageTitle       *string    `json:"page_title,omitempty"`
	MetaDescription *string    `json:"meta_description,omitempty"`
	SearchKeywords  *string    `json:"search_keywords,omitempty"`
	ImageURL        *string    `json:"image_url,omitempty"`
	CustomURL       *CustomURL `json:"custom_url,omitempty"`
	LayoutFile      *string    `json:"layout_file,omitempty"`
}

// Metafield represents a custom key-value pair on supported BigCommerce
// resources (catalog, customer, order).
type Metafield struct {
	ID            int    `json:"id,omitempty"`
	Namespace     string `json:"namespace"`
	Key           string `json:"key"`
	Value         string `json:"value"`
	Description   string `json:"description,omitempty"`
	PermissionSet string `json:"permission_set,omitempty"`
	ResourceType  string `json:"resource_type,omitempty"`
	ResourceID    int    `json:"resource_id,omitempty"`
}

// CategoryAssignment maps a single product to a single category for the
// PUT /v3/catalog/products/category-assignments upsert endpoint.
type CategoryAssignment struct {
	ProductID  int `json:"product_id"`
	CategoryID int `json:"category_id"`
}

// ProductChannelAssignment maps a product to a sales channel for
// GET/PUT/DELETE /v3/catalog/products/channel-assignments.
type ProductChannelAssignment struct {
	ProductID int `json:"product_id"`
	ChannelID int `json:"channel_id"`
}

// PriceList represents a BigCommerce price list (GET /v3/pricelists).
type PriceList struct {
	ID           int    `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	Active       bool   `json:"active"`
	DateCreated  string `json:"date_created,omitempty"`
	DateModified string `json:"date_modified,omitempty"`
}

// PriceListCreate is the payload for POST /v3/pricelists.
type PriceListCreate struct {
	Name   string `json:"name"`
	Active *bool  `json:"active,omitempty"`
}

// PriceListUpdate is the payload for PUT /v3/pricelists/{price_list_id}.
type PriceListUpdate struct {
	Name   *string `json:"name,omitempty"`
	Active *bool   `json:"active,omitempty"`
}

// PriceListListParams encodes supported list filters for GET /v3/pricelists.
type PriceListListParams struct {
	ID              int
	IDs             []int
	Name            string
	NameLike        string
	DateCreated     string
	DateModified    string
	DateCreatedMin  string
	DateCreatedMax  string
	DateModifiedMin string
	DateModifiedMax string
	Page            int
	Limit           int
	Before          string
	After           string
}

// PriceListBulkPricingTier is a quantity-price break for a price record.
type PriceListBulkPricingTier struct {
	QuantityMin int     `json:"quantity_min"`
	QuantityMax *int    `json:"quantity_max,omitempty"`
	Type        string  `json:"type"`
	Amount      float64 `json:"amount"`
}

// PriceListRecord is the read shape for GET /v3/pricelists/{id}/records.
type PriceListRecord struct {
	PriceListID      int                        `json:"price_list_id,omitempty"`
	VariantID        int                        `json:"variant_id,omitempty"`
	ProductID        int                        `json:"product_id,omitempty"`
	SKU              string                     `json:"sku,omitempty"`
	Currency         string                     `json:"currency,omitempty"`
	Price            *float64                   `json:"price,omitempty"`
	SalePrice        *float64                   `json:"sale_price,omitempty"`
	RetailPrice      *float64                   `json:"retail_price,omitempty"`
	MapPrice         *float64                   `json:"map_price,omitempty"`
	CalculatedPrice  *float64                   `json:"calculated_price,omitempty"`
	BulkPricingTiers []PriceListBulkPricingTier `json:"bulk_pricing_tiers,omitempty"`
	DateCreated      string                     `json:"date_created,omitempty"`
	DateModified     string                     `json:"date_modified,omitempty"`
}

// PriceListRecordUpsert is one row for PUT /v3/pricelists/{id}/records.
type PriceListRecordUpsert struct {
	VariantID        int                        `json:"variant_id,omitempty"`
	SKU              string                     `json:"sku,omitempty"`
	Currency         string                     `json:"currency,omitempty"`
	Price            *float64                   `json:"price,omitempty"`
	SalePrice        *float64                   `json:"sale_price,omitempty"`
	RetailPrice      *float64                   `json:"retail_price,omitempty"`
	MapPrice         *float64                   `json:"map_price,omitempty"`
	BulkPricingTiers []PriceListBulkPricingTier `json:"bulk_pricing_tiers,omitempty"`
}

// PriceListRecordListParams encodes record filters for GET /pricelists/{id}/records.
type PriceListRecordListParams struct {
	VariantIDs []int
	ProductIDs []int
	SKU        string
	SKUs       []string
	Currency   string
	Currencies []string
	Include    []string
	Page       int
	Limit      int
	Before     string
	After      string
}

// PriceListRecordDeleteParams controls filtered deletes for
// DELETE /v3/pricelists/{id}/records.
type PriceListRecordDeleteParams struct {
	Currency   string
	VariantIDs []int
	SKUs       []string
}

// PriceListAssignment is one row from /v3/pricelists/assignments.
type PriceListAssignment struct {
	ID              int `json:"id,omitempty"`
	PriceListID     int `json:"price_list_id,omitempty"`
	CustomerGroupID int `json:"customer_group_id,omitempty"`
	ChannelID       int `json:"channel_id,omitempty"`
}

// PriceListAssignmentCreate is one row for batch create assignments.
type PriceListAssignmentCreate struct {
	PriceListID     int `json:"price_list_id"`
	CustomerGroupID int `json:"customer_group_id,omitempty"`
	ChannelID       int `json:"channel_id,omitempty"`
}

// PriceListAssignmentUpsert is the payload for
// PUT /v3/pricelists/{price_list_id}/assignments.
type PriceListAssignmentUpsert struct {
	CustomerGroupID int `json:"customer_group_id"`
	ChannelID       int `json:"channel_id"`
}

// PriceListAssignmentListParams encodes list filters for
// GET /v3/pricelists/assignments.
type PriceListAssignmentListParams struct {
	ID               int
	PriceListID      int
	CustomerGroupID  int
	ChannelID        int
	IDs              []int
	PriceListIDs     []int
	CustomerGroupIDs []int
	ChannelIDs       []int
	Page             int
	Limit            int
	Before           string
	After            string
}

// PriceListAssignmentDeleteParams controls filtered deletes for
// DELETE /v3/pricelists/assignments.
type PriceListAssignmentDeleteParams struct {
	ID              int
	PriceListID     int
	CustomerGroupID int
	ChannelID       int
	ChannelIDs      []int
}

// Order represents a BigCommerce order (V2 shape).
type Order struct {
	ID              int             `json:"id"`
	CustomerID      int             `json:"customer_id,omitempty"`
	Status          string          `json:"status,omitempty"`
	StatusID        int             `json:"status_id,omitempty"`
	ChannelID       int             `json:"channel_id,omitempty"`
	ExternalOrderID string          `json:"external_order_id,omitempty"`
	TotalExTax      string          `json:"total_ex_tax,omitempty"`
	TotalIncTax     string          `json:"total_inc_tax,omitempty"`
	DateCreated     string          `json:"date_created,omitempty"`
	DateModified    string          `json:"date_modified,omitempty"`
	ItemsTotal      int             `json:"items_total,omitempty"`
	PaymentMethod   string          `json:"payment_method,omitempty"`
	BillingAddress  json.RawMessage `json:"billing_address,omitempty"`
}

// OrderListParams controls filters for GET /v2/orders.
type OrderListParams struct {
	MinID             int
	MaxID             int
	MinTotal          float64
	MaxTotal          float64
	CustomerID        int
	Email             string
	StatusID          int
	CartID            string
	PaymentMethod     string
	MinDateCreated    string
	MaxDateCreated    string
	MinDateModified   string
	MaxDateModified   string
	ChannelID         int
	ExternalOrderID   string
	Sort              string
	Include           []string
	ConsignmentStruct string
	Page              int
	Limit             int
}

// OrderCountParams controls filters for GET /v2/orders/count.
type OrderCountParams struct {
	MinID           int
	MaxID           int
	MinTotal        float64
	MaxTotal        float64
	CustomerID      int
	Email           string
	StatusID        int
	CartID          string
	PaymentMethod   string
	MinDateCreated  string
	MaxDateCreated  string
	MinDateModified string
	MaxDateModified string
	ChannelID       int
	ExternalOrderID string
}

// OrderGetParams controls optional include parameters for GET /v2/orders/{id}.
type OrderGetParams struct {
	Include           []string
	ConsignmentStruct string
}

// OrderProductListParams controls paging for GET /v2/orders/{id}/products.
type OrderProductListParams struct {
	Page  int
	Limit int
}

// OrderProduct is one product line in a V2 order.
type OrderProduct struct {
	ID          int    `json:"id"`
	OrderID     int    `json:"order_id,omitempty"`
	ProductID   int    `json:"product_id,omitempty"`
	VariantID   int    `json:"variant_id,omitempty"`
	Name        string `json:"name,omitempty"`
	SKU         string `json:"sku,omitempty"`
	Quantity    int    `json:"quantity,omitempty"`
	PriceExTax  string `json:"price_ex_tax,omitempty"`
	PriceIncTax string `json:"price_inc_tax,omitempty"`
}

// OrderShipmentListParams controls paging for GET /v2/orders/{id}/shipments.
type OrderShipmentListParams struct {
	Page  int
	Limit int
}

// OrderShipmentItem is an item row used in shipment create/read payloads.
type OrderShipmentItem struct {
	OrderProductID int `json:"order_product_id"`
	ProductID      int `json:"product_id,omitempty"`
	Quantity       int `json:"quantity"`
}

// OrderShipment represents one shipment row for an order.
type OrderShipment struct {
	ID                    int                 `json:"id"`
	OrderID               int                 `json:"order_id,omitempty"`
	OrderAddressID        int                 `json:"order_address_id,omitempty"`
	TrackingNumber        string              `json:"tracking_number,omitempty"`
	ShippingProvider      string              `json:"shipping_provider,omitempty"`
	TrackingCarrier       string              `json:"tracking_carrier,omitempty"`
	TrackingLink          string              `json:"tracking_link,omitempty"`
	GeneratedTrackingLink string              `json:"generated_tracking_link,omitempty"`
	Comments              string              `json:"comments,omitempty"`
	DateCreated           string              `json:"date_created,omitempty"`
	DateModified          string              `json:"date_modified,omitempty"`
	Items                 []OrderShipmentItem `json:"items,omitempty"`
}

// OrderShipmentCreate is the payload for POST /v2/orders/{id}/shipments.
type OrderShipmentCreate struct {
	OrderAddressID   int                 `json:"order_address_id"`
	TrackingNumber   string              `json:"tracking_number,omitempty"`
	ShippingProvider string              `json:"shipping_provider,omitempty"`
	TrackingCarrier  string              `json:"tracking_carrier,omitempty"`
	TrackingLink     string              `json:"tracking_link,omitempty"`
	Comments         string              `json:"comments,omitempty"`
	Items            []OrderShipmentItem `json:"items"`
}

// OrderStatus is one entry from GET /v2/order_statuses.
type OrderStatus struct {
	ID                int    `json:"id"`
	Name              string `json:"name,omitempty"`
	SystemLabel       string `json:"system_label,omitempty"`
	CustomLabel       string `json:"custom_label,omitempty"`
	SystemDescription string `json:"system_description,omitempty"`
	Order             int    `json:"order,omitempty"`
}

// OrderPaymentActionListParams controls GET /v3/orders/{id}/payment_actions.
type OrderPaymentActionListParams struct {
	Limit int
	Page  int
}

// OrderRefundListParams controls GET /v3/orders/{id}/payment_actions/refunds.
type OrderRefundListParams struct {
	TransactionID string
	Limit         int
	Page          int
}

// OrderLegacyRefundListParams controls GET /v2/orders/{id}/refunds.
type OrderLegacyRefundListParams struct {
	Page  int
	Limit int
}

// OrderTransactionListParams controls GET /v3/orders/{id}/transactions.
type OrderTransactionListParams struct {
	Limit int
	Page  int
}

// OrderCouponListParams controls paging for GET /v2/orders/{id}/coupons.
type OrderCouponListParams struct {
	Page  int
	Limit int
}

// OrderShippingAddressListParams controls paging for GET /v2/orders/{id}/shipping_addresses.
type OrderShippingAddressListParams struct {
	Page  int
	Limit int
}

// OrderTaxListParams controls optional paging for GET /v2/orders/{id}/taxes.
type OrderTaxListParams struct {
	Page  int
	Limit int
}

// OrderMessageListParams controls filters for GET /v2/orders/{id}/messages.
type OrderMessageListParams struct {
	MinID          int
	MaxID          int
	CustomerID     int
	MinDateCreated string
	MaxDateCreated string
	Status         string
	IsFlagged      *bool
	Page           int
	Limit          int
}

// OrderMetafieldListParams controls optional paging for
// GET /v3/orders/{id}/metafields.
type OrderMetafieldListParams struct {
	Page  int
	Limit int
}

// InventoryLocationListParams controls optional paging for
// GET /v3/inventory/locations.
type InventoryLocationListParams struct {
	Page  int
	Limit int
}

// InventoryLocationMetafieldListParams controls optional paging for
// GET /v3/inventory/locations/{id}/metafields.
type InventoryLocationMetafieldListParams struct {
	Page  int
	Limit int
}

// InventoryItemListParams controls filters/paging for GET /v3/inventory/items.
type InventoryItemListParams struct {
	LocationIDs []int
	ProductIDs  []int
	VariantIDs  []int
	SKUs        []string
	Page        int
	Limit       int
}

// CustomerAuthentication is the nested authentication object on V3 customer
// create/update payloads.
type CustomerAuthentication struct {
	ForcePasswordReset *bool   `json:"force_password_reset,omitempty"`
	NewPassword        *string `json:"new_password,omitempty"`
}

// CustomerStoreCreditAmount is one row of store_credit_amounts on a customer.
type CustomerStoreCreditAmount struct {
	Amount float64 `json:"amount"`
}

// CustomerFormField is a name/value pair for customer or address form_fields.
type CustomerFormField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// CustomerAttributeInline is attribute_id + attribute_value on customer create.
type CustomerAttributeInline struct {
	AttributeID    int    `json:"attribute_id"`
	AttributeValue string `json:"attribute_value"`
}

// CustomerAddressInline is an address embedded in POST /v3/customers.
type CustomerAddressInline struct {
	FirstName       string              `json:"first_name"`
	LastName        string              `json:"last_name"`
	Company         string              `json:"company,omitempty"`
	Address1        string              `json:"address1"`
	Address2        string              `json:"address2,omitempty"`
	City            string              `json:"city"`
	StateOrProvince string              `json:"state_or_province,omitempty"`
	PostalCode      string              `json:"postal_code,omitempty"`
	CountryCode     string              `json:"country_code"`
	Phone           string              `json:"phone,omitempty"`
	AddressType     string              `json:"address_type,omitempty"`
	FormFields      []CustomerFormField `json:"form_fields,omitempty"`
}

// Customer represents a BigCommerce customer (V3 GET/PUT response shape).
type Customer struct {
	ID                                  int                         `json:"id,omitempty"`
	Email                               string                      `json:"email"`
	FirstName                           string                      `json:"first_name"`
	LastName                            string                      `json:"last_name"`
	Company                             string                      `json:"company,omitempty"`
	Phone                               string                      `json:"phone,omitempty"`
	RegistrationIPAddress               string                      `json:"registration_ip_address,omitempty"`
	Notes                               string                      `json:"notes,omitempty"`
	TaxExemptCategory                   string                      `json:"tax_exempt_category,omitempty"`
	CustomerGroupID                     int                         `json:"customer_group_id,omitempty"`
	DateCreated                         string                      `json:"date_created,omitempty"`
	DateModified                        string                      `json:"date_modified,omitempty"`
	Authentication                      *CustomerAuthentication     `json:"authentication,omitempty"`
	AcceptsProductReviewAbandonedEmails *bool                       `json:"accepts_product_review_abandoned_cart_emails,omitempty"`
	StoreCreditAmounts                  []CustomerStoreCreditAmount `json:"store_credit_amounts,omitempty"`
	OriginChannelID                     int                         `json:"origin_channel_id,omitempty"`
	ChannelIDs                          []int                       `json:"channel_ids,omitempty"`
	Addresses                           []CustomerAddress           `json:"addresses,omitempty"`
	FormFields                          []CustomerFormField         `json:"form_fields,omitempty"`
	// ShopperProfileID is populated when GET /v3/customers is called with
	// include=shopper_profile_id. Empty for customers without a profile.
	ShopperProfileID string `json:"shopper_profile_id,omitempty"`
	// SegmentIDs is populated when include=segment_ids is set. Empty otherwise.
	SegmentIDs []string `json:"segment_ids,omitempty"`
}

// CustomerCreate is one element of the JSON array for POST /v3/customers.
type CustomerCreate struct {
	Email                               string                      `json:"email"`
	FirstName                           string                      `json:"first_name"`
	LastName                            string                      `json:"last_name"`
	Company                             string                      `json:"company,omitempty"`
	Phone                               string                      `json:"phone,omitempty"`
	Notes                               string                      `json:"notes,omitempty"`
	TaxExemptCategory                   string                      `json:"tax_exempt_category,omitempty"`
	CustomerGroupID                     int                         `json:"customer_group_id,omitempty"`
	Addresses                           []CustomerAddressInline     `json:"addresses,omitempty"`
	Attributes                          []CustomerAttributeInline   `json:"attributes,omitempty"`
	Authentication                      *CustomerAuthentication     `json:"authentication,omitempty"`
	AcceptsProductReviewAbandonedEmails *bool                       `json:"accepts_product_review_abandoned_cart_emails,omitempty"`
	TriggerAccountCreatedNotification   *bool                       `json:"trigger_account_created_notification,omitempty"`
	StoreCreditAmounts                  []CustomerStoreCreditAmount `json:"store_credit_amounts,omitempty"`
	OriginChannelID                     int                         `json:"origin_channel_id,omitempty"`
	ChannelIDs                          []int                       `json:"channel_ids,omitempty"`
	FormFields                          []CustomerFormField         `json:"form_fields,omitempty"`
}

// CustomerUpdate is one element of the JSON array for PUT /v3/customers.
type CustomerUpdate struct {
	ID                                  int                         `json:"id"`
	Email                               string                      `json:"email,omitempty"`
	FirstName                           string                      `json:"first_name,omitempty"`
	LastName                            string                      `json:"last_name,omitempty"`
	Company                             string                      `json:"company,omitempty"`
	Phone                               string                      `json:"phone,omitempty"`
	RegistrationIPAddress               string                      `json:"registration_ip_address,omitempty"`
	Notes                               string                      `json:"notes,omitempty"`
	TaxExemptCategory                   string                      `json:"tax_exempt_category,omitempty"`
	CustomerGroupID                     *int                        `json:"customer_group_id,omitempty"`
	Authentication                      *CustomerAuthentication     `json:"authentication,omitempty"`
	AcceptsProductReviewAbandonedEmails *bool                       `json:"accepts_product_review_abandoned_cart_emails,omitempty"`
	StoreCreditAmounts                  []CustomerStoreCreditAmount `json:"store_credit_amounts,omitempty"`
	OriginChannelID                     int                         `json:"origin_channel_id,omitempty"`
	ChannelIDs                          []int                       `json:"channel_ids,omitempty"`
	FormFields                          []CustomerFormField         `json:"form_fields,omitempty"`
}

// CustomerAddress is the read shape for GET /v3/customers/addresses and included addresses.
type CustomerAddress struct {
	ID              int                 `json:"id,omitempty"`
	CustomerID      int                 `json:"customer_id,omitempty"`
	FirstName       string              `json:"first_name"`
	LastName        string              `json:"last_name"`
	Company         string              `json:"company,omitempty"`
	Address1        string              `json:"address1"`
	Address2        string              `json:"address2,omitempty"`
	City            string              `json:"city"`
	StateOrProvince string              `json:"state_or_province,omitempty"`
	PostalCode      string              `json:"postal_code,omitempty"`
	CountryCode     string              `json:"country_code"`
	Country         string              `json:"country,omitempty"`
	Phone           string              `json:"phone,omitempty"`
	AddressType     string              `json:"address_type,omitempty"`
	FormFields      []CustomerFormField `json:"form_fields,omitempty"`
}

// CustomerAddressCreate is one row for POST /v3/customers/addresses.
type CustomerAddressCreate struct {
	CustomerID      int                 `json:"customer_id"`
	FirstName       string              `json:"first_name"`
	LastName        string              `json:"last_name"`
	Company         string              `json:"company,omitempty"`
	Address1        string              `json:"address1"`
	Address2        string              `json:"address2,omitempty"`
	City            string              `json:"city"`
	StateOrProvince string              `json:"state_or_province"`
	PostalCode      string              `json:"postal_code"`
	CountryCode     string              `json:"country_code"`
	Phone           string              `json:"phone,omitempty"`
	AddressType     string              `json:"address_type,omitempty"`
	FormFields      []CustomerFormField `json:"form_fields,omitempty"`
}

// CustomerAttribute is the read shape for /v3/customers/attributes.
type CustomerAttribute struct {
	ID           int    `json:"id,omitempty"`
	Name         string `json:"name"`
	Type         string `json:"type"` // "string" | "number" | "date"
	DateCreated  string `json:"date_created,omitempty"`
	DateModified string `json:"date_modified,omitempty"`
}

// CustomerAttributeCreate is one row for POST /v3/customers/attributes.
// Type is immutable after create.
type CustomerAttributeCreate struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CustomerAttributeUpdate is one row for PUT /v3/customers/attributes.
// Only `name` is mutable; `type` cannot be changed once set.
type CustomerAttributeUpdate struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CustomerAttributeValue is the read shape for /v3/customers/attribute-values.
// On GET, BigCommerce returns the field as `attribute_value`; on PUT (upsert),
// the request body uses `value`. The struct supports both with separate fields
// so callers can populate `Value` for upserts and read `AttributeValue` from GETs.
type CustomerAttributeValue struct {
	ID             int    `json:"id,omitempty"`
	AttributeID    int    `json:"attribute_id"`
	CustomerID     int    `json:"customer_id"`
	AttributeValue string `json:"attribute_value,omitempty"`
	Value          string `json:"value,omitempty"`
	DateCreated    string `json:"date_created,omitempty"`
	DateModified   string `json:"date_modified,omitempty"`
}

// CustomerAttributeValueUpsert is one row for PUT /v3/customers/attribute-values.
// Required: attribute_id, value, customer_id.
type CustomerAttributeValueUpsert struct {
	AttributeID int    `json:"attribute_id"`
	CustomerID  int    `json:"customer_id"`
	Value       string `json:"value"`
}

// CustomerPrivacySettings is nested under global/channel customer settings.
type CustomerPrivacySettings struct {
	AskShopperForTrackingConsent           *bool  `json:"ask_shopper_for_tracking_consent,omitempty"`
	PolicyURL                              string `json:"policy_url,omitempty"`
	AskShopperForTrackingConsentOnCheckout *bool  `json:"ask_shopper_for_tracking_consent_on_checkout,omitempty"`
}

// CustomerGroupSettings holds default and guest group IDs for customer settings.
type CustomerGroupSettings struct {
	GuestCustomerGroupID   int `json:"guest_customer_group_id,omitempty"`
	DefaultCustomerGroupID int `json:"default_customer_group_id,omitempty"`
}

// CustomerGlobalSettings is GET/PUT /v3/customers/settings (global level).
type CustomerGlobalSettings struct {
	PrivacySettings       *CustomerPrivacySettings `json:"privacy_settings,omitempty"`
	CustomerGroupSettings *CustomerGroupSettings   `json:"customer_group_settings,omitempty"`
}

// CustomerChannelSettings is GET/PUT /v3/customers/settings/channels/{channel_id}.
// allow_global_logins controls shared logins across storefront channels for this channel.
type CustomerChannelSettings struct {
	PrivacySettings       *CustomerPrivacySettings `json:"privacy_settings,omitempty"`
	CustomerGroupSettings *CustomerGroupSettings   `json:"customer_group_settings,omitempty"`
	AllowGlobalLogins     *bool                    `json:"allow_global_logins,omitempty"`
}

// CustomerConsent is GET/PUT /v3/customers/{id}/consent.
type CustomerConsent struct {
	Allow     []string `json:"allow"`
	Deny      []string `json:"deny"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

// DeclareCustomerConsentRequest is the PUT body for customer consent.
type DeclareCustomerConsentRequest struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// ValidateCustomerCredentialsRequest is POST /v3/customers/validate-credentials.
type ValidateCustomerCredentialsRequest struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	ChannelID *int   `json:"channel_id,omitempty"`
}

// ValidateCustomerCredentialsResponse is the 200 body for validate-credentials.
type ValidateCustomerCredentialsResponse struct {
	IsValid    bool `json:"is_valid"`
	CustomerID *int `json:"customer_id,omitempty"`
}

// ---------------------------------------------------------------------------
// Customer Segmentation (V3: /v3/segments and /v3/shopper-profiles)
// ---------------------------------------------------------------------------

// Segment is the read shape returned by /v3/segments.
type Segment struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// SegmentCreate is one element of POST /v3/segments. Name is required.
type SegmentCreate struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SegmentUpdate is one element of PUT /v3/segments. ID is required; Name and
// Description are optional.
type SegmentUpdate struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// ShopperProfile is the read shape returned by /v3/shopper-profiles and the
// segments membership endpoints. Each profile is 1:1 with a registered customer.
type ShopperProfile struct {
	ID         string `json:"id,omitempty"`
	CustomerID int    `json:"customer_id,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// ShopperProfileCreate is one element of POST /v3/shopper-profiles.
type ShopperProfileCreate struct {
	CustomerID int `json:"customer_id"`
}

// CustomerAddressUpdate is one row for PUT /v3/customers/addresses.
type CustomerAddressUpdate struct {
	ID              int                 `json:"id"`
	FirstName       string              `json:"first_name,omitempty"`
	LastName        string              `json:"last_name,omitempty"`
	Company         string              `json:"company,omitempty"`
	Address1        string              `json:"address1,omitempty"`
	Address2        string              `json:"address2,omitempty"`
	City            string              `json:"city,omitempty"`
	StateOrProvince string              `json:"state_or_province,omitempty"`
	PostalCode      string              `json:"postal_code,omitempty"`
	CountryCode     string              `json:"country_code,omitempty"`
	Phone           string              `json:"phone,omitempty"`
	AddressType     string              `json:"address_type,omitempty"`
	FormFields      []CustomerFormField `json:"form_fields,omitempty"`
}

// ---------------------------------------------------------------------------
// Customer Groups (V2: /v2/customer_groups)
// ---------------------------------------------------------------------------

// CategoryAccessType enumerates the supported category_access.type values for
// a customer group. The BigCommerce API will reject anything outside this set.
const (
	CategoryAccessAll      = "all"
	CategoryAccessSpecific = "specific"
	CategoryAccessNone     = "none"
)

// DiscountRuleType enumerates the supported discount_rules[].type values.
// Note: "price_list" is mutually exclusive with the other types.
const (
	DiscountRuleTypePriceList = "price_list"
	DiscountRuleTypeAll       = "all"
	DiscountRuleTypeCategory  = "category"
	DiscountRuleTypeProduct   = "product"
)

// DiscountRuleMethod enumerates the supported discount_rules[].method values
// for non-price_list rules. Ignored on price_list rules.
const (
	DiscountRuleMethodPercent = "percent"
	DiscountRuleMethodFixed   = "fixed"
	DiscountRuleMethodPrice   = "price"
)

// CategoryAccess controls what categories members of a customer group can see.
// Categories is only meaningful when Type == CategoryAccessSpecific.
type CategoryAccess struct {
	Type       string `json:"type"`
	Categories []int  `json:"categories,omitempty"`
}

// CustomerGroupDiscountRule is one row of a customer group's discount_rules.
// The same struct represents all four BC discount rule shapes (price_list /
// all / category / product); fields not relevant to a given Type are omitted.
//
// BigCommerce returns Amount as a string-encoded float (e.g. "5.0000"); we
// keep that shape on the wire to round-trip without precision loss.
type CustomerGroupDiscountRule struct {
	Type        string `json:"type"`
	Method      string `json:"method,omitempty"`
	Amount      string `json:"amount,omitempty"`
	PriceListID int    `json:"price_list_id,omitempty"`
	CategoryID  int    `json:"category_id,omitempty"`
	ProductID   int    `json:"product_id,omitempty"`
}

// CustomerGroup is the full read shape returned by GET /v2/customer_groups.
type CustomerGroup struct {
	ID               int                         `json:"id"`
	Name             string                      `json:"name"`
	IsDefault        bool                        `json:"is_default"`
	IsGroupForGuests bool                        `json:"is_group_for_guests"`
	CategoryAccess   *CategoryAccess             `json:"category_access,omitempty"`
	DiscountRules    []CustomerGroupDiscountRule `json:"discount_rules,omitempty"`
	DateCreated      string                      `json:"date_created,omitempty"`
	DateModified     string                      `json:"date_modified,omitempty"`
}

// CustomerGroupCreate is the payload for POST /v2/customer_groups.
// Required: Name. All other fields are optional.
type CustomerGroupCreate struct {
	Name             string                      `json:"name"`
	IsDefault        *bool                       `json:"is_default,omitempty"`
	IsGroupForGuests *bool                       `json:"is_group_for_guests,omitempty"`
	CategoryAccess   *CategoryAccess             `json:"category_access,omitempty"`
	DiscountRules    []CustomerGroupDiscountRule `json:"discount_rules,omitempty"`
}

// CustomerGroupUpdate is the payload for PUT /v2/customer_groups/{id}.
// Pointer fields distinguish "omit" (nil) from "set to zero/empty value".
//
// Note: BigCommerce treats discount_rules in bulk — sending the field
// overwrites the entire set. Leave DiscountRules nil to leave existing rules
// untouched; pass a non-nil (possibly empty) slice to replace them.
type CustomerGroupUpdate struct {
	Name             *string                     `json:"name,omitempty"`
	IsDefault        *bool                       `json:"is_default,omitempty"`
	IsGroupForGuests *bool                       `json:"is_group_for_guests,omitempty"`
	CategoryAccess   *CategoryAccess             `json:"category_access,omitempty"`
	DiscountRules    []CustomerGroupDiscountRule `json:"discount_rules,omitempty"`
}

// ---------------------------------------------------------------------------
// Promotions (V3: /v3/promotions)
// ---------------------------------------------------------------------------
//
// Promotions are the discount engine behind Marketing > Promotions in the
// merchant control panel. The outer shape is a small set of typed scalars,
// but the inner rules / actions / conditions / item-matchers / notifications
// trees are deeply polymorphic (5 action shapes, recursive AND/OR/NOT
// conditions and item matchers). Rather than translate every shape into a
// typed Go AST — and lock the surface to whatever variant set BC supports
// today — we keep the inner trees as json.RawMessage and validate shape at
// the tools layer (internal/tools/promotions/validation.go). Callers send
// JSON that matches BigCommerce's own schema verbatim.

// Promotion redemption types. The field is read-only after create — BC
// rejects PUTs that try to flip it.
const (
	PromotionRedemptionAutomatic = "AUTOMATIC"
	PromotionRedemptionCoupon    = "COUPON"
)

// Promotion status values. INVALID is read-only — set by BC when an enabled
// rule transitions to an unrunnable state.
const (
	PromotionStatusEnabled  = "ENABLED"
	PromotionStatusDisabled = "DISABLED"
	PromotionStatusInvalid  = "INVALID"
)

// Promotion is the read shape returned by GET /v3/promotions and
// /v3/promotions/{id}. Rules / Notifications / Customer / ShippingAddress /
// Schedule / Channels stay raw because they are polymorphic.
type Promotion struct {
	ID                             int             `json:"id,omitempty"`
	Name                           string          `json:"name,omitempty"`
	DisplayName                    string          `json:"display_name,omitempty"`
	RedemptionType                 string          `json:"redemption_type,omitempty"`
	Status                         string          `json:"status,omitempty"`
	StartDate                      string          `json:"start_date,omitempty"`
	EndDate                        string          `json:"end_date,omitempty"`
	CurrencyCode                   string          `json:"currency_code,omitempty"`
	MaxUses                        *int            `json:"max_uses,omitempty"`
	CurrentUses                    int             `json:"current_uses,omitempty"`
	Stop                           *bool           `json:"stop,omitempty"`
	CanBeUsedWithOtherPromotions   *bool           `json:"can_be_used_with_other_promotions,omitempty"`
	CouponOverridesOtherPromotions *bool           `json:"coupon_overrides_other_promotions,omitempty"`
	CouponType                     string          `json:"coupon_type,omitempty"`
	CreatedFrom                    string          `json:"created_from,omitempty"`
	Customer                       json.RawMessage `json:"customer,omitempty"`
	Rules                          json.RawMessage `json:"rules,omitempty"`
	Notifications                  json.RawMessage `json:"notifications,omitempty"`
	ShippingAddress                json.RawMessage `json:"shipping_address,omitempty"`
	Schedule                       json.RawMessage `json:"schedule,omitempty"`
	Channels                       json.RawMessage `json:"channels,omitempty"`
	Codes                          json.RawMessage `json:"codes,omitempty"`
	MultipleCodes                  json.RawMessage `json:"multiple_codes,omitempty"`
}

// PromotionListParams encodes the documented filter / sort / paging knobs on
// GET /v3/promotions. RedemptionType is forced to "automatic" by the
// automatic-promotions tools to keep COUPON promotions out of that subtree.
type PromotionListParams struct {
	ID             int    // exact match
	Name           string // exact match
	Code           string // for COUPON; ignored when RedemptionType=automatic
	Query          string // matches name or code
	CurrencyCode   string // exact, e.g. "USD"
	RedemptionType string // "automatic" | "coupon"
	Status         string // ENABLED | DISABLED | INVALID
	Channels       []int  // filter to promotions targeting these channel IDs
	Sort           string // id | name | priority | start_date
	Direction      string // asc | desc
	Page           int
	Limit          int
}

// CountActivePromotions returns the number of ENABLED promotions matching
// the given redemption type. Used by the automatic/create tool's soft-warn
// gate (BC recommends < 100 active promotions per store).
func CountActivePromotions(prs []Promotion) int {
	n := 0
	for _, p := range prs {
		if p.Status == PromotionStatusEnabled {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// Coupon codes (V3: /v3/promotions/{promotionId}/codes and /codegen)
// ---------------------------------------------------------------------------

// Coupon promotion `coupon_type` values. Defaults to SINGLE when not set.
const (
	CouponTypeSingle = "SINGLE"
	CouponTypeBulk   = "BULK"
)

// CouponCode is the read shape returned by GET /v3/promotions/{id}/codes
// and POST /v3/promotions/{id}/codes.
//
// Notes
//   - The parent promotion's max_uses overrides this code's max_uses field.
//   - max_uses=0 means unlimited (per BigCommerce).
//   - The endpoint has no PUT — coupon codes are immutable. To "update" a
//     code (e.g. change max_uses), delete and recreate it.
type CouponCode struct {
	ID                 int    `json:"id,omitempty"`
	PromotionID        int    `json:"promotion_id,omitempty"`
	Code               string `json:"code,omitempty"`
	MaxUses            int    `json:"max_uses,omitempty"`
	MaxUsesPerCustomer int    `json:"max_uses_per_customer,omitempty"`
	CurrentUses        int    `json:"current_uses,omitempty"`
	Created            string `json:"created,omitempty"`
}

// CouponCodeCreate is the POST body for /v3/promotions/{id}/codes.
type CouponCodeCreate struct {
	Code               string `json:"code"`
	MaxUses            *int   `json:"max_uses,omitempty"`
	MaxUsesPerCustomer *int   `json:"max_uses_per_customer,omitempty"`
}

// CouponCodeListParams encodes the cursor-paginated GET filters/options for
// /v3/promotions/{id}/codes.
type CouponCodeListParams struct {
	Before string // cursor for previous page
	After  string // cursor for next page
	Limit  int    // page size (BigCommerce default applies when 0)
}

// CouponCodeListResponse wraps a single page of codes plus the cursor meta
// returned by BigCommerce.
type CouponCodeListResponse struct {
	Codes  []CouponCode      `json:"data"`
	Cursor CouponCodeCursors `json:"cursor"`
}

// CouponCodeCursors is BigCommerce's `meta.cursor` envelope for cursor pagination.
type CouponCodeCursors struct {
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

// CodeGenRequest is the POST body for /v3/promotions/{id}/codegen. BC caps
// batch_size at 250 per call; we surface that limit explicitly here.
type CodeGenRequest struct {
	BatchSize          int    `json:"batch_size"`
	Prefix             string `json:"prefix,omitempty"`
	Suffix             string `json:"suffix,omitempty"`
	Length             int    `json:"length,omitempty"` // 6..16 documented
	Format             string `json:"format,omitempty"` // "NUMBERS" | "LETTERS" | "ALPHANUMERIC"
	Separator          string `json:"separator,omitempty"`
	MaxUses            *int   `json:"max_uses,omitempty"`
	MaxUsesPerCustomer *int   `json:"max_uses_per_customer,omitempty"`
}

// CodeGenResponse is the response shape from /v3/promotions/{id}/codegen.
// BigCommerce returns the freshly generated codes inline.
type CodeGenResponse struct {
	Generated []CouponCode `json:"data"`
}

// ---------------------------------------------------------------------------
// Promotion settings (V3: /v3/promotions/settings)
// ---------------------------------------------------------------------------

// MaxCouponsAtCheckout is BigCommerce's documented upper bound on the
// number_of_coupons_allowed_at_checkout setting (1 default, 5 max). Values
// > 1 require an Enterprise-plan store; non-Enterprise stores 403.
const MaxCouponsAtCheckout = 5

// PromotionSettings mirrors the live shape returned by
// GET /v3/promotions/settings (verified against a live store). All fields
// are typed scalars — BigCommerce's PUT replaces the document, so the tools
// layer fetches current and merges before posting back.
//
// Field semantics (per BigCommerce control-panel mirror at
// Settings > Promotions and Coupons):
//
//   - PromotionsTriggeredByZeroPriceProducts: when true, $0 line items can
//     satisfy promotion conditions. Default false.
//   - PromotionsApplyOnCustomPricedProducts: when true, line items with
//     manual / cart-API price overrides are eligible for promotions.
//     Default false.
//   - NumberOfCouponsAllowedAtCheckout: 1..5. Default 1. Values > 1 are an
//     Enterprise-plan-only feature.
//   - PromotionsAppliedOnOriginalProductPrice: when true, discounts are
//     calculated against each item's original list price; when false, they
//     cascade against running discounted subtotals. Default true.
type PromotionSettings struct {
	PromotionsTriggeredByZeroPriceProducts  bool `json:"promotions_triggered_by_products_with_zero_product_price"`
	PromotionsApplyOnCustomPricedProducts   bool `json:"promotions_apply_on_products_with_custom_product_price"`
	NumberOfCouponsAllowedAtCheckout        int  `json:"number_of_coupons_allowed_at_checkout"`
	PromotionsAppliedOnOriginalProductPrice bool `json:"promotions_applied_on_original_product_price"`
}
