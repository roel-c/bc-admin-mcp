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

## Quick Start

```bash
git clone https://github.com/roel-c/bc-admin-mcp.git && cd bc-admin-mcp
cp .env.example .env   # then edit with your BC_STORE_HASH + BC_AUTH_TOKEN
make build && make run # stdio transport — for Cursor, Claude Desktop, etc.
```

Point your MCP client at the built binary (see [Integration with Cursor](#integration-with-cursor)
for a `.cursor/mcp.json` example). Then, from the client:

1. **`discover_tools("")`** — see the live category roots (`catalog`, `orders`, `customers`, `marketing`, `inventory`, `storefront`, `webhooks`, `carts`, plus `b2b` when enabled).
2. **`discover_tools("<path>")`** — drill down (e.g. `"catalog"` → `"catalog/products"`) until you see tool stubs with a `tier`.
3. **`execute_tool`** — run one, with the full path and its arguments nested under `arguments`:
   ```json
   {
     "tool_path": "catalog/products/metafields/set",
     "arguments": { "product_id": 19402, "namespace": "my_integration", "key": "external_ref", "value": "pim-12345", "confirmed": false }
   }
   ```
   Tools tiered R1+ (writes) return a preview until you re-call with `"confirmed": true` — see [Tool Tiers](#tool-tiers-from-docsdevelopmentmd) below.

That's the whole interaction model. Everything past this point is detail: full setup/safety checklist ([Setup](#setup)), the complete tool inventory ([Implemented Tools](#implemented-tools)), and design rationale ([Architecture](#architecture)).

## Documentation Map

Not every doc here needs to be read up front. Use this table to find the right one for what you're doing right now:

| Doc | Read this if... |
|---|---|
| **This README** | You're setting up or operating the server |
| **[docs/AGENT.md](./docs/AGENT.md)** | You're an agent/LLM calling tools — operating rules, safety, tiers, response format; Script Manager frontend injection points to an external Stencil guide |
| **[docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md)** | You need exact numeric caps, OAuth scopes, or tier policy |
| **[docs/B2B.md](./docs/B2B.md)** | You're using or extending the B2B Edition tools |
| **[docs/WORKFLOW.md](./docs/WORKFLOW.md)** | You're adding a new tool/endpoint (contributor cadence) |
| **[docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)** | You want design rationale, token budget analysis, or registration mechanics (contributor) |
| **[docs/BC-API-Reference.md](./docs/BC-API-Reference.md)**, **[docs/BC-API-SPECIFICITY.md](./docs/BC-API-SPECIFICITY.md)** | You need one specific BigCommerce endpoint/field detail — jump to the section, these aren't meant to be read start to end |
| **[docs/MSF.md](./docs/MSF.md)**, **[docs/SECURITY.md](./docs/SECURITY.md)**, **[docs/FOLLOW-UPS.md](./docs/FOLLOW-UPS.md)** | You want implementation history, the security audit, or tracked technical debt |

## Architecture

This server uses **progressive disclosure** to minimize token consumption and maximize LLM accuracy. Instead of registering all BigCommerce tools upfront (~40,000+ tokens), only two meta-tools are exposed:

- **`discover_tools`** — Navigate a hierarchical tree of available tool categories
- **`execute_tool`** — Execute any tool by its full path with arguments

This reduces initial token usage to ~600 tokens (a 60-100x reduction) and keeps the LLM focused on only the tools relevant to the current task.

### Tool Hierarchy

**`discover_tools("")`** returns eight always-on roots — **`catalog`**, **`orders`**, **`customers`**, **`marketing`**, **`inventory`**, **`storefront`**, **`webhooks`**, and **`carts`** — plus **`b2b`** when B2B Edition is enabled (`BC_B2B_ENABLED=true`). The live MCP tree matches implemented tools (no empty placeholder roots). Planned domains (e.g. `store/`) are described in **[docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)** section 7, not in discovery, until tools register.

```
catalog/     — Products, categories, brands, variants, channels/MSF, and price lists (full tree under this root).
orders/      — V2 order management, fulfillment shipments, payments (capture/void), and refunds.
customers/   — V3 customer records, addresses, attributes, metafields, settings, consent, stored instruments, credential validation, segments, shopper profiles, and V2 customer groups.
marketing/   — Promotions engine: automatic promotions, coupon promotions + codes, and store-wide promotion settings.
inventory/   — Locations, items, backorders (limit + qty_backordered), and guarded absolute/relative stock adjustments.
storefront/  — Script Manager script injection and management.
webhooks/    — Webhook registration CRUD and delivery-event inspection (/v3/hooks).
carts/       — Server-side cart lifecycle, cart items, cart metafields, and the checkout flow (coupons, addresses, consignments, convert-to-order).
b2b/         — (Gated) Company accounts, buyer users, and company addresses via B2B Edition.
```

**Variants:** use **`catalog/products/variants`** for product-scoped CRUD, options-linked creates, and variant metafields. Use **`catalog/variants`** for **global** `GET /v3/catalog/variants` list/search and **`PUT /v3/catalog/variants`** batch updates (IMS-style); see tool table rows below.

**Adding a new tool domain?** Follow [`docs/WORKFLOW.md`](./docs/WORKFLOW.md) — the research → implement → gate → reload → live-validate → docs → commit cadence used to build every domain in this table.

### Tool Tiers (from [docs/DEVELOPMENT.md](./docs/DEVELOPMENT.md))

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

To enable the optional **B2B Edition** tools, set `BC_B2B_ENABLED=true` in `.env`
(the store's API account must have the B2B Edition scope). It reuses the same
`BC_AUTH_TOKEN` + `BC_STORE_HASH` — see [docs/B2B.md](./docs/B2B.md).

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

- **[Channel assignments vs listings](./docs/DEVELOPMENT.md#9-channel-assignments-vs-channel-listings)** — how catalog channel assignments relate to per-channel listings, and when to use **`catalog/products/channel_summary`**.
- **[MCP discovery and preview drills](./docs/ARCHITECTURE.md#9-testing-strategy)** — how to exercise **`discover_tools`** / **`execute_tool`** and preview-then-confirm (includes automated regression tests you can run in CI).

**Full agent copy-paste reference:** **[docs/AGENT.md](./docs/AGENT.md)** — includes the universal `execute_tool` envelope, catalog examples, and operating constraints used across domains. Using that structure reduces malformed MCP calls and speeds up correct previews.

## Implemented Tools

A human-browsable snapshot of every tool path, for skimming without a running server. **Agents:** prefer live `discover_tools`/`execute_tool` over parsing this table — it's the same data, delivered lazily, and each tool's own description (surfaced at the leaf) carries its exact argument shapes and quirks.

| Tool Path | Tier | Description |
|-----------|------|-------------|
| `catalog/products/search` | R0 | Filter search (name, SKU, price range, category, brand, visibility, keyword, MSF **`channel_ids`** → `channel_id:in`) |
| `catalog/products/get` | R0 | Single product with variant pricing detection |
| `catalog/products/create` | R1 | Create product with all writable fields, optional inline images, optional MSF `channel_ids` (additive post-create channel assignment) |
| `catalog/products/update` | R1 | Unified update: any writable field(s) on one or more products; target by product_ids, sku, product_name, or category_id; optional MSF `channel_ids` for additive post-update channel assignment (≤ 500 product×channel pairs) |
| `catalog/products/bulk_sku_update` | R1 | Batch-update the SKU of multiple specific products in one call — one `product_id` → one new SKU per entry (up to **100** pairs/call). Use instead of `catalog/products/update` when each product needs a *different* SKU |
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
| `catalog/products/custom_fields/create` | R1 | Always **create** a new custom field (never upserts) — use when you need multiple fields with the same name; otherwise prefer `custom_fields/set` |
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
| `catalog/brands/delete` | R3 | `DELETE /v3/catalog/brands/{id}` — permanently delete a brand (products keep existing, brand link cleared); preview → confirm |
| `catalog/brands/image/set` | R1 | Set/replace a brand image by public `image_url` (via brand update); preview → confirm |
| `catalog/brands/image/delete` | R2 | `DELETE /v3/catalog/brands/{id}/image` — remove the brand image; preview → confirm |
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
| `inventory/locations/items/list` | R0 | `GET /v3/inventory/locations/{location_id}/items` (includes `qty_backordered`, `settings.backorder_limit`) |
| `inventory/locations/items/update` | R2 | `PUT /v3/inventory/locations/{location_id}/items` with `settings[]` (e.g. `backorder_limit`); max 10 rows; preview → confirm |
| `inventory/items/list` | R0 | `GET /v3/inventory/items` with optional `location_ids`, `product_ids`, `variant_ids`, `skus`; requires a filter or `list_all=true`; returns backorder fields |
| `inventory/items/get` | R0 | Inventory for one variant via list filter (includes per-location `qty_backordered` / `backorder_limit`) |
| `inventory/items/update_batch` | R2 | `PUT /v3/inventory/items` using caller-supplied `update` payload (`items[]` or `data[]`, max 10 rows); preview → confirm; use `locations/items/update` for `backorder_limit` |
| `inventory/adjustments/absolute` | R2 | `PUT /v3/inventory/adjustments/absolute` for up to 10 rows; identity = `variant_id` \| `product_id` \| `sku`; optional `quantity` / `qty_backordered`; preview → confirm |
| `inventory/adjustments/relative` | R2 | `POST /v3/inventory/adjustments/relative` for up to 10 rows; same identity rules; optional `quantity` / `qty_backordered` deltas; preview → confirm |
| `customers/groups/list` | R0 | List/search customer groups (`list_all` or filters: name, name_like, is_default, is_group_for_guests, date_created*, date_modified*) |
| `customers/groups/get` | R0 | Single customer group by `group_id` (full category_access + discount_rules) |
| `customers/groups/count` | R0 | `GET /v2/customer_groups/count` — total customer group count |
| `customers/groups/create` | R1 | Create group (`name`, optional `is_default`, `is_group_for_guests`, `category_access_*`, `discount_rules`); preview → confirm. `price_list` rules are mutually exclusive — mixed input silently keeps the price_list rule and warns |
| `customers/groups/update` | R1 | Update group by `group_id`; only supplied fields change. **Note:** sending `discount_rules` overwrites the entire set (BC bulk semantics); preview → confirm |
| `customers/groups/delete` | R3 | Destructive delete by `group_id` — BC unassigns all members automatically; preview → `confirmed=true` |
| `customers/list` | R0 | Search customers (`list_all` or filters); GET `/v3/customers` |
| `customers/get` | R0 | One customer by `customer_id` (wraps `id:in`) |
| `customers/create` | R2 | POST `/v3/customers` (≤10); preview → `confirmed=true`; `new_password` also needs `set_password=true`; supports `origin_channel_id` / `channel_ids` for MSF storefront-scoped identities |
| `customers/update` | R2 | PUT `/v3/customers` (≤10 rows in `customer_batch`); same password double gate; batch rows may set `origin_channel_id` / `channel_ids` |
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
| `storefront/scripts/list` | R0 | `GET /v3/content/scripts` — list Script Manager scripts |
| `storefront/scripts/get` | R0 | `GET /v3/content/scripts/{uuid}` — single script |
| `storefront/scripts/create` | R1 | `POST /v3/content/scripts` — inject a script (HTML/`src`); preview → **`confirmed`** |
| `storefront/scripts/update` | R1 | `PUT /v3/content/scripts/{uuid}` — update script fields; preview → **`confirmed`** |
| `storefront/scripts/toggle` | R1 | Enable/disable a script without editing its body; preview → **`confirmed`** |
| `storefront/scripts/delete` | R3 | `DELETE /v3/content/scripts/{uuid}` — permanently remove; preview → **`confirmed`** |
| `carts/cart/create` | R1 | `POST /v3/carts` — create a server-side cart with optional line/custom items, `customer_id`, `channel_id`; preview → **`confirmed`** |
| `carts/cart/get` | R0 | `GET /v3/carts/{id}` — line items, totals, currency; optional `include_redirect_urls` |
| `carts/cart/update` | R1 | `PUT /v3/carts/{id}` — update `customer_id`, `channel_id`, or `locale`; preview → **`confirmed`** |
| `carts/cart/delete` | R3 | `DELETE /v3/carts/{id}` — permanently delete a cart; preview shows summary → **`confirmed`** |
| `carts/cart/items/add` | R1 | `POST /v3/carts/{id}/items` — add catalog/custom items; preview → **`confirmed`** |
| `carts/cart/items/update` | R1 | `PUT /v3/carts/{id}/items/{item_id}` — change a line item quantity; preview → **`confirmed`** |
| `carts/cart/items/remove` | R2 | `DELETE /v3/carts/{id}/items/{item_id}` — remove a line item; preview → **`confirmed`** |
| `carts/cart/checkout_url` | R0 | `POST /v3/carts/{id}/redirect_urls` — cart, checkout, and embedded-checkout URLs |
| `carts/cart/metafields/list` | R0 | `GET /v3/carts/{id}/metafields` — list cart metafields |
| `carts/cart/metafields/set` | R1 | Upsert cart metafield by namespace+key; preview → **`confirmed`** |
| `carts/cart/metafields/delete` | R1 | Delete cart metafield by id or namespace+key; preview → **`confirmed`** |
| `carts/checkout/get` | R0 | `GET /v3/checkouts/{id}` — billing address, consignments + shipping options, coupons, totals |
| `carts/checkout/coupon_apply` | R1 | `POST /v3/checkouts/{id}/coupons` — apply a coupon code; preview → **`confirmed`** |
| `carts/checkout/coupon_remove` | R2 | `DELETE /v3/checkouts/{id}/coupons/{code}` — remove a coupon; preview → **`confirmed`** |
| `carts/checkout/billing_address` | R1 | Set (POST) or update (PUT) the checkout billing address; preview → **`confirmed`** |
| `carts/checkout/consignment_add` | R1 | Add a shipping consignment (address + items) to reveal shipping options; preview → **`confirmed`** |
| `carts/checkout/consignment_update` | R1 | Update a consignment — typically to select a `shipping_option_id`; preview → **`confirmed`** |
| `carts/checkout/convert` | R2 | Convert a completed checkout into an order (consumes the cart, irreversible); preview → **`confirmed`** |

### B2B Edition tools (gated by `BC_B2B_ENABLED=true`)

The `b2b/` root only registers when `BC_B2B_ENABLED=true`; it reuses the existing `BC_AUTH_TOKEN` + `BC_STORE_HASH` (see [docs/B2B.md](./docs/B2B.md)).

| Tool Path | Tier | Description |
|-----------|------|-------------|
| `b2b/companies/list` | R0 | List companies; filter by status/name/email |
| `b2b/companies/get` | R0 | Company details by ID |
| `b2b/companies/create` | R1 | Create company + initial admin user; optional `customer_group_id` (Independent Companies behavior only); preview → **`confirmed`** |
| `b2b/companies/update` | R1 | Update company profile fields, including reassigning `customer_group_id`; preview → **`confirmed`** |
| `b2b/companies/set_status` | R2 | Approve, reject, or deactivate a company; preview → **`confirmed`** |
| `b2b/companies/delete` | R3 | Permanently delete company, all users, and (by default) their linked BC customer accounts (`delete_bc_customers=false` to keep); preview → **`confirmed`** |
| `b2b/companies/extra_fields` | R0 | List company extra-field (custom field) definitions |
| `b2b/companies/update_catalog` | R2 | Assign a price list/catalog to a company; preview → **`confirmed`** |
| `b2b/companies/users/list` | R0 | List buyer users; filter by company/role/email |
| `b2b/companies/users/get` | R0 | Get one user by B2B user ID (includes extra fields) |
| `b2b/companies/users/get_by_customer` | R0 | Resolve the B2B user from a BigCommerce customer ID |
| `b2b/companies/users/create` | R1 | Create buyer user (0=admin, 1=senior, 2=junior); preview → **`confirmed`** |
| `b2b/companies/users/bulk_create` | R1 | Create up to 10 users in one call; preview → **`confirmed`** |
| `b2b/companies/users/update` | R1 | Update user name, phone, or role; preview → **`confirmed`** |
| `b2b/companies/users/delete` | R2 | Remove user from the buyer portal; preview → **`confirmed`** |
| `b2b/companies/users/extra_fields` | R0 | List user extra-field definitions |
| `b2b/companies/addresses/list` | R0 | List company addresses; filter by billing/shipping/country |
| `b2b/companies/addresses/create` | R1 | Add an address to a company; preview → **`confirmed`** |
| `b2b/companies/addresses/update` | R1 | Full PUT update of a company address; preview → **`confirmed`** |
| `b2b/companies/addresses/delete` | R2 | Remove a company address; preview → **`confirmed`** |
| `b2b/companies/attachments/list` | R0 | List a company's file attachments |
| `b2b/companies/attachments/add` | R1 | Upload a local file (≤10MB) to the company; preview → **`confirmed`** |
| `b2b/companies/attachments/delete` | R2 | Delete an attachment by ID; preview → **`confirmed`** |
| `b2b/companies/roles/list` \| `get` | R0 | List roles / get a role and its permissions |
| `b2b/companies/roles/create` \| `update` | R1 | Create / replace a custom role's permissions; preview → **`confirmed`** |
| `b2b/companies/roles/delete` | R2 | Delete a custom role; preview → **`confirmed`** |
| `b2b/companies/permissions/list` | R0 | List permission definitions (discover codes) |
| `b2b/companies/permissions/create` \| `update` | R1 | Create / update a custom permission; preview → **`confirmed`** |
| `b2b/companies/permissions/delete` | R2 | Delete a custom permission; preview → **`confirmed`** |
| `b2b/companies/hierarchy/get` \| `subsidiaries` | R0 | View a company's full hierarchy / its subsidiaries |
| `b2b/companies/hierarchy/attach_parent` | R1 | Set a parent above a company; preview → **`confirmed`** |
| `b2b/companies/hierarchy/detach_subsidiary` | R2 | Remove a subsidiary's parent link; preview → **`confirmed`** |
| `b2b/channels/list` \| `get` | R0 | List storefront channels / get one by BigCommerce channel ID |
| `b2b/orders/get` | R0 | B2B view of an order (PO number, company, extra fields) by BC order ID |
| `b2b/orders/update` | R1 | Set an order's PO number / extra fields; preview → **`confirmed`** |
| `b2b/orders/assign_customer_orders` | R2 | Attach a buyer's historical orders to their company; preview → **`confirmed`** |
| `b2b/orders/reassign` | R2 | Reassign orders by customer group (Dependent-behavior stores only); preview → **`confirmed`** |
| `b2b/orders/extra_fields` | R0 | List order extra-field definitions |
| `b2b/quotes/list` \| `get` \| `extra_fields` | R0 | List / get full detail / extra-field definitions for sales quotes |
| `b2b/quotes/create` \| `update` | R1 | Create / update a quote (`quote_json`); preview → **`confirmed`** |
| `b2b/quotes/delete` | R3 | Permanently delete a quote (use `update` with `status=archived` to hide); preview → **`confirmed`** |
| `b2b/quotes/checkout` | R1 | Generate cart/checkout URLs for a quote; preview → **`confirmed`** |
| `b2b/quotes/assign_to_order` | R2 | Associate an existing order with a quote; preview → **`confirmed`** |
| `b2b/quotes/pdf_export` | R0 | Backend-detail PDF download link for a quote |
| `b2b/quotes/shipping/rates` \| `custom_methods` | R0 | Available shipping rates / store-wide custom shipping methods |
| `b2b/quotes/shipping/select` | R1 | Assign a shipping rate to a quote; preview → **`confirmed`** |
| `b2b/quotes/shipping/remove` | R2 | Clear a quote's shipping rate; preview → **`confirmed`** |
| `b2b/invoices/list` \| `get` \| `download_pdf` \| `extra_fields` | R0 | Invoice reads (served from a distinct `/ip` base URL) |
| `b2b/invoices/create` \| `create_from_order` \| `update` | R2 | Create from raw JSON / from an existing order / update; preview → **`confirmed`** |
| `b2b/invoices/delete` | R3 | Permanently delete an invoice; preview → **`confirmed`** |
| `b2b/receipts/list` \| `get` | R0 | Payment receipt reads |
| `b2b/receipts/lines/list_all` \| `list_for_receipt` \| `get` | R0 | Receipt line-item reads |
| `b2b/receipts/delete` \| `lines/delete` | R3 / R2 | Permanently delete a receipt / a single receipt line; preview → **`confirmed`** |
| `b2b/payment_records/list` \| `get` \| `transactions` \| `operations` | R0 | Reads for payments logged against invoices (`/ip` base URL) |
| `b2b/payment_records/create_offline` \| `update_offline` \| `perform_operation` \| `update_processing_status` | R2 | Log/update an offline payment, run an operation, or set processing status; preview → **`confirmed`** |
| `b2b/payment_records/delete` | R3 | Permanently delete a payment record; preview → **`confirmed`** |
| `b2b/payments/list` \| `active_methods` | R0 | Store-wide payment method definitions / cross-company active methods |
| `b2b/companies/payments/list` \| `update` | R0 / R2 | A company's payment methods and enabled state / enable-disable; preview → **`confirmed`** |
| `b2b/companies/credit/get` \| `update` | R0 / R2 | A company's credit settings / update; preview → **`confirmed`** |
| `b2b/companies/payment_terms/get` \| `update` | R0 / R2 | A company's net-terms configuration / update; preview → **`confirmed`** |
| `b2b/sales_staff/list` \| `get` | R0 | List/get B2B users with a Sales Staff role and their company assignments |
| `b2b/sales_staff/update_assignments` | R1 | Assign/unassign companies (non-destructive); preview → **`confirmed`** |
| `b2b/super_admins/list` \| `companies_overview` \| `get` \| `companies` | R0 | List Super Admins / companies-overview / get one / its assigned companies |
| `b2b/super_admins/create` \| `bulk_create` \| `update` | R1 | Create (or convert a BC customer) / bulk create / update details; preview → **`confirmed`** |
| `b2b/super_admins/update_assignments` | R1 | Assign/unassign companies for a Super Admin (non-destructive); preview → **`confirmed`** |
| `b2b/companies/super_admins/list` \| `update_assignments` | R0 / R1 | Company-perspective view/assignment of Super Admins; preview → **`confirmed`** |
| `b2b/shopping_lists/list` \| `get` | R0 | List lists visible to a buyer (`user_id` xor `customer_id`) / full detail |
| `b2b/shopping_lists/create` \| `update` | R1 | Create (optional `items_json`) / partial update (item `quantity=0` removes it); preview → **`confirmed`** |
| `b2b/shopping_lists/delete` | R3 | Permanently delete a list; preview → **`confirmed`** |
| `b2b/shopping_lists/items/remove` | R2 | Remove a single item; preview → **`confirmed`** |

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
    inventory/           — Location lifecycle, item visibility, backorders, adjustments
    promotions/          — Automatic and coupon promotions, coupon codes, settings
    storefront/          — Script Manager scripts
    webhooks/            — Webhook registrations (list/get/events/create/update/delete via /v3/hooks)
    carts/               — Cart lifecycle, cart items, cart metafields, and checkout flow (/v3/carts, /v3/checkouts)
    b2b/                 — B2B Edition companies, users, addresses (gated by BC_B2B_ENABLED)
    shared/              — Shared tool helpers (ToolError, ToolJSON response builders)
```

### Test Coverage

Run `go test ./...` — multiple testify suites across `internal/tools/catalog`, `internal/discovery`, `internal/config`, `internal/middleware`, and `internal/session` cover security-critical paths (type assertions, price bounds, auth, cache eviction, config validation, confirmed-param enforcement) and tool parameter parsing. Exact subtest counts change as suites grow; do not rely on a fixed number in docs.

### Linting & CI

`make lint` runs `golangci-lint` with the pinned config in [`.golangci.yml`](./.golangci.yml) (`errcheck`, `govet`, `staticcheck`, `gosimple`, `ineffassign`, `unused`). Install the pinned version once with `make lint-install`. The same build, `go vet`, `go test`, and lint steps run in CI on every push and pull request via [`.github/workflows/ci.yml`](./.github/workflows/ci.yml); the golangci-lint version is pinned identically in the Makefile, the workflow, and `.golangci.yml` — bump all three together.

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

The client layer implements the conservative defaults from [`docs/DEVELOPMENT.md`](./docs/DEVELOPMENT.md):

- 2 requests/second global throttle
- Pauses when `X-Rate-Limit-Requests-Left` drops below 25
- Exponential backoff on 429/5xx responses
- 0.5s delay between batch chunks
- Sequential writes by default (no parallel mutations)

## Documentation

See the [Documentation Map](#documentation-map) near the top of this file for which doc to read for a given task. One addition worth calling out here: **[docs/WORKFLOW.md](./docs/WORKFLOW.md)** is the implementation workflow for adding endpoints (research → implement → build/test/lint gate → reload → live-validate with cleanup → docs → commit → CI) — **follow it for all new endpoint work** — and it also documents the on-demand **Full Surface Check** (§10), an MCP-only, end-to-end capability review (D2C and B2B variants) you can run anytime, independent of code changes.
