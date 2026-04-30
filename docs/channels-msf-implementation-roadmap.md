# Channels & MSF — implementation roadmap (this application)

This document turns [multi-storefront research](./msf-research-outline.md) into **phased work** so users can manage store data through this MCP using BigCommerce **Store Management** APIs (same OAuth token and store hash as today’s catalog tools).

**Principles:** one canonical REST path per concern where possible; **preview → confirm** for writes; explicit **OAuth scopes** and **bulk caps** in each tool; optional **`channel_id` / `tree_id`** on catalog tools when MSF matters.

---

## Phase 0 — Done (baseline)

| Capability | API | MCP |
|------------|-----|-----|
| List channels for the connected store | `GET /v3/channels` | `catalog/channels/list` |

Requires **`store_channel_settings`** (or equivalent) for channel reads.

---

## Phase 1 — Category trees ↔ channels (navigation / MSF)

| Capability | API | MCP / code |
|------------|-----|----------------|
| List category trees; filter by channel | `GET /v3/catalog/trees` with optional `channel_id:in` | **`catalog/channels/category_trees`** (R0) |
| Resolve default tree for a channel (internal) | same | `GetTreeIDForChannel` on `*bigcommerce.Client` |

**Scopes:** `store_v2_products` / `store_v2_products_read_only` (per Developer Center for catalog trees).

**Next code (same phase):** optionally thread resolved `tree_id` into category list/create when the merchant passes `channel_id` (or env default) — see `GetDefaultTreeID` in `internal/bigcommerce/products.go`.

---

## Phase 2 — Product ↔ channel assignment (list / assign / remove)

| Capability | API | MCP |
|------------|-----|-----|
| List assignments | `GET /v3/catalog/products/channel-assignments` | **`catalog/products/channel_assignments/list`** (R0; requires `product_ids` and/or `channel_ids`; caps in tool docs) |
| Bulk assign | `PUT /v3/catalog/products/channel-assignments` | **`catalog/products/channel_assignments/assign`** (R1; preview → `confirmed`; cartesian product; chunked `ProductBatchSize`) |
| Remove | `DELETE` with `product_id:in` and optional `channel_id:in` | **`catalog/products/channel_assignments/remove`** (R2; **product_ids required**; channel-only delete not exposed) |

**Scopes:** **`store_v2_products`** (or read-only) for GET/PUT/DELETE on this path; channel listing still benefits from **`store_channel_settings`** when resolving channel IDs via `catalog/channels/list`.

---

## Phase 3 — Channel listings (list/delist + per-channel listing copy)

| Capability | API | MCP |
|------------|-----|-----|
| List listings | `GET /v3/channels/{channel_id}/listings` (cursor `limit` + `after`) | **`catalog/channels/listings/list`** (R0; optional `product_ids` → `product_id:in`; fetches up to 2000 rows) |
| Create listings | `POST /v3/channels/{channel_id}/listings` | **`catalog/channels/listings/create`** (R1; **`listings_json`** array; preview → `confirmed`; max **10** objects; each needs **product_id**, **state**, **variants** per OpenAPI) |
| Update listings (state, overrides, variants) | `PUT /v3/channels/{channel_id}/listings` | **`catalog/channels/listings/update`** (R2; same JSON cap; each object needs **listing_id**, **product_id**, **state**, **variants**) |

**Scopes:** **`store_channel_listings`** (modify) for POST/PUT; **`store_channel_listings_read_only`** for GET where applicable.

Docs: [Get channel listings](https://docs.bigcommerce.com/developer/api-reference/rest/admin/management/channels/listings/get-channel-listings). Request shapes follow BigCommerce **channels.v3** `CreateMultipleListingsReq` / `UpdateMultipleListingsReq`.

---

## Phase 4 — Products in channel context

| Capability | API | MCP |
|------------|-----|-----|
| List/filter products by channel | `GET /v3/catalog/products?channel_id:in=…` | **`catalog/products/search`** accepts **`channel_ids`** (max 20; mapped to `channel_id:in`) |
| Channel-aware category navigation | `GET /v3/catalog/trees/categories?tree_id:in=…` (tree resolved via `GET /v3/catalog/trees?channel_id:in=…`) | **`catalog/categories/list`** & **`catalog/categories/create`** accept **`channel_id`** (mutually exclusive with `tree_id`); resolution uses `GetTreeIDForChannel` |
| Channel-aware product writes | (no writeable `channels` field on products; assignment uses `PUT /v3/catalog/products/channel-assignments`) | **`catalog/products/create`** & **`catalog/products/update`** accept **`channel_ids`** as an *additive post-write side-effect*. Update enforces `len(products) × len(channel_ids) ≤ 500`. If any catalog write fails, the assignment step is **skipped** and the response is `partial_success` (never silently destructive). |
| Aggregated MSF snapshot | `GET /v3/channels` + `GET /v3/catalog/products/channel-assignments` + `GET /v3/channels/{id}/listings` | **`catalog/products/channel_summary`** (R0) joins all three reads, returns per-product `assigned_channels`, `listings_by_channel`, `channels_assigned_without_listing`, `channels_with_listing_but_no_assignment`. Max 5 products, 25 channels per call. |

**Verified against BigCommerce OpenAPI (April 2026):** `POST /v3/catalog/products` and `PUT /v3/catalog/products[/{id}]` do **not** accept a writeable `channels` field. The catalog → channel association is owned exclusively by `/v3/catalog/products/channel-assignments`, which is why threading `channel_ids` through create/update is implemented as a sequential post-write call rather than a payload field.

**Open follow-ups for phase 4:** consider extending the assignment side-effect to optionally seed channel **listings** (`POST /v3/channels/{id}/listings`) for marketplace-style channels where assignment alone does not surface the product. Today this stays a deliberate two-step (`channel_ids` → `catalog/channels/listings/create`) so the user explicitly opts into listing-state semantics.

---

## Phase 4b — Surface ergonomics

- **Listing state validation:** `catalog/channels/listings/create|update` reject unknown `state` values at the tool boundary so we never round-trip a 422 through BigCommerce. Listing-level enum: `active, disabled, error, pending, pending_disable, pending_delete, partially_rejected, queued, rejected, submitted, deleted`. Variants reuse the same set minus `partially_rejected`.
- **Assignments vs listings rubric:** `bc_system_prompt.md` documents which surface to choose per intent (availability vs state/copy).
- **Filter-based unassign:** **`catalog/products/unassign_categories`** (R2, preview → confirm) targets `DELETE /v3/catalog/products/category-assignments?product_id:in=…&category_id:in=…` so users can drop a few category links without the destructive `categories=[…]` replacement on `products/update`.
- **Scope-aware client errors:** `bigcommerce.APIError` now carries the failing path / method and the `Error()` / `SafeError()` strings include OAuth-scope hints for 401 / 403 (e.g. `store_channel_listings_read_only`, `store_v2_products`).

---

## Phase 5 — Rich per-channel / per-locale data (optional, plan-gated)

| Capability | Surface | Notes |
|------------|---------|--------|
| Broader overrides (SEO, modifiers, pre-order, …) | Admin **GraphQL** + MSF international enhancements | Often **Enterprise**; out of scope until product commits. [Overview](https://developer.bigcommerce.com/docs/store-operations/catalog/msf-international-enhancements/overview) |

---

## Verification (any phase)

1. Token includes the scopes documented for that endpoint.
2. On a multi-storefront store: `GET /v3/channels` then `GET /v3/catalog/trees?channel_id:in=` for each storefront id — tree ids should align with category operations.
3. Re-read OpenAPI for **listings** vs **channel-assignments** before implementing writes.

---

## References

- In-repo: [`msf-research-outline.md`](./msf-research-outline.md), [`BC-API-Reference.md`](./BC-API-Reference.md) §6.12
- External: [Channels API](https://developer.bigcommerce.com/api-reference/store-management/channels-api), [Channel assignments](https://developer.bigcommerce.com/docs/rest-catalog/products/channel-assignments), [MSF API guide](https://docs.bigcommerce.com/developer/docs/admin/multi-storefront/api-guide)
