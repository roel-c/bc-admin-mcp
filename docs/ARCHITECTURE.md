# BigCommerce MCP Server — Architecture & Design Decisions

A lightweight Go binary (`bc-mcp-server`) that lets AI agents manage a BigCommerce store through natural language. It implements the **Model Context Protocol (MCP)**, exposing exactly **two meta-tools** — `discover_tools` and `execute_tool` — instead of registering 200+ flat tool schemas. The agent navigates a category hierarchy on demand, keeping initial token cost ~600 tokens versus ~40k for a flat approach. Eight domains are always enabled: catalog, orders, customers, marketing, inventory, storefront/scripts, webhooks, and carts/checkout. A ninth domain — B2B Edition — registers only when `BC_B2B_ENABLED=true`. The binary is stateless except for an in-memory session cache; deployment is a single process with no database or queue.

This document captures the full architectural rationale, every design decision with alternatives considered, the current implementation state, known limitations, and a roadmap for extending the server's coverage.

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Architecture Overview](#2-architecture-overview)
3. [Design Decisions](#3-design-decisions)
   - 3.1 [Language & Runtime](#31-language--runtime)
   - 3.2 [SDK Choice](#32-sdk-choice)
   - 3.3 [Progressive Disclosure](#33-progressive-disclosure-pattern)
   - 3.4 [Tool Design Philosophy](#34-tool-design-philosophy)
   - 3.5 [Confirm-Before-Execute Pattern](#35-confirm-before-execute-pattern)
   - 3.6 [Session-Scoped Caching](#36-session-scoped-caching)
   - 3.7 [Rate Limiting Strategy](#37-rate-limiting-strategy)
   - 3.8 [Error Handling Model](#38-error-handling-model)
   - 3.9 [Transport Selection](#39-transport-selection)
   - 3.10 [Authentication Phases](#310-authentication-phases)
4. [Current Implementation](#4-current-implementation)
5. [Token Budget Analysis](#5-token-budget-analysis)
6. [Known Limitations & Technical Debt](#6-known-limitations--technical-debt)
7. [Expansion Roadmap](#7-expansion-roadmap)
8. [Adding a New Tool Domain](#8-adding-a-new-tool-domain)
9. [Testing Strategy](#9-testing-strategy)
10. [Observability & Production Readiness](#10-observability--production-readiness)
11. [Security](#11-security)

---

## 1. Problem Statement

BigCommerce merchants need AI-assisted store management — bulk pricing, SEO updates, inventory adjustments, order workflows — through conversational interfaces like Cursor, Claude Desktop, and VS Code Copilot. The Model Context Protocol (MCP) is the standard for connecting these AI hosts to external tool servers.

The naive approach of registering every BigCommerce API endpoint as an MCP tool fails for three reasons:

1. **Token bloat**: 80-100+ tool schemas consume ~40,000 tokens in the system prompt, often exceeding context window limits before any work begins.
2. **LLM accuracy degradation**: Research shows tool selection accuracy drops measurably past 20-25 visible tools. At 100+ tools, LLMs frequently select wrong tools or hallucinate parameters.
3. **Round-trip explosion**: Without server-side batching, a simple "update prices for 87 products" becomes 90+ sequential LLM turns, each adding latency and token cost.

This server solves all three through progressive disclosure, use-case-driven tool design, and server-side batch orchestration.

---

## 2. Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                      AI Host (Cursor / Claude / Copilot)           │
│                                                                     │
│   The LLM sees exactly 2 tools:                                    │
│   • discover_tools(path) — navigate the tool hierarchy             │
│   • execute_tool(tool_path, arguments) — invoke any tool           │
└────────────────────────┬────────────────────────────────────────────┘
                         │ JSON-RPC 2.0 (stdio / Streamable HTTP / SSE)
                         │
┌────────────────────────▼────────────────────────────────────────────┐
│                         MCP Server (Go)                             │
│                                                                     │
│  ┌─────────────────┐  ┌──────────────┐  ┌───────────────────────┐  │
│  │   Discovery      │  │  Tier        │  │   Logging             │  │
│  │   Registry       │  │  Enforcer    │  │   Middleware           │  │
│  │                  │  │  (R0-R4)     │  │   (slog/JSON)         │  │
│  │  Categories:     │  │              │  │                       │  │
│  │  catalog/        │  │  R0: pass    │  │  Every tool call:     │  │
│  │  customers/      │  │  R1: preview │  │  • tool name          │  │
│  │  marketing/      │  │  R2: confirm │  │  • duration_ms        │  │
│  │  (+ roadmap      │  │  R3: per-ID  │  │  • success/error      │  │
│  │   roots omitted) │  │  R4: block   │  │                       │  │
│  │                  │  │  R4: block   │  │                       │  │
│  │                  │  └──────────────┘  └───────────────────────┘  │
│  │                  │                                               │
│  │                  │  ┌──────────────────────────────────────────┐  │
│  │  Tools:          │  │   Session Cache (TTL-based)              │  │
│  │  Domain tool     │  │                                          │  │
│  │  leaves (reg.)   │  │  Per-session, keyed by operation:        │  │
│  └─────────────────┘  │  • product_update → [Product...]         │  │
│                        │  • 60s default TTL, evictable             │  │
│                        └──────────────────────────────────────────┘  │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                Tool Handlers (internal/tools/*)               │   │
│  │                                                               │   │
│  │  catalog/products:                                            │   │
│  │  • search — R0, server-side pagination, lightweight response  │   │
│  │  • get — R0, includes variant pricing detection               │   │
│  │  • update — R1, unified field update, preview→confirm         │   │
│  │  • create — R1, all writable fields, preview→confirm          │   │
│  │  • delete — R3, requires confirmation, irreversible           │   │
│  │  • product metafields — R0/R1, bulk up to 50 products; shared execution │   │
│  │                                                               │   │
│  │  catalog/categories:                                          │   │
│  │  • list — R0, declarative filters, list_all mode              │   │
│  │  • get — R0                                                   │   │
│  │  • create — R1, parent_name resolution, preview→confirm       │   │
│  │  • bulk_update — R1, preview→confirm, SEO + visibility fields │   │
│  │  • delete — R3, child safeguard + include_children gate       │   │
│  │  • bulk_delete — R3, child safeguard + include_children gate  │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                         │                                           │
└─────────────────────────┼───────────────────────────────────────────┘
                          │
┌─────────────────────────▼───────────────────────────────────────────┐
│                   BigCommerce API Client                            │
│                                                                     │
│  • 2 req/s global throttle (conservative default)                  │
│  • X-Rate-Limit-Requests-Left header parsing                      │
│  • Pause at ≤25 remaining requests                                 │
│  • Exponential backoff on 429 / 5xx                                │
│  • 0.5s inter-chunk delay for batch writes                         │
│  • Sequential writes by default (configurable)                     │
│  • Connection pooling (20 idle connections)                        │
│  • V2 and V3 URL routing                                           │
│                                                                     │
│  Batch operations: 10 products/PUT, 10 variants/PUT               │
│  Pagination: auto-follows offset pages at limit=250               │
└─────────────────────────┬───────────────────────────────────────────┘
                          │
                ┌─────────▼─────────┐
                │  BigCommerce REST  │
                │  Management API    │
                │  V2 + V3           │
                └────────────────────┘
```

**Diagram note (tiers):** `TierEnforcer.Check()` only **rejects R4** at the meta-tool boundary. **R1–R3 preview / `confirmed: true`** enforcement lives in **tool handlers** — most call `middleware.IsConfirmed(req)` (or the equivalent `TierEnforcer.CheckConfirmation`) directly and return a preview when the flag is missing — plus **registration-time** checks in `internal/discovery/registry.go` that every R1+ tool's input schema declares a `confirmed` boolean. The R0–R4 labels in the Tier column are shorthand for the policy model in [`DEVELOPMENT.md`](./DEVELOPMENT.md), not a literal per-request branch inside `Check()`.

---

## 3. Design Decisions

### 3.1 Language & Runtime

**Chosen: Go 1.26**

**Alternatives considered:**

| Language | Avg Latency | RPS | Memory | Verdict |
|----------|-------------|-----|--------|---------|
| Go | 0.86ms | 1,624 | 18MB | **Selected** — best throughput-to-memory ratio |
| Java (Spring Boot) | 0.84ms | 1,624 | 226MB | Marginally faster, 12x memory cost |
| Node.js (TypeScript) | 10.66ms | 559 | 110MB | Most existing BC MCP servers use this |
| Python (FastMCP) | 26.45ms | 292 | 98MB | Slowest, but richest MCP ecosystem |
| Rust | Sub-1ms | 4,845 | 11MB | Best raw performance, immature MCP SDK |

**Rationale**: Go matches Java's throughput at 92% less memory (18MB vs 226MB), making it ideal for cloud-native deployment. The existing codebase conventions (gomock, testify/suite, golangci-lint) align with Go. Rust was tempting for raw performance but its MCP SDK ecosystem is less mature.

---

### 3.2 SDK Choice

**Chosen: `mark3labs/mcp-go` v0.47.1**

**Alternatives considered:**

| SDK | Stars | Transports | Maturity |
|-----|-------|------------|----------|
| `mark3labs/mcp-go` | 8,504 | stdio, HTTP, SSE, in-process | 72 releases, 180 contributors |
| `modelcontextprotocol/go-sdk` | 4,251 | stdio, command | Official (Anthropic + Google) |

**Rationale**: `mark3labs/mcp-go` was selected for three reasons:

1. **Transport breadth**: Built-in Streamable HTTP and SSE support are essential for Phase 2 (remote deployment). The official SDK only supports stdio and command transports, which would require building HTTP transport from scratch.
2. **Community maturity**: More examples, more contributors, more battle-tested edge cases handled.
3. **Ergonomic API**: `mcp.NewTool()` with chained builders (`mcp.WithString()`, `mcp.WithNumber()`) is more idiomatic than the official SDK's struct-tag approach for complex tool schemas.

**Trade-off acknowledged**: The official SDK has Anthropic/Google backing and will likely become the long-term standard. If the official SDK adds HTTP transport support, migration should be evaluated. The tool handler signatures (`func(ctx, CallToolRequest) (*CallToolResult, error)`) are compatible between SDKs, so migration would primarily affect server initialization and transport setup — not tool implementations.

---

### 3.3 Progressive Disclosure Pattern

**Chosen: Two meta-tool architecture (`discover_tools` + `execute_tool`)**

**Alternatives considered:**

| Approach | Initial Tokens | Accuracy | Complexity |
|----------|---------------|----------|------------|
| All tools registered upfront | ~40,000+ | Degrades past ~25 tools | Low |
| Namespace gateway tools (4-5 composite tools) | ~3,000 | Good for <50 tools | Medium |
| **Two meta-tools with hierarchy** | **~600** | **Highest (verified by Anthropic)** | **High** |

**How it works:**

The `discover_tools(path)` meta-tool navigates a hierarchical category tree. Calling it with an empty path returns the always-on roots (**`catalog`**, **`orders`**, **`customers`**, **`marketing`**, **`inventory`**, **`storefront`**, **`webhooks`**, **`carts`**), plus **`b2b`** when B2B Edition is enabled. Planned domains (e.g. `store/`) remain in the expansion roadmap and are **not** registered until tools exist (avoids empty `discover_tools` leaves). Drilling into a root (for example `"catalog"`) returns subcategories; drilling into e.g. `"catalog/products"` reveals tools and deeper categories.

The `execute_tool(tool_path, arguments)` meta-tool invokes any tool by its full path. The full tool schema (parameters, types, descriptions) is never sent to the LLM — it lives server-side and is resolved when the tool is executed.

**Token impact (verified estimates):**
- System prompt: ~600 tokens (2 meta-tool schemas)
- Per-discovery call: ~50–200 tokens per category explored (enforced ≤150 chars per summary stub by `TestFullRegistrationCategorySummaryLength`; compact JSON output further reduces whitespace overhead)
- A typical 5-tool-call session: ~1,800–2,800 total tokens
- Equivalent flat-tool session: ~95,000–110,000 tokens (35–40x reduction)

**Accuracy impact**: Anthropic's benchmarks show Opus 4 accuracy improving from 49% to 74% with lazy loading; Opus 4.5 from 79.5% to 88.1%. Fewer tools in view means better tool selection.

**Implementation**: `internal/discovery/registry.go` — The `Registry` struct holds both `categories` (navigation nodes) and `tools` (leaf nodes with handlers). Categories are registered in `internal/server/server.go`; tools self-register via their domain package's `RegisterTools(reg)` method.

---

### 3.4 Tool Design Philosophy

**Chosen: Use-case-driven tools, not 1:1 API endpoint mirrors**

**Example — Bulk Price Update:**

A naive mirror approach would require the LLM to:
1. Call `list_categories` to find the category
2. Call `list_products` with pagination (multiple calls)
3. For each product, calculate new price
4. Call `update_product` 87 times
5. Check for variant-level pricing
6. Call `list_variants` for flagged products
7. Call `update_variant` for each variant

That is **90+ LLM turns** with calculation logic delegated to the LLM.

Our `catalog/products/update` tool handles steps 2-7 **server-side** in a single tool invocation:
- Accepts flexible product selection: by `category_id` (with optional `limit`), explicit `product_ids` array, or single product by `sku`/`product_name`
- Fetches products (server-side pagination for category mode, individual fetches for ID mode)
- Accepts any writable field — the LLM passes only the fields to change
- Batches updates in groups of 10 with rate-limit-aware pacing
- Returns a structured preview with before/after diffs, then executes on confirmation

**The LLM's job reduces to**: understand intent → find products → trigger update → report result. That's 4-5 turns, each one simple and deterministic.

---

### 3.5 Confirm-Before-Execute Pattern

**Chosen: Two-phase tool execution (preview → confirm)**

All R1+ tools (write operations) implement a preview-then-confirm flow:

**Phase 1 — Preview** (no `confirmed` argument or `confirmed=false`):
- Fetches affected data
- Calculates proposed changes
- Caches data in the session cache for the apply phase
- Returns a structured preview with sample changes, total impact, and a message prompting confirmation

**Phase 2 — Execute** (`confirmed=true`):
- Reuses cached data (zero redundant API calls)
- Executes the batch operation
- Clears the cache entry
- Returns a structured result with success/failure counts

**Why not MCP Elicitation?**

The MCP spec includes a native `elicitation/create` mechanism for server-initiated user prompts. We chose the `confirmed` argument pattern instead because:

1. **Broader client support**: Not all MCP clients support elicitation yet. The `confirmed` flag works universally.
2. **LLM-mediated confirmation**: The LLM can present the preview to the user, add context ("this will cost approximately $547 more across your catalog"), and then relay confirmation. Elicitation bypasses LLM judgment.
3. **Composability**: The preview response is a standard tool result that the LLM can reason about, compare with other data, or present alongside recommendations.

**Future**: When elicitation support matures across clients, R2 and R3 tier tools may migrate to native elicitation for stronger safety guarantees.

---

### 3.6 Session-Scoped Caching

**Chosen: Per-session TTL cache (`internal/session/cache.go`)**

**Problem solved**: In the bulk price update flow, Phase 1 (preview) fetches all products in a category. Phase 2 (execute) needs the same product list. Without caching, Phase 2 re-fetches the same data — wasting API quota and adding latency.

**Design**:
- `session.Store` manages per-session `Cache` instances, keyed by MCP session ID
- Each `Cache` is a `sync.RWMutex`-protected `map[string]entry` with TTL expiration
- Default TTL: 60 seconds (configurable via `BC_CACHE_TTL_SECONDS`)
- Cache keys are operation-scoped (e.g., `product_delete` for the delete tool). For `catalog/products/update` the key additionally embeds a **SHA-256 fingerprint of targeting + fields + channel_ids** (`UpdateParams.cacheKey`) so a confirm call whose arguments differ from any cached preview misses the cache and falls back to a fresh fetch — preventing a confirm shaped like preview A from applying its field changes to preview B's products
- Entries are explicitly deleted after successful execution to prevent stale data reuse

**Standard pattern — `CacheOrFetch[T]`**: The canonical way to implement preview→confirm caching in any tool handler is the generic helper `session.CacheOrFetch`:

```go
// Preview and confirm handlers call the exact same code — CacheOrFetch
// fetches-and-stores on first call, returns from cache on second call.
data, err := session.CacheOrFetch(p.cache.ForContext(ctx), key, func() ([]MyType, error) {
    return p.bc.FetchData(ctx, params)
})
```

`session.ForContext(ctx)` extracts the MCP session ID via `session.SessionIDFromContext(ctx)` (which calls `mcpserver.ClientSessionFromContext`), falling back to `"default"` for stdio/single-session deployments. This keeps session-ID logic in one place rather than duplicated per domain.

**Known limitation**: Whether `ClientSessionFromContext` returns a real per-session ID depends on whether the `execute_tool` meta-tool's context propagation carries the MCP session through. In multi-session HTTP/SSE deployments this should work; in stdio deployments, the `"default"` fallback keeps all operations in the same session bucket, which is correct for single-user use. See [Known Limitations §6](#6-known-limitations--technical-debt) for the remaining multi-session concern.

---

### 3.7 Rate Limiting Strategy

**Chosen: Header-driven adaptive throttling with conservative defaults**

The BigCommerce API has per-store quotas that refresh every 30 seconds. Our client implements a layered approach from `DEVELOPMENT.md`:

| Layer | Mechanism | Default |
|-------|-----------|---------|
| Global throttle | Token bucket via `time.Tick` | 2 req/s |
| Quota awareness | Parse `X-Rate-Limit-Requests-Left` header | Pause at ≤25 remaining |
| 429 handling | Wait `X-Rate-Limit-Time-Reset-Ms`, then retry | Up to 6 retries |
| 5xx handling | Exponential backoff (2^attempt seconds, max 60s) | Up to 6 retries |
| Batch pacing | Inter-chunk delay | 0.5s between batches |
| Write concurrency | Sequential by default | 1 concurrent write |

**Conservative vs Throughput mode**: The BigCommerce docs permit 3-5 parallel write threads for catalog batches. Our default is **sequential writes** (1 thread) per the policy in `DEVELOPMENT.md`, which prioritizes live-store safety. Throughput mode can be enabled by setting `BC_MAX_WRITE_CONCURRENCY` to a higher value, but the current `BatchPut` implementation does not yet use this setting — it always writes sequentially. This is intentional for v0.1 safety.

---

### 3.8 Error Handling Model

**Chosen: Three-tier model following MCP best practices**

| Level | Scope | Implementation |
|-------|-------|---------------|
| Transport | Network failures, broken connections | Handled by mcp-go SDK transport layer |
| Protocol | Malformed JSON-RPC, invalid methods | JSON-RPC standard error codes (-32700, -32600, etc.) |
| Application | BigCommerce API errors, validation failures | Tool result with `IsError: true` and human-readable message |

**Critical distinction**: Application errors (e.g., "product 42 not found", "category has no products") are returned as **successful tool results** with `IsError: true`, not as protocol-level errors. This allows the LLM to read the error message, understand what went wrong, and self-correct (e.g., try a different category name).

BigCommerce-specific error mapping:
- 400/422 → Validation error details from response body
- 404 → Descriptive "resource not found" with the ID that was queried
- 429 → Handled automatically by client retry logic; never surfaces to LLM
- 500/503 → Retried with backoff; surfaces to LLM only after max retries exhausted

---

### 3.9 Transport Selection

**Chosen: stdio as default, with Streamable HTTP and SSE available**

| Transport | Use Case | Status |
|-----------|----------|--------|
| stdio | Local integration (Cursor, Claude Desktop) | **Default, fully implemented** |
| Streamable HTTP | Remote/shared server, team access | Implemented, untested in production |
| SSE | Legacy client compatibility | Implemented, untested in production |

Selection via `MCP_TRANSPORT` environment variable. The entry point (`cmd/server/main.go`) switches on this value.

---

### 3.10 Authentication Phases

**Phased approach:**

| Phase | Mechanism | Status | Use Case |
|-------|-----------|--------|----------|
| Phase 1 | BigCommerce credentials via env vars | **Implemented** | Local dev |
| Phase 2 | Bearer token for MCP server access | **Implemented** | Team use via HTTP/SSE |
| Phase 3 | OAuth 2.1 + PKCE | Not yet implemented | Public deployment |

The auth middleware layer (`internal/middleware/`) is designed to be pluggable:

- **Phase 1** (implemented): `BC_STORE_HASH` and `BC_AUTH_TOKEN` environment variables authenticate requests to the BigCommerce API.
- **Phase 2** (implemented): `internal/middleware/auth.go` provides `BearerAuth()` HTTP middleware that validates an `Authorization: Bearer <token>` header using constant-time comparison (`crypto/subtle.ConstantTimeCompare`). Config validation in `internal/config/config.go` **requires** `MCP_AUTH_TOKEN` for HTTP and SSE transports — the server will not start without it. Stdio transport is exempt since it is inherently process-local.
- **Phase 3**: OAuth token validation would replace or augment the bearer token middleware for public multi-tenant deployment.

---

## 4. Current Implementation

### Files and Their Roles

> Line counts are **approximate** — refresh with `wc -l <path>` after structural
> changes. They are listed here to give a sense of relative complexity, not as
> a contract.

| File | Lines | Purpose |
|------|-------|---------|
| `cmd/server/main.go` | ~65 | Entry point: config load, server wire, transport start, auth middleware |
| `internal/config/config.go` | ~160 | Environment-based config with comprehensive validation |
| `internal/server/server.go` | ~280 | MCP server wiring, category registration (all domains incl. full B2B subtree), tool registration |
| `internal/discovery/registry.go` | ~310 | Progressive disclosure: hierarchy, meta-tools, registration-time validation |
| `internal/middleware/tiers.go` | ~80 | R0-R4 tier enforcement, `IsConfirmed` check, `CheckConfirmation` utility |
| `internal/middleware/logging.go` | ~50 | Structured slog middleware wrapping all tool calls |
| `internal/middleware/auth.go` | ~40 | Bearer token HTTP middleware with constant-time comparison |
| `internal/session/cache.go` | ~230 | Per-session TTL cache with size limits and eviction. Exports `SessionIDFromContext` (MCP session ID from context, `"default"` fallback), `Store.ForContext` (session cache for the current request context), and `CacheOrFetch[T]` (canonical preview→confirm cache helper — checks cache, calls fetch on miss, stores result) |
| `internal/bigcommerce/client.go` | ~370 | HTTP client: throttle, retry, rate-limit headers, GetAll (with ceiling), BatchPut |
| `internal/bigcommerce/types.go` | ~1,820 | Domain types: Product, ProductUpdate, ProductCreate, Category/Tree types, Brand types, Variant types, Image/Option/Modifier types, Metafield, CategoryAssignment, ChannelAssignment, ChannelListing, CustomURL, API envelopes, `APIError` with `SafeError()` (core BC + B2B error-envelope parsing) and OAuth-scope hints |
| `internal/bigcommerce/products.go` | ~375 | Domain methods: product/category search, batch product updates, product CRUD, tree CRUD, tree ID resolution; `categoryBatchSize = 50` for `BatchUpdateCategories` |
| `internal/bigcommerce/channels.go` | ~95 | `ListStoreChannels`, `GetStoreChannel`, `UpdateStoreChannel` — GET/PUT /v3/channels (Management API); `StoreChannelUpdate` type |
| `internal/bigcommerce/webhooks.go` | ~130 | `ListWebhooks`, `GetWebhook`, `GetWebhookEvents`, `CreateWebhook`, `UpdateWebhook`, `DeleteWebhook` — full CRUD for GET/POST/PUT/DELETE /v3/hooks; `Webhook`, `WebhookEvent`, `WebhookCreate`, `WebhookUpdate` types |
| `internal/bigcommerce/carts.go` | ~330 | Cart client: create/get/update/delete, item add/update/remove, redirect URLs, cart metafields; `Cart`, `CartCreate`, `CartUpdate`, line-item types |
| `internal/bigcommerce/checkouts.go` | ~220 | Checkout client: get, coupon apply/remove, billing address set/update, consignment add/update, convert-to-order; `Checkout`, `CheckoutAddressInput`, consignment types |
| `internal/bigcommerce/b2b_client.go` | ~370 | `B2BClient` for api-b2b.bigcommerce.com (unified `X-Auth-Token` + `X-Store-Hash`); throttle, retry, offset pagination; `b2bUnmarshalSingle`/`b2bUnmarshalList` envelope helpers; `B2BPostMultipart` for file uploads |
| `internal/bigcommerce/b2b_companies.go` | ~610 | B2B company/user/address/attachment types and CRUD methods, extra-field configs, catalog assignment (Phase B1) |
| `internal/bigcommerce/b2b_roles.go` | ~200 | Company roles + custom permissions CRUD; `flexString` (tolerant string/number JSON unmarshaler for `roleType`/`permissionLevel`) |
| `internal/bigcommerce/b2b_hierarchy.go` | ~90 | Account hierarchy: subsidiaries list, full hierarchy tree, attach/detach parent |
| `internal/bigcommerce/b2b_channels.go` | ~50 | Storefront channels as seen by B2B Edition: list, get |
| `internal/bigcommerce/b2b_orders.go` | ~90 | B2B order metadata: get/update PO+extra fields, assign/reassign historical orders to a company |
| `internal/bigcommerce/b2b_quotes.go` | ~215 | Quote lifecycle CRUD, checkout, assign-to-order, PDF export, shipping rates/select/remove/custom-methods |
| `internal/bigcommerce/b2b_invoices.go` | ~245 | Invoices + receipts + receipt-lines; served from the distinct `/ip` base URL; invoice create/create-from-order/update/delete, receipt(-line) delete |
| `internal/bigcommerce/b2b_payments.go` | ~170 | Store-wide + per-company payment methods, company credit, company payment terms (reads + updates) |
| `internal/bigcommerce/b2b_payment_records.go` | ~165 | Payment records logged against invoices: reads, offline create/update, lifecycle operations, processing status, delete; also `/ip` base |
| `internal/bigcommerce/b2b_sales_staff.go` | ~60 | Sales Staff company-assignment list/get/update |
| `internal/bigcommerce/b2b_super_admins.go` | ~190 | Super Admin CRUD, bulk create, company assignments (both super-admin- and company-perspective) |
| `internal/bigcommerce/b2b_shopping_lists.go` | ~140 | Shopping list CRUD + item removal |
| `internal/bigcommerce/category_trees.go` | ~65 | `ListCategoryTrees`, `GetTreeIDForChannel` (`GET /v3/catalog/trees`) |
| `internal/bigcommerce/channel_assignments.go` | ~100 | `ListProductChannelAssignments`, `UpsertProductChannelAssignments`, `DeleteProductChannelAssignments` |
| `internal/bigcommerce/channel_listings.go` | ~120 | `ListChannelListings`, `CreateChannelListings`, `UpdateChannelListings` |
| `internal/bigcommerce/metafields.go` | ~315 | Client methods for category, brand, product, and variant metafield CRUD (REST paths per resource) plus product↔category assignment helpers |
| `internal/bigcommerce/variants_catalog.go` | ~80 | `SearchVariants`, `ListVariantsByProductIDs`, `BatchUpdateVariants` |
| `internal/bigcommerce/brands.go` | ~90 | REST client for GET/POST/PUT `catalog/brands` |
| `internal/bigcommerce/product_options.go` | ~70 | Client methods for product options |
| `internal/bigcommerce/product_variants.go` | ~70 | Client methods for product-scoped variant CRUD |
| `internal/bigcommerce/product_modifiers.go` | ~55 | Client methods for product modifiers |
| `internal/bigcommerce/product_images.go` | ~55 | Client methods for product images (JSON URL upload) |
| `internal/bigcommerce/product_custom_fields.go` | ~70 | Client methods for product custom fields |
| `internal/tools/catalog/products.go` | ~605 | Product tool registration + handlers: search (declarative filters), get, assign_categories, channel_summary, channel_assignments, create, update, delete |
| `internal/tools/catalog/products_create.go` | ~360 | Product create handler with optional inline images and additive `channel_ids` post-create assignment |
| `internal/tools/catalog/products_update.go` | ~910 | Unified product update handler: targeting, field parsing (`fieldExtractor` for type-safe arg extraction), preview/confirm, SHA-256 fingerprinted cache key (`UpdateParams.cacheKey`), additive `channel_ids` post-update assignment |
| `internal/tools/catalog/products_delete.go` | ~140 | Hard-delete handler with R3 confirmation and per-resource preview |
| `internal/tools/catalog/products_channel_assignments.go` | ~280 | MSF: list / assign / remove `catalog/products/channel_assignments/*` (GET/PUT/DELETE channel-assignments) with caps |
| `internal/tools/catalog/products_channel_summary.go` | ~245 | MSF aggregator: joins `/v3/channels`, channel-assignments, and per-channel listings (max 5 products / 25 channels) |
| `internal/tools/catalog/products_metafields.go` | ~340 | Product metafield tools; set/delete use shared core + app_only / preserve-permission-on-update options |
| `internal/tools/catalog/products_metafields_bulk.go` | ~400 | Bulk product metafield set/delete (≤ 50 products); reuses `metafieldUpsertExecute` / `metafieldResolveIDByNamespaceKey` / `productMetafieldOps` |
| `internal/tools/catalog/products_variants.go` | ~350 | Product-scoped variant CRUD handlers |
| `internal/tools/catalog/products_variants_metafields.go` | ~410 | Variant metafield tools; bulk execution shares `executeVariantMetafieldUpsert` → `metafieldUpsertExecute` + `variantMetafieldOps` |
| `internal/tools/catalog/products_variants_metafields_bulk.go` | ~910 | Variant bulk metafields (single product and cross-product) with caps |
| `internal/tools/catalog/products_options.go` | ~300 | Product options handlers (list/create/update/delete) |
| `internal/tools/catalog/products_modifiers.go` | ~230 | Product modifier handlers |
| `internal/tools/catalog/products_images.go` | ~210 | Product image handlers (list/add by URL/delete) |
| `internal/tools/catalog/products_custom_fields.go` | ~225 | Product custom field handlers |
| `internal/tools/catalog/product_resolve.go` | ~150 | `FetchProductsForWrite`: resolve products by IDs, exact SKU, or exact name |
| `internal/tools/catalog/categories.go` | ~1,285 | Category tool handlers: list (with `list_all` and optional `channel_id`), get, create (with `parent_name` resolution and MSF `channel_id` / `tree_id`), bulk_update, delete, bulk_delete |
| `internal/tools/catalog/categories_seo_audit.go` | ~85 | SEO audit scan for missing page_title, meta_description, search_keywords |
| `internal/tools/catalog/categories_products.go` | ~160 | List products in a category with price/SKU summaries |
| `internal/tools/catalog/categories_move.go` | ~225 | Category reparenting with cycle detection and descendant counting |
| `internal/tools/catalog/categories_reorder.go` | ~195 | Reorder sibling categories with configurable start/increment |
| `internal/tools/catalog/categories_metafields.go` | ~265 | Metafield param parsers + handlers (delegate list/set/delete to `metafield_shared`) |
| `internal/tools/catalog/categories_assignments.go` | ~180 | Additive `assign_categories` and filter-based `unassign_categories` (R2) with caps (`product_ids ≤ 100`, `category_ids ≤ 50`, pairs ≤ 500) |
| `internal/tools/catalog/brands.go` | ~495 | Brand list/get/create/update (preview→confirm on writes) |
| `internal/tools/catalog/brands_metafields.go` | ~325 | Brand metafield list, set (upsert), delete (shared `metafield_*` core) |
| `internal/tools/catalog/variants_global.go` | ~285 | Global variant list + batch update MCP handlers (`catalog/variants/list`, `bulk_update`) |
| `internal/tools/catalog/channel_tools.go` | ~290 | `catalog/channels/list`, `catalog/channels/get`, `catalog/channels/update` (R2 preview→confirm), `catalog/channels/category_trees`; delegates listing tools; `validChannelStatuses` |
| `internal/tools/webhooks/webhook_tools.go` | ~310 | `webhooks/list|get|events` (R0), `webhooks/create|update` (R1 preview→confirm), `webhooks/delete` (R3); `parseHeadersJSON` helper; HTTPS destination validation |
| `internal/tools/webhooks/interfaces.go` | ~25 | `WebhooksAPI` consumer-side interface + compile-time check |
| `internal/tools/carts/cart_tools.go` | ~510 | `carts/cart/*` handlers: create, get, update, delete, item add/update/remove, checkout_url |
| `internal/tools/carts/cart_metafields_tools.go` | ~210 | `carts/cart/metafields/*` handlers (list/set/delete) |
| `internal/tools/carts/checkout_tools.go` | ~525 | `carts/checkout/*` handlers: get, coupon apply/remove, billing address, consignment add/update, convert |
| `internal/tools/carts/interfaces.go` | ~40 | `CartAPI` consumer-side interface (cart + checkout methods) + compile-time check |
| `internal/tools/b2b/company_tools.go` | ~1,370 | `b2b/companies/**` handlers: company/user/address/attachment CRUD + status, extra fields, catalog assignment; cascades to linked BC customers on company delete |
| `internal/tools/b2b/role_tools.go` | ~400 | `b2b/companies/roles/**` and `b2b/companies/permissions/**` handlers |
| `internal/tools/b2b/hierarchy_tools.go` | ~165 | `b2b/companies/hierarchy/**` handlers |
| `internal/tools/b2b/channel_order_tools.go` | ~245 | `b2b/channels/**` and `b2b/orders/**` handlers |
| `internal/tools/b2b/quote_tools.go` | ~495 | `b2b/quotes/**` handlers including the `shipping/*` sub-tree |
| `internal/tools/b2b/invoice_tools.go` | ~580 | `b2b/invoices/**` and `b2b/receipts/**` handlers; `create_from_order` resolves the BC order ID to B2B Edition's internal order ID via `GetB2BOrder` before calling the invoice endpoint |
| `internal/tools/b2b/payment_tools.go` | ~310 | `b2b/payments/**` and `b2b/companies/payments\|credit\|payment_terms/**` handlers |
| `internal/tools/b2b/payment_record_tools.go` | ~380 | `b2b/payment_records/**` handlers |
| `internal/tools/b2b/sales_staff_tools.go` | ~140 | `b2b/sales_staff/**` handlers |
| `internal/tools/b2b/super_admin_tools.go` | ~490 | `b2b/super_admins/**` and `b2b/companies/super_admins/**` handlers |
| `internal/tools/b2b/shopping_list_tools.go` | ~345 | `b2b/shopping_lists/**` handlers |
| `internal/tools/b2b/interfaces.go` | ~150 | `B2BCompanyAPI` consumer-side interface (all Phase A/B methods) + compile-time check |
| `internal/tools/shared/` | ~40 | Shared `ToolError` / `ToolJSON` response builders used across newer domains |
| `internal/tools/catalog/channel_listings_tools.go` | ~370 | `catalog/channels/listings/list`, `create`, `update` (GET/POST/PUT listings) |
| `internal/tools/catalog/pricelists_tools.go` | ~1,080 | `catalog/pricelists/*`, `catalog/pricelists/records/*`, `catalog/pricelists/assignments/*` handlers (preview→confirm for R1+) |
| `internal/tools/catalog/metafield_shared.go` | ~370 | Shared catalog metafields: `MetafieldResourceOps`, list/upsert/delete MCP helpers, `metafieldUpsertExecute` (single execution path for confirmed tool + bulk upserts), `metafieldResolveIDByNamespaceKey`, product/variant/category/brand op factories |
| `internal/tools/catalog/metafield_shared.go`/`metafield_permissions.go` | ~40 | Shared metafield permission-set defaults and validation |
| `internal/tools/catalog/list_filter_helpers.go` | ~45 | Shared list/search helpers: `list_all`, BC filter vs data-param detection |
| `internal/tools/catalog/variant_update_parse.go` | ~85 | Shared variant field parsing from argument maps (single + bulk variant updates) |
| `internal/tools/catalog/helpers.go` | ~75 | Shared parsing helpers (positive/non-negative int slice, string slice) and cache-key constants. `cacheSessionID` now delegates to `session.SessionIDFromContext` — the canonical function lives in the session package so all domains use the same logic |
| `internal/tools/catalog/interfaces.go` | ~120 | `BigCommerceAPI` interface (mocked via gomock for tests) |
| `internal/tools/catalog/mock_bc_test.go` | ~1,060 | gomock-generated mock for `BigCommerceAPI` (test-only) |
| Test suites (`internal/tools/catalog/*_test.go`) | ~7,300 total | Per-tool testify suites covering search filters, parameter parsing, preview/confirm flows, caps, metafield CRUD, MSF surfaces, type-rejection, etc. |
| `internal/session/cache_test.go` | ~200 | Cache TTL, eviction, size limits; `CacheOrFetch` hit/miss/error/no-cache-on-error; `ForContext` and `SessionIDFromContext` fallback |
| `internal/middleware/auth_test.go` | ~70 | Bearer auth middleware |
| `internal/middleware/tiers_test.go` | ~110 | Tier enforcement and IsConfirmed |
| `internal/config/config_test.go` | ~170 | Config validation |
| `internal/discovery/registry_test.go` | ~185 | Registry confirmed-param validation, tool discovery |
| `internal/discovery/metatool_test.go` | ~235 | `discover_tools` / `execute_tool` meta-tool flows |
| `internal/server/registration_audit_test.go` | ~645 | Locks discovery shape: eight always-on roots (`catalog`, `orders`, `customers`, `marketing`, `inventory`, `storefront`, `webhooks`, `carts`); every active category has children; every tool's parent path exists; R1+ tools expose `confirmed`; BFS reachability; pricelist, orders, inventory, **carts/checkout**, storefront/webhooks subtrees; **b2b/ gating** (hidden when disabled, full subtree when enabled); and `TestFullRegistration{Category,Tool}SummaryLength` enforce ≤150 chars on every summary to prevent discovery token bloat |
| `docs/SECURITY.md` | — | Security review findings, threat model, and remediation details |
| `.gitignore` | — | Prevents `.env` and binaries from being committed |

### Catalog code reuse (current build)

- **Metafields:** Category, brand, product, and variant MCP metafield tools share `internal/tools/catalog/metafield_shared.go` (`MetafieldResourceOps`, preview/confirm wrappers, list JSON helpers). **Confirmed upserts and bulk upserts** both go through **`metafieldUpsertExecute`** so create/update semantics (defaults, empty `permission_set` on update for product/variant) stay aligned. Bulk deletes resolve ids via **`metafieldResolveIDByNamespaceKey`** and call **`Delete` on the same ops** as single-resource deletes.
- **List / search:** `list_filter_helpers.go` centralizes `list_all`, “data filter vs sort-only” BC query params, and invalid-sort errors for product search, category/brand lists, and global variant list.
- **Variant field maps:** `variant_update_parse.go` maps tool argument maps into `ProductVariantUpdate` for single-variant and bulk variant updates.

### Implemented Tools

> **Canonical, current list:** [README.md](../README.md#implemented-tools) and
> [`docs/AGENT.md`](./AGENT.md#implemented-tools) — this section intentionally
> does **not** duplicate the full per-tool table (a full duplicate here drifted
> out of sync with reality more than once; see git history). Below is a
> domain-level summary of what exists today, for architectural orientation.

| Domain | Representative tools | Full detail |
|--------|----------------------|--------------|
| `catalog/**` | products (CRUD, images, options, variants, modifiers, custom fields, metafields incl. bulk), categories, brands, global variants, channels + listings, price lists | README.md, this doc §4 file table |
| `orders/**` | management CRUD, fulfillment shipments, payment actions/transactions/capture/void, refunds | README.md |
| `customers/**` | records, groups, addresses, attributes + values, metafields, settings, consent, stored instruments, credential validation, segments, shopper profiles | README.md |
| `marketing/**` | automatic + coupon promotions, coupon codes, promotion settings | README.md |
| `inventory/**` | locations (+ metafields + per-location items/settings), items, absolute/relative adjustments (`qty_backordered`, `backorder_limit`) | README.md |
| `storefront/**` | Script Manager scripts (list/get/create/update/toggle/delete) | README.md |
| `webhooks/**` | registrations (list/get/events/create/update/delete) | README.md |
| `carts/**` | cart CRUD + items + metafields; checkout (coupons, billing address, consignments, convert to order) | README.md, `docs/DEVELOPMENT.md` |
| `b2b/**` (gated) | companies/users/addresses/attachments/roles/permissions/hierarchy, channels, orders, quotes (+shipping), invoices/receipts/payment records, payments/credit/payment-terms, sales staff, super admins, shopping lists | `docs/B2B.md`, README.md |

Each domain's tools follow the same tier + preview/confirm conventions described in [§3.5](#35-confirm-before-execute-pattern) and [DEVELOPMENT.md](./DEVELOPMENT.md).

### Registered Category Hierarchy

**Discovery (`discover_tools`)** registers eight always-on roots: **`catalog/**`**, **`orders/**`**, **`customers/**`**, **`marketing/**`**, **`inventory/**`**, **`storefront/**`**, **`webhooks/**`**, and **`carts/**`**. A ninth root — **`b2b/**`** — registers only when `BC_B2B_ENABLED=true`. Domains such as `store/` remain in the [Expansion Roadmap](#7-expansion-roadmap) and are **not** category nodes until tools ship (registration policy in [§8](#8-adding-a-new-tool-domain)).

```
catalog/                    — Product catalog: products, categories, brands, variants, price lists
  catalog/products/         — Product operations: search, get, create, update, delete, sub-resources
    catalog/products/channel_assignments/ — MSF catalog: list, assign, remove product↔channel
    catalog/products/images/         — Product image management: list, add by URL, delete
    catalog/products/options/        — Product option CRUD: list, create, update, delete
    catalog/products/variants/       — Product variant CRUD: list, create, update, delete
    catalog/products/variants/metafields/ — Variant metafield CRUD: list, set, delete; bulk by product or by product list + scope
    catalog/products/custom_fields/  — Product custom field management: list, set, delete
    catalog/products/modifiers/      — Product modifier management: list, create, delete
    catalog/products/metafields/     — Product metafield CRUD: list, set, delete, bulk_set, bulk_delete
  catalog/categories/       — Category operations: list, get, create, update, SEO, metafields
    catalog/categories/metafields/ — Category metafield CRUD: list, set, delete
  catalog/brands/           — Brand list, get, create, update, delete (V3 catalog/brands)
    catalog/brands/image/     — Brand image: set by URL (via update), delete (/brands/{id}/image)
    catalog/brands/metafields/ — Brand metafield list, set (upsert), delete
  catalog/variants/         — Global variant list (GET) and batch update (PUT); product CRUD under catalog/products/variants
  catalog/channels/         — Management GET/PUT /v3/channels (storefront IDs, MSF awareness, name/status updates)
    catalog/channels/listings/ — Channel product listings: list, create (POST), update (PUT)
  catalog/pricelists/       — Price list CRUD
    catalog/pricelists/records/ — Price record list/upsert/delete for one price list
    catalog/pricelists/assignments/ — Assignment list/create_batch/upsert/delete
customers/                  — Customer-domain operations: records, groups, attributes, settings, consent, segmentation
  customers/groups/         — V2 customer groups CRUD
  customers/addresses/      — Customer addresses CRUD
  customers/attributes/     — Customer attribute definitions CRUD
  customers/attribute_values/ — Customer attribute value list/upsert/delete
  customers/metafields/     — Customer metafields single + bulk
  customers/settings/       — Global/channel customer settings
  customers/consent/        — Per-customer consent
  customers/stored_instruments/ — Stored payment instruments listing
  customers/credentials/    — Credential validation
  customers/segments/       — Segments CRUD + segment shopper membership
    customers/segments/shoppers/ — Shopper-profile membership actions
  customers/shopper_profiles/ — Shopper profiles CRUD + segment lookup
marketing/                  — Marketing-domain operations
  marketing/promotions/     — Promotions engine
    marketing/promotions/automatic/ — Automatic promotions
    marketing/promotions/coupon/    — Coupon promotions
      marketing/promotions/coupon/codes/ — Coupon code lifecycle
    marketing/promotions/settings/  — Store-wide promotion settings
inventory/                  — Locations, per-location item settings (backorder_limit), items, adjustments
  inventory/locations/      — Location lifecycle + metafields
    inventory/locations/metafields/ — Location metafield list/set/delete
    inventory/locations/items/      — Per-location inventory list + settings update (backorder_limit)
  inventory/items/          — Cross-location inventory list/get/update_batch
  inventory/adjustments/    — Absolute/relative stock + qty_backordered adjustments
storefront/                 — Storefront operations
  storefront/scripts/       — Script Manager script injection/management
webhooks/                   — Webhook registration management (/v3/hooks)
carts/                      — Server-side cart + checkout lifecycle via /v3/carts and /v3/checkouts
  carts/cart/               — Cart CRUD: create, get, update, delete; checkout URL generation
    carts/cart/items/       — Cart item management: add, update, remove
    carts/cart/metafields/  — Cart metafield CRUD: list, set, delete
  carts/checkout/           — Checkout: get, coupon apply/remove, billing address, consignments, convert to order
b2b/                        — (Gated by BC_B2B_ENABLED) B2B Edition via api-b2b.bigcommerce.com
  b2b/companies/            — Company account CRUD + status lifecycle, extra fields, catalog assignment
    b2b/companies/users/    — Buyer portal user CRUD (admin/senior/junior roles)
    b2b/companies/addresses/ — Company billing/shipping address CRUD
    b2b/companies/attachments/ — Company file attachments: list, add, delete
    b2b/companies/roles/    — Custom company roles: list/get/create/update/delete
    b2b/companies/permissions/ — Custom company permissions: list/create/update/delete
    b2b/companies/hierarchy/ — Account hierarchy: get/subsidiaries/attach_parent/detach_subsidiary
    b2b/companies/payments/ — Per-company payment method availability: list/update
    b2b/companies/credit/   — Per-company credit settings: get/update
    b2b/companies/payment_terms/ — Per-company net-terms settings: get/update
    b2b/companies/super_admins/ — Company-perspective Super Admin assignments: list/update
  b2b/channels/             — Storefront channels as seen by B2B Edition: list, get
  b2b/orders/               — B2B order metadata: get/update, assign/reassign to companies, extra fields
  b2b/quotes/               — Sales quote lifecycle: list/get/create/update/delete/checkout/assign_to_order/pdf_export/extra_fields
    b2b/quotes/shipping/    — Quote shipping rates: list/select/remove/custom_methods
  b2b/invoices/             — Invoices (distinct /ip base URL): list/get/download_pdf/extra_fields/create/create_from_order/update/delete
  b2b/receipts/             — Payment receipts (same /ip base): list/get/delete
    b2b/receipts/lines/     — Receipt line items: list_all/list_for_receipt/get/delete
  b2b/payment_records/      — Payments logged against invoices (same /ip base): reads + offline create/update/operations/processing_status/delete
  b2b/payments/             — Store-wide payment method definitions + cross-company active methods (read-only)
  b2b/sales_staff/          — Backend sales rep company assignment: list/get/update_assignments
  b2b/super_admins/         — Frontend sales rep / masquerade accounts: CRUD + company assignments
  b2b/shopping_lists/       — Repeat-purchase lists: list/get/create/update/delete/items/remove
```

---

## 5. Token Budget Analysis

### Example Scenario: "Increase all Men's Shoes prices by 5%"

| Phase | Tokens | BC API Calls | Wall Time |
|-------|--------|-------------|-----------|
| System prompt (2 meta-tools) | ~600 | 0 | — |
| discover_tools("") → root categories | ~120 | 0 | <100ms |
| discover_tools("catalog") → subcategories | ~80 | 0 | <100ms |
| discover_tools("catalog/categories") → tools | ~80 | 0 | <100ms |
| execute_tool("catalog/categories/list", {name: "Men's Shoes"}) | ~130 | 1 | ~200ms |
| execute_tool("catalog/products/update", {category_id: 42, price: 52.49, ...}) → preview | ~350 | 2-3 | ~400ms |
| LLM presents preview → user confirms | ~100 | 0 | (user time) |
| execute_tool("catalog/products/update", {..., confirmed: true}) | ~300 | 10-12 | ~2-4s |
| **Total** | **~1,760** | **~13-16** | **~3-5s** |

> Discovery and execute responses now use compact JSON (`json.Marshal` instead of `json.MarshalIndent`), reducing per-call whitespace overhead by ~15–25%. Category summaries are enforced ≤150 chars by the registration audit tests.

### Comparison: Naive Architecture (same scenario)

| Phase | Tokens | BC API Calls |
|-------|--------|-------------|
| System prompt (80-100 tool schemas) | ~40,000 | 0 |
| LLM fetches product list | ~3,500 | 1-2 |
| LLM calls update_product 87 times | ~35,000-45,000 | 87 |
| LLM reasoning across 87+ turns | ~15,000-20,000 | 0 |
| **Total** | **~95,000-110,000** | **~90** |

**Reduction: ~50x fewer tokens, ~6x fewer API calls.**

---

## 6. Known Limitations & Technical Debt

> **Security review**: A comprehensive line-by-line security audit has been completed.
> All critical and high severity findings have been remediated. See **[SECURITY.md](./SECURITY.md)** for the
> full findings report, threat model, and remaining recommendations.

### Must Fix Before Production

1. **Session ID propagation**: `session.SessionIDFromContext(ctx)` (via `session.Store.ForContext`) extracts the MCP session ID from the tool handler's `context.Context` using `mcpserver.ClientSessionFromContext`. In single-user stdio deployments this falls back to `"default"`, which is correct. In multi-session HTTP/SSE deployments the real session ID is returned — **provided** the `execute_tool` meta-tool's context carries the MCP session through to the inner handler call. Whether this propagation holds depends on the `mark3labs/mcp-go` version; verify with an integration test before relying on session isolation in shared HTTP deployments.

2. **Concurrent batch writes**: The `BatchPut` method is sequential-only. The `MaxWriteConcurrency` config value is accepted but not used. For large catalogs (500+ products), this means batch updates take 25+ seconds when they could take 5-6s with controlled parallelism. This is intentionally conservative for v0.1 but should be configurable.

3. **~~No test coverage~~ — RESOLVED**: Extensive testify-based suites exist across products, categories (including metafields, move, reorder, SEO, assignments), cache, auth middleware, tier helpers, config validation, registry, and meta-tools. Run `go test ./...` for the current graph. Security-critical paths (type assertions, price floor clamping, auth middleware rejection, cache eviction, config validation, confirmed-param registration) are covered. Remaining: integration tests using mcp-go's in-process transport for full tool-call flows.

### Should Fix Soon

4. **Variant price detection per-product**: The `previewBulkPriceUpdate` method fetches variants for every product to check for variant-level pricing. For a category with 200 products, that's 200 additional API calls. Optimization: use `GET /v3/catalog/variants?product_id:in=1,2,3,...` to batch-fetch variants.

5. **Automatic cache eviction**: The cache now enforces size limits and evicts on write, but `Evict()` is not called on a background timer. A goroutine running every 60s would clean expired entries proactively. (See [SECURITY.md — S12](./SECURITY.md) for details.)

6. **No MCP Resources**: The architecture design included MCP Resources with URI templates (e.g., `bigcommerce://products/{id}`) for lightweight data passing between tools. This is not yet implemented. Currently, tool responses include full data in the text content.

7. **MCP endpoint rate limiting**: The BigCommerce API client is rate-limited, but the MCP HTTP/SSE endpoints themselves accept unlimited inbound requests. Consider adding request-per-second limits on the HTTP handler.

### Acceptable for Now

8. **Hardcoded base URL**: `https://api.bigcommerce.com/stores` is correct for all current BigCommerce environments. Sandbox stores use the same base URL with a different store hash.

9. **No GraphQL support**: The BC API reference notes that GraphQL Admin API is expanding and may replace some multi-REST-call patterns. This is deferred until specific use cases require it.

10. **`Client.Close()` lifecycle**: The client uses `time.NewTicker` for throttling; `Close()` stops the ticker. Long-lived deployments should ensure shutdown paths call `Close()` when client instances are retired (today the process exits with the server, so impact is low).

---

## 7. Expansion Roadmap

### Adding a new domain

Follow **[`WORKFLOW.md`](./WORKFLOW.md)** — the research → implement → build/test/lint gate → reload → live-validate-with-cleanup → docs → commit → CI cadence used to build every domain shipped so far (catalog, orders, customers, marketing, inventory, storefront, webhooks, carts/checkout, and B2B). §8 below covers the code-level registration mechanics.

Multi-storefront / channel work: see **[`MSF.md`](./MSF.md)** for API inventory, MSF detection heuristics, shipped tools, and open follow-ups.

### Current Status

This section used to carry a full per-domain "planned tools" table. Nearly
every row had shipped, so it had become a fourth restatement of the tool
inventory rather than an actual roadmap. For what's implemented, see the
**Implemented Tools** table in [`README.md`](../README.md) (or call
`discover_tools` live); for tracked bugs/technical debt, see
[`FOLLOW-UPS.md`](./FOLLOW-UPS.md).

**Genuinely not yet implemented** — the only domain from the original
roadmap that hasn't shipped:

| Domain | Would add | BC API | Tier |
|--------|-------------|--------|------|
| `store/settings/get` | Store info | `GET /v2/store` | R0 |
| `store/settings/seo` | Read/update SEO settings | `GET/PUT /v3/settings/SEO` | R1 |
| `store/shipping/zones` | List shipping zones | `GET /v2/shipping/zones` | R0 |

Everything else originally scoped here — orders management/fulfillment/payments/refunds, customers, marketing/promotions, inventory, carts/checkout, price lists, webhooks, and B2B — has shipped. Remaining follow-on work tends to be cross-domain orchestration (e.g. joining customers/promotions with inventory or order operations) rather than a new domain; raise those as needed rather than pre-planning them here.

---

## 8. Adding a New Tool Domain

Follow this pattern to add tools for any new BigCommerce domain. Using "orders" as an example:

### Registration Policy

Every registered category must satisfy these invariants — enforced by `internal/server/registration_audit_test.go`:

| Rule | Detail |
|------|--------|
| **No empty nodes** | Every category must return ≥1 child. Register `RegisterCategory` in the same commit as the first tool under it. |
| **Parent chain** | Every tool path must have each parent segment registered (e.g. `catalog/products/metafields/set` requires `catalog`, `catalog/products`, `catalog/products/metafields`). |
| **Summary ≤ 150 chars** | Category and tool `Summary` strings appear verbatim in LLM responses. Summaries describe what tools exist — not how to use them, not OAuth scopes, not API paths. Guidance belongs in `docs/` or tool descriptions. |
| **R1+ tools declare `confirmed`** | Registration panics at startup if an R1+ tool lacks a `confirmed` boolean in its schema. |
| **Future domains stay out** | `carts/`, `store/` remain unregistered until the first tool ships. Placeholder categories with no tools produce empty `discover_tools` leaves and confuse agents. |

Run after any registration change:

```bash
go test ./internal/server/... -count=1 -run 'TestFullRegistration'
```

---

### Step 1: Add BC client methods

Create `internal/bigcommerce/orders.go`:

```go
package bigcommerce

func (c *Client) ListOrders(ctx context.Context, params map[string]string) ([]Order, error) {
    // Use c.GetV2() since orders are V2
    // Handle pagination server-side
}

func (c *Client) GetOrder(ctx context.Context, orderID int) (*Order, error) {
    // GET /v2/orders/{id}
}
```

### Step 2: Create tool handlers

Create `internal/tools/orders/management.go`:

```go
package orders

type Management struct {
    bc    *bigcommerce.Client
    cache *session.Store
}

func NewManagement(bc *bigcommerce.Client, cache *session.Store) *Management {
    return &Management{bc: bc, cache: cache}
}

func (m *Management) RegisterTools(reg *discovery.Registry) {
    reg.RegisterTool(&discovery.ToolDef{
        Path:    "orders/management/list",
        Tier:    middleware.TierR0,
        Summary: "Search orders by status, date range, customer, or payment method",
        // ... tool definition and handler
    })
}
```

### Step 3: Wire into server

In `internal/server/server.go`, add to `registerTools()`:

```go
func registerTools(reg *discovery.Registry, bc *bigcommerce.Client, cache *session.Store) {
    // existing...
    products := catalog.NewProducts(bc, cache)
    products.RegisterTools(reg)

    // new
    orderMgmt := orders.NewManagement(bc, cache)
    orderMgmt.RegisterTools(reg)
}
```

Register the category path (and any parents) in `registerCategories` **before** or **with** the new tools so `discover_tools` stays non-empty at every node. Keep every category `Summary` string ≤ 150 characters — `TestFullRegistrationCategorySummaryLength` will fail the build otherwise. Summaries describe *what tools exist*, not how to use them; implementation guidance belongs in `docs/` or individual tool descriptions.

### Step 4: Use `CacheOrFetch` for preview→confirm caching

For any R1+ tool that fetches data in the preview phase and needs it again in the confirm phase, use the canonical helper to avoid redundant BC API calls:

```go
import "github.com/roel-c/bc-admin-mcp/internal/session"

// Both preview and confirm handlers call the same function.
// First call: fetches from BC, stores in session cache.
// Second call (confirmed=true): returns from cache, zero extra round-trip.
data, err := session.CacheOrFetch(h.cache.ForContext(ctx), cacheKey, func() ([]MyType, error) {
    return h.bc.FetchData(ctx, params)
})
if err != nil {
    return toolError("%s", err), nil
}

// On successful confirm, clear the cache entry.
h.cache.ForContext(ctx).Delete(cacheKey)
```

---

## 9. Testing Strategy

Per workspace conventions: `testify/suite`, `gomock`, `_test` package suffix. No table-driven tests; use `SetupTest()` for setup.

### Running Tests

```bash
# Full suite
go test ./... -count=1

# Registration audit only (fast; run after any tool/category change)
go test ./internal/server/... -count=1 -run 'TestFullRegistration'

# Live smoke tests (requires .env with real credentials)
make smoke          # all domains
make smoke-msf      # MSF/channel slice
```

### Unit Tests (implemented)

| Package | Coverage |
|---------|----------|
| `session` | Cache TTL/eviction/size limits, `CacheOrFetch` hit/miss/error, `ForContext`/`SessionIDFromContext` fallback, per-session isolation, concurrent access |
| `middleware` | TierEnforcer R4 block; `IsConfirmed` parsing; bearer auth rejection |
| `discovery` | Registry hierarchy, meta-tool forwarding, `arguments` unwrapping, confirmed-param validation |
| `config` | Env validation bounds |
| `bigcommerce` | Types, orders, inventory, pricelists, promotions |
| `tools/catalog` | Search filters, preview/confirm flows, caps, metafield CRUD, MSF surfaces |
| `tools/orders`, `customers`, `promotions`, `inventory`, `webhooks`, `storefront`, `carts`, `b2b` | Handler parsing, preview/confirm flows (carts covers cart + checkout; b2b covers company/user/address) |
| `server` (audit) | No empty discovery leaves; all tool parents exist; BFS reachability; R1+ tools expose `confirmed`; carts/checkout + storefront/webhooks subtrees; b2b/ gating; category/tool summary ≤ 150 chars |
| `server` (wire protocol) | `discover_tools`/`execute_tool` driven through the real `mcp-go` in-process client (not just the registry API) — root set, drill-down, tier exposure, and error paths. See "Integration Tests" below |
| `server` (docs sync) | Diffs registered tool paths against README.md's Implemented Tools table so an undocumented shipped tool fails CI instead of going unnoticed |

### Manual Drill — Discovery

Use your MCP host's "call tool" UI or JSON-RPC:

1. `discover_tools` with `path: ""` → expect the eight always-on roots (plus `b2b` when `BC_B2B_ENABLED=true`).
2. `discover_tools` with `path: "catalog"` → subcategories.
3. Drill to a leaf (e.g. `catalog/products/channel_assignments`) until you see tool stubs with `tier` fields.

### Manual Drill — Preview then Confirm

Pick any R1 tool (e.g. `catalog/categories/bulk_update`):

1. **First call:** full payload, `"confirmed": false` (or omit `confirmed`). Expect `status: "pending_confirmation"` with no store mutation.
2. **Second call:** identical `arguments` plus `"confirmed": true` to execute.

`execute_tool` shape reminder — all tool parameters go inside `arguments`, never beside `tool_path`:

```json
{
  "tool_path": "catalog/products/metafields/set",
  "arguments": { "product_id": 123, "namespace": "example", "key": "flag", "value": "1" }
}
```

### Full Surface Check (on-demand, MCP-only)

For a broader, real-data pass beyond the two drills above — creating sample
records across every domain (D2C variant) or additionally exercising B2B
company/hierarchy/catalog-restriction/payment scenarios **plus** quote →
checkout → order → invoice → offline payment (B2B variant) — see
[**`WORKFLOW.md` §10**](./WORKFLOW.md#10-full-surface-check-d2c--b2b). It's
written as a step-by-step runbook (not a script) since it involves
preview→confirm judgment calls, an explicit keep-or-delete decision point,
and — on multi-storefront stores — a required upfront channel-selection step;
run it after a batch of domain changes or before a demo.

### Integration Tests

`internal/server/mcp_wire_protocol_test.go` closes what was previously a gap:
in-process `mcp-go` transport tests that drive `discover_tools`/`execute_tool`
through the SDK's actual client (`client.NewInProcessClient`), not just the
`discovery.Registry` API directly the way the registration audit does. This
catches wiring/serialization bugs the registry-level and mocked handler-level
tests can't see — e.g. it would have caught the root-set drift found manually
during the 2026-07-16 documentation audit if it had existed then. Kept
hermetic (no BigCommerce credentials, no network) by only exercising
`discover_tools` and `execute_tool` paths that either short-circuit before
reaching a handler (unknown `tool_path`, missing `tool_path`) or trip
client-side validation that runs before any HTTP call (e.g. the
`assign_categories` pairs cap). Run in isolation with:

```bash
go test ./internal/server/... -run 'TestMCPWireProtocol' -v -count=1
```

`internal/server/docs_sync_test.go` is a related but distinct check: it
diffs every registered tool path against README.md's Implemented Tools
table and fails if any tool shipped in code never got a doc row — the exact
drift class (`catalog/products/bulk_sku_update`,
`catalog/products/custom_fields/create`) found by hand in the same audit.

The gomock unit suites still cover handler logic in isolation; the manual
Full Surface Check above remains the only thing that validates real
BigCommerce responses.

### Mock Strategy

Define a `BigCommerceAPI` interface per domain package; mock with gomock. The concrete `bigcommerce.Client` satisfies every domain interface. See `internal/tools/catalog/interfaces.go` as the reference pattern.

---

## 10. Observability & Production Readiness

### Current State

- **Structured logging**: All tool calls logged via `slog` middleware with JSON output, tool name, and duration_ms
- **Panic recovery**: `server.WithRecovery()` enabled on the MCP server
- **Rate limit logging**: Client logs when pausing for quota or receiving 429s

### Planned Additions

| Capability | Implementation | Priority |
|-----------|---------------|----------|
| OpenTelemetry tracing | Instrument `Client.Do()` and tool handlers with spans | High |
| Token size estimation | Log approximate response token count per tool call | Medium |
| Prometheus metrics | Expose tool call counts, latency histograms, error rates | Medium |
| Health check endpoint | `/health` on HTTP transport returning server status + BC API connectivity | Medium |
| Audit logging | Log all R1+ mutations with before/after state and correlation ID | High |

### Deployment Considerations

| Concern | Approach |
|---------|----------|
| Binary size | ~11MB compiled, statically linked |
| Container | Single-stage `FROM scratch` Docker image possible |
| Secrets | Environment variables only; never in binary or config files |
| Multi-store | Run separate instances per store (separate env vars) or extend config to support multi-tenant routing |

---

## 11. Security

A comprehensive line-by-line security audit was performed across all source files. The full findings report is in **[SECURITY.md](./SECURITY.md)** and covers:

- **Threat model** mapping attack vectors to mitigations
- **10 findings** (3 critical, 3 high, 3 medium) — all remediated
- **Remaining recommendations** for further hardening

### Key Security Controls Implemented

| Control | Location | Description |
|---------|----------|-------------|
| Bearer token auth | `internal/middleware/auth.go` | Constant-time token validation for HTTP/SSE transports; required by config |
| Safe type assertions | `internal/tools/catalog/*.go` | All LLM-provided arguments use two-value assertions with graceful error returns |
| Response body limit | `internal/bigcommerce/client.go` | 50 MB cap via `io.LimitReader` prevents OOM from upstream |
| Pagination ceiling | `internal/bigcommerce/client.go` | `MaxTotalRecords` (default 10k) prevents unbounded memory on large catalogs |
| Price validation | `internal/tools/catalog/products_update.go` | Unified update validates all field types; non-negative price enforcement via BigCommerce API |
| Registration-time validation | `internal/discovery/registry.go` | R1+ tools must declare a `confirmed` parameter or the server panics at startup |
| Config bounds checking | `internal/config/config.go` | `RequestsPerSecond`, `MaxRetries`, `DefaultPageLimit`, etc. validated at load |
| Cache size limits | `internal/session/cache.go` | Max 1,000 entries per session, max 100 sessions, with LRU-like eviction |
| Error truncation | `internal/bigcommerce/types.go` | API response bodies truncated; `SafeError()` for external callers |
| Secret exclusion | `.gitignore` | `.env` files and binaries excluded from version control |

### Security-Sensitive Config

| Variable | Sensitivity | Notes |
|----------|-------------|-------|
| `BC_AUTH_TOKEN` | **High** | BigCommerce API credential — never log or expose |
| `MCP_AUTH_TOKEN` | **High** | MCP server access secret — required for HTTP/SSE |
| `BC_STORE_HASH` | Medium | Store identifier — not a secret but identifies the target store |

---

## References

- [WORKFLOW.md](./WORKFLOW.md) — Implementation workflow for adding new endpoints/domains
- [SECURITY.md](./SECURITY.md) — Security review findings, threat model, and remediation details
- [DEVELOPMENT.md](./DEVELOPMENT.md) — Tool tiers (R0–R4), numeric caps, concurrency policy, OAuth scopes, and channel assignment model
- [AGENT.md](./AGENT.md) — Agent system prompt: tool tables, workflow, safety rules, and response format
- [MSF.md](./MSF.md) — Multi-storefront research, shipped tools, and open follow-ups
- [B2B.md](./B2B.md) — B2B Edition API research, unified auth, and phased implementation plan
- [BC-API-Reference.md](./BC-API-Reference.md) — Full BigCommerce REST API endpoint map with batch sizes, concurrency limits, and pagination patterns
- [BC-API-SPECIFICITY.md](./BC-API-SPECIFICITY.md) — Field-level API quirks, undocumented behaviors, and response shape differences
- [FOLLOW-UPS.md](./FOLLOW-UPS.md) — Tracked technical debt and deferred fixes from architecture/live-test audits
- [MCP Specification](https://modelcontextprotocol.io/specification/latest) — Protocol reference
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) — SDK documentation
- [Progressive Disclosure MCP: 85x Token Savings](https://matthewkruczek.ai/blog/progressive-disclosure-mcp-servers.html) — Research on the lazy loading pattern
- [BigCommerce Developer Center](https://developer.bigcommerce.com/) — Official API documentation
