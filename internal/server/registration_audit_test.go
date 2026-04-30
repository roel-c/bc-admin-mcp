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

func TestFullRegistrationCatalogCategoriesAreNonEmptyLeaves(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, cache)

	for _, cat := range reg.ListCategoryPaths() {
		require.True(t, strings.HasPrefix(cat, "catalog"), "only catalog categories expected, got %q", cat)
		entries, err := reg.Discover(cat)
		require.NoError(t, err)
		require.NotEmpty(t, entries, "discover_tools(%q) must not be empty — add tools or subcategories", cat)
	}
}

func TestFullRegistrationEveryToolParentCategoryExists(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, cache)

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

func TestFullRegistrationRootIsCatalogOnly(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, cache)

	entries, err := reg.Discover("")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "catalog", entries[0].Path)
	require.Equal(t, "category", entries[0].Type)
}

func TestFullRegistrationDiscoveryBFSCoversAllCategoriesAndTools(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, cache)

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
	registerCategories(reg)
	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, cache)

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
