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
	registerTools(reg, bcClient, b2bClient, cacheStore)

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
	reg.RegisterCategory("catalog", "Product catalog: products, categories, brands, variants, and price lists")
	reg.RegisterCategory("catalog/products", "Product operations: search, get, create, update, delete, and sub-resource management")
	reg.RegisterCategory("catalog/products/channel_assignments", "MSF: product ↔ channel catalog assignments (list, assign, remove via /v3/catalog/products/channel-assignments)")
	reg.RegisterCategory("catalog/products/images", "Product image management: list, add by URL, delete")
	reg.RegisterCategory("catalog/products/options", "Product option CRUD: list, create, update, delete variant-generating options")
	reg.RegisterCategory("catalog/products/variants", "Product variant CRUD: list, create, update, delete individual variants")
	reg.RegisterCategory("catalog/products/variants/metafields", "Variant metafield CRUD: list, set, delete; bulk by variant_ids, bulk_set_products, bulk_delete_products.")
	reg.RegisterCategory("catalog/products/custom_fields", "Product custom field management: list, set (upsert), delete")
	reg.RegisterCategory("catalog/products/modifiers", "Product modifier management: list, create, delete")
	reg.RegisterCategory("catalog/products/metafields", "Product metafield CRUD: list, set, delete, bulk_set, bulk_delete (namespace+key; permission_set for Storefront access)")
	reg.RegisterCategory("catalog/categories", "Category operations: list, get, create, update, SEO, metafields")
	reg.RegisterCategory("catalog/categories/metafields", "Category metafield CRUD: list, set, delete custom key-value data")
	reg.RegisterCategory("catalog/brands", "Brand operations: list, get, create, update, delete, image, metafields")
	reg.RegisterCategory("catalog/brands/image", "Brand image: set by URL (via update) or remove (DELETE /v3/catalog/brands/{id}/image)")
	reg.RegisterCategory("catalog/brands/metafields", "Brand metafield CRUD: list, set, delete custom key-value data")
	reg.RegisterCategory("catalog/variants", "Global catalog variants: list/search (GET /v3/catalog/variants) and batch update (PUT); product-scoped CRUD remains under catalog/products/variants")
	reg.RegisterCategory("catalog/channels", "Sales channels and MSF catalog context: list/get/update channels, category trees per channel, and channel listings.")
	reg.RegisterCategory("catalog/channels/listings", "Per-channel product listings: list, create (POST), update (PUT) for listing state and channel-specific copy")
	reg.RegisterCategory("catalog/pricelists", "Price list management: list/get/create/update/delete via /v3/pricelists.")
	reg.RegisterCategory("catalog/pricelists/records", "Price list records for variant/SKU pricing overrides: list/upsert/delete via /v3/pricelists/{id}/records.")
	reg.RegisterCategory("catalog/pricelists/assignments", "Price list assignment management for customer-group/channel targeting: list/create_batch/upsert/delete via /v3/pricelists/assignments.")

	reg.RegisterCategory("orders", "Order-domain operations (V2 primary + V3 payment actions): management, fulfillment shipments, payments, and refunds.")
	reg.RegisterCategory("orders/management", "Order management via /v2/orders and /v3/orders/{id}/metafields: list/get/create/update/delete/count/statuses plus order metafields.")
	reg.RegisterCategory("orders/management/products", "Order-product sub-resource reads via /v2/orders/{id}/products/{product_id}.")
	reg.RegisterCategory("orders/management/metafields", "Order metafield operations via /v3/orders/{id}/metafields: list/set/delete.")
	reg.RegisterCategory("orders/management/coupons", "Order coupon sub-resource listing via /v2/orders/{id}/coupons.")
	reg.RegisterCategory("orders/management/shipping_addresses", "Order shipping-address operations via /v2/orders/{id}/shipping_addresses: list/get/update.")
	reg.RegisterCategory("orders/management/messages", "Order message listing via /v2/orders/{id}/messages.")
	reg.RegisterCategory("orders/management/taxes", "Order tax listing via /v2/orders/{id}/taxes.")
	reg.RegisterCategory("orders/fulfillment", "Order fulfillment operations.")
	reg.RegisterCategory("orders/fulfillment/shipments", "Shipment operations via /v2/orders/{id}/shipments: list/get/create/update/delete.")
	reg.RegisterCategory("orders/payments", "Order payment surfaces via /v3/orders/{id}/payment_actions and /v3/orders/{id}/transactions: list actions/transactions, capture, and void.")
	reg.RegisterCategory("orders/payments/actions", "Read payment-action history on one order.")
	reg.RegisterCategory("orders/payments/transactions", "Read order transaction history on one order for parity/reconciliation checks.")
	reg.RegisterCategory("orders/refunds", "Order refunds via /v3/orders/{id}/payment_actions/refunds + refund_quotes, plus legacy /v2/orders/{id}/refunds reference reads.")

	reg.RegisterCategory("customers", "Customer-domain operations: records, addresses, attributes, metafields, settings, consent, stored instruments, segments, shopper profiles, and groups.")
	reg.RegisterCategory("customers/groups", "Customer Group CRUD via /v2/customer_groups: list, get, count, create, update, delete.")
	reg.RegisterCategory("customers/addresses", "Customer address CRUD via /v3/customers/addresses: list, create, update, delete.")
	reg.RegisterCategory("customers/attributes", "Customer attribute definition CRUD via /v3/customers/attributes: list, create, update (rename), delete.")
	reg.RegisterCategory("customers/attribute_values", "Customer attribute value CRUD via /v3/customers/attribute-values: list, upsert (per customer+attribute), delete by id.")
	reg.RegisterCategory("customers/metafields", "Customer metafield CRUD via /v3/customers/{id}/metafields and /v3/customers/metafields: list, set, delete, bulk_set, bulk_delete.")
	reg.RegisterCategory("customers/settings", "Global and per-channel customer settings (privacy, default groups, allow global logins).")
	reg.RegisterCategory("customers/settings/global", "GET/PUT /v3/customers/settings (store-wide defaults).")
	reg.RegisterCategory("customers/settings/channel", "GET/PUT /v3/customers/settings/channels/{channel_id} (per-channel overrides including allow_global_logins).")
	reg.RegisterCategory("customers/consent", "Per-customer cookie consent (GET/PUT /v3/customers/{id}/consent).")
	reg.RegisterCategory("customers/stored_instruments", "Stored payment instruments listing (GET /v3/customers/{id}/stored-instruments; gated acknowledgements).")
	reg.RegisterCategory("customers/credentials", "POST /v3/customers/validate-credentials (rate limited; preview + confirm).")
	reg.RegisterCategory("customers/segments", "Customer Segmentation (Enterprise): /v3/segments CRUD plus shoppers/* membership management.")
	reg.RegisterCategory("customers/segments/shoppers", "Shopper-profile membership in a segment: list (modify scope), add (max 50/call), remove (chunked).")
	reg.RegisterCategory("customers/shopper_profiles", "Shopper profiles (Enterprise): /v3/shopper-profiles list/create/delete and segments-for-profile lookup.")

	reg.RegisterCategory("marketing", "Marketing-domain operations: promotions (automatic and coupon redemption types). OAuth scope: store_v2_marketing.")
	reg.RegisterCategory("marketing/promotions", "Promotions engine: AUTOMATIC (cart-triggered) and COUPON (code-required) redemption types, plus store-wide promotion settings.")
	reg.RegisterCategory("marketing/promotions/automatic", "Automatic promotions: list/get/create/update/set_status/delete. Redemption type locked to AUTOMATIC; supports rules, actions, and conditions.")
	reg.RegisterCategory("marketing/promotions/coupon", "Coupon promotions: list/get/create/update/set_status/delete. Redemption type locked to COUPON; validates coupon_type and BULK-only multiple_codes.")
	reg.RegisterCategory("marketing/promotions/coupon/codes", "Coupon code management: list, create_single (R1), generate_bulk (R2, BULK promotions only), delete (R3, ≤40 ids/call).")
	reg.RegisterCategory("marketing/promotions/settings", "Store-wide promotion settings: get and update global toggles for multi-coupon checkout, zero-price triggers, and discount calculation behavior.")

	reg.RegisterCategory("inventory", "Inventory-domain operations: location lifecycle, item visibility/updates, and guarded absolute/relative adjustments.")
	reg.RegisterCategory("inventory/locations", "Inventory location operations via /v3/inventory/locations (list/create/update/delete) and location metafields.")
	reg.RegisterCategory("inventory/locations/metafields", "Inventory location metafield operations via /v3/inventory/locations/{id}/metafields: list/set/delete.")
	reg.RegisterCategory("inventory/items", "Inventory item operations via /v3/inventory/items and /v3/inventory/items/{variant_id} (read + guarded batch update).")
	reg.RegisterCategory("inventory/adjustments", "Inventory adjustment submissions via /v3/inventory/adjustments/absolute and /v3/inventory/adjustments/relative.")

	reg.RegisterCategory("webhooks", "Webhook registrations for the store: list, get, view events, create, update, delete via /v3/hooks.")

	reg.RegisterCategory("storefront", "Storefront operations: script injection and management via the BigCommerce Scripts API.")
	reg.RegisterCategory("storefront/scripts", "Script Manager: list, get, create (R1), update (R1), toggle enabled (R1), delete (R3) via /v3/content/scripts.")

	reg.RegisterCategory("carts", "Server-side cart and checkout lifecycle via /v3/carts and /v3/checkouts.")
	reg.RegisterCategory("carts/cart", "Cart CRUD: create, get, update, delete; checkout URL generation.")
	reg.RegisterCategory("carts/cart/items", "Cart item management: add, update quantity, remove items.")
	reg.RegisterCategory("carts/cart/metafields", "Cart metafield CRUD: list, set (upsert), delete.")
	reg.RegisterCategory("carts/checkout", "Checkout: get, coupon apply/remove, billing address, consignment, convert to order.")

	// B2B Edition — gated by BC_B2B_ENABLED; only registers when the store has B2BE.
	if b2bEnabled {
		reg.RegisterCategory("b2b", "B2B Edition: company accounts, buyer users, and company addresses.")
		reg.RegisterCategory("b2b/companies", "Company account CRUD and lifecycle status management.")
		reg.RegisterCategory("b2b/companies/users", "Buyer portal user CRUD; roles: admin, senior buyer, junior buyer.")
		reg.RegisterCategory("b2b/companies/addresses", "Company address CRUD: billing and shipping locations.")
		reg.RegisterCategory("b2b/companies/attachments", "Company file attachments: list and delete.")
		reg.RegisterCategory("b2b/companies/roles", "Company user roles: list/get/create/update/delete custom roles with permissions.")
		reg.RegisterCategory("b2b/companies/permissions", "Company permission definitions: list plus custom permission CRUD.")
		reg.RegisterCategory("b2b/companies/hierarchy", "Account hierarchy: view parents/subsidiaries, attach parent, detach subsidiary.")
		reg.RegisterCategory("b2b/channels", "Storefront channels as seen by B2B Edition: list and get.")
		reg.RegisterCategory("b2b/orders", "B2B order metadata: get/update PO+extra fields, assign/reassign orders to companies.")
		reg.RegisterCategory("b2b/invoices", "B2B invoices (read-only): list, get, PDF download, extra-field configs.")
		reg.RegisterCategory("b2b/receipts", "B2B payment receipts (read-only): list, get, and receipt line items.")
		reg.RegisterCategory("b2b/receipts/lines", "Receipt line items: list across all receipts, list for one receipt, get a single line.")
		reg.RegisterCategory("b2b/quotes", "Sales quotes: list, get, create, update, delete, checkout, assign-to-order, PDF export, extra fields.")
		reg.RegisterCategory("b2b/quotes/shipping", "Quote shipping: available rates, select/remove a rate, store-wide custom shipping methods.")
		reg.RegisterCategory("b2b/payments", "Store-wide payment methods (read-only): definitions and cross-company active methods.")
		reg.RegisterCategory("b2b/companies/payments", "Per-company payment method availability (read-only).")
		reg.RegisterCategory("b2b/companies/credit", "Per-company credit settings (read-only).")
		reg.RegisterCategory("b2b/companies/payment_terms", "Per-company net-terms settings (read-only).")
	}

	// store/* remains omitted until tools exist to avoid empty discover_tools leaves.
}

// registerTools wires up all tool implementations into the registry.
// b2bBC is nil when B2B Edition is disabled; tools are skipped in that case.
func registerTools(reg *discovery.Registry, bc *bigcommerce.Client, b2bBC *bigcommerce.B2BClient, cache *session.Store) {
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
		b2bCompanies := b2b.NewCompanyTools(b2bBC, bc, cache)
		b2bCompanies.RegisterTools(reg)
	}
}
