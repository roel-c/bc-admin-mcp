# BC Tool Boundaries — Read / Write / Risk Tiers & Caps

This document consolidates **tool design rules** for the MCP server and any agent-facing API layer. It merges:

- **`bc_system_prompt.md`** — operator policy (what the agent must do)
- **`BC-API-Reference.md`** — BigCommerce limits, concurrency, Section 9 LLM guidelines
- **MCP server implementation** (`internal/bigcommerce/client.go`) — implemented constants and behavior

For **field-level request/response shapes**, use `BC-API-Reference.md` and the official Management API docs.

---

## 1. Tool tiers (recommended)

Use these tiers when defining MCP tools (or HTTP actions) so permissions and confirmations stay explicit.

| Tier | Intent | Examples | Operator confirmation |
|------|--------|----------|------------------------|
| **R0 — Read** | Fetch only; no mutation | Store profile, list/get products, orders, customers, categories, inventory levels | None |
| **R1 — Write (standard)** | Idempotent-ish catalog/settings updates | Product SEO fields, category SEO, metafields (non-payment), redirects, `is_visible` toggles | **Preview + confirm** for bulk; single-record may be lighter-touch per policy |
| **R2 — Write (high-risk)** | Financial / inventory / pricing | Price list record upserts, inventory adjustments, cart/checkout server calls | **Always confirm** scope (list name, record count, before/after) |
| **R3 — Destructive** | Irreversible or legally sensitive | Product **DELETE**, order payment capture/refund/void, customer password/auth fields | **Explicit per-resource confirmation**; default deny |
| **R4 — Forbidden (default)** | Unless task explicitly says so | Hard-delete products, `description` HTML overwrite, payment status changes without order ID + approval | Block at tool layer |

**Principle:** R0 tools can be exposed broadly. R1–R2 should accept a **`confirmed: bool`** or separate **`propose_*`** vs **`apply_*`** tools. R3 should require **`confirmation_token`** or human-approved step.

---

## 2. Numeric caps (single source of truth)

### 2.1 Implemented in the MCP server (`internal/config/config.go`)

| Constant | Value | Meaning |
|----------|-------|---------|
| `DEFAULT_REQUESTS_PER_SECOND` | `2.0` | Global throttle between requests |
| `QUOTA_SAFETY_BUFFER` | `25` | If `X-Rate-Limit-Requests-Left` ≤ this, pause until reset |
| `MAX_RETRIES` | `6` | 429 / 5xx backoff rounds |
| `PRODUCT_BATCH_SIZE` | `10` | Max items per batch **PUT** `/v3/catalog/products` |
| `VARIANT_BATCH_SIZE` | `10` | Max items per batch **PUT** `/v3/catalog/variants` |
| `INVENTORY_BATCH_SIZE` | `10` | Safe batch size for inventory adjustments |
| `DEFAULT_PAGE_LIMIT` | `250` | Page size for most V3 list endpoints |

`batch_put` / `batch_post` also use **`delay_between_chunks`** default **`0.5`** s (on top of throttle).

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
| `/v3/pricelists/{id}/records` (upsert) | **1 — serial only** | Up to **1000** records per request (per reference) |
| `/v3/catalog/products` batch PUT | Recommend **≤ 3** parallel batch requests | **10** products per request |
| `/v3/catalog/variants` batch PUT | Recommend **≤ 3** parallel | **10** per request |
| `/v3/inventory/adjustments` | Recommend **≤ 5** parallel | **10** per request |
| General Management | **10–20** possible | Monitor 429s |
| Webhook registration | **Serial** | Single |

**Project policy (`bc_system_prompt.md`):** default to **sequential** writes (no extra threads) unless the operator explicitly opts into higher concurrency. That is **stricter** than the reference’s “3–5 threads” throughput pattern — intentional for live-store safety.

### 2.4 Operator “test mode” (prompt policy)

- First bulk run: **≤ 5 records** sample; scale after confirmation.

---

## 3. Read vs write: rules of engagement

| Rule | Source |
|------|--------|
| **GET before PUT/POST/DELETE** on the same logical resource | BC-API-Reference §9, `bc_system_prompt` |
| **Show diffs** (before/after for key fields) before bulk apply | `bc_system_prompt` |
| **Paginate exhaustively** before large bulk writes (know full ID set) | BC-API-Reference §9 |
| **Soft delete preferred:** `is_visible: false` vs DELETE | `bc_system_prompt`, §9 |
| **Never** bulk overwrite `description` unless explicitly requested | `bc_system_prompt` |
| **Never** payment capture/refund/void without per-order confirmation | `bc_system_prompt` |
| **Never** customer password/auth fields unless that is the task | `bc_system_prompt` |
| **Price lists:** confirm list **name** + **record count** before upsert | `bc_system_prompt` |
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
| Customers | `store_v2_customers` |
| Inventory | `store_inventory` |
| Price lists | `store_price_lists` |
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

**High-risk (R2):** `upsert_price_list_records` (**serial only**), `apply_inventory_adjustments` (batch ≤ 10), …

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

*Last aligned with: `bc_system_prompt.md`, `BC-API-Reference.md` §§3–5 & 9, MCP server implementation (`internal/config/config.go`, `internal/bigcommerce/client.go`).*
