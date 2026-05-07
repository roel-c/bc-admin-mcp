package catalog

import (
	"context"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
)

// Compile-time check that *bigcommerce.Client satisfies BigCommerceAPI.
var _ BigCommerceAPI = (*bigcommerce.Client)(nil)

// BigCommerceAPI defines the BigCommerce client methods used by catalog tool
// handlers. The concrete *bigcommerce.Client satisfies this interface.
// Defined here (consumer side) per Go convention so that tests can mock it
// without depending on the full client implementation.
type BigCommerceAPI interface {
	// Products
	SearchProducts(ctx context.Context, params map[string]string) ([]bigcommerce.Product, error)
	GetProduct(ctx context.Context, productID int) (*bigcommerce.Product, error)
	GetProductsByIDs(ctx context.Context, productIDs []int) ([]bigcommerce.Product, error)
	ListProductsByCategory(ctx context.Context, categoryID int, opts bigcommerce.ProductListOptions) ([]bigcommerce.Product, error)
	BatchUpdateProducts(ctx context.Context, updates []bigcommerce.ProductUpdate) (*bigcommerce.BatchResult, error)
	CreateProduct(ctx context.Context, payload bigcommerce.ProductCreate) (*bigcommerce.Product, error)
	DeleteProduct(ctx context.Context, productID int) error
	DeleteProducts(ctx context.Context, productIDs []int) (deleted []int, errors []error)

	// Variants (batch)
	ListVariantsForProduct(ctx context.Context, productID int) ([]bigcommerce.Variant, error)
	ListVariantsByProductIDs(ctx context.Context, productIDs []int) ([]bigcommerce.Variant, error)
	SearchVariants(ctx context.Context, params map[string]string) ([]bigcommerce.Variant, error)
	BatchUpdateVariants(ctx context.Context, updates []bigcommerce.CatalogVariantUpdate) (*bigcommerce.BatchResult, error)

	// Variants (single CRUD)
	GetVariant(ctx context.Context, productID, variantID int) (*bigcommerce.ProductVariantFull, error)
	CreateVariant(ctx context.Context, productID int, payload bigcommerce.ProductVariantCreate) (*bigcommerce.ProductVariantFull, error)
	UpdateVariant(ctx context.Context, productID, variantID int, payload bigcommerce.ProductVariantUpdate) (*bigcommerce.ProductVariantFull, error)
	DeleteVariant(ctx context.Context, productID, variantID int) error

	// Product images
	ListProductImages(ctx context.Context, productID int) ([]bigcommerce.ProductImage, error)
	CreateProductImage(ctx context.Context, productID int, payload bigcommerce.ProductImageCreate) (*bigcommerce.ProductImage, error)
	DeleteProductImage(ctx context.Context, productID, imageID int) error

	// Product custom fields
	ListProductCustomFields(ctx context.Context, productID int) ([]bigcommerce.ProductCustomField, error)
	CreateProductCustomField(ctx context.Context, productID int, payload bigcommerce.ProductCustomFieldCreate) (*bigcommerce.ProductCustomField, error)
	UpdateProductCustomField(ctx context.Context, productID, fieldID int, payload bigcommerce.ProductCustomFieldCreate) (*bigcommerce.ProductCustomField, error)
	DeleteProductCustomField(ctx context.Context, productID, fieldID int) error

	// Product options
	ListProductOptions(ctx context.Context, productID int) ([]bigcommerce.ProductOption, error)
	CreateProductOption(ctx context.Context, productID int, payload bigcommerce.ProductOptionCreate) (*bigcommerce.ProductOption, error)
	UpdateProductOption(ctx context.Context, productID, optionID int, payload bigcommerce.ProductOptionUpdate) (*bigcommerce.ProductOption, error)
	DeleteProductOption(ctx context.Context, productID, optionID int) error

	// Product modifiers
	ListProductModifiers(ctx context.Context, productID int) ([]bigcommerce.ProductModifier, error)
	CreateProductModifier(ctx context.Context, productID int, payload bigcommerce.ProductModifierCreate) (*bigcommerce.ProductModifier, error)
	DeleteProductModifier(ctx context.Context, productID, modifierID int) error

	// Categories
	SearchCategories(ctx context.Context, params map[string]string) ([]bigcommerce.Category, error)
	GetCategory(ctx context.Context, categoryID int) (*bigcommerce.Category, error)
	GetCategoriesByIDs(ctx context.Context, categoryIDs []int) ([]bigcommerce.Category, error)
	CreateCategory(ctx context.Context, payload bigcommerce.CategoryCreate) ([]bigcommerce.Category, error)
	BatchUpdateCategories(ctx context.Context, updates []bigcommerce.CategoryUpdate) (*bigcommerce.BatchResult, error)
	DeleteCategories(ctx context.Context, ids []int) error
	GetDefaultTreeID(ctx context.Context) (int, error)

	// Category metafields
	ListCategoryMetafields(ctx context.Context, categoryID int) ([]bigcommerce.Metafield, error)
	CreateCategoryMetafield(ctx context.Context, categoryID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	UpdateCategoryMetafield(ctx context.Context, categoryID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	DeleteCategoryMetafield(ctx context.Context, categoryID, metafieldID int) error

	// Product metafields
	ListProductMetafields(ctx context.Context, productID int) ([]bigcommerce.Metafield, error)
	CreateProductMetafield(ctx context.Context, productID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	UpdateProductMetafield(ctx context.Context, productID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	DeleteProductMetafield(ctx context.Context, productID, metafieldID int) error

	// Variant metafields (scoped under product + variant)
	ListVariantMetafields(ctx context.Context, productID, variantID int) ([]bigcommerce.Metafield, error)
	CreateVariantMetafield(ctx context.Context, productID, variantID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	UpdateVariantMetafield(ctx context.Context, productID, variantID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	DeleteVariantMetafield(ctx context.Context, productID, variantID, metafieldID int) error

	// Category assignments
	UpsertCategoryAssignments(ctx context.Context, assignments []bigcommerce.CategoryAssignment) error
	DeleteCategoryAssignments(ctx context.Context, productID, categoryID int) error
	DeleteCategoryAssignmentsByFilter(ctx context.Context, productIDs, categoryIDs []int) error

	// Channels (storefronts / MSF awareness)
	ListStoreChannels(ctx context.Context, params map[string]string) ([]bigcommerce.StoreChannel, error)
	ListCategoryTrees(ctx context.Context, params map[string]string) ([]bigcommerce.CategoryTree, error)
	GetTreeIDForChannel(ctx context.Context, channelID int) (int, error)

	// Channel listings (per-channel product listing state / overrides)
	ListChannelListings(ctx context.Context, channelID int, query map[string]string) ([]bigcommerce.ChannelListing, error)
	CreateChannelListings(ctx context.Context, channelID int, body any) ([]byte, error)
	UpdateChannelListings(ctx context.Context, channelID int, body any) ([]byte, error)

	// Product channel assignments (MSF catalog)
	ListProductChannelAssignments(ctx context.Context, params map[string]string) ([]bigcommerce.ProductChannelAssignment, error)
	UpsertProductChannelAssignments(ctx context.Context, assignments []bigcommerce.ProductChannelAssignment) error
	DeleteProductChannelAssignments(ctx context.Context, productIDs, channelIDs []int) error

	// Price lists
	ListPriceLists(ctx context.Context, params bigcommerce.PriceListListParams) ([]bigcommerce.PriceList, error)
	GetPriceList(ctx context.Context, priceListID int) (*bigcommerce.PriceList, error)
	CreatePriceList(ctx context.Context, payload bigcommerce.PriceListCreate) (*bigcommerce.PriceList, error)
	UpdatePriceList(ctx context.Context, priceListID int, payload bigcommerce.PriceListUpdate) (*bigcommerce.PriceList, error)
	DeletePriceList(ctx context.Context, priceListID int) error
	ListPriceListRecords(ctx context.Context, priceListID int, params bigcommerce.PriceListRecordListParams) ([]bigcommerce.PriceListRecord, error)
	UpsertPriceListRecords(ctx context.Context, priceListID int, records []bigcommerce.PriceListRecordUpsert) error
	DeletePriceListRecords(ctx context.Context, priceListID int, params bigcommerce.PriceListRecordDeleteParams) error
	ListPriceListAssignments(ctx context.Context, params bigcommerce.PriceListAssignmentListParams) ([]bigcommerce.PriceListAssignment, error)
	CreatePriceListAssignments(ctx context.Context, assignments []bigcommerce.PriceListAssignmentCreate) error
	UpsertPriceListAssignment(ctx context.Context, priceListID int, payload bigcommerce.PriceListAssignmentUpsert) (*bigcommerce.PriceListAssignment, error)
	DeletePriceListAssignments(ctx context.Context, params bigcommerce.PriceListAssignmentDeleteParams) error

	// Brands
	SearchBrands(ctx context.Context, params map[string]string) ([]bigcommerce.Brand, error)
	GetBrand(ctx context.Context, brandID int) (*bigcommerce.Brand, error)
	CreateBrand(ctx context.Context, input bigcommerce.BrandCreate) (*bigcommerce.Brand, error)
	UpdateBrand(ctx context.Context, brandID int, input bigcommerce.BrandUpdate) (*bigcommerce.Brand, error)

	// Brand metafields
	ListBrandMetafields(ctx context.Context, brandID int) ([]bigcommerce.Metafield, error)
	CreateBrandMetafield(ctx context.Context, brandID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	UpdateBrandMetafield(ctx context.Context, brandID, metafieldID int, mf bigcommerce.Metafield) (*bigcommerce.Metafield, error)
	DeleteBrandMetafield(ctx context.Context, brandID, metafieldID int) error
}
