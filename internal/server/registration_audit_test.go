package bcserver

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/stretchr/testify/require"
)

func testBigCommerceConfig() config.BigCommerceConfig {
	return config.BigCommerceConfig{
		StoreHash:           "audit",
		AuthToken:           "audit",
		RequestsPerSecond:   2,
		QuotaSafetyBuffer:   25,
		MaxRetries:          6,
		ProductBatchSize:    10,
		VariantBatchSize:    10,
		InventoryBatchSize:  10,
		DefaultPageLimit:    250,
		MaxTotalRecords:     10000,
		DelayBetweenChunks:  500 * time.Millisecond,
		MaxWriteConcurrency: 1,
		CacheTTL:            60 * time.Second,
	}
}

func TestFullRegistrationActiveCategoriesAreNonEmptyLeaves(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	allowedRoots := []string{"catalog", "orders", "customers", "marketing", "inventory", "storefront", "webhooks", "carts"}
	for _, cat := range reg.ListCategoryPaths() {
		hasAllowedRoot := false
		for _, root := range allowedRoots {
			if cat == root || strings.HasPrefix(cat, root+"/") {
				hasAllowedRoot = true
				break
			}
		}
		require.True(t, hasAllowedRoot, "category %q does not live under an allowed root (%v)", cat, allowedRoots)
		entries, err := reg.Discover(cat)
		require.NoError(t, err)
		require.NotEmpty(t, entries, "discover_tools(%q) must not be empty — add tools or subcategories", cat)
	}
}

func TestFullRegistrationEveryToolParentCategoryExists(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	for _, toolPath := range reg.ListToolPaths() {
		parent := toolPath
		for {
			idx := strings.LastIndex(parent, "/")
			if idx <= 0 {
				break
			}
			parent = parent[:idx]
			require.True(t, reg.HasCategory(parent), "parent category %q for tool %q must be registered in registerCategories", parent, toolPath)
		}
	}
}

func TestFullRegistrationActiveRoots(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	entries, err := reg.Discover("")
	require.NoError(t, err)

	roots := make(map[string]string, len(entries))
	for _, e := range entries {
		require.Equal(t, "category", e.Type, "root entry %q must be a category", e.Path)
		roots[e.Path] = e.Type
	}
	require.Contains(t, roots, "catalog", "catalog root must be registered")
	require.Contains(t, roots, "orders", "orders root must be registered")
	require.Contains(t, roots, "customers", "customers root must be registered")
	require.Contains(t, roots, "marketing", "marketing root must be registered")
	require.Contains(t, roots, "inventory", "inventory root must be registered")
	require.Contains(t, roots, "storefront", "storefront root must be registered")
	require.Contains(t, roots, "webhooks", "webhooks root must be registered")
	require.Contains(t, roots, "carts", "carts root must be registered")

	// b2b is gated by BC_B2B_ENABLED and must NOT appear when disabled.
	require.NotContains(t, roots, "b2b", "b2b root must stay hidden when B2B is disabled")
}

func TestFullRegistrationDiscoveryBFSCoversAllCategoriesAndTools(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	queue := []string{""}
	seenCat := map[string]bool{}
	seenTool := map[string]bool{}

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]

		entries, err := reg.Discover(path)
		require.NoError(t, err)

		for _, e := range entries {
			switch e.Type {
			case "category":
				if !seenCat[e.Path] {
					seenCat[e.Path] = true
					queue = append(queue, e.Path)
				}
			case "tool":
				seenTool[e.Path] = true
			}
		}
	}

	for _, p := range reg.ListCategoryPaths() {
		require.True(t, seenCat[p], "category %q not reachable via discover_tools BFS from root", p)
	}
	for _, p := range reg.ListToolPaths() {
		require.True(t, seenTool[p], "tool %q not reachable via discover_tools BFS from root", p)
	}
}

func TestFullRegistrationR1PlusToolsExposeConfirmedParameter(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	for _, path := range reg.ListToolPaths() {
		def := reg.GetTool(path)
		require.NotNil(t, def, "tool %q", path)
		if !middleware.RequiresConfirmation(def.Tier) {
			continue
		}
		props := def.Tool.InputSchema.Properties
		require.Contains(t, props, "confirmed", "tool %q (tier %s) must declare confirmed boolean", path, def.Tier)
	}
}

func TestFullRegistrationCatalogPriceListSubtreeIsFullyRegistered(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	requiredCategories := []string{
		"catalog/pricelists",
		"catalog/pricelists/records",
		"catalog/pricelists/assignments",
	}
	for _, cat := range requiredCategories {
		require.True(t, reg.HasCategory(cat), "missing required category %q", cat)
	}

	requiredTools := []string{
		"catalog/pricelists/list",
		"catalog/pricelists/get",
		"catalog/pricelists/create",
		"catalog/pricelists/update",
		"catalog/pricelists/delete",
		"catalog/pricelists/records/list",
		"catalog/pricelists/records/upsert",
		"catalog/pricelists/records/delete",
		"catalog/pricelists/assignments/list",
		"catalog/pricelists/assignments/create_batch",
		"catalog/pricelists/assignments/upsert",
		"catalog/pricelists/assignments/delete",
	}
	for _, toolPath := range requiredTools {
		def := reg.GetTool(toolPath)
		require.NotNil(t, def, "missing required tool %q", toolPath)
	}

	entries, err := reg.Discover("catalog/pricelists")
	require.NoError(t, err)
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Path] = true
	}

	require.True(t, seen["catalog/pricelists/records"], "catalog/pricelists should expose records category")
	require.True(t, seen["catalog/pricelists/assignments"], "catalog/pricelists should expose assignments category")
	require.True(t, seen["catalog/pricelists/list"], "catalog/pricelists should expose list tool")
	require.True(t, seen["catalog/pricelists/get"], "catalog/pricelists should expose get tool")
	require.True(t, seen["catalog/pricelists/create"], "catalog/pricelists should expose create tool")
	require.True(t, seen["catalog/pricelists/update"], "catalog/pricelists should expose update tool")
	require.True(t, seen["catalog/pricelists/delete"], "catalog/pricelists should expose delete tool")
}

func TestFullRegistrationOrdersInitialSubtreeIsFullyRegistered(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	requiredCategories := []string{
		"orders",
		"orders/management",
		"orders/management/products",
		"orders/management/metafields",
		"orders/management/coupons",
		"orders/management/shipping_addresses",
		"orders/management/messages",
		"orders/management/taxes",
		"orders/fulfillment",
		"orders/fulfillment/shipments",
		"orders/payments",
		"orders/payments/actions",
		"orders/payments/transactions",
		"orders/refunds",
	}
	for _, cat := range requiredCategories {
		require.True(t, reg.HasCategory(cat), "missing required category %q", cat)
	}

	requiredTools := []string{
		"orders/management/list",
		"orders/management/get",
		"orders/management/create",
		"orders/management/update",
		"orders/management/delete",
		"orders/management/count",
		"orders/management/statuses",
		"orders/management/update_status",
		"orders/management/products/get",
		"orders/management/metafields/list",
		"orders/management/metafields/set",
		"orders/management/metafields/delete",
		"orders/management/coupons/list",
		"orders/management/shipping_addresses/list",
		"orders/management/shipping_addresses/get",
		"orders/management/shipping_addresses/update",
		"orders/management/messages/list",
		"orders/management/taxes/list",
		"orders/fulfillment/shipments/list",
		"orders/fulfillment/shipments/get",
		"orders/fulfillment/shipments/create",
		"orders/fulfillment/shipments/update",
		"orders/fulfillment/shipments/delete",
		"orders/payments/actions/list",
		"orders/payments/transactions/list",
		"orders/payments/capture",
		"orders/payments/void",
		"orders/refunds/list",
		"orders/refunds/legacy_list",
		"orders/refunds/quote",
		"orders/refunds/create",
	}
	for _, toolPath := range requiredTools {
		def := reg.GetTool(toolPath)
		require.NotNil(t, def, "missing required tool %q", toolPath)
	}
}

// maxSummaryLen is the upper bound for both category and tool Summary strings.
// Summaries appear verbatim in discover_tools responses consumed by the LLM,
// so keeping them short is the single most effective way to control discovery
// token cost. Guidance, API paths, OAuth scopes, and implementation notes
// belong in docs/ or tool error messages — never in discovery stubs.
const maxSummaryLen = 150

func TestFullRegistrationCategorySummaryLength(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	for _, path := range reg.ListCategoryPaths() {
		path := path
		cat := reg.GetCategory(path)
		require.NotNil(t, cat, "category %q has no definition", path)
		t.Run(path, func(t *testing.T) {
			require.LessOrEqualf(t, len(cat.Summary), maxSummaryLen,
				"category %q summary is %d chars (limit %d) — move guidance to docs/ or tool descriptions",
				path, len(cat.Summary), maxSummaryLen,
			)
		})
	}
}

func TestFullRegistrationToolSummaryLength(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	for _, path := range reg.ListToolPaths() {
		path := path
		def := reg.GetTool(path)
		require.NotNil(t, def, "tool %q has no definition", path)
		t.Run(path, func(t *testing.T) {
			require.LessOrEqualf(t, len(def.Summary), maxSummaryLen,
				"tool %q summary is %d chars (limit %d) — trim to a single short sentence",
				path, len(def.Summary), maxSummaryLen,
			)
		})
	}
}

func TestFullRegistrationInventoryInitialSubtreeIsFullyRegistered(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	requiredCategories := []string{
		"inventory",
		"inventory/locations",
		"inventory/locations/metafields",
		"inventory/items",
		"inventory/adjustments",
	}
	for _, cat := range requiredCategories {
		require.True(t, reg.HasCategory(cat), "missing required category %q", cat)
	}

	requiredTools := []string{
		"inventory/locations/list",
		"inventory/locations/create",
		"inventory/locations/update",
		"inventory/locations/delete",
		"inventory/locations/metafields/list",
		"inventory/locations/metafields/set",
		"inventory/locations/metafields/delete",
		"inventory/items/list",
		"inventory/items/get",
		"inventory/items/update_batch",
		"inventory/adjustments/absolute",
		"inventory/adjustments/relative",
	}
	for _, toolPath := range requiredTools {
		def := reg.GetTool(toolPath)
		require.NotNil(t, def, "missing required tool %q", toolPath)
	}
}

func TestFullRegistrationCartsSubtreeIsFullyRegistered(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	requiredCategories := []string{
		"carts",
		"carts/cart",
		"carts/cart/items",
		"carts/cart/metafields",
		"carts/checkout",
	}
	for _, cat := range requiredCategories {
		require.True(t, reg.HasCategory(cat), "missing required category %q", cat)
	}

	requiredTools := []string{
		"carts/cart/create",
		"carts/cart/get",
		"carts/cart/update",
		"carts/cart/delete",
		"carts/cart/items/add",
		"carts/cart/items/update",
		"carts/cart/items/remove",
		"carts/cart/checkout_url",
		"carts/cart/metafields/list",
		"carts/cart/metafields/set",
		"carts/cart/metafields/delete",
		"carts/checkout/get",
		"carts/checkout/coupon_apply",
		"carts/checkout/coupon_remove",
		"carts/checkout/billing_address",
		"carts/checkout/consignment_add",
		"carts/checkout/consignment_update",
		"carts/checkout/convert",
	}
	for _, toolPath := range requiredTools {
		def := reg.GetTool(toolPath)
		require.NotNil(t, def, "missing required tool %q", toolPath)
	}
}

func TestFullRegistrationStorefrontAndWebhooksSubtreesAreFullyRegistered(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, false)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, nil, cache)

	requiredCategories := []string{
		"storefront",
		"storefront/scripts",
		"webhooks",
	}
	for _, cat := range requiredCategories {
		require.True(t, reg.HasCategory(cat), "missing required category %q", cat)
	}

	requiredTools := []string{
		"storefront/scripts/list",
		"storefront/scripts/get",
		"storefront/scripts/create",
		"storefront/scripts/update",
		"storefront/scripts/toggle",
		"storefront/scripts/delete",
		"webhooks/list",
		"webhooks/get",
		"webhooks/events",
		"webhooks/create",
		"webhooks/update",
		"webhooks/delete",
	}
	for _, toolPath := range requiredTools {
		def := reg.GetTool(toolPath)
		require.NotNil(t, def, "missing required tool %q", toolPath)
	}
}

// TestFullRegistrationB2BRootIsGatedByFlag verifies the b2b/ domain only
// registers when B2B Edition is enabled — disabled stores must never see it,
// and enabled stores must get the full Phase B1 subtree.
func TestFullRegistrationB2BRootIsGatedByFlag(t *testing.T) {
	cfg := testBigCommerceConfig()

	// Disabled: no b2b category or tools.
	disabled := discovery.NewRegistry()
	registerCategories(disabled, false)
	bcDisabled := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bcDisabled.Close() })
	registerTools(disabled, bcDisabled, nil, session.NewStore(cfg.CacheTTL))
	require.False(t, disabled.HasCategory("b2b"), "b2b root must not register when disabled")
	require.Nil(t, disabled.GetTool("b2b/companies/list"), "b2b tools must not register when disabled")

	// Enabled: full subtree registers.
	enabled := discovery.NewRegistry()
	registerCategories(enabled, true)
	bcEnabled := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bcEnabled.Close() })
	b2bClient := bigcommerce.NewB2BClient(cfg.StoreHash, cfg.AuthToken, cfg.MaxRetries, slog.Default())
	t.Cleanup(func() { b2bClient.Close() })
	registerTools(enabled, bcEnabled, b2bClient, session.NewStore(cfg.CacheTTL))

	requiredCategories := []string{
		"b2b",
		"b2b/companies",
		"b2b/companies/users",
		"b2b/companies/addresses",
	}
	for _, cat := range requiredCategories {
		require.True(t, enabled.HasCategory(cat), "missing required category %q when enabled", cat)
	}

	requiredTools := []string{
		"b2b/companies/list",
		"b2b/companies/get",
		"b2b/companies/create",
		"b2b/companies/update",
		"b2b/companies/set_status",
		"b2b/companies/delete",
		"b2b/companies/users/list",
		"b2b/companies/users/create",
		"b2b/companies/users/update",
		"b2b/companies/users/delete",
		"b2b/companies/addresses/list",
		"b2b/companies/addresses/create",
		"b2b/companies/addresses/update",
		"b2b/companies/addresses/delete",
	}
	for _, toolPath := range requiredTools {
		def := enabled.GetTool(toolPath)
		require.NotNil(t, def, "missing required tool %q when enabled", toolPath)
	}

	// The gated subtree must also satisfy the same discovery invariants:
	// every active category returns at least one child.
	for _, cat := range enabled.ListCategoryPaths() {
		entries, err := enabled.Discover(cat)
		require.NoError(t, err)
		require.NotEmpty(t, entries, "discover_tools(%q) must not be empty", cat)
	}
}
