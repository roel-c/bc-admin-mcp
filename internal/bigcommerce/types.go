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
	body := string(e.Body)
	if len(body) > 500 {
		body = body[:500] + "... (truncated)"
	}
	hint := scopeHint(e.StatusCode, e.Method, e.Path)
	if hint != "" {
		return fmt.Sprintf("BigCommerce API error %d on %s %s: %s | hint: %s", e.StatusCode, e.Method, e.Path, body, hint)
	}
	return fmt.Sprintf("BigCommerce API error %d: %s", e.StatusCode, body)
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
	case strings.HasPrefix(path, "orders"):
		if read {
			return "403 — token likely missing 'store_v2_orders_read_only' (or 'store_v2_orders')."
		}
		return "403 — write requires 'store_v2_orders'."
	case strings.HasPrefix(path, "customers"):
		if read {
			return "403 — token likely missing 'store_v2_customers_read_only' (or 'store_v2_customers')."
		}
		return "403 — write requires 'store_v2_customers'."
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
	ID              int        `json:"id"`
	Name            string     `json:"name"`
	Type            string     `json:"type,omitempty"`
	SKU             string     `json:"sku,omitempty"`
	Description     string     `json:"description,omitempty"`
	Weight          float64    `json:"weight,omitempty"`
	Width           float64    `json:"width,omitempty"`
	Height          float64    `json:"height,omitempty"`
	Depth           float64    `json:"depth,omitempty"`
	Price           float64    `json:"price"`
	CalculatedPrice float64   `json:"calculated_price,omitempty"`
	CostPrice       float64    `json:"cost_price,omitempty"`
	RetailPrice     float64    `json:"retail_price,omitempty"`
	SalePrice       float64    `json:"sale_price,omitempty"`
	MapPrice        float64    `json:"map_price,omitempty"`
	TaxClassID      int        `json:"tax_class_id,omitempty"`
	Categories      []int      `json:"categories,omitempty"`
	BrandID         int        `json:"brand_id,omitempty"`

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
	ID                       int                `json:"id"`
	ProductID                int                `json:"product_id"`
	SKU                      string             `json:"sku,omitempty"`
	Price                    *float64           `json:"price,omitempty"`
	CalculatedPrice          float64            `json:"calculated_price,omitempty"`
	CostPrice                float64            `json:"cost_price,omitempty"`
	RetailPrice              float64            `json:"retail_price,omitempty"`
	SalePrice                float64            `json:"sale_price,omitempty"`
	MapPrice                 float64            `json:"map_price,omitempty"`
	Weight                   *float64           `json:"weight,omitempty"`
	Width                    *float64           `json:"width,omitempty"`
	Height                   *float64           `json:"height,omitempty"`
	Depth                    *float64           `json:"depth,omitempty"`
	InventoryLevel           int                `json:"inventory_level,omitempty"`
	InventoryWarningLevel    int                `json:"inventory_warning_level,omitempty"`
	BinPickingNumber         string             `json:"bin_picking_number,omitempty"`
	UPC                      string             `json:"upc,omitempty"`
	GTIN                     string             `json:"gtin,omitempty"`
	MPN                      string             `json:"mpn,omitempty"`
	PurchasingDisabled       bool               `json:"purchasing_disabled,omitempty"`
	PurchasingDisabledMsg    string             `json:"purchasing_disabled_message,omitempty"`
	ImageURL                 string             `json:"image_url,omitempty"`
	OptionValues             []VariantOptionVal `json:"option_values,omitempty"`
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
	ID          int                      `json:"id"`
	ProductID   int                      `json:"product_id"`
	DisplayName string                   `json:"display_name"`
	Type        string                   `json:"type"`
	Required    bool                     `json:"required"`
	SortOrder   int                      `json:"sort_order,omitempty"`
	Config      json.RawMessage          `json:"config,omitempty"`
	OptionValues []ProductModifierValue  `json:"option_values,omitempty"`
}

type ProductModifierValue struct {
	ID        int    `json:"id,omitempty"`
	Label     string `json:"label"`
	SortOrder int    `json:"sort_order,omitempty"`
	IsDefault bool   `json:"is_default,omitempty"`
}

type ProductModifierCreate struct {
	DisplayName  string                  `json:"display_name"`
	Type         string                  `json:"type"`
	Required     bool                    `json:"required,omitempty"`
	SortOrder    int                     `json:"sort_order,omitempty"`
	Config       json.RawMessage         `json:"config,omitempty"`
	OptionValues []ProductModifierValue  `json:"option_values,omitempty"`
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

// Metafield represents a custom key-value pair on a catalog resource (product,
// category, brand, variant). The same struct works for all resource types.
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

// Order represents a BigCommerce order (V2 shape).
type Order struct {
	ID              int     `json:"id"`
	CustomerID      int     `json:"customer_id,omitempty"`
	Status          string  `json:"status,omitempty"`
	StatusID        int     `json:"status_id,omitempty"`
	TotalExTax      string  `json:"total_ex_tax,omitempty"`
	TotalIncTax     string  `json:"total_inc_tax,omitempty"`
	DateCreated     string  `json:"date_created,omitempty"`
	DateModified    string  `json:"date_modified,omitempty"`
	ItemsTotal      int     `json:"items_total,omitempty"`
	PaymentMethod   string  `json:"payment_method,omitempty"`
	BillingAddress  json.RawMessage `json:"billing_address,omitempty"`
}

// Customer represents a BigCommerce customer (V3 shape).
type Customer struct {
	ID              int    `json:"id,omitempty"`
	Email           string `json:"email"`
	FirstName       string `json:"first_name"`
	LastName        string `json:"last_name"`
	Company         string `json:"company,omitempty"`
	CustomerGroupID int    `json:"customer_group_id,omitempty"`
}
