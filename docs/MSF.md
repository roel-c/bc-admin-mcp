# Multi-Storefront (MSF) — Research & Implementation Record

Merged from `msf-research-outline.md` (API research) and `channels-msf-implementation-roadmap.md` (phased delivery). All phases 0–4 are shipped. Phase 5 is plan-gated.

**Terminology:** BigCommerce uses **channel** in REST paths; merchants often say **storefront**. A storefront channel is typically `type: "storefront"` on `GET /v3/channels`.

---

## Shipped Tools (Phases 0–4)

| Phase | Capability | MCP Tool | Tier |
|-------|-----------|---------|------|
| 0 | List store channels | `catalog/channels/list` | R0 |
| 0 | Get a single channel | `catalog/channels/get` | R0 |
| 0 | Update channel name/status | `catalog/channels/update` | R2 |
| 1 | List category trees; filter by channel | `catalog/channels/category_trees` | R0 |
| 2 | List product↔channel assignments | `catalog/products/channel_assignments/list` | R0 |
| 2 | Bulk assign products to channels | `catalog/products/channel_assignments/assign` | R1 |
| 2 | Remove assignments | `catalog/products/channel_assignments/remove` | R2 |
| 3 | List channel product listings | `catalog/channels/listings/list` | R0 |
| 3 | Create listings | `catalog/channels/listings/create` | R1 |
| 3 | Update listings | `catalog/channels/listings/update` | R2 |
| 4 | Filter products by channel | `catalog/products/search` (`channel_ids`) | R0 |
| 4 | Channel-aware category list/create | `catalog/categories/list`, `catalog/categories/create` (`channel_id`) | R0/R1 |
| 4 | Additive post-write channel assignment | `catalog/products/create`, `catalog/products/update` (`channel_ids`) | R1 |
| 4 | Aggregated MSF snapshot | `catalog/products/channel_summary` | R0 |

**OAuth scopes required:**
- Channel reads: `store_channel_settings_read_only`
- Channel update: `store_channel_settings`
- Listings (read): `store_channel_listings_read_only`
- Listings (write): `store_channel_listings`
- Category trees + assignments: `store_v2_products` / `store_v2_products_read_only`

---

## MSF Detection Heuristics

There is **no single documented flag** that states "multi-storefront is on." Infer from channel and catalog shape:

| Signal | API | Interpretation |
|--------|-----|----------------|
| Multiple active storefront channels | `GET /v3/channels` | Filter `type == storefront` and `status` in active lifecycle. >1 active storefront ⇒ MSF matters. |
| Multiple category trees | `GET /v3/catalog/trees` | Multiple trees or distinct `channel_id` values ⇒ channel-scoped navigation is in use. |
| Product listings per channel | `GET /v3/channels/{id}/listings` | Presence of listing records supports "listed on storefront A but not B." |

Every store has at least **channel id 1** (default Stencil storefront). Counting ≥2 storefront channels is the strongest signal.

---

## Key Implementation Details

### Channel assignments vs listings

Products are **assigned** to channels at the catalog level (`/v3/catalog/products/channel-assignments`). Assignments control availability — whether a product is part of that channel's sellable catalog at all.

**Listings** (`/v3/channels/{id}/listings`) control channel-specific state and presentation. A product can be assigned but have a `disabled` listing, which is why it appears assigned but not visible on the storefront.

See `DEVELOPMENT.md §9` for the full decision rubric.

### Additive channel_ids side-effect

`catalog/products/create` and `catalog/products/update` accept `channel_ids` as a post-write side-effect. BigCommerce's product API does not accept a `channels` field directly — the association is owned by `/v3/catalog/products/channel-assignments`. The side-effect is:
- **Additive** — never removes existing assignments.
- **Skipped** if any catalog write fails (`partial_success` response).
- Capped at `len(products) × len(channel_ids) ≤ 500` pairs per call.

### Listing state values

`active`, `disabled`, `error`, `pending`, `pending_disable`, `pending_delete`, `partially_rejected`, `queued`, `rejected`, `submitted`, `deleted`. Variant-level listings use the same set minus `partially_rejected`. Validated at the tool boundary; unknown states are rejected before the BC API call.

### Filter-based category unassign

`catalog/products/unassign_categories` (R2) targets `DELETE /v3/catalog/products/category-assignments?product_id:in=…&category_id:in=…`. This lets users drop specific category links without the destructive `categories=[…]` replacement on `products/update`.

### Scope-aware client errors

`bigcommerce.APIError` includes OAuth-scope hints in `Error()` / `SafeError()` for 401 / 403 responses (e.g. `store_channel_listings_read_only`, `store_v2_products`).

---

## Surface checks on MSF stores

When running the D2C or B2B **Full Surface Check** (`WORKFLOW.md` §10) on a store
with multiple active storefront channels, **ask the operator which channel(s) to
target before creating any sample data**. Do not assume channel 1 or reuse a
prior run's channel. Scope catalog writes (`channel_id` / `channel_ids`),
customer identities (`origin_channel_id` / `channel_ids`), and carts
(`channel_id`) to the chosen channel. B2B runs additionally confirm the channel
is B2B-enabled via `b2b/channels/list` and link pre-created BC customers via
`bc_customer_id`.

---

## Open Follow-ups

| Item | Status | Notes |
|------|--------|-------|
| Optional listing-seeding alongside channel assignment | Deliberate two-step today | `channel_ids` on create/update → then separately `catalog/channels/listings/create` for marketplace channels that need an explicit listing state. Consider optional `seed_listings=true` flag. |
| GraphQL Admin / locale overlays (MSF+) | Deferred — Enterprise/plan-gated | International per-channel name/SEO overrides; see [MSF international enhancements](https://developer.bigcommerce.com/docs/store-operations/catalog/msf-international-enhancements/overview). |
| `BC_DEFAULT_CHANNEL_ID` config | Not implemented | Could resolve default channel from `GET /v3/channels` rather than hardcoding `1`. |

---

## Verification Checklist (before adding new MSF tools)

1. Call `GET /v3/channels` with production token; record `type`, `status`, `id` for storefront rows.
2. Call `GET /v3/catalog/trees` unfiltered and with `channel_id:in=` for each storefront id; compare tree IDs.
3. Call `GET /v3/catalog/products/channel-assignments` with a small `product_id:in` sample; confirm filter syntax.
4. Confirm token includes the required scopes; note failures for docs.
5. Re-read OpenAPI for `/v3/channels/{id}/listings` vs catalog channel-assignments for your target workflow.

---

## References

- `DEVELOPMENT.md §9` — Channel assignments vs listings decision rubric
- `BC-API-Reference.md §6.12` — Channels & MSF endpoint map
- `internal/bigcommerce/channels.go` — `ListStoreChannels`, `GetStoreChannel`, `UpdateStoreChannel`
- `internal/bigcommerce/category_trees.go` — `ListCategoryTrees`, `GetTreeIDForChannel`
- `internal/bigcommerce/channel_assignments.go` — assignment CRUD
- `internal/bigcommerce/channel_listings.go` — listing CRUD
- [Channels API](https://developer.bigcommerce.com/api-reference/store-management/channels-api)
- [Product channel assignments](https://developer.bigcommerce.com/docs/rest-catalog/products/channel-assignments)
- [MSF API guide](https://docs.bigcommerce.com/developer/docs/admin/multi-storefront/api-guide)
