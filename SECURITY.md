# Security Review — BigCommerce MCP Server

**Review date:** 2026-04-13
**Scope:** All source files in the repository at the time of review
**Reviewed by:** Automated security audit + manual code review

---

## Executive Summary

A line-by-line security review was performed on the BigCommerce MCP server
codebase at the time of review (April 2026). **Nine** primary findings (S1–S9)
were identified across critical/high/medium severity; additional follow-ups
(S10–S12) were tracked at lower severity or as recommendations. All S1–S9 findings below were remediated in code at the time of the audit; S10–S12
are lower severity, documented-only, or post-fix hygiene. This document records
each item, its root cause, risk, and the fix or disposition.

---

## Findings

### CRITICAL Severity

#### S1 — Unsafe Type Assertions Cause Server Panics

| | |
|---|---|
| **File(s)** | `internal/tools/catalog/products.go`, `internal/tools/catalog/categories.go` |
| **Lines** | Multiple handler functions |
| **Risk** | Denial of service: a malformed LLM argument (e.g. string where float64 expected) crashes the server |
| **Root Cause** | Go type assertions like `v.(float64)` panic when the underlying type does not match |

**Before:**
```go
pid := int(pidRaw.(float64))  // panics if pidRaw is a string
```

**After:**
```go
pidFloat, fOk := pidRaw.(float64)
if !fOk {
    return toolError("product_id must be a number"), nil
}
pid := int(pidFloat)
```

**Fix applied:** All 9 type assertions across both tool handler files now use
the two-value form with graceful error return. The cached product slice
assertion also uses the safe form with fallback to re-fetching.

---

#### S2 — Unbounded Response Body Read (OOM Risk)

| | |
|---|---|
| **File** | `internal/bigcommerce/client.go` |
| **Line** | `Do()` method, `io.ReadAll(resp.Body)` |
| **Risk** | Memory exhaustion: a malicious or malformed upstream response could allocate unbounded memory |
| **Root Cause** | `io.ReadAll` reads until EOF with no size limit |

**Fix applied:** Wrapped the body reader with `io.LimitReader(resp.Body, 50MB)`.
The 50 MB limit is generous enough for the largest legitimate BigCommerce API
responses while preventing unbounded allocation.

---

#### S3 — HTTP/SSE Transports Have Zero Authentication

| | |
|---|---|
| **File** | `cmd/server/main.go` |
| **Risk** | Unauthenticated access: anyone who can reach the server's address:port can call any tool, including write/destructive operations |
| **Root Cause** | The mcp-go SDK's `Start()` methods don't include auth middleware |

**Fix applied:**
1. Added `MCP_AUTH_TOKEN` configuration (in `internal/config/config.go`)
2. Config validation now **requires** `MCP_AUTH_TOKEN` for HTTP and SSE transports
3. New `internal/middleware/auth.go` implements Bearer token authentication with
   constant-time comparison (`crypto/subtle.ConstantTimeCompare`) to prevent
   timing side-channels
4. `main.go` wraps the HTTP/SSE handlers with the auth middleware before serving
5. Stdio transport (inherently process-local) is exempt

---

### HIGH Severity

#### S4 — Negative Price Calculation

| | |
|---|---|
| **File** | `internal/tools/catalog/products.go` |
| **Function** | `calculateNewPrice()`, `handleBulkPriceUpdate()` |
| **Risk** | Data integrity: a large negative adjustment sets product prices below zero, causing catalog corruption |
| **Root Cause** | No floor check on computed price; no input bounds on adjustment value |

**Fix applied:**
1. `calculateNewPrice()` now clamps results to a minimum of `0.00`
2. Input validation rejects percentage adjustments below `-100%` and above `+1000%`

---

#### S5 — No Pagination Ceiling (Unbounded Memory)

| | |
|---|---|
| **File** | `internal/bigcommerce/client.go` |
| **Function** | `GetAll()` |
| **Risk** | Memory exhaustion: a store with 100k+ products causes `GetAll` to load all of them into memory |
| **Root Cause** | `GetAll` loops until all pages are consumed with no upper bound |

**Fix applied:**
1. Added `MaxTotalRecords` config field (default: 10,000, configurable via `BC_MAX_TOTAL_RECORDS`)
2. `GetAll()` truncates results and logs a warning when the ceiling is reached
3. Config validation ensures the value is non-negative

---

#### S6 — TierEnforcer Does Not Centrally Enforce Confirmation

| | |
|---|---|
| **File** | `internal/middleware/tiers.go`, `internal/discovery/registry.go` |
| **Risk** | Authorization bypass: a developer writes an R1+ tool handler that forgets to check `IsConfirmed()`, accidentally exposing an unconfirmed write path |
| **Root Cause** | `TierEnforcer.Check()` only blocked R4; confirmation was purely opt-in per handler |

**Fix applied:**
1. Added `CheckConfirmation()` utility method to `TierEnforcer` for handlers
2. **Registration-time validation:** `RegisterTool()` now panics at startup if
   an R1+ tool's MCP input schema does not declare a `confirmed` boolean
   parameter. This catches the developer mistake at build time, not runtime.

---

### MEDIUM Severity

#### S7 — Config Validation Gaps

| | |
|---|---|
| **File** | `internal/config/config.go` |
| **Risk** | Server crash: `RequestsPerSecond=0` causes a divide-by-zero panic in `NewClient`; other out-of-range values cause undefined behavior |
| **Root Cause** | Only `ProductBatchSize` and `VariantBatchSize` were validated |

**Fix applied:** Added validation for:
- `RequestsPerSecond` (must be > 0 and <= 30)
- `MaxRetries` (must be 1-20)
- `DefaultPageLimit` (must be 1-250)
- `MaxTotalRecords` (must be >= 0)
- `CacheTTL` (must be >= 0)

---

#### S8 — No Cache Size Limit (Memory Exhaustion)

| | |
|---|---|
| **File** | `internal/session/cache.go` |
| **Risk** | Memory exhaustion: repeated preview operations with different keys, or many concurrent sessions, grow the cache without bound |
| **Root Cause** | `Cache.items` and `Store.caches` maps had no maximum size |

**Fix applied:**
1. `Cache` now has a `maxEntries` limit (default: 1,000). On overflow, expired
   entries are evicted first, then the oldest entry is removed (LRU-like).
2. `Store` now has a `maxSessions` limit (default: 100). When exceeded, the
   oldest session's cache is evicted.
3. `Set()` and `SetWithTTL()` trigger eviction automatically.

---

#### S9 — Credential Leakage via Error Messages

| | |
|---|---|
| **File** | `internal/bigcommerce/types.go`, `internal/tools/catalog/products.go` |
| **Risk** | Information disclosure: raw BigCommerce API error responses (which may contain internal details) are returned verbatim to the LLM |
| **Root Cause** | `APIError.Error()` includes the full response body; `toolError()` has no length limit |

**Fix applied:**
1. `APIError.Error()` now truncates the body at 500 characters
2. Added `APIError.SafeError()` for external-facing messages (returns only the
   status code, no body content)
3. `toolError()` truncates messages at 1,000 characters

---

### LOW Severity (Documented, Not Fixed)

#### S10 — Throttle ticker lifecycle (resolved)

| | |
|---|---|
| **File** | `internal/bigcommerce/client.go` |
| **Historical issue** | Earlier revisions used `time.Tick`, which cannot be stopped |
| **Current state** | The client uses `time.NewTicker` and `Close()` stops it; call `Close()` when retiring a client instance |
| **Residual note** | Ensure any future multi-tenant or pooled-client design disposes clients so tickers are stopped |

#### S11 — No TLS Certificate Pinning

| | |
|---|---|
| **File** | `internal/bigcommerce/client.go` |
| **Risk** | MITM if the system CA store is compromised |
| **Impact** | Low — standard practice for SaaS API clients |
| **Recommendation** | Consider certificate pinning if compliance requires it |

#### S12 — `Evict()` Never Called Automatically

| | |
|---|---|
| **File** | `internal/session/cache.go` |
| **Risk** | Stale entries persist until overwritten (mitigated by the new size limits) |
| **Recommendation** | Add a background goroutine that calls `Evict()` periodically (e.g., every 60s) |

---

## Files Added/Modified

| File | Change |
|---|---|
| `internal/tools/catalog/products.go` | Safe type assertions, price floor, adjustment bounds, error truncation, declarative search filters |
| `internal/tools/catalog/categories.go` | Safe type assertions, create (POST+parent_name), bulk_update, delete/bulk_delete with child safeguards |
| `internal/bigcommerce/client.go` | Response body size limit, pagination ceiling |
| `internal/bigcommerce/types.go` | Truncated error messages, `SafeError()`, `CategoryCreate` struct with `omitempty` |
| `internal/bigcommerce/products.go` | `CreateCategory` (POST), `DeleteCategories`, `GetDefaultTreeID`, URL construction fix |
| `internal/config/config.go` | `AuthToken`, `MaxTotalRecords` fields; comprehensive validation |
| `internal/middleware/tiers.go` | `CheckConfirmation()` utility |
| `internal/middleware/auth.go` | **New** — Bearer token auth middleware |
| `internal/discovery/registry.go` | Registration-time confirmation param validation |
| `internal/session/cache.go` | Entry and session count limits with eviction |
| `cmd/server/main.go` | Auth middleware wiring for HTTP/SSE |
| `.gitignore` | **New** — prevents `.env` and binary from being committed |
| `internal/tools/catalog/products_test.go` | **New** — 300 lines, search filter and parameter parsing tests |
| `internal/tools/catalog/categories_test.go` | **New** — 406 lines, create/delete parameter parsing tests |
| `internal/session/cache_test.go` | **New** — cache TTL, eviction, size limit tests |
| `internal/middleware/auth_test.go` | **New** — bearer auth middleware tests |
| `internal/middleware/tiers_test.go` | **New** — tier enforcement tests |
| `internal/config/config_test.go` | **New** — config validation tests |
| `internal/discovery/registry_test.go` | **New** — registry confirmed-param validation tests |

---

## Remaining Recommendations

1. **~~Add unit tests for all security-critical paths~~ — RESOLVED**: Substantial
   testify-based coverage now spans type assertion handling, price floor clamping,
   auth middleware rejection, cache eviction, config validation, confirmed-param
   enforcement, and tool parameter parsing (products + categories including delete
   and sub-resources). Run `go test ./...` for the current set. Remaining: integration
   tests for full MCP tool-call flows via mcp-go's in-process transport.
2. **Run `govulncheck`** against dependencies to check for known CVEs in
   mcp-go and transitive deps.
3. **Add rate limiting on the MCP HTTP endpoints** — currently only BigCommerce
   API calls are rate-limited; the MCP server itself accepts unlimited requests.
4. **Consider CORS headers** for the HTTP transport if browsers will ever call it.
5. **Add audit logging** for confirmed mutations (tier R1+) including the tool
   path, category/product IDs, and the operator session ID.
6. **Implement session authentication** — currently the session cache uses a
   hardcoded `"default"` session ID. Wire actual MCP session IDs when the
   transport supports them.
7. **Review mcp-go's `WithRecovery()`** — while it catches panics in tool
   handlers, verify it doesn't swallow security-relevant information.

---

## Threat Model Summary

| Attack Vector | Mitigation |
|---|---|
| Malformed LLM arguments → server crash | Safe type assertions with error returns |
| Oversized API response → OOM | 50 MB response body limit |
| Unauthenticated HTTP/SSE access | Bearer token auth middleware (required for non-stdio transports) |
| Negative/extreme price adjustments | Price floor at $0.00; percentage bounds -100% to +1000% |
| Huge catalog → memory exhaustion | Pagination ceiling (default 10k records) |
| Missing confirmation on write tools | Registration-time schema validation |
| Invalid config → crash/undefined behavior | Comprehensive bounds checking at startup |
| Cache growth → memory exhaustion | Entry and session count limits with eviction |
| Credential leakage in errors | Truncated error bodies; `SafeError()` for external callers |
| `.env` committed to VCS | `.gitignore` excludes `.env` files |
