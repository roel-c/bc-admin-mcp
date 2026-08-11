package bcserver

import (
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/roel-c/bc-admin-mcp/internal/tools/b2b"
	"github.com/roel-c/bc-admin-mcp/internal/tools/carts"
	"github.com/roel-c/bc-admin-mcp/internal/tools/catalog"
	"github.com/roel-c/bc-admin-mcp/internal/tools/customers"
	"github.com/roel-c/bc-admin-mcp/internal/tools/inventory"
	"github.com/roel-c/bc-admin-mcp/internal/tools/orders"
	"github.com/roel-c/bc-admin-mcp/internal/tools/promotions"
	"github.com/roel-c/bc-admin-mcp/internal/tools/storefront"
	"github.com/roel-c/bc-admin-mcp/internal/tools/webhooks"
)

// New creates a fully wired MCPServer with all BigCommerce tools registered
// behind the progressive disclosure meta-tools.
func New(cfg *config.Config, logger *slog.Logger) *server.MCPServer {
	bcClient := bigcommerce.NewClient(cfg.BigCommerce, logger)
	var b2bClient *bigcommerce.B2BClient
	if cfg.BigCommerce.B2BEnabled {
		b2bClient = bigcommerce.NewB2BClient(
			cfg.BigCommerce.StoreHash,
			cfg.BigCommerce.AuthToken,
			cfg.BigCommerce.MaxRetries,
			logger,
		)
	}
	cacheStore := session.NewStore(cfg.BigCommerce.CacheTTL)
	tierEnforcer := middleware.NewTierEnforcer()

	reg := discovery.NewRegistry()
	registerCategories(reg, cfg.BigCommerce.B2BEnabled)
	registerTools(reg, bcClient, b2bClient, cacheStore, cfg.BigCommerce.UploadDir)

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
// b2bEnabled gates the b2b/ root — only register it when BC_B2B_ENABLED=true.
func registerCategories(reg *discovery.Registry, b2bEnabled bool) {
	// Category summaries appear in discover_tools responses: describe what tools
	// exist (≤150 chars). Do not include OAuth scopes or REST API paths.
	reg.RegisterCategory("catalog", "Product catalog: products, categories, brands, variants, and price lists")
	reg.RegisterCategory("catalog/products", "Product operations: search, get, create, update, delete, and sub-resource management")
	reg.RegisterCategory("catalog/products/channel_assignments", "MSF product↔channel catalog assignments: list, assign, and remove.")
	reg.RegisterCategory("catalog/products/images", "Product image management: list, add by URL, delete")
	reg.RegisterCategory("catalog/products/options", "Product option CRUD: list, create, update, delete variant-generating options")
	reg.RegisterCategory("catalog/products/variants", "Product variant CRUD: list, create, create_batch, update, delete")
	reg.RegisterCategory("catalog/products/variants/metafields", "Variant metafield CRUD: list, set, delete; bulk by variant_ids, bulk_set_products, bulk_delete_products.")
	reg.RegisterCategory("catalog/products/custom_fields", "Product custom field management: list, set (upsert), delete")
	reg.RegisterCategory("catalog/products/modifiers", "Product modifier management: list, create, delete")
	reg.RegisterCategory("catalog/products/metafields", "Product metafield CRUD: list, set, delete, bulk_set, bulk_delete (namespace+key; permission_set for Storefront access)")
	reg.RegisterCategory("catalog/categories", "Category operations: list, get, create, update, SEO, metafields")
	reg.RegisterCategory("catalog/categories/metafields", "Category metafield CRUD: list, set, delete custom key-value data")
	reg.RegisterCategory("catalog/brands", "Brand operations: list, get, create, update, delete, image, metafields")
	reg.RegisterCategory("catalog/brands/image", "Brand image: set by public URL or remove the brand image.")
	reg.RegisterCategory("catalog/brands/metafields", "Brand metafield CRUD: list, set, delete custom key-value data")
	reg.RegisterCategory("catalog/variants", "Global variant list/search and batch update; product-scoped CRUD stays under catalog/products/variants.")
	reg.RegisterCategory("catalog/channels", "Sales channels and MSF catalog context: list/get/update channels, category trees per channel, and channel listings.")
	reg.RegisterCategory("catalog/channels/listings", "Per-channel product listings: list, create, and update listing state and channel-specific copy.")
	reg.RegisterCategory("catalog/pricelists", "Price list management: list, get, create, update, and delete.")
	reg.RegisterCategory("catalog/pricelists/records", "Price list records for variant/SKU pricing overrides: list, upsert, and delete.")
	reg.RegisterCategory("catalog/pricelists/assignments", "Price list assignments for customer-group/channel targeting: list, create_batch, upsert, delete.")

	reg.RegisterCategory("orders", "Order operations: management, fulfillment shipments, payments, and refunds.")
	reg.RegisterCategory("orders/management", "Order management: list/get/create/update/delete/count/statuses plus order metafields.")
	reg.RegisterCategory("orders/management/products", "Order line-item (product) sub-resource reads for one order.")
	reg.RegisterCategory("orders/management/metafields", "Order metafield operations: list, set, and delete.")
	reg.RegisterCategory("orders/management/coupons", "Order coupon sub-resource listing.")
	reg.RegisterCategory("orders/management/shipping_addresses", "Order shipping-address operations: list, get, and update.")
	reg.RegisterCategory("orders/management/messages", "Order message listing.")
	reg.RegisterCategory("orders/management/taxes", "Order tax listing.")
	reg.RegisterCategory("orders/fulfillment", "Order fulfillment operations.")
	reg.RegisterCategory("orders/fulfillment/shipments", "Shipment operations: list, get, create, update, and delete.")
	reg.RegisterCategory("orders/payments", "Order payments: list actions/transactions, capture, and void.")
	reg.RegisterCategory("orders/payments/actions", "Read payment-action history on one order.")
	reg.RegisterCategory("orders/payments/transactions", "Read order transaction history on one order for parity/reconciliation checks.")
	reg.RegisterCategory("orders/refunds", "Order refunds: list, quote, create, plus legacy refund reference reads.")

	reg.RegisterCategory("customers", "Customer-domain operations: records, addresses, attributes, metafields, settings, consent, stored instruments, segments, shopper profiles, and groups.")
	reg.RegisterCategory("customers/groups", "Customer group CRUD: list, get, count, create, update, delete.")
	reg.RegisterCategory("customers/addresses", "Customer address CRUD: list, create, update, delete.")
	reg.RegisterCategory("customers/attributes", "Customer attribute definition CRUD: list, create, update (rename), delete.")
	reg.RegisterCategory("customers/attribute_values", "Customer attribute values: list, upsert (per customer+attribute), delete by id.")
	reg.RegisterCategory("customers/metafields", "Customer metafield CRUD: list, set, delete, bulk_set, bulk_delete.")
	reg.RegisterCategory("customers/settings", "Global and per-channel customer settings (privacy, default groups, allow global logins).")
	reg.RegisterCategory("customers/settings/global", "Store-wide customer settings: get and update defaults.")
	reg.RegisterCategory("customers/settings/channel", "Per-channel customer settings overrides (including allow_global_logins).")
	reg.RegisterCategory("customers/consent", "Per-customer cookie consent: get and update.")
	reg.RegisterCategory("customers/stored_instruments", "Stored payment instruments listing (gated acknowledgements; tokens redacted by default).")
	reg.RegisterCategory("customers/credentials", "Validate customer credentials (rate limited; preview + confirm).")
	reg.RegisterCategory("customers/segments", "Customer segmentation (Enterprise): segment CRUD plus shopper membership management.")
	reg.RegisterCategory("customers/segments/shoppers", "Shopper-profile membership in a segment: list, add, and remove.")
	reg.RegisterCategory("customers/shopper_profiles", "Shopper profiles (Enterprise): list/create/delete and segments-for-profile lookup.")

	reg.RegisterCategory("marketing", "Marketing: automatic and coupon promotions, coupon codes, and store-wide promotion settings.")
	reg.RegisterCategory("marketing/promotions", "Promotions engine: AUTOMATIC (cart-triggered) and COUPON (code-required) redemption types, plus store-wide promotion settings.")
	reg.RegisterCategory("marketing/promotions/automatic", "Automatic promotions: list/get/create/update/set_status/delete. Redemption type locked to AUTOMATIC; supports rules, actions, and conditions.")
	reg.RegisterCategory("marketing/promotions/coupon", "Coupon promotions: list/get/create/update/set_status/delete. Redemption type locked to COUPON; validates coupon_type and BULK-only multiple_codes.")
	reg.RegisterCategory("marketing/promotions/coupon/codes", "Coupon code management: list, create_single (R1), generate_bulk (R2, BULK promotions only), delete (R3, ≤40 ids/call).")
	reg.RegisterCategory("marketing/promotions/settings", "Store-wide promotion settings: get and update global toggles for multi-coupon checkout, zero-price triggers, and discount calculation behavior.")

	reg.RegisterCategory("inventory", "Inventory-domain operations: location lifecycle, item visibility/updates, backorders, and guarded absolute/relative adjustments.")
	reg.RegisterCategory("inventory/locations", "Inventory locations: list/create/update/delete plus location metafields.")
	reg.RegisterCategory("inventory/locations/metafields", "Inventory location metafields: list, set, and delete.")
	reg.RegisterCategory("inventory/locations/items", "Per-location inventory item reads and settings updates (including backorder_limit).")
	reg.RegisterCategory("inventory/items", "Cross-location inventory items: read and guarded batch update (includes qty_backordered and backorder_limit).")
	reg.RegisterCategory("inventory/adjustments", "Inventory adjustments: absolute and relative submissions (supports qty_backordered).")

	reg.RegisterCategory("webhooks", "Webhook registrations: list, get, view delivery events, create, update, and delete.")

	reg.RegisterCategory("storefront", "Storefront operations: Script Manager script injection and management.")
	reg.RegisterCategory("storefront/scripts", "Script Manager: list, get, create, update, toggle enabled, and delete scripts.")

	reg.RegisterCategory("carts", "Server-side cart and checkout lifecycle.")
	reg.RegisterCategory("carts/cart", "Cart CRUD: create, get, update, delete; checkout URL generation.")
	reg.RegisterCategory("carts/cart/items", "Cart item management: add, update quantity, remove items.")
	reg.RegisterCategory("carts/cart/metafields", "Cart metafield CRUD: list, set (upsert), delete.")
	reg.RegisterCategory("carts/checkout", "Checkout: get, coupon apply/remove, billing address, consignment, convert to order.")

	// B2B Edition — gated by BC_B2B_ENABLED; only registers when the store has B2BE.
	if b2bEnabled {
		reg.RegisterCategory("b2b", "B2B Edition: companies/users, quotes, invoices/receipts, payments, shopping lists, and more.")
		reg.RegisterCategory("b2b/companies", "Company account CRUD and lifecycle status management.")
		reg.RegisterCategory("b2b/companies/users", "Buyer portal user CRUD; roles: admin, senior buyer, junior buyer.")
		reg.RegisterCategory("b2b/companies/addresses", "Company address CRUD: billing and shipping locations.")
		reg.RegisterCategory("b2b/companies/attachments", "Company file attachments: list, upload from configured BC_UPLOAD_DIR, and delete.")
		reg.RegisterCategory("b2b/companies/roles", "Company user roles: list/get/create/update/delete custom roles with permissions.")
		reg.RegisterCategory("b2b/companies/permissions", "Company permission definitions: list plus custom permission CRUD.")
		reg.RegisterCategory("b2b/companies/hierarchy", "Account hierarchy: view parents/subsidiaries, attach parent, detach subsidiary.")
		reg.RegisterCategory("b2b/channels", "Storefront channels as seen by B2B Edition: list and get.")
		reg.RegisterCategory("b2b/orders", "B2B order metadata: get/update PO+extra fields, assign/reassign orders to companies.")
		reg.RegisterCategory("b2b/invoices", "B2B invoices: list, get, PDF download, extra-field configs, create/create-from-order/update/delete.")
		reg.RegisterCategory("b2b/receipts", "B2B payment receipts: list, get, receipt line items, delete.")
		reg.RegisterCategory("b2b/receipts/lines", "Receipt line items: list across all receipts, list for one receipt, get a single line, delete.")
		reg.RegisterCategory("b2b/quotes", "Sales quotes: list, get, create, update, delete, checkout, assign-to-order, PDF export, extra fields.")
		reg.RegisterCategory("b2b/quotes/shipping", "Quote shipping: available rates, select/remove a rate, store-wide custom shipping methods.")
		reg.RegisterCategory("b2b/payments", "Store-wide payment methods (read-only): definitions and cross-company active methods.")
		reg.RegisterCategory("b2b/companies/payments", "Per-company payment method availability: read + update.")
		reg.RegisterCategory("b2b/companies/credit", "Per-company credit settings: read + update.")
		reg.RegisterCategory("b2b/companies/payment_terms", "Per-company net-terms settings: read + update.")
		reg.RegisterCategory("b2b/payment_records", "Payment records logged against invoices: reads, offline payment CRUD, lifecycle operations, processing status.")
		reg.RegisterCategory("b2b/sales_staff", "Backend sales rep company assignment: list, get, update assignments.")
		reg.RegisterCategory("b2b/super_admins", "Frontend sales rep / Super Admin accounts: CRUD, company assignments, masquerade-eligible roster.")
		reg.RegisterCategory("b2b/companies/super_admins", "Company-perspective Super Admin assignments: list, update.")
		reg.RegisterCategory("b2b/shopping_lists", "Repeat-purchase shopping lists: list, get, create, update, delete, and item removal.")
	}

	// store/* remains omitted until tools exist to avoid empty discover_tools leaves.
}

// registerTools wires up all tool implementations into the registry.
// b2bBC is nil when B2B Edition is disabled; tools are skipped in that case.
func registerTools(reg *discovery.Registry, bc *bigcommerce.Client, b2bBC *bigcommerce.B2BClient, cache *session.Store, uploadDirs ...string) {
	var uploadDir string
	if len(uploadDirs) > 0 {
		uploadDir = uploadDirs[0]
	}

	products := catalog.NewProducts(bc, cache)
	products.RegisterTools(reg)

	globalVariants := catalog.NewGlobalVariants(bc, cache)
	globalVariants.RegisterTools(reg)

	channelTools := catalog.NewChannelTools(bc)
	channelTools.RegisterTools(reg)
	priceLists := catalog.NewPriceLists(bc)
	priceLists.RegisterTools(reg)
	products.RegisterImageTools(reg)
	products.RegisterOptionTools(reg)
	products.RegisterVariantTools(reg)
	products.RegisterVariantMetafieldTools(reg)
	products.RegisterVariantMetafieldBulkTools(reg)
	products.RegisterCustomFieldTools(reg)
	products.RegisterModifierTools(reg)
	products.RegisterProductMetafieldTools(reg)
	products.RegisterProductMetafieldBulkTools(reg)

	orderMgmt := orders.NewManagement(bc, cache)
	orderMgmt.RegisterTools(reg)

	orderMetafields := orders.NewOrderMetafields(bc)
	orderMetafields.RegisterTools(reg)

	orderSubresources := orders.NewSubresources(bc)
	orderSubresources.RegisterTools(reg)

	orderFulfillment := orders.NewFulfillment(bc)
	orderFulfillment.RegisterTools(reg)

	orderPayments := orders.NewPayments(bc)
	orderPayments.RegisterTools(reg)

	categories := catalog.NewCategories(bc, cache)
	categories.RegisterTools(reg)

	brands := catalog.NewBrands(bc)
	brands.RegisterTools(reg)

	customerGroups := customers.NewGroups(bc)
	customerGroups.RegisterTools(reg)

	customerRecords := customers.NewCustomerRecords(bc)
	customerRecords.RegisterTools(reg)

	customerAddrs := customers.NewCustomerAddresses(bc)
	customerAddrs.RegisterTools(reg)

	customerAttrs := customers.NewCustomerAttributes(bc)
	customerAttrs.RegisterTools(reg)

	customerAttrValues := customers.NewCustomerAttributeValues(bc)
	customerAttrValues.RegisterTools(reg)

	customerMetafields := customers.NewCustomerMetafields(bc)
	customerMetafields.RegisterTools(reg)

	customerSettings := customers.NewCustomerSettings(bc)
	customerSettings.RegisterTools(reg)

	customerConsent := customers.NewCustomerConsentTools(bc)
	customerConsent.RegisterTools(reg)

	customerStored := customers.NewCustomerStoredInstruments(bc)
	customerStored.RegisterTools(reg)

	customerCreds := customers.NewCustomerValidateCredentials(bc)
	customerCreds.RegisterTools(reg)

	customerSegments := customers.NewCustomerSegments(bc)
	customerSegments.RegisterTools(reg)

	shopperProfiles := customers.NewShopperProfiles(bc)
	shopperProfiles.RegisterTools(reg)

	autoPromotions := promotions.NewAutomaticPromotions(bc, cache)
	autoPromotions.RegisterTools(reg)

	couponPromotions := promotions.NewCouponPromotions(bc, cache)
	couponPromotions.RegisterTools(reg)

	couponCodes := promotions.NewCouponCodes(bc)
	couponCodes.RegisterTools(reg)

	promotionSettings := promotions.NewPromotionSettingsTools(bc, cache)
	promotionSettings.RegisterTools(reg)

	inventoryTools := inventory.New(bc)
	inventoryTools.RegisterTools(reg)

	scriptTools := storefront.NewScripts(bc)
	scriptTools.RegisterTools(reg)

	webhookTools := webhooks.NewWebhooks(bc, cache)
	webhookTools.RegisterTools(reg)

	cartTools := carts.NewCarts(bc, cache)
	cartTools.RegisterTools(reg)
	cartTools.RegisterMetafieldTools(reg)
	cartTools.RegisterCheckoutTools(reg)

	if b2bBC != nil {
		b2bCompanies := b2b.NewCompanyTools(b2bBC, bc, cache, uploadDir)
		b2bCompanies.SetCheckoutAPI(bc)
		b2bCompanies.RegisterTools(reg)
	}
}
