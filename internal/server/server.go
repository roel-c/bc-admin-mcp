package bcserver

import (
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
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
	reg.RegisterCategory("catalog/products/channel_assignments", "MSF: product ↔ channel catalog assignments (list, assign, remove via /v3/catalog/products/channel-assignments)")
	reg.RegisterCategory("catalog/products/images", "Product image management: list, add by URL, delete")
	reg.RegisterCategory("catalog/products/options", "Product option CRUD: list, create, update, delete variant-generating options")
	reg.RegisterCategory("catalog/products/variants", "Product variant CRUD: list, create, update, delete individual variants")
	reg.RegisterCategory("catalog/products/variants/metafields", "Variant metafield CRUD: list, set, delete; bulk by variant_ids (one product); bulk_set_products / bulk_delete_products (many products + variant_scope; caps in tool docs)")
	reg.RegisterCategory("catalog/products/custom_fields", "Product custom field management: list, set (upsert), delete")
	reg.RegisterCategory("catalog/products/modifiers", "Product modifier management: list, create, delete")
	reg.RegisterCategory("catalog/products/metafields", "Product metafield CRUD: list, set, delete, bulk_set, bulk_delete (namespace+key; permission_set for Storefront access)")
	reg.RegisterCategory("catalog/categories", "Category operations: list, get, create, update, SEO, metafields")
	reg.RegisterCategory("catalog/categories/metafields", "Category metafield CRUD: list, set, delete custom key-value data")
	reg.RegisterCategory("catalog/brands", "Brand operations: list, get, create, update, metafields")
	reg.RegisterCategory("catalog/brands/metafields", "Brand metafield CRUD: list, set, delete custom key-value data")
	reg.RegisterCategory("catalog/variants", "Global catalog variants: list/search (GET /v3/catalog/variants) and batch update (PUT); product-scoped CRUD remains under catalog/products/variants")
	reg.RegisterCategory("catalog/channels", "Sales channels and MSF catalog context: list channels (GET /v3/channels), category trees per channel (GET /v3/catalog/trees), channel listings (GET/POST/PUT .../listings); see docs/channels-msf-implementation-roadmap.md")
	reg.RegisterCategory("catalog/channels/listings", "Per-channel product listings: list, create (POST), update (PUT) for listing state and channel-specific copy")

	// Non-catalog domain roots (orders, customers, carts, …) are intentionally omitted
	// until tools exist — empty discover_tools leaves confused agents. Planned hierarchy
	// lives in ARCHITECTURE.md (expansion roadmap); add RegisterCategory + tools in the same change.
}

// registerTools wires up all tool implementations into the registry.
func registerTools(reg *discovery.Registry, bc *bigcommerce.Client, cache *session.Store) {
	products := catalog.NewProducts(bc, cache)
	products.RegisterTools(reg)

	globalVariants := catalog.NewGlobalVariants(bc, cache)
	globalVariants.RegisterTools(reg)

	channelTools := catalog.NewChannelTools(bc)
	channelTools.RegisterTools(reg)
	products.RegisterImageTools(reg)
	products.RegisterOptionTools(reg)
	products.RegisterVariantTools(reg)
	products.RegisterVariantMetafieldTools(reg)
	products.RegisterVariantMetafieldBulkTools(reg)
	products.RegisterCustomFieldTools(reg)
	products.RegisterModifierTools(reg)
	products.RegisterProductMetafieldTools(reg)
	products.RegisterProductMetafieldBulkTools(reg)

	categories := catalog.NewCategories(bc, cache)
	categories.RegisterTools(reg)

	brands := catalog.NewBrands(bc, cache)
	brands.RegisterTools(reg)
}
