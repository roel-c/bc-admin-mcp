# Catalog completion checklist (before remaining domains)

Work through this list to keep the catalog surface honest, align discovery with reality, and lock in **patterns** (preview/confirm, bulk caps, metafield semantics, progressive paths) so remaining domains (orders, carts, inventory, store) can reuse the same structures.

---

## Preconditions (why this list exists)

- **Catalog** is the most complex surface we have implemented so far (products, categories, variants, options, images, custom fields, modifiers, metafields + bulk).
- **Active domains** now include `catalog/`, `orders/`, `customers/`, `marketing/`, and `inventory/` (initial read tools). Remaining domains (`carts/`, `store/`) stay roadmapped in `ARCHITECTURE.md` and are **not** registered in `discover_tools` until tools exist (avoids empty discovery leaves).
- Shipping **orders / customers / …** with the **same** tiering, preview/confirm, naming, and bulk discipline will be easier if catalog is **honest and complete** for the scope we claim.

---

## Checklist

- [x] **Brands (`catalog/brands`)** — Brand tools: list (filters + `list_all`), get, create, update (V3 `catalog/brands`; preview → `confirmed`). Brand metafields: `catalog/brands/metafields/list`, `set`, `delete` (same patterns as category metafields; `brand_id` or exact `brand_name`).

- [x] **`catalog/variants` vs `catalog/products/variants`** — **Product-scoped** variant CRUD + metafields remain under **`catalog/products/variants`**. **Global** catalog variants: tools **`catalog/variants/list`** (R0, `GET /v3/catalog/variants`; filters, `product_ids` / `variant_ids` caps 100, `list_all`) and **`catalog/variants/bulk_update`** (R2, `PUT /v3/catalog/variants`; up to **200** rows per call, chunked by **`BC_VARIANT_BATCH_SIZE`** default 10; preview → `confirmed`). Client: `SearchVariants`, `ListVariantsByProductIDs` (wrapper), `BatchUpdateVariants` in `internal/bigcommerce/variants_catalog.go`.

- [x] **Discovery ↔ `registerTools` audit** — Placeholder categories without tools are omitted so `discover_tools` never returns empty navigable leaves. **`internal/server/registration_audit_test.go`** locks active roots (`catalog`, `orders`, `customers`, `marketing`, `inventory`), non-empty active categories, and tool-parent category integrity. Registration policy documented in **`ARCHITECTURE.md §8`**.

- [ ] **Multi-storefront / channels / routes (MSF)** — When catalog work must respect **channel-specific** listings, pricing, or metafield visibility, add API + tool design **before** claiming parity with Control Panel. Until then, keep **single-channel / default channel** assumptions explicit in README / `AGENT.md`. **Research, shipped tools, and open follow-ups:** [`docs/MSF.md`](./MSF.md). **Shipped so far:** **`catalog/channels/list`**, **`catalog/channels/category_trees`**, **`catalog/channels/listings/*`** (list / create / update), **`catalog/products/channel_assignments/*`**, **`catalog/products/unassign_categories`** (filter-based DELETE), **`channel_ids`** filter on `catalog/products/search`, **`channel_id`** option on `catalog/categories/list` / `catalog/categories/create` (resolves `tree_id` server-side via `GetTreeIDForChannel`), additive **`channel_ids`** post-write side-effect on `catalog/products/create` / `catalog/products/update` (≤ 500 product×channel pairs per call; `partial_success` if any catalog write fails), and **`catalog/products/channel_summary`** aggregator. Listing `state` enums are validated at the tool boundary; client `APIError` now returns OAuth-scope hints for 401 / 403; assignments-vs-listings rubric is documented in `bc_system_prompt.md`. Open follow-up: optional listing-seeding alongside assignment for marketplace channels (currently a deliberate two-step).

- [ ] **Bulk and batch beyond current caps** — Review whether **50 product IDs**, **50 variant IDs**, **500 cross-product variant ops**, and **substring SKU** targeting are sufficient for real migrations; document escalation path (chained calls, external jobs) or add **cursor/pagination** if productized.

- [x] **Pricing-adjacent catalog (scope decision)** — **Price lists are now in-scope under `catalog/pricelists/*`**, including records and assignments subtrees (`catalog/pricelists/records/*`, `catalog/pricelists/assignments/*`) with preview→confirm guardrails and serial policy for record upserts.

- [ ] **Other V3 catalog resources (as needed)** — Examples merchants sometimes ask for: **product videos**, **complex rules**, **bulk pricing imports** — triage against your product promise; add one line each to this doc or spawn `docs/catalog-future.md` when deferred.

- [ ] **Category tree vs legacy category APIs** — Already called out for category metafields in code comments; confirm **docs** state which paths are used so agents do not assume tree-only behavior everywhere.

- [ ] **Agent prompt sync** — After checklist changes, refresh **`AGENT.md`** tool tables vs `RegisterTool` paths and **`README.md`** counts/examples. (Discovery root / README hierarchy updated with the discovery audit; full stub table parity is still optional follow-up.)

- [ ] **Pattern freeze for cross-domain reuse** — Capture a short **internal** note (can live at the bottom of this file): e.g. R0–R4 tiers, `confirmed` preview flow, `tool_path` + `arguments`, bulk naming (`bulk_*`, `*_products`), and mock/test conventions — to copy when implementing **orders/** and **customers/**.

---

## Catalog implementation reference (synced with current build)

Use this when extending catalog or mirroring patterns in new domains:

| Area | Location | Notes |
|------|-----------|--------|
| Metafield MCP + execution | `internal/tools/catalog/metafield_shared.go` | `MetafieldResourceOps` binds List/Create/Update/Delete per resource. `metafieldUpsertCore` / `metafieldDeleteCore` handle preview → confirm for tools. **`metafieldUpsertExecute`** is the shared **confirmed** upsert (used by MCP after confirm and by **`executeProductMetafieldUpsert` / `executeVariantMetafieldUpsert`** for bulk). Product/variant use **`metafieldUpsertOptions`** (`PreserveEmptyPermissionOnUpdate`, `AppOnlyStylePreview`, etc.); category/brand use defaults (`write` on create, no permission preview). |
| List / filter helpers | `internal/tools/catalog/list_filter_helpers.go` | `ReadListAllBoolean`, `HasDataFilterBCParams`, `ErrInvalidBCSort` — products, categories, brands, global variants. |
| Variant updates from maps | `internal/tools/catalog/variant_update_parse.go` | `ApplyProductVariantUpdateFromMap`, `HasProductVariantUpdateChanges` — product variant update + global bulk rows. |
| Bulk metafield caps | `products_metafields_bulk.go`, `products_variants_metafields_bulk.go` | Product bulk: max **50** `product_ids`. Variant bulk: per-tool caps (50 variant ids / substring scope, 50 products × cross-product writes up to **500** / call) — unchanged; orchestration stays in bulk handlers. |

**Docs:** `ARCHITECTURE.md` section 4 (file table + “Catalog code reuse”), `README.md` project structure, section 8 (registration policy), and this section should move together when catalog layout changes.

---

## After this checklist

When the items above are either **done** or **explicitly deferred with doc links**, proceed to **orders**, **customers**, etc., reusing:

- Progressive category paths + thin stubs where not implemented  
- Same **execute_tool** envelope and preview/confirm story  
- Similar **bulk** caps and error messages  
- Same **BigCommerceAPI** interface + gomock pattern for tests  

---

## Maintenance

- Bump this checklist when adding a new **catalog** tool category or removing a placeholder.  
- Keep “deferred” items **one click** away (issue link or `docs/catalog-future.md`) so the list stays honest.  
- After refactors that change **shared helpers** or **tool counts**, update **`ARCHITECTURE.md`** (diagram + section 4) and the **implementation reference** table above.
