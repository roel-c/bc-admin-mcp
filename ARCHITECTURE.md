# BigCommerce MCP Server — Architecture & Design Decisions

This document captures the full architectural rationale, every design decision with alternatives considered, the current implementation state, known limitations, and a roadmap for extending the server's coverage across the BigCommerce platform.

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
│  │  orders/         │  │  R1: preview │  │  • tool name          │  │
│  │  customers/      │  │  R2: confirm │  │  • duration_ms        │  │
│  │  carts/          │  │  R3: per-ID  │  │  • success/error      │  │
│  │  inventory/      │  │  R4: block   │  │                       │  │
│  │  marketing/      │  └──────────────┘  └───────────────────────┘  │
│  │  store/          │                                               │
│  │                  │  ┌──────────────────────────────────────────┐  │
│  │  Tools:          │  │   Session Cache (TTL-based)              │  │
│  │  ~36 implemented │  │                                          │  │
│  │  ~50+ planned    │  │  Per-session, keyed by operation:        │  │
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

**Diagram note (tiers):** `TierEnforcer.Check()` only **rejects R4** at the meta-tool boundary. **R1–R3 preview / `confirmed: true`** enforcement lives in **tool handlers** (via `CheckConfirmation`) plus **registration-time** checks that R1+ schemas declare `confirmed`. The R0–R4 labels in the Tier column are shorthand for the policy model in `BC-Tool-Boundaries.md`, not a literal per-request branch inside `Check()`.

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

The `discover_tools(path)` meta-tool navigates a hierarchical category tree. Calling it with an empty path returns 7 root categories (~150 tokens). Drilling into `"catalog"` returns subcategories. Drilling into `"catalog/products"` reveals individual tools with <=150-character summaries.

The `execute_tool(tool_path, arguments)` meta-tool invokes any tool by its full path. The full tool schema (parameters, types, descriptions) is never sent to the LLM — it lives server-side and is resolved when the tool is executed.

**Token impact (verified estimates):**
- System prompt: ~600 tokens (2 meta-tool schemas)
- Per-discovery call: ~150-200 tokens per category explored
- A typical 5-tool-call session: ~1,800-2,800 total tokens
- Equivalent flat-tool session: ~95,000-110,000 tokens (35-40x reduction)

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
- Cache keys are operation-scoped (e.g., `product_update` for the unified update tool, `product_delete` for the delete tool)
- Entries are explicitly deleted after successful execution to prevent stale data reuse

**Known limitation**: The current implementation uses a hardcoded `"default"` session ID because the `execute_tool` meta-tool doesn't propagate the MCP session context into the inner tool call. This is noted in the [Known Limitations](#6-known-limitations--technical-debt) section.

---

### 3.7 Rate Limiting Strategy

**Chosen: Header-driven adaptive throttling with conservative defaults**

The BigCommerce API has per-store quotas that refresh every 30 seconds. Our client implements a layered approach from `BC-Tool-Boundaries.md`:

| Layer | Mechanism | Default |
|-------|-----------|---------|
| Global throttle | Token bucket via `time.Tick` | 2 req/s |
| Quota awareness | Parse `X-Rate-Limit-Requests-Left` header | Pause at ≤25 remaining |
| 429 handling | Wait `X-Rate-Limit-Time-Reset-Ms`, then retry | Up to 6 retries |
| 5xx handling | Exponential backoff (2^attempt seconds, max 60s) | Up to 6 retries |
| Batch pacing | Inter-chunk delay | 0.5s between batches |
| Write concurrency | Sequential by default | 1 concurrent write |

**Conservative vs Throughput mode**: The BigCommerce docs permit 3-5 parallel write threads for catalog batches. Our default is **sequential writes** (1 thread) per the policy in `BC-Tool-Boundaries.md`, which prioritizes live-store safety. Throughput mode can be enabled by setting `BC_MAX_WRITE_CONCURRENCY` to a higher value, but the current `BatchPut` implementation does not yet use this setting — it always writes sequentially. This is intentional for v0.1 safety.

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

| File | Lines | Purpose |
|------|-------|---------|
| `cmd/server/main.go` | 65 | Entry point: config load, server wire, transport start, auth middleware |
| `internal/config/config.go` | 158 | Environment-based config with comprehensive validation |
| `internal/server/server.go` | 82 | MCP server wiring, category registration, tool registration |
| `internal/discovery/registry.go` | 278 | Progressive disclosure: hierarchy, meta-tools, registration-time validation |
| `internal/middleware/tiers.go` | 77 | R0-R4 tier enforcement, `IsConfirmed` check, `CheckConfirmation` utility |
| `internal/middleware/logging.go` | 49 | Structured slog middleware wrapping all tool calls |
| `internal/middleware/auth.go` | 37 | Bearer token HTTP middleware with constant-time comparison |
| `internal/session/cache.go` | 165 | Per-session TTL cache with size limits and eviction |
| `internal/bigcommerce/client.go` | 342 | HTTP client: throttle, retry, rate-limit headers, GetAll (with ceiling), BatchPut |
| `internal/bigcommerce/types.go` | ~600 | Domain types: Product, ProductUpdate, ProductCreate, Category, CategoryCreate, CategoryUpdate, Variant types, Image/Option/Modifier types, Metafield, CategoryAssignment, CustomURL, API envelopes, `SafeError()` |
| `internal/bigcommerce/products.go` | ~300 | Domain methods: product/category search, batch product updates, product CRUD, variant ops, tree ID resolution |
| `internal/bigcommerce/metafields.go` | ~90 | Client methods for category metafield CRUD and category-assignment upsert/delete |
| `internal/tools/catalog/products.go` | ~540 | Product tool handlers: search (declarative filters), get, assign_categories; registration for search, get, assign_categories, create, update, delete |
| `internal/tools/catalog/product_resolve.go` | ~100 | FetchProductsForWrite: resolve products by IDs, exact SKU, or exact name |
| `internal/tools/catalog/categories.go` | ~1,300 | Category tool handlers: list (with list_all), get, create, bulk_update, move, reorder, products, seo_audit, metafields, delete, bulk_delete |
| `internal/tools/catalog/categories_seo_audit.go` | ~85 | SEO audit scan for missing page_title, meta_description, search_keywords |
| `internal/tools/catalog/categories_products.go` | ~160 | List products in a category with price/SKU summaries |
| `internal/tools/catalog/categories_move.go` | ~195 | Category reparenting with cycle detection and descendant counting |
| `internal/tools/catalog/categories_reorder.go` | ~160 | Reorder sibling categories with configurable start/increment |
| `internal/tools/catalog/categories_metafields.go` | ~280 | Metafield list, set (upsert), delete handlers |
| `internal/tools/catalog/categories_assignments.go` | ~85 | Additive category assignment via dedicated BC endpoint |
| `internal/tools/catalog/products_test.go` | 300 | Product tests: search filters, ExtractFilters, empty filter guard |
| `internal/tools/catalog/categories_test.go` | 406 | Category tests: search filters, create params, single/bulk delete params |
| `internal/tools/catalog/categories_seo_audit_test.go` | ~60 | SEO audit field detection tests |
| `internal/tools/catalog/categories_products_test.go` | ~70 | Category product listing param tests |
| `internal/tools/catalog/categories_move_test.go` | ~85 | Move/reparent param parsing tests |
| `internal/tools/catalog/categories_reorder_test.go` | ~70 | Reorder param parsing and validation tests |
| `internal/tools/catalog/categories_metafields_test.go` | ~140 | Metafield set/delete param parsing tests |
| `internal/tools/catalog/categories_assignments_test.go` | ~55 | Category assignment param tests |
| `internal/session/cache_test.go` | 142 | Cache TTL, eviction, size limits |
| `internal/middleware/auth_test.go` | 71 | Bearer auth middleware |
| `internal/middleware/tiers_test.go` | 109 | Tier enforcement and IsConfirmed |
| `internal/config/config_test.go` | 172 | Config validation |
| `internal/discovery/registry_test.go` | 183 | Registry confirmed param validation, tool discovery |
| `SECURITY.md` | — | Security review findings, threat model, and remediation details |
| `.gitignore` | — | Prevents `.env` and binaries from being committed |

### Implemented Tools

| Tool Path | Tier | Description |
|-----------|------|-------------|
| `catalog/products/search` | R0 | Declarative filter search (name, SKU, price range, category, brand, visibility, keyword), server-side pagination |
| `catalog/products/get` | R0 | Single product with variant pricing detection and `calculated_price` |
| `catalog/products/create` | R1 | Create a product with all writable fields, optional inline images, categories; preview→confirm |
| `catalog/products/update` | R1 | **Unified update**: any writable field on one or more products; target by product_ids, sku, product_name, or category_id; preview→confirm |
| `catalog/products/delete` | R3 | Permanently delete products; preview with warnings; requires confirmation |
| `catalog/products/assign_categories` | R1 | Additive product-to-category assignment via dedicated BC endpoint |
| `catalog/products/images/list` | R0 | List all images for a product |
| `catalog/products/images/add` | R1 | Add image by URL (JPEG, PNG, GIF, WebP); preview→confirm |
| `catalog/products/images/delete` | R2 | Delete a product image; preview→confirm |
| `catalog/products/options/list` | R0 | List variant-generating options with values |
| `catalog/products/options/create` | R1 | Create option with values; preview→confirm |
| `catalog/products/options/update` | R1 | Update option name, sort order, or values; preview→confirm |
| `catalog/products/options/delete` | R2 | Delete option (removes variants); preview→confirm |
| `catalog/products/variants/list` | R0 | List all variants with full details |
| `catalog/products/variants/create` | R1 | Create variant with option values; preview→confirm |
| `catalog/products/variants/update` | R1 | Update variant fields; preview→confirm |
| `catalog/products/variants/delete` | R2 | Delete variant; preview→confirm |
| `catalog/products/custom_fields/list` | R0 | List product custom fields |
| `catalog/products/custom_fields/set` | R1 | Upsert custom field by name; preview→confirm |
| `catalog/products/custom_fields/delete` | R2 | Delete custom field; preview→confirm |
| `catalog/products/modifiers/list` | R0 | List product modifiers |
| `catalog/products/modifiers/create` | R1 | Create modifier; preview→confirm |
| `catalog/products/modifiers/delete` | R2 | Delete modifier; preview→confirm |
| `catalog/categories/list` | R0 | Declarative filter search (name, keyword, parent_id, tree_id, visibility) with `list_all` mode |
| `catalog/categories/get` | R0 | Single category by ID |
| `catalog/categories/create` | R1 | Create categories with `parent_name` resolution (name→ID); handles `tree_id` inheritance for subcategories |
| `catalog/categories/bulk_update` | R1 | Preview→confirm batch update of category fields (name, description, SEO, visibility, sort order) |
| `catalog/categories/products` | R0 | List products in a category (by ID or name) with price/SKU/category summaries |
| `catalog/categories/seo_audit` | R0 | Scan categories for missing SEO fields (page_title, meta_description, search_keywords) |
| `catalog/categories/move` | R2 | Reparent a category with cycle detection, subtree preview, and descendant count |
| `catalog/categories/reorder` | R1 | Reorder sibling categories by providing them in desired display order |
| `catalog/categories/metafields/list` | R0 | List all metafields on a category |
| `catalog/categories/metafields/set` | R1 | Create or update a metafield (upsert by namespace+key) |
| `catalog/categories/metafields/delete` | R1 | Delete a metafield by ID or namespace+key |
| `catalog/categories/delete` | R3 | Single category deletion; child detection → `include_children` safety gate; products remain in store |
| `catalog/categories/bulk_delete` | R3 | Multi-category deletion; same child safeguard as single delete |

### Registered Category Hierarchy

```
catalog/                    — Product catalog: products, categories, brands, variants
  catalog/products/         — Product operations: search, get, create, update, delete, sub-resources
    catalog/products/images/         — Product image management: list, add by URL, delete
    catalog/products/options/        — Product option CRUD: list, create, update, delete
    catalog/products/variants/       — Product variant CRUD: list, create, update, delete
    catalog/products/custom_fields/  — Product custom field management: list, set, delete
    catalog/products/modifiers/      — Product modifier management: list, create, delete
  catalog/categories/       — Category operations: list, get, create, update, SEO, metafields
    catalog/categories/metafields/ — Category metafield CRUD: list, set, delete
  catalog/brands/           — Placeholder category (no tools registered yet)
  catalog/variants/         — Placeholder category (no tools registered yet)
orders/                     — Order management: list, get, update status, shipments, refunds
  orders/management/        — Core order operations: list, get, update status
  orders/fulfillment/       — Shipment creation, tracking, and management
  orders/refunds/           — Refund processing and history
customers/                  — Customer management: list, get, create, update, segments
  customers/management/     — Core customer operations
  customers/groups/         — Customer group management
  customers/addresses/      — Customer address management
carts/                      — Cart and checkout operations
  carts/management/         — Cart CRUD and item management
  carts/checkout/           — Checkout link creation and management
inventory/                  — Inventory levels and adjustments across locations
marketing/                  — Promotions, coupons, and marketing tools
  marketing/promotions/     — Promotion management
  marketing/coupons/        — Coupon code management
store/                      — Store settings, SEO, shipping configuration
  store/settings/           — Store-level configuration and SEO
  store/shipping/           — Shipping zones, methods, and carriers
```

---

## 5. Token Budget Analysis

### Example Scenario: "Increase all Men's Shoes prices by 5%"

| Phase | Tokens | BC API Calls | Wall Time |
|-------|--------|-------------|-----------|
| System prompt (2 meta-tools) | ~600 | 0 | — |
| discover_tools("") → root categories | ~150 | 0 | <100ms |
| discover_tools("catalog") → subcategories | ~100 | 0 | <100ms |
| discover_tools("catalog/categories") → tools | ~100 | 0 | <100ms |
| execute_tool("catalog/categories/list", {name: "Men's Shoes"}) | ~150 | 1 | ~200ms |
| execute_tool("catalog/products/update", {category_id: 42, price: 52.49, ...}) → preview | ~400 | 2-3 | ~400ms |
| LLM presents preview → user confirms | ~100 | 0 | (user time) |
| execute_tool("catalog/products/update", {..., confirmed: true}) | ~350 | 10-12 | ~2-4s |
| **Total** | **~1,950** | **~13-16** | **~3-5s** |

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

1. **Session ID propagation**: The `execute_tool` meta-tool constructs an inner `CallToolRequest` that does not carry the MCP session context. Tool handlers currently use a hardcoded `"default"` session ID for cache operations. This means multi-session deployments will share cache state. Fix: extract session ID from the `context.Context` using `server.ClientSessionFromContext(ctx)` and pass it to cache operations.

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

### Priority 1 — High-Value Merchant Operations

These cover the most common merchant requests based on BC ecosystem data:

| Domain | Tools to Add | BC API | Tier | Notes |
|--------|-------------|--------|------|-------|
| `orders/management/list` | Search orders by status, date, customer | GET /v2/orders | R0 | V2 endpoint — use `GetV2()` |
| `orders/management/get` | Full order details with line items | GET /v2/orders/{id} + /products | R0 | |
| `orders/management/update_status` | Change order status | PUT /v2/orders/{id} | R1 | |
| `orders/fulfillment/create_shipment` | Create shipment with tracking | POST /v2/orders/{id}/shipments | R1 | |
| `inventory/adjust` | Absolute or relative stock adjustments | POST /v3/inventory/adjustments | R2 | Batch ≤10, ≤5 concurrent |

### Priority 2 — Customer & Marketing

| Domain | Tools to Add | BC API | Tier |
|--------|-------------|--------|------|
| `customers/management/list` | Search customers | GET /v3/customers | R0 |
| `customers/management/get` | Full customer detail | GET /v3/customers/{id} | R0 |
| `customers/management/update` | Update customer fields | PUT /v3/customers | R1 |
| `marketing/coupons/list` | List coupons | GET /v2/coupons | R0 |
| `marketing/coupons/create` | Create a coupon | POST /v2/coupons | R1 |
| `marketing/promotions/list` | List promotions | GET /v3/promotions | R0 |
| `marketing/promotions/create` | Create a promotion | POST /v3/promotions | R1 |

### Priority 3 — Store Operations

| Domain | Tools to Add | BC API | Tier |
|--------|-------------|--------|------|
| `store/settings/get` | Store info | GET /v2/store | R0 |
| `store/settings/seo` | Read/update SEO settings | GET/PUT /v3/settings/SEO | R1 |
| `store/shipping/zones` | List shipping zones | GET /v2/shipping/zones | R0 |
| `carts/management/create` | Create a server-side cart | POST /v3/carts | R1 |
| `carts/checkout/create_link` | Generate checkout URL | POST /v3/carts/{id}/redirect_urls | R0 |

### Priority 4 — Advanced / Low Frequency

| Domain | Tools to Add | BC API | Tier | Notes |
|--------|-------------|--------|------|-------|
| `catalog/pricelists/upsert` | Bulk price list records | PUT /v3/pricelists/{id}/records | R2 | **Serial only** — no parallelism |
| `store/webhooks/create` | Register webhooks | POST /v3/hooks | R1 | Serial only |
| `catalog/products/delete` | Hard delete products | DELETE /v3/catalog/products | R3 | **Implemented** — prefer `is_visible: false` via update (R1) |
| `orders/refunds/create` | Issue refund | POST /v3/orders/{id}/payment_actions/refund | R3 | Per-order confirmation required |
| `orders/payments/capture` | Capture payment | POST /v3/orders/{id}/payment_actions/capture | R3 | Per-order confirmation required |

---

## 8. Adding a New Tool Domain

Follow this pattern to add tools for any new BigCommerce domain. Using "orders" as an example:

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

The category hierarchy is already registered (`orders/management/` exists in `registerCategories`), so the new tools will automatically appear when the LLM calls `discover_tools("orders/management")`.

---

## 9. Testing Strategy

Per workspace conventions: `testify/suite`, `gomock`, `_test` package suffix.

### Unit Tests (Priority 1)

| Package | What to Test |
|---------|-------------|
| `session` | Cache Set/Get/TTL expiration, Store per-session isolation, concurrent access |
| `middleware` | TierEnforcer blocks R4, allows R0-R3; IsConfirmed parsing |
| `discovery` | RegisterCategory/Tool hierarchy, Discover traversal, root entries |
| `bigcommerce` | `calculateNewPrice` math, URL construction, pagination parsing |
| `tools/catalog` | Preview response structure, confirmed=true flow, variant detection |

### Integration Tests (Priority 2)

Use `mark3labs/mcp-go`'s in-process transport to test full tool calls without HTTP:

```go
func (s *ProductsTestSuite) TestUnifiedUpdatePreview() {
    // Create server with mock BC client
    // Call execute_tool with update args (no confirmed)
    // Assert response contains preview with sample_changes
}
```

### Mock Strategy

Define a `BigCommerceAPI` interface in the tools layer; mock it with gomock. The concrete `bigcommerce.Client` satisfies this interface.

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

- [SECURITY.md](./SECURITY.md) — Security review findings, threat model, and remediation details
- [BC-API-Reference.md](./BC-API-Reference.md) — Full BigCommerce REST API endpoint map with batch sizes, concurrency limits, and pagination patterns
- [BC-API-SPECIFICITY.md](./BC-API-SPECIFICITY.md) — Field-level API quirks, undocumented behaviors, and response shape differences discovered during development
- [BC-Tool-Boundaries.md](./BC-Tool-Boundaries.md) — Tool tiers (R0-R4), numeric caps, safety rules, and MCP tool shape guidelines
- [bc_system_prompt.md](./bc_system_prompt.md) — Agent operating guidelines, workflow patterns, and safety constraints
- [MCP Specification](https://modelcontextprotocol.io/specification/latest) — Protocol reference
- [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) — SDK documentation
- [Progressive Disclosure MCP: 85x Token Savings](https://matthewkruczek.ai/blog/progressive-disclosure-mcp-servers.html) — Research on the lazy loading pattern
- [BigCommerce Developer Center](https://developer.bigcommerce.com/) — Official API documentation
