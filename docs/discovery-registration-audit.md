# Discovery vs `registerTools` audit

**Date:** 2026-04-28 (aligned with current `main` / catalog build)

## Policy

1. **No empty `discover_tools` nodes** — Every registered category under the active tree must return at least one child (subcategory or tool). Placeholder categories without tools produced `[]` and confused agents.
2. **Tool parent chain** — Every `RegisterTool` path must have each parent segment registered as a category (e.g. `catalog/products/metafields/set` requires `catalog`, `catalog/products`, `catalog/products/metafields`).
3. **Future domains** — `orders/`, `customers/`, etc. stay in **ARCHITECTURE.md** roadmap only until the first tool ships; then add `RegisterCategory` for the path prefix **in the same change** as tools.

## Outcome

| Check | Result |
|--------|--------|
| Catalog-only discovery root | `discover_tools("")` returns **`catalog`** only. |
| Non-catalog placeholder categories | **Removed** from `internal/server/server.go` `registerCategories` (previously registered `orders/*`, `customers/*`, `carts/*`, `inventory`, `marketing/*`, `store/*` with no tools). |
| Catalog subtree | Every `catalog/**` category has **≥1** child in `Registry.Discover` after full `registerTools`. |
| Tool → category linkage | Enforced by **`internal/server/registration_audit_test.go`** (`TestFullRegistrationEveryToolParentCategoryExists`, `TestFullRegistrationCatalogCategoriesAreNonEmptyLeaves`, `TestFullRegistrationRootIsCatalogOnly`, **`TestFullRegistrationDiscoveryBFSCoversAllCategoriesAndTools`**, **`TestFullRegistrationR1PlusToolsExposeConfirmedParameter`**). |

## Registry helpers

- `internal/discovery/registry.go`: **`ListCategoryPaths`**, **`ListToolPaths`**, **`HasCategory`** — used by server audit tests; safe for future doc generators.

## Maintenance

When adding a new domain:

1. Add category nodes from root down to the tool’s parent.
2. Register tools (paths under that parent).
3. Run `go test ./internal/server/...` — audit tests must pass.
