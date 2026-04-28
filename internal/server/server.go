package bcserver

import (
	"log/slog"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/mark3labs/mcp-go/server"
)

// New creates a fully wired MCPServer with all BigCommerce tools registered
// behind the progressive disclosure meta-tools.
func New(cfg *config.Config, logger *slog.Logger) *server.MCPServer {
	bcClient := bigcommerce.NewClient(cfg.BigCommerce, logger)
	cacheStore := session.NewStore(cfg.BigCommerce.CacheTTL)
	tierEnforcer := middleware.NewTierEnforcer()

	reg := discovery.NewRegistry()
	registerCategories(reg)
	registerTools(reg, bcClient, cacheStore)

	mcpServer := server.NewMCPServer(
		cfg.Server.Name,
		cfg.Server.Version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithRecovery(),
		server.WithToolHandlerMiddleware(middleware.WithLogging(logger)),
		server.WithLogging(),
	)

	metaTools := reg.MetaTools(tierEnforcer)
	mcpServer.AddTools(metaTools...)

	return mcpServer
}

// registerCategories sets up the tool hierarchy for progressive disclosure.
func registerCategories(reg *discovery.Registry) {
	reg.RegisterCategory("catalog", "Product catalog: products, categories, brands, variants")
	reg.RegisterCategory("catalog/products", "Product operations: search, get, create, update, delete, and sub-resource management")
	reg.RegisterCategory("catalog/products/images", "Product image management: list, add by URL, delete")
	reg.RegisterCategory("catalog/products/options", "Product option CRUD: list, create, update, delete variant-generating options")
	reg.RegisterCategory("catalog/products/variants", "Product variant CRUD: list, create, update, delete individual variants")
	reg.RegisterCategory("catalog/products/custom_fields", "Product custom field management: list, set (upsert), delete")
	reg.RegisterCategory("catalog/products/modifiers", "Product modifier management: list, create, delete")
	reg.RegisterCategory("catalog/categories", "Category operations: list, get, create, update, SEO, metafields")
	reg.RegisterCategory("catalog/categories/metafields", "Category metafield CRUD: list, set, delete custom key-value data")
	reg.RegisterCategory("catalog/brands", "Brand operations: list, get, create, update")
	reg.RegisterCategory("catalog/variants", "Variant operations: list, get, update pricing")

	reg.RegisterCategory("orders", "Order management: list, get, update status, shipments, refunds")
	reg.RegisterCategory("orders/management", "Core order operations: list, get, update status")
	reg.RegisterCategory("orders/fulfillment", "Shipment creation, tracking, and management")
	reg.RegisterCategory("orders/refunds", "Refund processing and history")

	reg.RegisterCategory("customers", "Customer management: list, get, create, update, segments")
	reg.RegisterCategory("customers/management", "Core customer operations")
	reg.RegisterCategory("customers/groups", "Customer group management")
	reg.RegisterCategory("customers/addresses", "Customer address management")

	reg.RegisterCategory("carts", "Cart and checkout operations")
	reg.RegisterCategory("carts/management", "Cart CRUD and item management")
	reg.RegisterCategory("carts/checkout", "Checkout link creation and management")

	reg.RegisterCategory("inventory", "Inventory levels and adjustments across locations")

	reg.RegisterCategory("marketing", "Promotions, coupons, and marketing tools")
	reg.RegisterCategory("marketing/promotions", "Promotion management")
	reg.RegisterCategory("marketing/coupons", "Coupon code management")

	reg.RegisterCategory("store", "Store settings, SEO, shipping configuration")
	reg.RegisterCategory("store/settings", "Store-level configuration and SEO")
	reg.RegisterCategory("store/shipping", "Shipping zones, methods, and carriers")
}

// registerTools wires up all tool implementations into the registry.
func registerTools(reg *discovery.Registry, bc *bigcommerce.Client, cache *session.Store) {
	products := catalog.NewProducts(bc, cache)
	products.RegisterTools(reg)
	products.RegisterImageTools(reg)
	products.RegisterOptionTools(reg)
	products.RegisterVariantTools(reg)
	products.RegisterCustomFieldTools(reg)
	products.RegisterModifierTools(reg)

	categories := catalog.NewCategories(bc, cache)
	categories.RegisterTools(reg)
}
