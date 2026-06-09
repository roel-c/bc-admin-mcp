# Discovery vs `registerTools` audit

**Date:** 2026-05-07 (catalog + orders + customers + customer segmentation + marketing/promotions + initial inventory read tools; catalog price-list subtrees and initial order subtrees are registered). Updated 2026-06-03 to add `storefront/scripts` and `webhooks/*` roots.

## Policy

1. **No empty `discover_tools` nodes** — Every registered category under the active tree must return at least one child (subcategory or tool). Placeholder categories without tools produced `[]` and confused agents.
2. **Tool parent chain** — Every `RegisterTool` path must have each parent segment registered as a category (e.g. `catalog/products/metafields/set` requires `catalog`, `catalog/products`, `catalog/products/metafields`).
3. **Future domains** — `orders/`, `customers/`, etc. stay in **ARCHITECTURE.md** roadmap only until the first tool ships; then add `RegisterCategory` for the path prefix **in the same change** as tools.

## Outcome

| Check | Result |
|--------|--------|
| Discovery roots | `discover_tools("")` returns **`catalog`**, **`orders`**, **`customers`**, **`marketing`**, **`inventory`**, **`storefront`**, and **`webhooks`**. Each root must have at least one tool reachable below it. |
| Non-active placeholder categories | Remaining roots (`carts/*`, `store/*`) remain **omitted** from `internal/server/server.go` `registerCategories` until tools ship. |
| Active subtrees | Every `catalog/**`, `orders/**`, `customers/**`, `marketing/**`, `inventory/**`, `storefront/**`, and `webhooks/**` category has **≥1** child in `Registry.Discover` after full `registerTools`. |
| Tool → category linkage | Enforced by **`internal/server/registration_audit_test.go`** (`TestFullRegistrationEveryToolParentCategoryExists`, `TestFullRegistrationActiveCategoriesAreNonEmptyLeaves`, `TestFullRegistrationActiveRoots`, **`TestFullRegistrationDiscoveryBFSCoversAllCategoriesAndTools`**, **`TestFullRegistrationR1PlusToolsExposeConfirmedParameter`**). |

## Registry helpers

- `internal/discovery/registry.go`: **`ListCategoryPaths`**, **`ListToolPaths`**, **`HasCategory`** — used by server audit tests; safe for future doc generators.

## Maintenance

When adding a new domain:

1. Add category nodes from root down to the tool’s parent.
2. Register tools (paths under that parent).
3. Run `go test ./internal/server/...` — audit tests must pass.
