# Development Guide — Tool Boundaries, Risk Tiers & Caps

The developer reference for building and extending tools in this MCP server. Covers risk tiers, numeric caps, OAuth scopes, concurrency policy, and the channel assignment model. Read this before adding a new tool or domain.

Consolidates rules from:
- `AGENT.md` — operator policy (what the agent must do)
- `BC-API-Reference.md` — BigCommerce limits, concurrency, and LLM guidelines
- `internal/bigcommerce/client.go` and `internal/config/config.go` — implemented constants

For field-level request/response shapes see `BC-API-Reference.md` and the official Developer Center.

---

## 1. Tool tiers (recommended)

Use these tiers when defining MCP tools (or HTTP actions) so permissions and confirmations stay explicit.

| Tier | Intent | Examples | Operator confirmation |
|------|--------|----------|------------------------|
| **R0 — Read** | Fetch only; no mutation | Store profile, list/get products, orders, customers, categories, inventory levels | None |
| **R1 — Write (standard)** | Idempotent-ish catalog/settings updates | Product SEO fields, category SEO, inventory location metafields, redirects, `is_visible` toggles | **Preview + confirm** for bulk; single-record may be lighter-touch per policy |
| **R2 — Write (high-risk)** | Financial / inventory / pricing | Price list record upserts, inventory adjustments/location create-update, cart/checkout server calls | **Always confirm** scope (list name, record count, before/after) |
| **R3 — Destructive** | Irreversible or legally sensitive | Product **DELETE**, inventory location delete, order payment capture/refund/void, customer password/auth fields | **Explicit per-resource confirmation**; default deny |
| **R4 — Forbidden (default)** | Unless task explicitly says so | Hard-delete products, `description` HTML overwrite, payment status changes without order ID + approval | Block at tool layer |

**Principle:** R0 tools can be exposed broadly. R1–R2 should accept a **`confirmed: bool`** or separate **`propose_*`** vs **`apply_*`** tools. R3 should require **`confirmation_token`** or human-approved step.

---

## 2. Numeric caps (single source of truth)

### 2.1 Implemented in the MCP server (`internal/config/config.go`)

| Env var | Default | Meaning |
|---------|---------|---------|
| `BC_REQUESTS_PER_SECOND` | `2.0` | Global throttle between requests |
| `BC_QUOTA_SAFETY_BUFFER` | `25` | If `X-Rate-Limit-Requests-Left` ≤ this, pause until reset |
| `BC_MAX_RETRIES` | `6` | 429 / 5xx backoff rounds |
| `BC_PRODUCT_BATCH_SIZE` | `10` | Max items per batch **PUT** `/v3/catalog/products` (validated 1–10) |
| `BC_VARIANT_BATCH_SIZE` | `10` | Max items per batch **PUT** `/v3/catalog/variants` (validated 1–10) |
| `BC_INVENTORY_BATCH_SIZE` | `10` | Safe batch size for inventory high-risk writes (`inventory/items/update_batch`, `inventory/adjustments/absolute`, `inventory/adjustments/relative`) |
| `BC_DEFAULT_PAGE_LIMIT` | `250` | Page size for most V3 list endpoints (validated 1–250) |
| `BC_MAX_TOTAL_RECORDS` | `10000` | Pagination ceiling for `GetAll` (set `0` for unlimited) |
| `BC_DELAY_BETWEEN_CHUNKS_MS` | `500` | Inter-chunk pause inside `BatchPut` (on top of the throttle) |
| `BC_MAX_WRITE_CONCURRENCY` | `1` | Reserved for throughput mode; **`BatchPut` is sequential today** regardless of this value |
| `BC_CACHE_TTL_SECONDS` | `60` | Per-session cache TTL for preview/confirm snapshots |

The `categories` batch-update endpoint (`PUT /v3/catalog/trees/categories`) uses an internal `categoryBatchSize = 50` constant in `internal/bigcommerce/products.go` — not configurable today.

### 2.2 Store plan quotas (from `BC-API-Reference.md`)

| Plan | Requests / 30 s (typical) | Notes |
|------|---------------------------|--------|
| Standard / Plus | 150 | Global window |
| Pro | 450 | Higher throughput possible with care |
| Enterprise | Custom | Follow headers |

Always honor response headers; **the MCP server does not raise plan-specific ceilings** — it uses conservative defaults.

### 2.3 Per-endpoint concurrency (BigCommerce)

| Endpoint pattern | Concurrent calls | Batch inner size |
|------------------|------------------|-------------------|
| `/v3/pricelists/{id}/records` (upsert) | **1 — serial only** | Endpoint supports large batches; MCP tool currently caps at **100** rows/call for safety |
| `/v3/catalog/products` batch PUT | Recommend **≤ 3** parallel batch requests | **10** products per request |
| `/v3/catalog/variants` batch PUT | Recommend **≤ 3** parallel | **10** per request |
| `/v3/inventory/items` and `/v3/inventory/adjustments` | Recommend **≤ 5** parallel | **10** rows per request |
| General Management | **10–20** possible | Monitor 429s |
| Webhook registration | **Serial** | Single |

**Project policy (`AGENT.md`):** default to **sequential** writes (no extra threads) unless the operator explicitly opts into higher concurrency. That is **stricter** than the reference’s “3–5 threads” throughput pattern — intentional for live-store safety.

### 2.4 Operator “test mode” (prompt policy)

- First bulk run: **≤ 5 records** sample; scale after confirmation.

### 2.5 Per-tool caps enforced in handlers

These caps live in `internal/tools/catalog/` and are validated **before** any BigCommerce request fires. Exceeding them returns an explicit error so the LLM can split the call instead of round-tripping a 422.

| Tool | Cap | Source |
|------|-----|--------|
| `catalog/products/update` | ≤ 500 product × `channel_ids` pairs (additive post-write assignment) | `products_update.go` |
| `catalog/products/assign_categories` | `product_ids ≤ 100`, `category_ids ≤ 50`, pairs ≤ 500 | `categories_assignments.go` |
| `catalog/products/unassign_categories` | `product_ids ≤ 100`, `category_ids ≤ 50` | `categories_assignments.go` |
| `catalog/products/channel_assignments/list` | `product_ids ≤ 100`, `channel_ids ≤ 20` | `products_channel_assignments.go` |
| `catalog/products/channel_assignments/assign` | pairs ≤ 500 | `products_channel_assignments.go` |
| `catalog/products/channel_assignments/remove` | `product_ids ≤ 100`, `channel_ids ≤ 20` | `products_channel_assignments.go` |
| `catalog/products/channel_summary` | `product_ids ≤ 5`, channels touched ≤ 25 | `products_channel_summary.go` |
| `catalog/products/metafields/bulk_set` / `bulk_delete` | `product_ids ≤ 50` | `products_metafields_bulk.go` |
| `catalog/products/variants/metafields/bulk_set` / `bulk_delete` | one product, ≤ 50 variants | `products_variants_metafields_bulk.go` |
| `catalog/products/variants/metafields/bulk_set_products` / `bulk_delete_products` | `product_ids ≤ 50`, total variant writes ≤ 500 | `products_variants_metafields_bulk.go` |
| `catalog/variants/list` | `product_ids ≤ 100`, `variant_ids ≤ 100` | `variants_global.go` |
| `catalog/variants/bulk_update` | ≤ 200 rows per call (server chunks by `BC_VARIANT_BATCH_SIZE`) | `variants_global.go` |
| `catalog/brands/delete` | **R3 destructive**; `DELETE /v3/catalog/brands/{id}`; products keep existing (brand link cleared); preview then confirm | `internal/tools/catalog/brands.go` |
| `catalog/brands/image/set` | **R1**; sets `image_url` via brand update (BC fetches the public URL; multipart upload not supported); preview then confirm | `internal/tools/catalog/brands.go` |
| `catalog/brands/image/delete` | **R2**; `DELETE /v3/catalog/brands/{id}/image`; preview then confirm | `internal/tools/catalog/brands.go` |
| `catalog/channels/get` | R0; `channel_id` required; `GET /v3/channels/{id}` | `internal/tools/catalog/channel_tools.go` |
| `catalog/channels/update` | **R2**; `channel_id` required; at least one of `name` / `status`; valid statuses: active, inactive, connected, disconnected, prelaunch; deleted/terminated channels cannot be updated (BC returns 422); preview then confirm | `internal/tools/catalog/channel_tools.go` |
| `catalog/channels/listings/list` | up to 2000 rows fetched per call; `product_ids` filter ≤ 50 | `channel_listings_tools.go` |
| `catalog/channels/listings/create` / `update` | `listings_json` ≤ 10 objects, payload ≤ 256 KiB | `channel_listings_tools.go` |
| `catalog/pricelists/list` / `catalog/pricelists/assignments/list` / `catalog/pricelists/records/list` | supports offset and cursor pagination; reject `page` + cursor combinations | `pricelists_tools.go` |
| `catalog/pricelists/create` / `update` | preview-then-confirm; update is fetch-merge-PUT | `pricelists_tools.go` |
| `catalog/pricelists/delete` | **R3 destructive** — preview then `confirmed=true` | `pricelists_tools.go` |
| `catalog/pricelists/records/upsert` | `records ≤ 100` rows/call (serial write policy) | `pricelists_tools.go` |
| `catalog/pricelists/records/delete` | **R2**; requires selectors (`variant_ids` or `skus`), optional currency | `pricelists_tools.go` |
| `catalog/pricelists/assignments/create_batch` | `assignments ≤ 25` rows/call | `pricelists_tools.go` |
| `catalog/pricelists/assignments/upsert` | **R2**; requires `price_list_id`, `customer_group_id`, `channel_id`; preview then confirm | `pricelists_tools.go` |
| `catalog/pricelists/assignments/delete` | at least one selector required (`id`, `price_list_id`, `customer_group_id`, `channel_id`, or `channel_ids`) | `pricelists_tools.go` |
| `inventory/locations/create` | **R2**; requires `location` object; preview then confirm | `internal/tools/inventory/tools.go` |
| `inventory/locations/update` | **R2**; requires `location_id` + `patch` object; preview then confirm | `internal/tools/inventory/tools.go` |
| `inventory/locations/delete` | **R3 destructive**; requires `location_id`; preview then confirm | `internal/tools/inventory/tools.go` |
| `inventory/locations/metafields/list` | R0; requires `location_id`; optional `page`/`limit` | `internal/tools/inventory/tools.go` |
| `inventory/locations/metafields/set` | **R1**; upsert by `namespace` + `key`; new metafields default `permission_set=app_only`; preview then confirm | `internal/tools/inventory/tools.go` |
| `inventory/locations/metafields/delete` | **R1**; delete by `metafield_id` or `namespace` + `key`; preview then confirm | `internal/tools/inventory/tools.go` |
| `inventory/items/update_batch` | **R2**; requires `update` object with either `items[]` or `data[]`; max **10** rows/call; preview then confirm | `internal/tools/inventory/tools.go` |
| `customers/groups/list` | offset paginated; respects `BC_DEFAULT_PAGE_LIMIT` and `BC_MAX_TOTAL_RECORDS` (max 50 pages) | `internal/bigcommerce/customer_groups.go` |
| `customers/groups/create` / `update` | `discount_rules` mixing `price_list` with other rule types is silently pruned (price_list wins) and surfaced as a `warnings` field; PUT overwrites discount_rules in bulk per BC | `internal/tools/customers/groups.go` |
| `customers/groups/delete` | **R3 destructive** — preview then `confirmed=true`; BC unassigns all members automatically | `internal/tools/customers/groups.go` |
| `customers/list` | R0; requires a real filter or `list_all=true`; GET `/v3/customers` | `internal/tools/customers/customer_records.go` |
| `customers/get` | R0; single customer via `id:in` | `internal/tools/customers/customer_records.go` |
| `customers/create` / `update` | **R2**; `new_password` needs `set_password=true` and `confirmed=true`; BC max **10** per POST/PUT | `internal/tools/customers/customer_records.go` |
| `customers/delete` | **R3**; max **50** ids; preview then confirm | `internal/tools/customers/customer_records.go` |
| `customers/assign_group` | **R2**; max **100** ids, chunked PUTs of **10**; `group_id` **0** clears assignment | `internal/tools/customers/customer_records.go` |
| `customers/addresses/list` | R0; filter or `list_all=true` | `internal/tools/customers/customer_addresses_tools.go` |
| `customers/addresses/create` / `update` | R1; max **25** rows per call | `internal/tools/customers/customer_addresses_tools.go` |
| `customers/addresses/delete` | **R3**; max **50** ids | `internal/tools/customers/customer_addresses_tools.go` |
| `customers/attributes/list` | R0; filter or `list_all=true`; GET `/v3/customers/attributes` | `internal/tools/customers/attributes_tools.go` |
| `customers/attributes/create` | R1; max **10** per call; `type` immutable after create (validated to one of `string`, `number`, `date`) | `internal/tools/customers/attributes_tools.go` |
| `customers/attributes/update` | R1; only `name` mutable — passing `type` is rejected; max **10** per call | `internal/tools/customers/attributes_tools.go` |
| `customers/attributes/delete` | **R3 destructive** — cascades to every stored value of the attribute on every customer; max **50** ids; preview then confirm | `internal/tools/customers/attributes_tools.go` |
| `customers/attribute_values/list` | R0; requires `customer_ids`, `attribute_ids`, `attribute_value`/`attribute_value_in`, or `list_all=true` | `internal/tools/customers/attribute_values_tools.go` |
| `customers/attribute_values/upsert` | R1; max **10** rows; key is `(customer_id, attribute_id)`; BC coerces `value` to attribute type | `internal/tools/customers/attribute_values_tools.go` |
| `customers/attribute_values/delete` | **R2**; max **50** ids; deletes individual values (definition unaffected) | `internal/tools/customers/attribute_values_tools.go` |
| `customers/metafields/list` | R0; per-customer when `customer_id` is set; otherwise filter or `list_all=true` against `/v3/customers/metafields` | `internal/tools/customers/metafields_tools.go` |
| `customers/metafields/set` | R1; namespace+key upsert on one customer; `permission_set` defaults to `app_only` (not Storefront-readable) | `internal/tools/customers/metafields_tools.go` |
| `customers/metafields/delete` | R1; by `metafield_id` or `namespace`+`key`; preview then confirm | `internal/tools/customers/metafields_tools.go` |
| `customers/metafields/bulk_set` / `bulk_delete` | R1; sequential per-customer API calls; max **50** customers per call; `bulk_delete` skips customers without that namespace+key | `internal/tools/customers/metafields_tools.go` |
| `customers/settings/global/get` | R0; GET `/v3/customers/settings` | `internal/tools/customers/customer_settings_tools.go` |
| `customers/settings/global/update` | **R2**; merges `settings` into current then PUT; preview shows merged_preview; `confirmed=true` | `internal/tools/customers/customer_settings_tools.go` |
| `customers/settings/channel/get` | R0; GET `/v3/customers/settings/channels/{channel_id}` | `internal/tools/customers/customer_settings_tools.go` |
| `customers/settings/channel/update` | **R2**; merges `settings` then PUT; if patch includes **`allow_global_logins`**, execute only when **`confirm_allow_global_logins=true`** and **`confirmed=true`** (names cross-channel shared logins) | `internal/tools/customers/customer_settings_tools.go` |
| `customers/consent/get` | R0; GET `/v3/customers/{id}/consent` | `internal/tools/customers/customer_consent_tools.go` |
| `customers/consent/update` | R1; PUT consent; preview with current vs `would_apply`; `confirmed=true` | `internal/tools/customers/customer_consent_tools.go` |
| `customers/stored_instruments/list` | R0; requires **`acknowledge_stored_instruments=true`** (gate 1). Default response redacts **`token`**. Raw tokens only when **`include_sensitive_token_data=true`** and **`confirmed=true`** after redacted preview (gate 2). Needs stored-instruments OAuth scope | `internal/tools/customers/customer_stored_instruments_tools.go` |
| `customers/credentials/validate` | **R2**; POST validate-credentials is rate limited (429); preview masks email; password never echoed | `internal/tools/customers/customer_validate_credentials_tools.go` |
| `customers/segments/list` | R0; supports `id:in` filter (UUIDs, ≤ 40 ids/call); paginated GET `/v3/segments` | `internal/tools/customers/segments_tools.go` |
| `customers/segments/get` | R0; wraps `id:in={uuid}` because BC has no single-segment GET | `internal/tools/customers/segments_tools.go` |
| `customers/segments/create` | **R1**; max **10** rows per call (BC concurrency cap); store-wide cap **1000** segments; `name` required; preview then `confirmed=true` | `internal/tools/customers/segments_tools.go` |
| `customers/segments/update` | **R1**; max **10** rows per call; row requires `id` plus at least one of `name`, `description`; preview shows current vs `would_apply` | `internal/tools/customers/segments_tools.go` |
| `customers/segments/delete` | **R3 destructive**; max **40** ids per call; preview lists current names/descriptions; `confirmed=true` to apply; **does not delete the associated shopper profiles** | `internal/tools/customers/segments_tools.go` |
| `customers/segments/shoppers/list` | R0; **note: BC requires the `store_v2_customers` (modify) scope on this GET** — only `read_only` is insufficient | `internal/tools/customers/segments_tools.go` |
| `customers/segments/shoppers/add` | **R1**; accepts `shopper_profile_ids` (UUIDs) **or** `customer_ids` (numeric, ≤ 50/call) — numeric ids resolved via `customers?include=shopper_profile_id`; customers without a profile are surfaced under `missing_shopper_profiles`; max **50** profile ids per call after resolution; preview then `confirmed=true` | `internal/tools/customers/segments_tools.go` |
| `customers/segments/shoppers/remove` | **R1**; max **40** profile ids per call; preview shows current membership; `confirmed=true` to apply; profile records themselves remain | `internal/tools/customers/segments_tools.go` |
| `customers/shopper_profiles/list` | R0; paginated `/v3/shopper-profiles`. **No `id:in` or `customer_id` filter on this endpoint** — use `customers?include=shopper_profile_id` to map customers ↔ profiles | `internal/tools/customers/shopper_profiles_tools.go` |
| `customers/shopper_profiles/create` | **R1**; accepts `customer_ids` or `profiles_batch=[{customer_id}]`; deduped before send; max **50** rows/call; preview then `confirmed=true`; duplicates 409 because each customer is 1:1 with a profile | `internal/tools/customers/shopper_profiles_tools.go` |
| `customers/shopper_profiles/delete` | **R2 high-risk**; max **40** ids/call; deletes profile **and all of its segment memberships** (customer record itself is unaffected); preview then `confirmed=true` | `internal/tools/customers/shopper_profiles_tools.go` |
| `customers/shopper_profiles/list_segments` | R0; GET `/v3/shopper-profiles/{shopperProfileId}/segments` | `internal/tools/customers/shopper_profiles_tools.go` |
| `marketing/promotions/automatic/list` | R0; GET `/v3/promotions` with `redemption_type` hard-pinned to `automatic`; defensively filters out any COUPON entries returned by older stores; sort fields validated to `id`/`name`/`priority`/`start_date` | `internal/tools/promotions/automatic_tools.go` |
| `marketing/promotions/automatic/get` | R0; GET `/v3/promotions/{id}` — refuses to return when stored `redemption_type=COUPON` (points operator at `marketing/promotions/coupon/get`) | `internal/tools/promotions/automatic_tools.go` |
| `marketing/promotions/automatic/create` | **R2 high-risk**; POST single promotion (BC has no bulk POST). `redemption_type` overridden to `AUTOMATIC` regardless of input. Deep validation: rules required, action one-of (`cart_items`/`cart_value`/`shipping`/`gift_item`/`fixed_price_set`), discount one-of (`percentage_amount`/`fixed_amount`), condition tree (`cart`/`and`/`or`/`not`), item matcher (`products`/`categories`/`brands`/`variants`/`and`/`or`/`not`), notifications type+location enums, `customer.group_ids` vs `excluded_group_ids` mutual exclusion, status ∈ `ENABLED`/`DISABLED`, currency_code 3-letter or `*`. Soft-warn on previews when store already has ≥100 ENABLED promotions or rules count > 10. Preview required; `confirmed=true` to apply | `internal/tools/promotions/automatic_tools.go` |
| `marketing/promotions/automatic/update` | **R2 high-risk**; fetch-merge-PUT. Top-level scalars in `patch` override current; `patch.rules` (when provided) **replaces** the rules array in full and emits a warning. Positional rule edits via `rules_patch=[{index, replace_with}]` keep the rest of the array intact. Read-only fields (`id`, `redemption_type`, `current_uses`, `created_from`) rejected in `patch`. Refuses on COUPON promotions. Same deep validation as create runs on the merged document. Preview shows current vs would_apply; `confirmed=true` to apply | `internal/tools/promotions/automatic_tools.go` |
| `marketing/promotions/automatic/set_status` | **R2 high-risk**; convenience wrapper over update — flips `status` to `ENABLED`/`DISABLED` without touching rules. Returns `noop` when already at the requested status. Preview required | `internal/tools/promotions/automatic_tools.go` |
| `marketing/promotions/automatic/delete` | **R3 destructive**; DELETE `?id:in=…` (max **40** ids/call — BC documents 50; we leave headroom). Preview lists name/status/current_uses for each id. BC returns 422 when any promotion still has coupon codes attached; the tool surfaces a hint pointing at `marketing/promotions/coupon/codes/delete` and the cascade flag on `marketing/promotions/coupon/delete` | `internal/tools/promotions/automatic_tools.go` |
| `marketing/promotions/coupon/list` | R0; GET `/v3/promotions` with `redemption_type` hard-pinned to `coupon`; defensively filters out AUTOMATIC entries; supports the `code` filter (full-string match, no partial) | `internal/tools/promotions/coupon_tools.go` |
| `marketing/promotions/coupon/get` | R0; GET `/v3/promotions/{id}` — refuses on `redemption_type=AUTOMATIC` (points operator at `marketing/promotions/automatic/get`) | `internal/tools/promotions/coupon_tools.go` |
| `marketing/promotions/coupon/create` | **R2 high-risk**; POST single coupon promotion. `redemption_type` overridden to `COUPON`. Reuses automatic-promotions deep-shape validation plus coupon-specific cross-field checks: `coupon_type ∈ SINGLE \| BULK`; `coupon_overrides_other_promotions=true` requires `can_be_used_with_other_promotions=false`; `multiple_codes` only on BULK. **Deprecated** `coupon_overrides_automatic_when_offering_higher_discounts` rejected outright (operators must use `coupon_overrides_other_promotions`). Codes are added afterwards via `marketing/promotions/coupon/codes/*`. Preview required | `internal/tools/promotions/coupon_tools.go` |
| `marketing/promotions/coupon/update` | **R2 high-risk**; fetch-merge-PUT, same merge / `rules_patch` semantics as automatic/update. Refuses on AUTOMATIC promotions. Coupon cross-field validation runs on the merged document | `internal/tools/promotions/coupon_tools.go` |
| `marketing/promotions/coupon/set_status` | **R2 high-risk**; ENABLED/DISABLED toggle; `noop` when already at requested status; refuses on AUTOMATIC | `internal/tools/promotions/coupon_tools.go` |
| `marketing/promotions/coupon/delete` | **R3 destructive**; DELETE `?id:in=…` (max **40** ids/call). Preview surfaces attached-codes count + sample (best-effort, first page). Optional `delete_codes_first=true` cascade walks each promotion's codes via cursor pagination, deletes them in chunks of **40**, then deletes the promotion. Cascade hard-bounded at **1000 codes per promotion**; above that, the tool refuses and points at `coupon/codes/delete` for manual cleanup. 422-with-coupon-attached errors surface a hint about both paths (manual delete or cascade flag) | `internal/tools/promotions/coupon_tools.go` |
| `marketing/promotions/coupon/codes/list` | R0; GET `/v3/promotions/{id}/codes` — **cursor-paginated** via `before`/`after`. BigCommerce default rate limit on this endpoint is 10 concurrent (lower than other coupon-codes endpoints). Surfaces `has_more` and the cursor in the response | `internal/tools/promotions/coupon_codes_tools.go` |
| `marketing/promotions/coupon/codes/create_single` | **R1**; POST single `code`. Charset validated client-side: letters / numbers / spaces / underscores / hyphens, ≤50 chars. Pre-flights parent promotion (refuses on AUTOMATIC). Surfaces a warning when parent's `max_uses` overrides the code's. **Coupon codes are immutable** — message tells operators to delete-and-recreate to "edit" a code. Preview required | `internal/tools/promotions/coupon_codes_tools.go` |
| `marketing/promotions/coupon/codes/generate_bulk` | **R2 high-risk**; POST `/v3/promotions/{id}/codegen`. Pre-flights parent promotion's `coupon_type=BULK`; refuses on SINGLE. `batch_size` hard-capped at **250** (BC's per-call limit); `length` validated to 6..16 when set; `format` enum validated (`NUMBERS` / `LETTERS` / `ALPHANUMERIC`). Response sample is truncated to 5 codes plus `generated_count` to keep responses small. Preview required | `internal/tools/promotions/coupon_codes_tools.go` |
| `marketing/promotions/coupon/codes/delete` | **R3 destructive**; DELETE `?id:in=…`, max **40** ids/call (BC documents 50; we leave headroom). Use this to clear codes before `coupon/delete` on a promotion, or as cleanup after a `generate_bulk` run. Preview required | `internal/tools/promotions/coupon_codes_tools.go` |
| `marketing/promotions/settings/get` | R0; GET `/v3/promotions/settings`. Returns store-wide policy flags controlling zero-price triggers, custom-priced-product eligibility, max coupons at checkout, and original-price-vs-cumulative application mode. Includes notes that settings are global and coupon count >1 is Enterprise-only | `internal/tools/promotions/settings_tools.go` |
| `marketing/promotions/settings/update` | **R2 high-risk**; fetch-merge-PUT over `/v3/promotions/settings` so only supplied fields change. Type-checks booleans, validates `number_of_coupons_allowed_at_checkout ∈ 1..5`, soft-warns (warn-only) when setting coupon count >1 (Enterprise-only), and short-circuits to `noop` when patch equals current. Preview shows current vs would_apply; `confirmed=true` to apply | `internal/tools/promotions/settings_tools.go` |
| `webhooks/list` | R0; optional filters: `scope` (exact event string), `is_active` (bool), `channel_id`; paginated via `GetAll` | `internal/tools/webhooks/webhook_tools.go` |
| `webhooks/get` | R0; `id` required; `GET /v3/hooks/{id}` | `internal/tools/webhooks/webhook_tools.go` |
| `webhooks/events` | R0; `id` required; `GET /v3/hooks/{id}/events` — recent delivery attempts | `internal/tools/webhooks/webhook_tools.go` |
| `webhooks/create` | **R1**; `scope` + `destination` required; `destination` must be HTTPS (validated client-side before BC call); `is_active` defaults to `true`; optional `channel_id` (channel-scoped vs store-wide); optional `headers_json` (JSON string of string→string map; non-string values rejected); preview then confirm; **serial write policy** | `internal/tools/webhooks/webhook_tools.go` |
| `webhooks/update` | **R1**; `id` required; at least one of `scope`, `destination`, `is_active`, `headers_json`; fetch-merge-PUT: fetches current state, merges provided fields; `channel_id` immutable after creation; HTTPS validated on `destination`; preview then confirm | `internal/tools/webhooks/webhook_tools.go` |
| `webhooks/delete` | **R3 destructive**; `id` required; fetches current hook for preview (scope + destination shown); `confirmed=true` to permanently delete | `internal/tools/webhooks/webhook_tools.go` |
| `storefront/scripts/list` / `get` | R0; Script Manager reads via `/v3/content/scripts` | `internal/tools/storefront/scripts.go` |
| `storefront/scripts/create` / `update` / `toggle` | **R1**; preview then confirm; `toggle` flips `enabled` without editing the body | `internal/tools/storefront/scripts.go` |
| `storefront/scripts/delete` | **R3 destructive**; preview then `confirmed=true` | `internal/tools/storefront/scripts.go` |
| `carts/cart/create` / `update` | **R1**; preview then confirm; `line_items_json` / `custom_items_json` validated (quantity ≥ 1) | `internal/tools/carts/cart_tools.go` |
| `carts/cart/get` / `checkout_url` | R0; require `cart_id` (UUID) | `internal/tools/carts/cart_tools.go` |
| `carts/cart/delete` | **R3 destructive**; preview shows cart summary; `confirmed=true` | `internal/tools/carts/cart_tools.go` |
| `carts/cart/items/add` / `update` | **R1**; preview then confirm; quantity ≥ 1 | `internal/tools/carts/cart_tools.go` |
| `carts/cart/items/remove` | **R2**; preview shows the item; `confirmed=true` | `internal/tools/carts/cart_tools.go` |
| `carts/cart/metafields/list` / `set` / `delete` | R0 / **R1** / **R1**; upsert by namespace+key; scope `store_cart` | `internal/tools/carts/cart_metafields_tools.go` |
| `carts/checkout/get` | R0; `checkout_id` = cart UUID; scope `store_checkouts` | `internal/tools/carts/checkout_tools.go` |
| `carts/checkout/coupon_apply` | **R1**; preview then confirm | `internal/tools/carts/checkout_tools.go` |
| `carts/checkout/coupon_remove` | **R2**; preview then confirm | `internal/tools/carts/checkout_tools.go` |
| `carts/checkout/billing_address` | **R1**; POST to set / PUT (`billing_address_id`) to update; requires first_name, last_name, address1, city, country_code | `internal/tools/carts/checkout_tools.go` |
| `carts/checkout/consignment_add` / `consignment_update` | **R1**; add assigns items to a shipping address; update selects a `shipping_option_id` | `internal/tools/carts/checkout_tools.go` |
| `carts/checkout/convert` | **R2**; converts checkout to an order (cart consumed, irreversible); preview warns if billing address or consignment missing | `internal/tools/carts/checkout_tools.go` |
| `b2b/companies/list` / `get` | R0; B2B Edition; requires `BC_B2B_ENABLED=true` | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/create` / `update` | **R1**; preview then confirm; create also provisions the initial admin user | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/set_status` | **R2**; approve / reject / deactivate (status 0–3) | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/delete` | **R3 destructive**; deletes the company, all its users, and (by default) the users' linked BC customer accounts — resolved by `bcCustomerId` or email fallback; `delete_bc_customers=false` keeps them | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/extra_fields` / `update_catalog` | R0 / **R2**; extra-field config discovery; catalog assign (read-only on Independent-behavior stores) | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/users/list` / `get` / `get_by_customer` | R0; single get includes extra fields; `get_by_customer` maps a BC customer ID → B2B user | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/users/create` / `bulk_create` / `update` | **R1**; roles 0=admin, 1=senior, 2=junior; bulk ≤10; `extra_fields_json` supported | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/users/delete` / `extra_fields` | **R2** / R0; delete preserves the underlying BC customer; extra-field config listing | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/addresses/list` / `create` / `update` | R0 / **R1** / **R1**; company billing/shipping addresses | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/addresses/delete` | **R2**; removes an address (existing orders/quotes unaffected) | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/attachments/list` / `add` / `delete` | R0 / **R1** / **R2**; `add` uploads a local file (≤10MB, multipart) to the Attachments tab | `internal/tools/b2b/company_tools.go` |
| `b2b/companies/roles/*` | R0 reads; **R1** create/update; **R2** delete; custom roles only (predefined are read-only); `permissions_json` sets `{code, permissionLevel}` | `internal/tools/b2b/role_tools.go` |
| `b2b/companies/permissions/*` | R0 list; **R1** create/update; **R2** delete; custom company permissions | `internal/tools/b2b/role_tools.go` |

---

## 3. Read vs write: rules of engagement

| Rule | Source |
|------|--------|
| **GET before PUT/POST/DELETE** on the same logical resource | BC-API-Reference §9, `AGENT.md` |
| **Show diffs** (before/after for key fields) before bulk apply | `AGENT.md` |
| **Paginate exhaustively** before large bulk writes (know full ID set) | BC-API-Reference §9 |
| **Soft delete preferred:** `is_visible: false` vs DELETE | `AGENT.md`, §9 |
| **Never** bulk overwrite `description` unless explicitly requested | `AGENT.md` |
| **Never** payment capture/refund/void without per-order confirmation | `AGENT.md` |
| **Never** customer password/auth fields unless that is the task | `AGENT.md` |
| **Price lists:** confirm list **name** + **record count** before upsert | `AGENT.md` |
| Use **serial requests** for price list record upserts — **no parallel** | MCP server policy, reference |

---

## 4. OAuth scopes → tool blast radius

From `BC-API-Reference.md`: grant **minimum scopes** per tool group.

| Tool group | Typical scopes |
|------------|----------------|
| Catalog read | `store_v2_products` read-only or products read |
| Catalog write | `store_v2_products` write |
| Categories | `store_catalog_categories` |
| Orders | `store_v2_orders` (read vs write split as needed) |
| Customers | `store_v2_customers` (covers `/v3/customers` **and** `/v2/customer_groups`) |
| Customer Segmentation | `store_v2_customers` (writes); `store_v2_customers_read_only` is sufficient for `/v3/segments` and `/v3/shopper-profiles` GETs **except** `GET /v3/segments/{id}/shopper-profiles`, which requires the modify scope. **Enterprise plan only** — non-Enterprise stores 403 across the entire family. |
| Promotions | `store_v2_marketing` (writes); `store_v2_marketing_read_only` is sufficient for `/v3/promotions`, `/v3/promotions/{id}/codes`, and `/v3/promotions/settings` GETs. Default rate limit: 40 concurrent requests on most endpoints; **`/v3/promotions/{id}/codes` GET and `/codegen` POST are limited to 10 concurrent**. Shipped MCP subtrees: `marketing/promotions/automatic/*` (`redemption_type=AUTOMATIC`), `marketing/promotions/coupon/*` + `marketing/promotions/coupon/codes/*` (`redemption_type=COUPON` and code lifecycle), and `marketing/promotions/settings/*` (store-wide policy settings). Coupon codes are **immutable** (no PUT) — to change a code, delete and recreate. |
| Customer stored instruments | `store_stored_payment_instruments` or `store_stored_payment_instruments_read_only` (Management API stored-instruments list) |
| Inventory | `store_inventory` |
| Price lists | `store_price_lists` |
| Channels (MSF) | `store_channel_settings` (write, for `catalog/channels/update`); `store_channel_settings_read_only` (for `catalog/channels/list`, `catalog/channels/get`); channel listings need `store_channel_listings` (write) or `store_channel_listings_read_only` |
| Webhooks | `store_v2_information_read_only` sufficient for `webhooks/list`, `webhooks/get`, `webhooks/events`; `store_v2_information` (modify) required for `webhooks/create`, `webhooks/update`, `webhooks/delete` |
| Storefront scripts | `store_content` (Script Manager `/v3/content/scripts`) |
| Carts | `store_cart` (covers `carts/cart/**` including cart metafields) |
| Checkout | `store_checkouts` (covers `carts/checkout/**`; convert-to-order also touches orders) |
| B2B Edition | B2B Edition scope on the store-level API account (same `X-Auth-Token`); gated by `BC_B2B_ENABLED=true` |
| Store / SEO / content | `store_v2_information`, `store_content`, etc. |

**LLM note:** a single long-lived token with every scope maximizes damage from one bad tool call. Prefer **narrow tokens** or **separate environments** (sandbox vs production) when testing new tools.

---

## 5. Error handling expectations (for tool wrappers)

| Code | Tool behavior |
|------|----------------|
| **400 / 422** | Return validation message; do not blindly retry |
| **404** | Surface “resource missing”; do not assume client bug |
| **429** | Server backs off automatically; tools should not double-retry |
| **500 / 503** | Backoff; if persistent, stop batch and report |
| **509** | Treat like rate limit (reference) |

---

## 6. Suggested MCP tool shapes (naming)

Follow **`{action}_{resource}`** in snake_case (BC-API-Reference §9).

**Read (R0):** `get_store_profile`, `get_products`, `get_product_by_id`, `get_categories`, `get_orders`, …

**Write (R1):** `bulk_update_products` (max 10 items + chunking in implementation), `update_category`, …

**High-risk (R2):** `catalog/pricelists/records/upsert` (**serial only**), `catalog/pricelists/assignments/create_batch`, `catalog/pricelists/assignments/upsert`, `catalog/pricelists/assignments/delete`, `inventory/locations/create`, `inventory/locations/update`, `inventory/items/update_batch` (batch ≤ 10), `inventory/adjustments/absolute` (batch ≤ 10), `inventory/adjustments/relative` (batch ≤ 10), …

**Parameters:** mirror BC limits — e.g. `maxItems: 10` on bulk product arrays; optional `dry_run: bool` for proposals.

---

## 7. Tension: throughput vs safety (resolved default)

| Mode | Throughput | When to use |
|------|------------|-------------|
| **Conservative (project default)** | ~2 req/s, sequential writes, 0.5 s between chunks | Live store, agentic workflows, MCP v1 |
| **Throughput (reference)** | 3–5 parallel write threads for catalog batches, higher read parallelism | Batch jobs with monitoring, explicit approval |

**Default MCP server implementation should implement the conservative row** unless configuration enables “throughput mode.”

---

## 8. Server implementation notes

Current MCP server components:

1. Caps are defined in **`internal/config/config.go`** — avoid duplicating magic numbers.
2. Tier checks are implemented in **`internal/middleware/tiers.go`** (`TierEnforcer`).
3. All R1+ tools require **`confirmed: true`** — enforced at registration time by the discovery registry.
4. Log **operation, record count, correlation id** for audit (BC-API-Reference §4 `X-Correlation-Id` for chained calls).

---

*Last aligned with: `AGENT.md`, `BC-API-Reference.md` §§3–5 & 9, MCP server implementation (`internal/config/config.go`, `internal/bigcommerce/client.go`).*

---

## 9. Channel Assignments vs Channel Listings

BigCommerce separates **which channels may sell a product** from **how that product appears on each channel's storefront**. Choosing the wrong surface causes "assigned but not visible" confusion.

### Channel catalog assignments — `catalog/products/channel_assignments/*`

**REST:** `GET|PUT|DELETE /v3/catalog/products/channel-assignments`

Links a product to one or more sales channels at the catalog level. A product with no row for a channel is not part of that channel's sellable catalog.

**Use when:** "Make this SKU available on the AU storefront channel" / "Remove this product from the wholesale channel."

### Channel listings — `catalog/channels/listings/*`

**REST:** `GET|POST|PUT /v3/channels/{channel_id}/listings`

Controls channel-specific **presentation and state** (e.g. `active` vs `disabled`), name/description overrides, and visibility rules beyond mere assignment.

**Use when:** "This product is assigned to the channel but doesn't appear on the storefront" — check listings for that `channel_id` and `product_id`. Also use when you need to set channel-specific copy or override the listing state.

### Combined read

`catalog/products/channel_summary` aggregates both surfaces for a small product batch (≤5 product IDs). Use it to diagnose MSF visibility without correlating two APIs by hand.

### Listing `state` values

`active`, `disabled`, `error`, `pending`, `pending_disable`, `pending_delete`, `partially_rejected`, `queued`, `rejected`, `submitted`, `deleted`. Variant-level listings use the same set minus `partially_rejected`.

### Smoke check

`make smoke-msf` (`scripts/smoke_msf_slice.sh`) exercises both channel-assignments and listings with live credentials.
