# BigCommerce MCP Server — Executive Architecture Summary

## What It Is

A lightweight Go server that lets AI agents (Claude, GPT, Cursor, etc.) manage a BigCommerce store's product catalog through natural language — no custom integrations or bespoke API glue code required.

It implements the **Model Context Protocol (MCP)**, the emerging open standard for connecting AI models to external tools and data sources.

---

## How It Works

| Layer | What It Does | Why It Matters |
|-------|-------------|----------------|
| **Transport** | Supports stdio, **streamable HTTP**, and SSE with bearer-token auth on the HTTP transports | Deploy locally for dev (stdio) or as a remote service with no code changes |
| **Progressive Disclosure** | Only 2 tools are exposed to the AI: `discover_tools` and `execute_tool` | Prevents context-window bloat — the AI navigates **~36** catalog tools on-demand via stubs instead of loading full schemas upfront |
| **Middleware** | R4 blocklist in `Check()`, confirmation helpers for handlers, bearer auth, structured JSON logging | R1–R3 preview / `confirmed` flows are enforced in **handlers** and schema registration, not solely in `Check()` |
| **Tool Registry** | Hierarchical catalog of BigCommerce operations (products, categories, images, variants, options, modifiers, metafields) | Organized for discoverability; extensible to orders, customers, inventory, and marketing |
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
- **R2** — High-risk writes (e.g. category move, image/option/variant/modifier deletes, custom field delete)
- **R3** — Destructive (delete operations)
- **R4** — Forbidden (blocked entirely)

R4 is blocked in middleware `Check()`. R1–R3 use a shared **preview-then-confirm** pattern implemented per handler (calling `CheckConfirmation` and requiring a `confirmed` field in schemas), so behavior stays consistent without claiming all logic lives in one middleware function.

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

### Implemented (Catalog)
- Products: search, CRUD, images, variants, options, modifiers, custom fields
- Categories: list, CRUD, reorder, move, SEO audit, metafields, product assignments

### Planned (Registered, Not Yet Implemented)
- Orders: management, fulfillment, refunds
- Customers: management, groups, addresses
- Carts: management, checkout
- Inventory: levels and adjustments
- Marketing: promotions, coupons
- Store: settings, shipping

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
