# BigCommerce MCP Server

A high-performance Model Context Protocol (MCP) server for BigCommerce store management, built in Go with the [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) SDK.

**Source repository:** [github.com/roel-c/bc-admin-mcp](https://github.com/roel-c/bc-admin-mcp)

## Architecture

This server uses **progressive disclosure** to minimize token consumption and maximize LLM accuracy. Instead of registering all BigCommerce tools upfront (~40,000+ tokens), only two meta-tools are exposed:

- **`discover_tools`** — Navigate a hierarchical tree of available tool categories
- **`execute_tool`** — Execute any tool by its full path with arguments

This reduces initial token usage to ~600 tokens (a 60-100x reduction) and keeps the LLM focused on only the tools relevant to the current task.

### Tool Hierarchy

```
catalog/          — Products, categories, brands, variants
orders/           — Order management, fulfillment, refunds
customers/        — Customer management, groups, addresses
carts/            — Cart and checkout operations
inventory/        — Stock levels and adjustments
marketing/        — Promotions and coupons
store/            — Settings, SEO, shipping
```

Only **`catalog/`** has registered tools today (products, categories, and product sub-resources). Other roots exist for `discover_tools` navigation and future work; **`catalog/brands`** and **`catalog/variants`** are category placeholders with **no tools** yet (brand-like and standalone-variant operations are not implemented at those paths).

### Tool Tiers (from BC-Tool-Boundaries.md)

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

For full architectural detail, design decisions with alternatives considered, and the expansion roadmap, see **[ARCHITECTURE.md](./ARCHITECTURE.md)**.

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

## Implemented Tools

| Tool Path | Tier | Description |
|-----------|------|-------------|
| `catalog/products/search` | R0 | Filter search (name, SKU, price range, category, brand, visibility, keyword) |
| `catalog/products/get` | R0 | Single product with variant pricing detection |
| `catalog/products/create` | R1 | Create product with all writable fields, optional inline images |
| `catalog/products/update` | R1 | Unified update: any writable field(s) on one or more products; target by product_ids, sku, product_name, or category_id |
| `catalog/products/delete` | R3 | Permanently delete products (destructive, requires confirmation) |
| `catalog/products/assign_categories` | R1 | Additive product-to-category assignment via dedicated BC endpoint |
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
| `catalog/products/custom_fields/list` | R0 | List custom fields |
| `catalog/products/custom_fields/set` | R1 | Upsert a custom field by name |
| `catalog/products/custom_fields/delete` | R2 | Delete a custom field |
| `catalog/products/modifiers/list` | R0 | List modifiers |
| `catalog/products/modifiers/create` | R1 | Create a modifier |
| `catalog/products/modifiers/delete` | R2 | Delete a modifier |
| `catalog/categories/list` | R0 | Filter search with `list_all` mode for full catalog dump |
| `catalog/categories/get` | R0 | Single category by ID |
| `catalog/categories/create` | R1 | Create with name-based parent resolution (no numeric IDs needed) |
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
    catalog/             — Product and category tool handlers (search, CRUD, bulk ops)
    orders/              — (scaffold — not yet implemented)
    customers/           — (scaffold — not yet implemented)
    carts/               — (scaffold — not yet implemented)
    inventory/           — (scaffold — not yet implemented)
    marketing/           — (scaffold — not yet implemented)
    store/               — (scaffold — not yet implemented)
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

For the full security review with findings, threat model, and remaining recommendations, see **[SECURITY.md](./SECURITY.md)** (enumerated findings S1–S9 plus follow-up items S10–S12).

## Rate Limiting

The client layer implements the conservative defaults from `BC-Tool-Boundaries.md`:

- 2 requests/second global throttle
- Pauses when `X-Rate-Limit-Requests-Left` drops below 25
- Exponential backoff on 429/5xx responses
- 0.5s delay between batch chunks
- Sequential writes by default (no parallel mutations)

## Documentation

- **[ARCHITECTURE.md](./ARCHITECTURE.md)** — Full architecture, design decisions, token analysis, security controls, known limitations, expansion roadmap, and guide for adding new tool domains
- **[SECURITY.md](./SECURITY.md)** — Security review findings (10 findings, all remediated), threat model, and remaining recommendations
- [BC-API-Reference.md](./BC-API-Reference.md) — Full BigCommerce API endpoint map
- [BC-API-SPECIFICITY.md](./BC-API-SPECIFICITY.md) — Field-level API quirks, undocumented behaviors, and response shape differences discovered during development
- [BC-Tool-Boundaries.md](./BC-Tool-Boundaries.md) — Tool tiers, caps, and safety rules
- [bc_system_prompt.md](./bc_system_prompt.md) — Agent operating guidelines
