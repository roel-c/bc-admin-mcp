# BigCommerce MCP Server

A high-performance Model Context Protocol (MCP) server for BigCommerce store management, built in Go with the [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) SDK.

**Source repository:** [github.com/roel-c/bc-admin-mcp](https://github.com/roel-c/bc-admin-mcp)

### How this repo relates to BigCommerce `mcp-proxy`

If you also have the **`mcp-proxy`** monorepo open (for example side-by-side in a multi-root workspace), note that it is a **different system**:

| | **This repo (`bc-admin-mcp`)** | **BigCommerce `mcp-proxy`** |
|--|-------------------------------|------------------------------|
| **Role** | Standalone MCP server you run locally (stdio / streamable HTTP) | Hosted **gateway** between agents and **internal** BC services |
| **Tools** | Implemented here in Go; talks to **Management REST** with `X-Auth-Token` | Registered in-process; GraphQL, gRPC, Blaze, storefront flows, OAuth, Redis, LaunchDarkly (see that repo’s `docs/architecture.md`) |
| **Build** | `make build` in this directory only | `make build` / `goreman start` inside **`mcp-proxy`** only |

There is **no code or runtime dependency** between the two: you do **not** need `mcp-proxy` checked out or running to build or use this server. Opening both folders together is an editor convenience, not a combined product.

## Architecture

This server uses **progressive disclosure** to minimize token consumption and maximize LLM accuracy. Instead of registering all BigCommerce tools upfront (~40,000+ tokens), only two meta-tools are exposed:

- **`discover_tools`** — Navigate a hierarchical tree of available tool categories
- **`execute_tool`** — Execute any tool by its full path with arguments

This reduces initial token usage to ~600 tokens (a 60-100x reduction) and keeps the LLM focused on only the tools relevant to the current task.

### Tool Hierarchy

**`discover_tools("")`** returns **`catalog`**, **`orders`**, **`customers`**, **`marketing`**, **`inventory`**, **`storefront`**, and **`webhooks`** — the live MCP tree matches implemented tools (no empty placeholder roots). Planned domains (carts, store, …) are described in **[docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)** section 7, not in discovery, until tools register.

```
catalog/          — Products, categories, brands, variants, and price lists (full tree under this root)
orders/           — V2 order management and fulfillment shipments (`orders/management/*`, `orders/fulfillment/shipments/*`).
customers/        — V3 customer records, addresses, attributes, attribute values, metafields, settings, consent, stored instruments, credential validation, customer segments, shopper profiles, and V2 customer groups.
marketing/        — Promotions engine. AUTOMATIC promotions under marketing/promotions/automatic/*; COUPON promotions and coupon-code sub-resources under marketing/promotions/coupon/* and marketing/promotions/coupon/codes/*; store-wide policy toggles under marketing/promotions/settings/*.
```

**Variants:** use **`catalog/products/variants`** for product-scoped CRUD, options-linked creates, and variant metafields. Use **`catalog/variants`** for **global** `GET /v3/catalog/variants` list/search and **`PUT /v3/catalog/variants`** batch updates (IMS-style); see tool table rows below.

**Before implementing remaining domains (carts, inventory, store, etc.):** work through the catalog hardening and scope checklist in [`docs/catalog-completion-checklist.md`](docs/catalog-completion-checklist.md) so discovery matches reality and patterns (tiers, bulk, metafields) stay consistent when those domains are added.

### Tool Tiers (from [docs/BC-Tool-Boundaries.md](./docs/BC-Tool-Boundaries.md))

| Tier | Intent | Confirmation |
|------|--------|-------------|
| R0 | Read only | None |
| R1 | Standard writes | Preview + confirm for bulk |
| R2 | High-risk (pricing, inventory) | Always confirm |
| R3 | Destructive | Per-resource confirmation |
| R4 | Forbidden | Blocked at tool layer |

### Key Design Patterns

- **Preview-then-confirm** for all write operations (R1+): first call returns a preview, pass `confirmed=true` to execute
- **Three-phase safety for destructive ops** (R3): child detection → `include_children` gate → preview → confirm
- **Name-based resolution**: category tools accept human-friendly names (e.g., `parent_name: "Electronics"`) and resolve to IDs server-side, with ambiguity detection
- **Flexible product selection**: target an entire category, the first N products via `limit`, or specific `product_ids`
- **Session-scoped caching** between tool calls to avoid redundant API requests
- **Server-side pagination** so the LLM never needs to paginate manually
- **Server-side batch operations** with rate-limit-aware throttling
- **Variant-aware pricing**: correctly handles BigCommerce's `price: 0` inheritance pattern — variants that inherit from the product price are left untouched during bulk updates
- **Declarative search filters**: adding a new searchable field requires one table entry and one schema declaration

For full architectural detail, design decisions with alternatives considered, and the expansion roadmap, see **[docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)**.

## Setup

```bash
git clone https://github.com/roel-c/bc-admin-mcp.git
cd bc-admin-mcp
```

### Prerequisites

- Go **1.26.2** or newer (see `go.mod`; older toolchains will not build)
- BigCommerce API credentials (Store Hash + Auth Token)

### Configuration

```bash
cp .env.example .env
# Edit .env with your BigCommerce credentials
```

Use your own `.env` locally and keep it uncommitted. This repository is intended
to be safely shareable publicly (for example GitHub) while each operator uses
their own private credentials.

### Local-Only Operating Model (current posture)

- Run this server on your own machine against your own BigCommerce store token.
- Default and recommended transport is `stdio` for local MCP clients.
- If using `streamable-http` or `sse`, bind to `127.0.0.1` and keep `MCP_AUTH_TOKEN` set.
- Do not expose the MCP endpoint directly to the internet at this stage.

### Build & Run

```bash
# Build
make build

# Run with stdio transport (for Cursor, Claude Desktop, etc.)
make run

# Run with Streamable HTTP transport (local machine only; requires MCP_AUTH_TOKEN)
make run-http
```

### Local Operator Quickstart + Safety Checklist

- Copy `.env.example` to `.env` and set only your own `BC_STORE_HASH` + `BC_AUTH_TOKEN`.
- Start with `MCP_TRANSPORT=stdio` for local IDE use; prefer this mode unless you need HTTP/SSE.
- Treat all R1/R2/R3 tool previews as mandatory review steps before passing `confirmed=true`.
- Begin with read-only checks (`discover_tools`, list/get tools) before write or delete operations.
- Keep batch sizes conservative (defaults are tuned for safety); avoid increasing concurrency early.
- If you run HTTP/SSE locally, keep `MCP_ADDRESS=127.0.0.1` and set a strong `MCP_AUTH_TOKEN`.
- Never commit `.env`; rotate tokens immediately if credentials are exposed.

### Integration with Cursor

Add to your `.cursor/mcp.json`:

```json
{
  "mcpServers": {
    "bigcommerce": {
      "command": "/path/to/bc-mcp-server",
      "env": {
        "BC_STORE_HASH": "your_store_hash",
        "BC_AUTH_TOKEN": "your_api_token"
      }
    }
  }
}
```

### Calling tools via `execute_tool` (Cursor / MCP hosts)

The server exposes **`discover_tools`** and **`execute_tool`**. For every real operation, call **`execute_tool`** with:

- **`tool_path`** — full path string (e.g. `catalog/products/metafields/set`)
- **`arguments`** — JSON object containing **only** that tool’s parameters (`product_id`, `namespace`, `confirmed`, …)

Do not flatten tool parameters next to `tool_path` at the top level; the registry forwards **`arguments`** unchanged to the handler.

**Minimal example — preview a product metafield upsert:**

```json
{
  "tool_path": "catalog/products/metafields/set",
  "arguments": {
    "product_id": 19402,
    "namespace": "my_integration",
    "key": "external_ref",
    "value": "pim-12345",
    "confirmed": false
  }
}
```

Use **`confirmed: true`** on the second call after reviewing the preview.

### Operator references

- **[Channel assignments vs listings](./docs/channel-assignments-vs-listings.md)** — how catalog channel assignments relate to per-channel listings, and when to use **`catalog/products/channel_summary`**.
- **[MCP discovery and preview drills](./docs/mcp-operator-drill.md)** — how to exercise **`discover_tools`** / **`execute_tool`** and preview-then-confirm (includes automated regression tests you can run in CI).

**Full agent copy-paste reference:** **[docs/bc_system_prompt.md](./docs/bc_system_prompt.md)** — includes the universal `execute_tool` envelope, catalog examples, and operating constraints used across domains. Using that structure reduces malformed MCP calls and speeds up correct previews.

## Implemented Tools

| Tool Path | Tier | Description |
|-----------|------|-------------|
| `catalog/products/search` | R0 | Filter search (name, SKU, price range, category, brand, visibility, keyword, MSF **`channel_ids`** → `channel_id:in`) |
| `catalog/products/get` | R0 | Single product with variant pricing detection |
| `catalog/products/create` | R1 | Create product with all writable fields, optional inline images, optional MSF `channel_ids` (additive post-create channel assignment) |
| `catalog/products/update` | R1 | Unified update: any writable field(s) on one or more products; target by product_ids, sku, product_name, or category_id; optional MSF `channel_ids` for additive post-update channel assignment (≤ 500 product×channel pairs) |
| `catalog/products/delete` | R3 | Permanently delete products (destructive, requires confirmation) |
| `catalog/products/assign_categories` | R1 | Additive product-to-category assignment via dedicated BC endpoint |
| `catalog/products/unassign_categories` | R2 | Filter-based **DELETE** on `/v3/catalog/products/category-assignments` — remove specific (product, category) links without clobbering other categories; preview → `confirmed` |
| `catalog/products/channel_summary` | R0 | MSF snapshot: per-product assignments + per-channel listing state aggregated from `/v3/channels`, `/v3/catalog/products/channel-assignments`, `/v3/channels/{id}/listings`; max 5 products / 25 channels per call |
| `catalog/products/channel_assignments/list` | R0 | `GET .../channel-assignments` — list assignments (requires `product_ids` and/or `channel_ids`; Products scope) |
| `catalog/products/channel_assignments/assign` | R1 | `PUT` — assign products to channels (cartesian); preview → `confirmed`; max 500 pairs per call |
| `catalog/products/channel_assignments/remove` | R2 | `DELETE` — remove assignments (`product_ids` required; optional `channel_ids`); preview → `confirmed` |
| `catalog/products/images/list` | R0 | List images for a product |
| `catalog/products/images/add` | R1 | Add an image by URL |
| `catalog/products/images/delete` | R2 | Delete a product image |
| `catalog/products/options/list` | R0 | List variant-generating options |
| `catalog/products/options/create` | R1 | Create an option with values |
| `catalog/products/options/update` | R1 | Update option name, sort order, or values |
| `catalog/products/options/delete` | R2 | Delete an option (removes dependent variants) |
| `catalog/products/variants/list` | R0 | List variants with details |
| `catalog/products/variants/create` | R1 | Create a variant with option value mapping |
| `catalog/products/variants/update` | R1 | Update variant fields |
| `catalog/products/variants/delete` | R2 | Delete a variant |
| `catalog/products/variants/metafields/list` | R0 | List metafields on a variant (product: `product_id` / `sku` / `product_name`; variant: `variant_id` or `variant_sku`) |
| `catalog/products/variants/metafields/set` | R1 | Upsert variant metafield; default `permission_set` **`app_only`** (same semantics as product metafields); preview → confirm |
| `catalog/products/variants/metafields/delete` | R1 | Delete by `metafield_id` or namespace+key; preview → confirm |
| `catalog/products/variants/metafields/bulk_set` | R1 | Same namespace+key+value on up to **50** variants: either explicit `variant_ids` **or** `variant_sku_contains` (case-insensitive substring on variant SKU); preview → confirm |
| `catalog/products/variants/metafields/bulk_delete` | R1 | Same targeting as bulk_set (`variant_ids` or `variant_sku_contains`); skips missing metafields; preview → confirm |
| `catalog/products/variants/metafields/bulk_set_products` | R1 | Same variant metafield on **many products**: `product_ids` (max **50**) + `variant_scope` `all_variants`, `first_variant_only`, or `sku_contains` (with `variant_sku_contains` substring, case-insensitive); max **500** total variant writes per call; preview → confirm |
| `catalog/products/variants/metafields/bulk_delete_products` | R1 | Delete namespace+key across the same cross-product `variant_scope`; skips missing; same caps as bulk_set_products |
| `catalog/products/custom_fields/list` | R0 | List custom fields |
| `catalog/products/custom_fields/set` | R1 | Upsert a custom field by name |
| `catalog/products/custom_fields/delete` | R2 | Delete a custom field |
| `catalog/products/modifiers/list` | R0 | List modifiers |
| `catalog/products/modifiers/create` | R1 | Create a modifier |
| `catalog/products/modifiers/delete` | R2 | Delete a modifier |
| `catalog/products/metafields/list` | R0 | List metafields on a product (by `product_id`, `sku`, or `product_name`) |
| `catalog/products/metafields/set` | R1 | Create or update a metafield; optional `permission_set` (defaults to `app_only`; use `read_and_sf_access` / `write_and_sf_access` for Storefront) |
| `catalog/products/metafields/delete` | R1 | Delete a metafield by id or namespace+key |
| `catalog/products/metafields/bulk_set` | R1 | Same namespace+key+value upsert on up to **50** products (`product_ids`); sequential API calls; preview → confirm |
| `catalog/products/metafields/bulk_delete` | R1 | Remove namespace+key metafield from each listed product (skips if missing); up to **50** `product_ids`; preview → confirm |
| `catalog/categories/list` | R0 | Filter search with `list_all` mode for full catalog dump (optional **`channel_id`** for MSF — resolves `tree_id` server-side) |
| `catalog/categories/get` | R0 | Single category by ID |
| `catalog/categories/create` | R1 | Create with name-based parent resolution (no numeric IDs needed); optional **`channel_id`** or **`tree_id`** for MSF |
| `catalog/categories/bulk_update` | R1 | Batch update name, description, SEO, visibility, sort order |
| `catalog/categories/products` | R0 | List products belonging to a category (by ID or name) with price/SKU summaries |
| `catalog/categories/seo_audit` | R0 | Scan categories for missing `page_title`, `meta_description`, or `search_keywords` |
| `catalog/categories/move` | R2 | Reparent a category (with cycle detection and subtree preview) |
| `catalog/categories/reorder` | R1 | Reorder sibling categories by providing them in desired display order |
| `catalog/categories/metafields/list` | R0 | List all metafields on a category |
| `catalog/categories/metafields/set` | R1 | Create or update a metafield (upsert by namespace+key) |
| `catalog/categories/metafields/delete` | R1 | Delete a metafield by ID or namespace+key |
| `catalog/categories/delete` | R3 | Single delete with child-cascade safeguard |
| `catalog/categories/bulk_delete` | R3 | Multi-delete with child-cascade safeguard |
| `catalog/brands/list` | R0 | List or search brands (`list_all` or filters: name, name_like, keyword, page_title, id, sort) |
| `catalog/brands/get` | R0 | Single brand by `brand_id` |
| `catalog/brands/create` | R1 | Create brand (name + optional SEO, image URL, layout, custom URL path); preview → confirm |
| `catalog/brands/update` | R1 | Update brand fields by `brand_id`; preview → confirm |
| `catalog/brands/metafields/list` | R0 | List metafields on a brand (`brand_id` or exact `brand_name`) |
| `catalog/brands/metafields/set` | R1 | Upsert metafield by namespace+key; default `permission_set` **write**; preview → confirm |
| `catalog/brands/metafields/delete` | R1 | Delete by `metafield_id` or namespace+key; preview → confirm |
| `catalog/variants/list` | R0 | Global variant search (`GET /v3/catalog/variants`): `product_id` / `product_ids` (max 100), `variant_id` / `variant_ids` (max 100), `sku`, `sku_like`, optional `sort`, or `list_all` |
| `catalog/variants/bulk_update` | R2 | Batch `PUT /v3/catalog/variants`: `updates` array (max **200** rows, ≥1 field per row besides `variant_id`); server chunks by **10**; preview → confirm |
| `catalog/channels/list` | R0 | `GET /v3/channels` — channels for the connected store; optional `type` / `status`; response includes `multi_storefront_likely` (needs **`store_channel_settings`** on the API account) |
| `catalog/channels/get` | R0 | `GET /v3/channels/{id}` — full details for one channel (name, platform, type, status, timestamps); scope **`store_channel_settings_read_only`** |
| `catalog/channels/update` | R2 | `PUT /v3/channels/{id}` — update channel `name` and/or `status` (preview → **`confirmed`**); valid statuses: active, inactive, connected, disconnected, prelaunch; scope **`store_channel_settings`** |
| `catalog/channels/category_trees` | R0 | `GET /v3/catalog/trees` — category trees (optional **`channel_id`** → `channel_id:in` for MSF); needs **Products** scope (`store_v2_products_read_only` or `store_v2_products`) |
| `catalog/channels/listings/list` | R0 | `GET .../channels/{id}/listings` — optional **`product_ids`** filter; cursor pagination (up to 2000 rows); **`store_channel_listings_read_only`** or modify scope |
| `catalog/channels/listings/create` | R1 | `POST` — **`listings_json`** array (max 10 listings; BC requires **variants** per row); preview → **`confirmed`**; **`store_channel_listings`** |
| `catalog/channels/listings/update` | R2 | `PUT` — same JSON limits; each row needs **listing_id** (from list); preview → **`confirmed`** |
| `catalog/pricelists/list` | R0 | `GET /v3/pricelists` with optional id/name/date filters and offset/cursor pagination |
| `catalog/pricelists/get` | R0 | `GET /v3/pricelists/{price_list_id}` |
| `catalog/pricelists/create` | R1 | `POST /v3/pricelists` (`name`, optional `active`); preview → confirm |
| `catalog/pricelists/update` | R1 | Fetch-merge-`PUT /v3/pricelists/{price_list_id}`; preview diff → confirm |
| `catalog/pricelists/delete` | R3 | Destructive `DELETE /v3/pricelists/{price_list_id}`; preview → confirm |
| `catalog/pricelists/records/list` | R0 | `GET /v3/pricelists/{price_list_id}/records` with variant/product/SKU/currency filters and offset/cursor pagination |
| `catalog/pricelists/records/upsert` | R2 | `PUT /v3/pricelists/{price_list_id}/records` (max **100** rows/tool call); preview → confirm; serial write policy |
| `catalog/pricelists/records/delete` | R2 | Selector-based `DELETE /v3/pricelists/{price_list_id}/records` (requires `variant_ids` or `skus`); preview → confirm |
| `catalog/pricelists/assignments/list` | R0 | `GET /v3/pricelists/assignments` with id/price_list/customer_group/channel filters and offset/cursor pagination |
| `catalog/pricelists/assignments/create_batch` | R2 | `POST /v3/pricelists/assignments` (max **25** rows/tool call); preview → confirm |
| `catalog/pricelists/assignments/upsert` | R2 | `PUT /v3/pricelists/{price_list_id}/assignments` for one customer-group + channel tuple; preview → confirm |
| `catalog/pricelists/assignments/delete` | R2 | Filter-based `DELETE /v3/pricelists/assignments` (requires at least one filter); preview → confirm |
| `orders/management/list` | R0 | `GET /v2/orders` with status/customer/date/payment/channel filters; explicit page/limit for single-page mode or server auto-pagination with `list_all=true` |
| `orders/management/get` | R0 | `GET /v2/orders/{order_id}` plus `GET /v2/orders/{order_id}/products` for line items |
| `orders/management/create` | R2 | `POST /v2/orders` with caller-supplied order payload object; preview → confirm |
| `orders/management/update` | R2 | Targeted `PUT /v2/orders/{order_id}` patch payload; preview → confirm with warnings about possible promotion/discount side effects |
| `orders/management/delete` | R3 | `DELETE /v2/orders/{order_id}`; destructive preview → confirm |
| `orders/management/count` | R0 | `GET /v2/orders/count` with the same filter family as list |
| `orders/management/statuses` | R0 | `GET /v2/order_statuses` |
| `orders/management/update_status` | R1 | `PUT /v2/orders/{order_id}` (`status_id` only); preview → confirm |
| `orders/management/products/get` | R0 | `GET /v2/orders/{order_id}/products/{product_id}` |
| `orders/management/metafields/list` | R0 | `GET /v3/orders/{order_id}/metafields` with optional page/limit |
| `orders/management/metafields/set` | R1 | Upsert order metafield by `namespace` + `key`; preview → confirm; defaults new rows to `app_only` when permission_set omitted |
| `orders/management/metafields/delete` | R1 | Delete order metafield by `metafield_id` or `namespace`+`key`; preview → confirm |
| `orders/management/coupons/list` | R0 | `GET /v2/orders/{order_id}/coupons` |
| `orders/management/shipping_addresses/list` | R0 | `GET /v2/orders/{order_id}/shipping_addresses` |
| `orders/management/shipping_addresses/get` | R0 | `GET /v2/orders/{order_id}/shipping_addresses/{shipping_address_id}` |
| `orders/management/shipping_addresses/update` | R1 | `PUT /v2/orders/{order_id}/shipping_addresses/{shipping_address_id}` patch payload; preview → confirm |
| `orders/management/messages/list` | R0 | `GET /v2/orders/{order_id}/messages` with optional `min_id`, `max_id`, `customer_id`, date range, `status`, `is_flagged`, page/limit |
| `orders/management/taxes/list` | R0 | `GET /v2/orders/{order_id}/taxes` |
| `orders/fulfillment/shipments/list` | R0 | `GET /v2/orders/{order_id}/shipments` with optional page/limit |
| `orders/fulfillment/shipments/get` | R0 | `GET /v2/orders/{order_id}/shipments/{shipment_id}` |
| `orders/fulfillment/shipments/create` | R1 | `POST /v2/orders/{order_id}/shipments` (`order_address_id` + `items` required); preview → confirm |
| `orders/fulfillment/shipments/update` | R1 | `PUT /v2/orders/{order_id}/shipments/{shipment_id}` patch payload; preview → confirm |
| `orders/fulfillment/shipments/delete` | R3 | `DELETE /v2/orders/{order_id}/shipments/{shipment_id}`; destructive preview → confirm |
| `orders/payments/actions/list` | R0 | `GET /v3/orders/{order_id}/payment_actions` with optional page/limit |
| `orders/payments/transactions/list` | R0 | `GET /v3/orders/{order_id}/transactions` with optional page/limit (parity/reconciliation checks) |
| `orders/payments/capture` | R3 | `POST /v3/orders/{order_id}/payment_actions/capture`; per-order preview → confirm |
| `orders/payments/void` | R3 | `POST /v3/orders/{order_id}/payment_actions/void`; per-order preview → confirm |
| `orders/refunds/list` | R0 | `GET /v3/orders/{order_id}/payment_actions/refunds` with optional `transaction_id` and pagination |
| `orders/refunds/legacy_list` | R0 | `GET /v2/orders/{order_id}/refunds` for legacy parity/reference reads |
| `orders/refunds/quote` | R2 | `POST /v3/orders/{order_id}/payment_actions/refund_quotes`; preview → confirm |
| `orders/refunds/create` | R3 | `POST /v3/orders/{order_id}/payment_actions/refunds`; financially sensitive, sequential-per-order guidance, preview → confirm |
| `inventory/locations/list` | R0 | `GET /v3/inventory/locations` with optional page/limit |
| `inventory/locations/create` | R2 | `POST /v3/inventory/locations` with caller-supplied `location` object; preview → confirm |
| `inventory/locations/update` | R2 | `PUT /v3/inventory/locations/{location_id}` with caller-supplied `patch` object; preview → confirm |
| `inventory/locations/delete` | R3 | `DELETE /v3/inventory/locations/{location_id}`; destructive preview → confirm |
| `inventory/locations/metafields/list` | R0 | `GET /v3/inventory/locations/{location_id}/metafields` with optional page/limit |
| `inventory/locations/metafields/set` | R1 | Upsert by `namespace` + `key` via `POST/PUT /v3/inventory/locations/{location_id}/metafields`; preview → confirm |
| `inventory/locations/metafields/delete` | R1 | Delete by `metafield_id` or `namespace` + `key`; preview → confirm |
| `inventory/items/list` | R0 | `GET /v3/inventory/items` with optional `location_ids`, `product_ids`, `variant_ids`, `skus`; requires a filter or `list_all=true` |
| `inventory/items/get` | R0 | `GET /v3/inventory/items/{variant_id}` |
| `inventory/items/update_batch` | R2 | `PUT /v3/inventory/items` using caller-supplied `update` payload (`items[]` or `data[]`, max 10 rows); preview → confirm |
| `inventory/adjustments/absolute` | R2 | `PUT /v3/inventory/adjustments/absolute` for up to 10 rows per call; preview → confirm |
| `inventory/adjustments/relative` | R2 | `POST /v3/inventory/adjustments/relative` for up to 10 rows per call; preview → confirm |
| `customers/groups/list` | R0 | List/search customer groups (`list_all` or filters: name, name_like, is_default, is_group_for_guests, date_created*, date_modified*) |
| `customers/groups/get` | R0 | Single customer group by `group_id` (full category_access + discount_rules) |
| `customers/groups/count` | R0 | `GET /v2/customer_groups/count` — total customer group count |
| `customers/groups/create` | R1 | Create group (`name`, optional `is_default`, `is_group_for_guests`, `category_access_*`, `discount_rules`); preview → confirm. `price_list` rules are mutually exclusive — mixed input silently keeps the price_list rule and warns |
| `customers/groups/update` | R1 | Update group by `group_id`; only supplied fields change. **Note:** sending `discount_rules` overwrites the entire set (BC bulk semantics); preview → confirm |
| `customers/groups/delete` | R3 | Destructive delete by `group_id` — BC unassigns all members automatically; preview → `confirmed=true` |
| `customers/list` | R0 | Search customers (`list_all` or filters); GET `/v3/customers` |
| `customers/get` | R0 | One customer by `customer_id` (wraps `id:in`) |
| `customers/create` | R2 | POST `/v3/customers` (≤10); preview → `confirmed=true`; `new_password` also needs `set_password=true` |
| `customers/update` | R2 | PUT `/v3/customers` (≤10 rows in `customer_batch`); same password double gate |
| `customers/delete` | R3 | DELETE by `customer_ids` (≤50); preview → confirm |
| `customers/assign_group` | R2 | Batch set `customer_group_id` (≤100 ids, chunked PUTs of 10); `group_id` 0 unassigns |
| `customers/addresses/list` | R0 | List addresses (`list_all` or filters) |
| `customers/addresses/create` | R1 | POST address batch (≤25); preview → confirm |
| `customers/addresses/update` | R1 | PUT address batch (≤25); preview → confirm |
| `customers/addresses/delete` | R3 | DELETE by `address_ids` (≤50); preview → confirm |
| `customers/attributes/list` | R0 | List per-store attribute definitions (`list_all` or filters: `attribute_ids`, `name`, `name_like`) |
| `customers/attributes/create` | R1 | POST attribute definitions (≤10); `type` validated to one of `string`, `number`, `date`; preview → confirm |
| `customers/attributes/update` | R1 | PUT renames (≤10); only `name` mutable — passing `type` is rejected |
| `customers/attributes/delete` | R3 | DELETE by `attribute_ids` (≤50); cascades to every stored value of that attribute on every customer |
| `customers/attribute_values/list` | R0 | List stored values; requires a filter (`customer_ids`, `attribute_ids`, `attribute_value`/`attribute_value_in`) or `list_all=true` |
| `customers/attribute_values/upsert` | R1 | PUT upsert by `(customer_id, attribute_id)` (≤10 rows); BC coerces `value` to the attribute's type |
| `customers/attribute_values/delete` | R2 | DELETE by `value_ids` (≤50); preview → confirm |
| `customers/metafields/list` | R0 | Per-customer when `customer_id` set; otherwise filter or `list_all=true` against `/v3/customers/metafields` |
| `customers/metafields/set` | R1 | Upsert by namespace+key on one customer; `permission_set` defaults to `app_only` (not Storefront-readable) |
| `customers/metafields/delete` | R1 | Delete by `metafield_id` or `namespace`+`key`; preview → confirm |
| `customers/metafields/bulk_set` | R1 | Apply same namespace+key+value to many customers (sequential per-customer calls; ≤50 customers) |
| `customers/metafields/bulk_delete` | R1 | Delete namespace+key across customers; skips customers without that metafield (≤50 customers) |
| `customers/settings/global/get` | R0 | GET `/v3/customers/settings` |
| `customers/settings/global/update` | R2 | PUT global settings; merges `settings` into current; preview → `confirmed=true` |
| `customers/settings/channel/get` | R0 | GET `/v3/customers/settings/channels/{channel_id}` |
| `customers/settings/channel/update` | R2 | PUT channel settings; merges `settings`; **`allow_global_logins`** in patch requires **`confirm_allow_global_logins=true`** + `confirmed=true` |
| `customers/consent/get` | R0 | GET `/v3/customers/{customer_id}/consent` |
| `customers/consent/update` | R1 | PUT consent (`allow` / `deny` category arrays); preview → confirm |
| `customers/stored_instruments/list` | R0 | GET stored instruments; gate 1 `acknowledge_stored_instruments=true`; gate 2 raw `token` only with `include_sensitive_token_data=true` + `confirmed=true` (otherwise redacted) |
| `customers/credentials/validate` | R2 | POST validate-credentials (rate limited); preview masks email; password never returned |
| `customers/segments/list` | R0 | GET `/v3/segments` (paginated, supports `id:in` UUID list ≤ 40); **Enterprise-only feature** |
| `customers/segments/get` | R0 | Single segment by UUID (wraps `id:in`) |
| `customers/segments/create` | R1 | POST batch (≤10 rows; store cap 1000 segments); `name` required; preview → confirm |
| `customers/segments/update` | R1 | PUT batch (≤10 rows); each row needs `id` + at least one of `name`, `description`; preview → confirm |
| `customers/segments/delete` | R3 | DELETE `id:in` (≤40 ids); preview → `confirmed=true`; **does not delete shopper profiles** |
| `customers/segments/shoppers/list` | R0 | GET shoppers in a segment — **requires `store_v2_customers` (modify) scope** even though it is a GET |
| `customers/segments/shoppers/add` | R1 | POST shopper-profile UUIDs to a segment; accepts `shopper_profile_ids` or `customer_ids` (≤50 numeric ids/call, resolved via `customers?include=shopper_profile_id`); ≤50 profile ids/call after resolution; missing profiles surfaced separately; preview → confirm |
| `customers/segments/shoppers/remove` | R1 | DELETE `id:in` profile UUIDs from a segment (≤40); preview → confirm; profile records remain |
| `customers/shopper_profiles/list` | R0 | GET `/v3/shopper-profiles` paginated; **no `id:in` or `customer_id` filter** — use `customers?include=shopper_profile_id` to map customers ↔ profiles |
| `customers/shopper_profiles/create` | R1 | POST batch (≤50; deduped); accepts `customer_ids` or `profiles_batch=[{customer_id}]`; duplicates 409 (1:1 profile↔customer); preview → confirm |
| `customers/shopper_profiles/delete` | R2 | DELETE `id:in` (≤40); deletes profile **and all of its segment memberships** (customer record unaffected); preview → confirm |
| `customers/shopper_profiles/list_segments` | R0 | GET `/v3/shopper-profiles/{id}/segments` |
| `marketing/promotions/automatic/list` | R0 | GET `/v3/promotions` with `redemption_type` hard-pinned to `automatic` (defensively filters out COUPON entries from older stores). Sort/direction/channel filters validated. |
| `marketing/promotions/automatic/get` | R0 | GET `/v3/promotions/{id}`; refuses to return COUPON promotions (points at the coupon subtree) |
| `marketing/promotions/automatic/create` | R2 | POST single promotion. `redemption_type` overridden to AUTOMATIC. Deep validation (rules/actions/conditions/item-matchers/notifications/customer/status/currency_code). Soft-warn at ≥100 ENABLED promotions or >10 rules. Preview → confirm |
| `marketing/promotions/automatic/update` | R2 | Fetch-merge-PUT. `patch` overrides top-level scalars; `patch.rules` replaces in full (warns); `rules_patch=[{index, replace_with}]` for positional rule edits. Read-only fields rejected. Refuses COUPON promotions. Preview → confirm |
| `marketing/promotions/automatic/set_status` | R2 | Convenience wrapper — flips status to ENABLED/DISABLED. Noop when already at target. Preview → confirm |
| `marketing/promotions/automatic/delete` | R3 | DELETE `?id:in=…` (≤40 ids/call). Preview shows name/status/current_uses. 422-hint points at `coupon/codes/delete` and the cascade flag on `coupon/delete`. Preview → confirm |
| `marketing/promotions/coupon/list` | R0 | GET `/v3/promotions` hard-pinned to `redemption_type=coupon`; supports the BC `code` (full-string match) filter |
| `marketing/promotions/coupon/get` | R0 | GET `/v3/promotions/{id}`; refuses on AUTOMATIC promotions |
| `marketing/promotions/coupon/create` | R2 | POST single coupon promotion. `redemption_type` overridden to COUPON. Coupon-specific cross-field validation: `coupon_type ∈ SINGLE\|BULK`; `coupon_overrides_other_promotions=true` requires `can_be_used_with_other_promotions=false`; `multiple_codes` only on BULK; **deprecated** `coupon_overrides_automatic_when_offering_higher_discounts` rejected outright. Codes added via the `coupon/codes/*` tools afterward. Preview → confirm |
| `marketing/promotions/coupon/update` | R2 | Fetch-merge-PUT, same merge / `rules_patch` semantics as automatic/update. Refuses on AUTOMATIC. Coupon cross-field validation runs on the merged document. Preview → confirm |
| `marketing/promotions/coupon/set_status` | R2 | ENABLED/DISABLED toggle for coupon promotions. Refuses on AUTOMATIC. Preview → confirm |
| `marketing/promotions/coupon/delete` | R3 | DELETE `?id:in=…` (≤40 ids/call). Preview surfaces attached-codes count + sample (best-effort first page). Optional `delete_codes_first=true` cascades through attached codes (chunked, ≤1000 per promotion) before the promotion delete. 422 with hint points at both the manual codes-delete path and the cascade flag. Preview → confirm |
| `marketing/promotions/coupon/codes/list` | R0 | GET `/v3/promotions/{id}/codes`; **cursor-paginated** via `before`/`after`; surfaces `has_more` and the cursor |
| `marketing/promotions/coupon/codes/create_single` | R1 | POST `/codes`. Charset validation client-side (letters/numbers/spaces/underscores/hyphens, ≤50 chars). Pre-flights parent (refuses on AUTOMATIC). Surfaces parent-`max_uses`-overrides-code warning. **Codes are immutable — delete and recreate to "edit"**. Preview → confirm |
| `marketing/promotions/coupon/codes/generate_bulk` | R2 | POST `/codegen`. Pre-flights parent's `coupon_type=BULK`; refuses on SINGLE. `batch_size` capped at **250** (BC max); `length` validated 6..16; `format ∈ NUMBERS\|LETTERS\|ALPHANUMERIC`. Response sample truncated to 5 codes plus `generated_count`. Preview → confirm |
| `marketing/promotions/coupon/codes/delete` | R3 | DELETE `?id:in=…` (≤40 ids/call). Use this before `coupon/delete` on a promotion with attached codes, or to clean up after a `generate_bulk` run. Preview → confirm |
| `marketing/promotions/settings/get` | R0 | GET `/v3/promotions/settings`; returns the four global policy flags (`zero_price` trigger, custom-price eligibility, coupon-count cap, original-price calculation mode) plus notes about Enterprise-only multi-coupon behavior |
| `marketing/promotions/settings/update` | R2 | Fetch-merge-PUT on `/v3/promotions/settings`. Type-checks booleans; validates `number_of_coupons_allowed_at_checkout ∈ 1..5`; warns (warn-only) when setting coupon count >1 (Enterprise-only); returns `noop` when patch equals current; preview → confirm |
| `webhooks/list` | R0 | `GET /v3/hooks` — list all webhook registrations; optional `scope`, `is_active`, `channel_id` filters; scope **`store_v2_information_read_only`** |
| `webhooks/get` | R0 | `GET /v3/hooks/{id}` — full details for one webhook (scope, destination, is_active, channel_id, headers) |
| `webhooks/events` | R0 | `GET /v3/hooks/{id}/events` — recent delivery attempts; useful for diagnosing failures |
| `webhooks/create` | R1 | `POST /v3/hooks` — destination must be **HTTPS**; optional `channel_id` (channel-scoped vs store-wide); optional `headers_json` (custom delivery headers); preview → **`confirmed`**; scope **`store_v2_information`** |
| `webhooks/update` | R1 | Fetch-merge-`PUT /v3/hooks/{id}` — scope, destination, is_active, or headers; `channel_id` immutable; preview → **`confirmed`** |
| `webhooks/delete` | R3 | `DELETE /v3/hooks/{id}` — permanently remove; preview shows scope + destination → **`confirmed`** |

## Project Structure

```
cmd/server/              — Entry point
internal/
  config/                — Environment-based configuration with bounds checking
  server/                — MCP server wiring
  discovery/             — Progressive disclosure registry and meta-tools
  middleware/             — R4 blocklist, confirmation helpers, bearer auth, logging (R1–R3 preview/confirm enforced in handlers)
  session/               — Per-session TTL cache with size limits
  bigcommerce/           — BigCommerce REST API client (rate limiting, retries, batching)
  tools/
    catalog/             — Product, category, brand, global variant handlers; shared: metafield_shared.go, list_filter_helpers.go, variant_update_parse.go (see docs/ARCHITECTURE.md section 4)
    orders/              — Order management, fulfillment, payments, refunds
    customers/           — Customer records, addresses, attributes, segments, shopper profiles
    inventory/           — Location lifecycle, item visibility, adjustments
    promotions/          — Automatic and coupon promotions, coupon codes, settings
    storefront/          — Script Manager scripts
    webhooks/            — Webhook registrations (list/get/events/create/update/delete via /v3/hooks)
```

### Test Coverage

Run `go test ./...` — multiple testify suites across `internal/tools/catalog`, `internal/discovery`, `internal/config`, `internal/middleware`, and `internal/session` cover security-critical paths (type assertions, price bounds, auth, cache eviction, config validation, confirmed-param enforcement) and tool parameter parsing. Exact subtest counts change as suites grow; do not rely on a fixed number in docs.

## Security

Security is a first-class concern throughout this project. A comprehensive security review has been performed and all critical/high findings have been remediated. Key controls include:

- **Authentication**: Bearer token required for HTTP/SSE transports (`MCP_AUTH_TOKEN`); constant-time comparison prevents timing attacks
- **Input validation**: All LLM-provided arguments use safe type assertions — malformed input returns an error, never a panic
- **Price safety**: Price adjustments are bounded (`-100%` to `+1000%`) with a `$0.00` floor
- **Resource limits**: Response body cap (50 MB), pagination ceiling (default 10k records per `GetAll`; set `BC_MAX_TOTAL_RECORDS=0` for unlimited), cache size limits (1k entries/session, 100 sessions)
- **Write protection**: R1+ tools must declare a `confirmed` parameter — enforced at registration time (server won't start without it)
- **Secret handling**: Credentials never logged; error messages truncated before returning to LLM; `.gitignore` excludes `.env`

Current release posture is **local-first** (developer-run, operator-owned credentials,
local transport by default). Wider hosted/multi-tenant controls are documented and
intentionally deferred until needed.

For the full security review with findings, threat model, and remaining recommendations, see **[docs/SECURITY.md](./docs/SECURITY.md)** (enumerated findings S1–S9 plus follow-up items S10–S12).

## Rate Limiting

The client layer implements the conservative defaults from [`docs/BC-Tool-Boundaries.md`](./docs/BC-Tool-Boundaries.md):

- 2 requests/second global throttle
- Pauses when `X-Rate-Limit-Requests-Left` drops below 25
- Exponential backoff on 429/5xx responses
- 0.5s delay between batch chunks
- Sequential writes by default (no parallel mutations)

## Documentation

- **[docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)** — Full architecture, design decisions, token analysis, security controls, known limitations, expansion roadmap, and guide for adding new tool domains
- **[docs/discovery-registration-audit.md](./docs/discovery-registration-audit.md)** — `discover_tools` vs registration policy (active roots + non-empty categories + tool parent-chain guarantees)
- **[docs/msf-research-outline.md](./docs/msf-research-outline.md)** — Multi-storefront / channels: API review, MSF detection heuristics, insertion points (research)
- **[docs/channels-msf-implementation-roadmap.md](./docs/channels-msf-implementation-roadmap.md)** — Phased MSF MCP features (channels, trees, assignments, listings)
- **[docs/SECURITY.md](./docs/SECURITY.md)** — Security review findings (S1–S9 remediated, S10–S12 documented), threat model, and remaining recommendations
- [docs/BC-API-Reference.md](./docs/BC-API-Reference.md) — Full BigCommerce API endpoint map
- [docs/BC-API-SPECIFICITY.md](./docs/BC-API-SPECIFICITY.md) — Field-level API quirks, undocumented behaviors, and response shape differences discovered during development
- [docs/BC-Tool-Boundaries.md](./docs/BC-Tool-Boundaries.md) — Tool tiers, caps, and safety rules
- [docs/bc_system_prompt.md](./docs/bc_system_prompt.md) — Agent operating guidelines
- [docs/catalog-completion-checklist.md](./docs/catalog-completion-checklist.md) — Catalog completeness gate before adding new tool domains
- [docs/architecture-executive-summary.md](./docs/architecture-executive-summary.md) — Executive-level architecture summary
