# Implementation Workflow — Adding BigCommerce Endpoints

This is the repeatable procedure for surfacing new BigCommerce endpoints as MCP
tools. It is the process used to build the catalog, carts/checkout, and B2B
domains, and **all future endpoint work should follow it**. It complements
[DEVELOPMENT.md](./DEVELOPMENT.md) (which defines *tool boundaries, risk tiers,
and caps*) — this doc is the *how we build and ship* cadence.

> TL;DR loop: **research → scope into a small batch → implement all layers →
> pass the build/test/lint gate → rebuild binary → reload MCP → live-validate
> with cleanup → update docs → themed commit + push → confirm CI green.**

---

## 1. Research the API first

Use BigCommerce's own docs index — do not guess payloads.

- Section index: append `/llms.txt` to any docs section URL, e.g.
  `https://docs.bigcommerce.com/developer/api-reference/rest/b2b/management/llms.txt`
  enumerates every endpoint in that section.
- Clean Markdown of any page: append `.md` to the page URL (includes the
  OpenAPI request/response schemas).
- Enumerate the full resource surface, then diff it against what the MCP
  already exposes to build the gap list.
- Confirm the **exact** request body, response envelope, and path for each
  endpoint before writing code. B2B responses wrap data as
  `{code, data, meta}`; some list endpoints paginate, some don't; some fields
  documented as strings come back as numbers (see §4).

## 2. Scope into a small, shippable batch

- Group 4–10 related endpoints into a batch that can be built, tested, and
  live-validated in one pass.
- **Defer, with a written rationale**, anything that is: ambiguous in contract,
  a binary/multipart upload with no clean text path (unless explicitly needed),
  redundant with an existing tool, or gated behind a store feature you can't
  exercise. Record deferrals in the domain doc and/or `FOLLOW-UPS.md`.
- Ask the user for scope/priority when there are meaningful trade-offs
  (e.g. financial write access, sequencing).

## 3. Implement all layers (in this order)

1. **Client method** — `internal/bigcommerce/<domain>*.go`. Add the typed
   request/response structs and the HTTP call. Reuse the shared client
   (`Do`, `B2BGet/Post/Put/Delete`, `B2BGetAll`, `b2bUnmarshalSingle`,
   `b2bUnmarshalList`, `B2BPostMultipart`).
2. **Interface** — add the method to the domain's `interfaces.go` (the
   compile-time `var _ Interface = (*Client)(nil)` check keeps client and
   interface in sync).
3. **Tool registration + handler** — `internal/tools/<domain>/*.go`. Register
   the tool path + tier, define args, and write the handler (validate inputs →
   preview → confirm → call client → shaped JSON response).
4. **Category registration** — new sub-trees must be registered in
   `internal/server/server.go` (`registerCategories`).
5. **Regenerate mocks** —
   `mockgen -source=internal/tools/<domain>/interfaces.go -destination=internal/tools/<domain>/mock_bc_test.go -package=<domain>_test`.

### Conventions (non-negotiable)

- **Tiers**: R0 read, R1 standard write, R2 high-risk write, R3 destructive.
  See DEVELOPMENT.md §1.
- **Preview → confirm**: every R1+ tool returns a preview unless
  `confirmed=true`. Destructive/financial previews must fail closed if they
  can't fetch the target.
- **Tolerant parsing**: when the API is inconsistent about string-vs-number
  enums, use a flexible type (e.g. `flexString`) rather than a bare `string`.
- **Structured errors**: surface BigCommerce's `title`/`detail`/`errors` via
  `APIError.SafeError`; never swallow a 4xx body.
- **Extra fields / custom fields**: expose the config-listing tool and support
  `extra_fields_json` pass-through so required custom fields don't cause opaque
  422s.
- **Partial success**: bulk handlers report `partial_success` when some items
  fail.

## 4. Pass the build/test/lint gate

Run before every reload and before every commit:

```bash
go build ./...
go vet ./...
go test ./...
golangci-lint run ./...        # installed via `make lint-install` (go install)
```

- Add/extend unit tests for each new tool (preview + confirm + a rejection
  path). Use `suite.Suite`, `SetupTest`, gomock; tests live in `_test` packages.
- Update `internal/server/registration_audit_test.go` for every new category
  and tool path (it enforces registration + discovery invariants).
- **golangci-lint must be compiled with the repo's Go toolchain** (`go install`,
  as the Makefile and CI do) — the action's prebuilt binaries can fail to
  typecheck newer Go sources.

## 5. Rebuild the binary and reload the MCP

```bash
go build -o bc-mcp-server ./cmd/server
```

- The running MCP server does **not** hot-reload. After rebuilding, the user
  must reload it. A plain Settings → disable/enable sometimes reattaches to the
  old process; a full **Cmd+Q + reopen** reliably respawns from the new binary.
- **Always verify the live binary**: compare the server process start time to
  the binary mtime before validating.
  ```bash
  ps -eo pid,lstart,comm | grep "[b]c-mcp-server"
  stat -f %Sm bc-mcp-server
  ```
  The process start time must be **after** the binary build time.

## 6. Live-validate against the POC store (validate-as-we-go)

For each batch, exercise the real API through the MCP tools:

1. Create the **minimal fixtures** needed (e.g. a throwaway company).
2. Exercise reads, then previews, then confirmed writes.
3. Verify results (re-read/list to confirm state changed as expected).
4. **Clean up**: delete everything created (prefer cascade deletes), then
   confirm the store is clean (`list` shows only real/pre-existing records).
5. When a write returns an **opaque 4xx**, diagnose with a direct `curl` to the
   endpoint (sourcing `.env` for `BC_AUTH_TOKEN`/`BC_STORE_HASH`) to see the raw
   response, then fix the client and re-test. This is how the attachment
   `octet-stream` 422 and several field-type bugs were found.
6. If a feature is store-gated (plan/behavior) and returns 404/403, record it
   as environment-gated — not a code defect.

Cleanup policy: never leave sample/test data in the store. Test artifacts use
an identifiable prefix (e.g. `mcp-…`) so strays are easy to spot and remove.

## 7. Update documentation as the batch lands

Keep these in sync in the same change:
- Domain doc (e.g. `B2B.md`, `MSF.md`) — the authoritative tool table.
- `README.md` — the implemented-tools table.
- `AGENT.md` — the compact tier table the agent reads.
- `DEVELOPMENT.md` — tool boundaries / caps / scopes if they changed.

## 8. Commit and push

- Only commit when the user asks. Group into **themed commits** (e.g.
  `chore(tooling)` / `feat(<domain>)` / `docs`) with each commit building.
- Never commit secrets, `.env`, or the `bc-mcp-server` binary (all git-ignored).
- After pushing to `main`, confirm CI is green
  (`gh run watch <id> --exit-status`).

## 9. Safety rules (always)

- Gated features stay gated (e.g. B2B behind `BC_B2B_ENABLED`).
- Validate all inputs; enforce per-tool caps (DEVELOPMENT.md §2).
- Destructive and financial operations are preview→confirm and fail closed.
- Prefer reversible operations during testing; clean up promptly.
