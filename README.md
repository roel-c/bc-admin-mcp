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

**`discover_tools("")`** returns **`catalog`** only — the live MCP tree matches implemented tools (no empty placeholder roots). Planned domains (orders, customers, carts, …) are described in **[docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md)** section 7, not in discovery, until tools register.

```
catalog/          — Products, categories, brands, variants (full tree under this root)
```

**Variants:** use **`catalog/products/variants`** for product-scoped CRUD, options-linked creates, and variant metafields. Use **`catalog/variants`** for **global** `GET /v3/catalog/variants` list/search and **`PUT /v3/catalog/variants`** batch updates (IMS-style); see tool table rows below.

**Before implementing orders, customers, carts, etc.:** work through the catalog hardening and scope checklist in [`docs/catalog-completion-checklist.md`](docs/catalog-completion-checklist.md) so discovery matches reality and patterns (tiers, bulk, metafields) stay consistent when those domains are added.

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

### Build & Run

```bash
# Build
make build

# Run with stdio transport (for Cursor, Claude Desktop, etc.)
make run

# Run with Streamable HTTP transport (for remote/shared access — requires MCP_AUTH_TOKEN)
make run-http
```

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

The server exposes **`discover_tools`** and **`execute_tool`**. For every real catalog operation, call **`execute_tool`** with:

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

**Full agent copy-paste reference:** **[docs/bc_system_prompt.md](./docs/bc_system_prompt.md)** — in order: *Universal `execute_tool` shape (all catalog tools)*, *Product metafields*, *Variant metafields* (single-variant + bulk + **cross-product bulk**), *Category metafields*, *Brands* (list / get / create / update / **brand metafields**), *High-traffic reads* (search / get / category list), *Unified product update* (minimal examples), *Additive category assignment*. Using that structure reduces malformed MCP calls and speeds up correct previews.

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
| `catalog/channels/category_trees` | R0 | `GET /v3/catalog/trees` — category trees (optional **`channel_id`** → `channel_id:in` for MSF); needs **Products** scope (`store_v2_products_read_only` or `store_v2_products`) |
| `catalog/channels/listings/list` | R0 | `GET .../channels/{id}/listings` — optional **`product_ids`** filter; cursor pagination (up to 2000 rows); **`store_channel_listings_read_only`** or modify scope |
| `catalog/channels/listings/create` | R1 | `POST` — **`listings_json`** array (max 10 listings; BC requires **variants** per row); preview → **`confirmed`**; **`store_channel_listings`** |
| `catalog/channels/listings/update` | R2 | `PUT` — same JSON limits; each row needs **listing_id** (from list); preview → **`confirmed`** |

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
    (future domains: add internal/tools/<domain> + RegisterCategory in the same change — see docs/discovery-registration-audit.md)
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
- **[docs/discovery-registration-audit.md](./docs/discovery-registration-audit.md)** — `discover_tools` vs registration policy (catalog-only root until other domains ship)
- **[docs/msf-research-outline.md](./docs/msf-research-outline.md)** — Multi-storefront / channels: API review, MSF detection heuristics, insertion points (research)
- **[docs/channels-msf-implementation-roadmap.md](./docs/channels-msf-implementation-roadmap.md)** — Phased MSF MCP features (channels, trees, assignments, listings)
- **[docs/SECURITY.md](./docs/SECURITY.md)** — Security review findings (S1–S9 remediated, S10–S12 documented), threat model, and remaining recommendations
- [docs/BC-API-Reference.md](./docs/BC-API-Reference.md) — Full BigCommerce API endpoint map
- [docs/BC-API-SPECIFICITY.md](./docs/BC-API-SPECIFICITY.md) — Field-level API quirks, undocumented behaviors, and response shape differences discovered during development
- [docs/BC-Tool-Boundaries.md](./docs/BC-Tool-Boundaries.md) — Tool tiers, caps, and safety rules
- [docs/bc_system_prompt.md](./docs/bc_system_prompt.md) — Agent operating guidelines
- [docs/catalog-completion-checklist.md](./docs/catalog-completion-checklist.md) — Catalog completeness gate before adding new tool domains
- [docs/architecture-executive-summary.md](./docs/architecture-executive-summary.md) — Executive-level architecture summary
