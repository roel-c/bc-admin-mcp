# BigCommerce MCP Server — Executive Architecture Summary

## What It Is

A lightweight Go server that lets AI agents (Claude, GPT, Cursor, etc.) manage BigCommerce admin surfaces (catalog, orders, customers, and promotions) through natural language — no custom integrations or bespoke API glue code required.

It implements the **Model Context Protocol (MCP)**, the emerging open standard for connecting AI models to external tools and data sources.

---

## How It Works

| Layer | What It Does | Why It Matters |
|-------|-------------|----------------|
| **Transport** | Supports stdio, **streamable HTTP**, and SSE with bearer-token auth on the HTTP transports | Deploy locally for dev (stdio) or as a remote service with no code changes |
| **Progressive Disclosure** | Only 2 tools are exposed to the AI: `discover_tools` and `execute_tool` | Prevents context-window bloat — the AI navigates category and tool stubs on-demand (`catalog`, `orders`, `customers`, `marketing`, `inventory`, `storefront`, `webhooks`) instead of loading full schemas upfront |
| **Middleware** | R4 blocklist in `Check()`, confirmation helpers for handlers, bearer auth, structured JSON logging | R1–R3 preview / `confirmed` flows are enforced in **handlers** and schema registration, not solely in `Check()` |
| **Tool Registry** | Hierarchical BigCommerce operations across catalog, orders, customers, and marketing | Organized for discoverability; extensible to carts, inventory, and store settings |
| **BC HTTP Client** | Rate limiting, exponential backoff, auto-pagination, batch chunking | Respects BigCommerce API quotas automatically; no manual throttling or pagination logic needed |

---

## Key Design Decisions

### 1. Progressive Disclosure over Flat Tool Lists
Instead of registering 30+ tools on the MCP surface (which wastes LLM context), we expose exactly **two meta-tools**. The AI navigates a hierarchical menu to find what it needs. This keeps token usage low and improves tool-selection accuracy.

### 2. Preview → Confirm Safety Pattern
Every write/delete operation follows a two-step flow:
1. **Preview** — shows exactly what will change before anything is committed
2. **Confirm** — executes only after explicit `confirmed=true`

This prevents the AI from accidentally bulk-deleting products or making unintended changes.

### 3. Risk Tiers (R0–R4)
Each tool is classified by risk level:
- **R0** — Read-only (search, list, get)
- **R1** — Standard writes (create, update)
- **R2** — High-risk writes (e.g. category move, image/option/variant/modifier deletes, custom field delete, filter-based unassign, channel-listings update, global variant bulk update)
- **R3** — Destructive (hard product/category deletes)
- **R4** — Forbidden (blocked entirely)

R4 is blocked in middleware `Check()`. R1–R3 use a shared **preview-then-confirm** pattern implemented per handler (calling `middleware.IsConfirmed` — or the equivalent `TierEnforcer.CheckConfirmation` — and requiring a `confirmed` field in the tool's input schema, enforced at registration time). Behavior stays consistent without claiming all logic lives in one middleware function.

### 4. Batch-First Performance
The client batches API calls wherever possible — fetching 100 categories in one request instead of 100 individual calls. Combined with automatic pagination and rate-limit awareness, this keeps operations fast without hitting BigCommerce quotas.

### 5. Minimal runtime footprint
Built in Go with **no databases or queues** at runtime: the MCP SDK plus `testify` (tests only at build time). No container orchestration required. Ships as a single binary.

---

## Tech Stack

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | **Go 1.26** | Fast compilation, single binary deployment, strong concurrency primitives |
| MCP Library | **mark3labs/mcp-go** | Reference Go implementation of the MCP standard |
| Testing | **testify + gomock** | Suite-based tests with interface mocking for full handler coverage |
| Logging | **log/slog** (stdlib) | Structured JSON logging with zero dependencies |
| Configuration | **Environment variables** | 12-factor app compatible; no config files to manage |

---

## Current Coverage & Roadmap

### Implemented (Catalog + Orders + Customers + Marketing + Inventory + Storefront + Webhooks)

The live `discover_tools` tree contains seven active roots — **`catalog`**, **`orders`**, **`customers`**, **`marketing`**, **`inventory`**, **`storefront`**, and **`webhooks`**. Placeholder categories without shipped tools are omitted so agents never land on empty leaves.

- **Products** — search (filters + MSF `channel_ids`), get, create, unified update, delete (R3); product↔category assignment (additive `assign_categories` + filter-based `unassign_categories`); MSF helpers `channel_summary`, `channel_assignments/list|assign|remove`; sub-resources for **images**, **options**, **variants**, **modifiers**, **custom fields**, **metafields** (single + bulk + cross-product variant bulk).
- **Categories** — list (with `list_all` and optional `channel_id` → `tree_id`), get, create (with `parent_name` resolution and MSF), bulk_update, products, SEO audit, move, reorder, metafields (list/set/delete), delete and bulk_delete with child-cascade safety gates (R3).
- **Brands** — list (filters + `list_all`), get, create, update; brand metafields (list/set/delete).
- **Variants (global)** — `catalog/variants/list` (`GET /v3/catalog/variants`), `catalog/variants/bulk_update` (`PUT /v3/catalog/variants` ≤ 200 rows, chunked by `BC_VARIANT_BATCH_SIZE`).
- **Channels (MSF)** — `catalog/channels/list`, `catalog/channels/get`, `catalog/channels/update` (R2, preview→confirm; name/status), `catalog/channels/category_trees`, `catalog/channels/listings/list|create|update`.
- **Price Lists (catalog pricing overlays)** — `catalog/pricelists/list|get|create|update|delete`, `catalog/pricelists/records/list|upsert|delete`, and `catalog/pricelists/assignments/list|create_batch|upsert|delete` (preview→confirm on writes; record upserts stay serial with conservative row caps).
- **Orders (V2 + V3 payment actions)** — `orders/management/list|get|create|update|delete|count|statuses|update_status|products/get` plus order sub-resources (`metafields/list|set|delete`, `coupons`, `shipping_addresses/list|get|update`, `messages`, `taxes`), `orders/fulfillment/shipments/list|get|create|update|delete`, and `orders/payments/actions/list|transactions/list|capture|void` with `orders/refunds/list|legacy_list|quote|create`; writes use preview→confirm, with per-order confirmation for financial actions.
- **Inventory (V3)** — `inventory/locations/list|create|update|delete`, `inventory/locations/metafields/list|set|delete`, `inventory/items/list|get|update_batch`, and `inventory/adjustments/absolute|relative` for dedicated inventory-domain operations separate from catalog product/variant projections. Writes use preview→confirm; `locations/delete` and other destructive operations are R3, while high-risk writes are R2 (batch caps stay at 10 rows where applicable).
- **Customers** — V3 customer records, addresses, attributes, attribute values, metafields, settings, consent, stored instruments, credential validation, segments/shopper profiles, plus V2 customer groups.
- **Marketing (Promotions)** — automatic promotion tools, coupon promotion tools, coupon code lifecycle (`list`, `create_single`, `generate_bulk`, `delete`), and store-wide promotions settings.
- **Storefront** — `storefront/scripts/list|get|create|update|toggle|delete` for Script Manager script injection via `GET/POST/PUT/DELETE /v3/content/scripts`.
- **Webhooks** — `webhooks/list|get|events|create|update|delete` for registering and managing event subscriptions via `GET/POST/PUT/DELETE /v3/hooks`; scope `store_v2_information`; optional `channel_id` for channel-scoped delivery.

### Planned — roadmapped only, **not registered** in `discover_tools`
- Orders: remaining lower-frequency endpoints (e.g., consignments, quotes, and deeper transaction/refund lifecycle details)
- Inventory: remaining lower-frequency inventory administrative surfaces (for example broader location/metafield batch management patterns)
- Carts: management, checkout
- Store: settings, shipping

These domains are documented in [`ARCHITECTURE.md` §7 — Expansion Roadmap](./ARCHITECTURE.md#7-expansion-roadmap). They will be added to the discovery tree (`RegisterCategory`) **in the same change** as the first tool in that domain, per the [discovery vs registration policy](./discovery-registration-audit.md).

The architecture supports adding new domains by implementing tool handlers and registering them — no changes to the transport, discovery, or middleware layers required.

---

## Deployment Model

```
┌─────────────────────────────────────────────┐
│  Option A: Local (stdio)                    │
│  AI IDE (Cursor) ←→ bc-mcp-server binary    │
│  Single process, zero network overhead      │
└─────────────────────────────────────────────┘

┌─────────────────────────────────────────────┐
│  Option B: Remote (HTTP/SSE)                │
│  Any MCP Client ←→ bc-mcp-server service    │
│  Bearer-token auth, runs on any host        │
└─────────────────────────────────────────────┘
```

Both options use the exact same binary — only an environment variable (`MCP_TRANSPORT`) changes.
