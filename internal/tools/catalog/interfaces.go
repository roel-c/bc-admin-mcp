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

	// Category assignments
	UpsertCategoryAssignments(ctx context.Context, assignments []bigcommerce.CategoryAssignment) error
	DeleteCategoryAssignments(ctx context.Context, productID, categoryID int) error
}
