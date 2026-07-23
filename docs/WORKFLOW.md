# Implementation Workflow — Adding BigCommerce Endpoints

This is the repeatable procedure for surfacing new BigCommerce endpoints as MCP
tools. It is the process used to build the catalog, carts/checkout, and B2B
domains, and **all future endpoint work should follow it**. It complements
[DEVELOPMENT.md](./DEVELOPMENT.md) (which defines *tool boundaries, risk tiers,
and caps*) — this doc is the *how we build and ship* cadence.

> TL;DR loop: **research → scope into a small batch → implement all layers →
> pass the build/test/lint gate → rebuild binary → reload MCP → smoke-check
> credentials/scopes → live-validate through MCP tools with cleanup → update
> docs → themed commit + push → confirm CI green.**

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
- **Pre-flight smoke check before spending MCP round-trips**: run
  `make smoke` (all domains) and, for MSF-touching batches, `make smoke-msf`.
  These hit BigCommerce directly via `curl` (bypassing the MCP layer) to
  confirm `BC_AUTH_TOKEN`/`BC_STORE_HASH` are valid and the OAuth scopes for
  the domain(s) in this batch are present. A FAIL here means fix credentials
  first — don't burn §6/§10 MCP calls diagnosing what's actually a scope or
  token problem. A WARN is a known scope gap/Enterprise gate; note it and
  proceed. This is a reachability check only, not a substitute for §6/§10
  (which validate the actual MCP tool contracts).

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

---

## 10. Full Surface Check (D2C / B2B)

A repeatable, on-demand, **MCP-only** capability review — every call goes through
`discover_tools`/`execute_tool`, never a direct API call — that creates real
sample data across every domain, exercises reads → previews → confirmed
writes, verifies results, and confirms cross-domain behavior (e.g. an
automatic promotion discounting a cart created afterward). Unlike §1–§9
(which govern *adding* a tool), this is a *using* checklist: run it whenever
you want an end-to-end health check of the live server, after a batch of
domain changes, or before a demo. It does not require code changes to run —
skip straight to §10.2/§10.3 if nothing changed since the last live-validate.

Origin: this codifies the pass we first ran ad hoc and wrote up as `FU-8` in
[`FOLLOW-UPS.md`](./FOLLOW-UPS.md) — read that entry for the full list of live
API quirks discovered the first time through (several are called out inline
below too).

There are two variants:

- **§10.2 D2C Surface Check** — the 8 always-on domains (catalog, customers,
  marketing, inventory, storefront, webhooks, carts/checkout, orders). Run
  standalone whenever B2B isn't in scope.
- **§10.3 B2B Surface Check** — **extends** the D2C check with B2B Edition
  company/hierarchy/catalog-restriction/payment-method scenarios **and** the
  commercial path quote → checkout → order → invoice → offline payment.
  Always run §10.2 first (or confirm its artifacts already exist) — the B2B
  check reuses the D2C sample product and category rather than creating its
  own.

On multi-storefront stores, **both variants require the operator to confirm
which channel(s) to target before any data is created** (§10.1).

### 10.1 Prerequisites

- Confirm the running MCP server reflects the current binary if any tool code
  changed recently: compare `ps -eo pid,lstart,comm | grep bc-mcp-server`
  against `stat -f %Sm bc-mcp-server`; if the process predates the binary,
  ask the operator to reload (§5) before starting.
- For §10.3 only: `BC_B2B_ENABLED=true`. Also check whether **Account
  Hierarchy** is enabled on the store before the subsidiary step
  (§10.3 step 6) — if `b2b/companies/hierarchy/attach_parent` 404s/403s,
  treat it as environment-gated (not a defect), note it, and continue with
  the rest of the checklist.
- Prefix every created record so strays are easy to spot: `MCP Test *` /
  `mcp-test-*` for D2C artifacts. The B2B category name is the one deliberate
  exception — see §10.3 step 1.
- Decide nothing about cleanup up front — that's an explicit decision point
  at the end of each variant (§10.2 step 10, §10.3 step 13). Don't delete
  anything without the operator's answer.
- **Multi-storefront channel selection (required when MSF is enabled):** before
  creating any sample data in §10.2 or §10.3, call `catalog/channels/list`.
  When the store has more than one active storefront channel
  (`multi_storefront_likely: true` or `active_storefront_channel_count > 1`),
  **ask the operator which storefront channel or channels the run should
  target** — do not assume channel 1, the "default" storefront, or reuse a
  prior run's channel without confirmation. Record the chosen BigCommerce
  `channel_id`(s) and use them for every channel-scoped write in both variants:
  categories (`channel_id`), products (`channel_ids`), customers
  (`origin_channel_id` / `channel_ids`), carts (`channel_id`), and — for B2B —
  the `bc_customer_id` linking pattern in §10.3. For B2B runs, also confirm the
  chosen channel appears in `b2b/channels/list`. Single-storefront stores may
  skip the question and use the lone active channel implicitly.

### 10.2 D2C Surface Check

1. **Catalog** — `catalog/brands/create` ("MCP Test Brand") →
   `catalog/categories/create` with `name: "MCP Test Category"` and, when MSF
   applies (§10.1), `channel_id: <target channel id>` →
   `catalog/products/create` with an inline `variants` array (two option
   values, e.g. Size: Small/Large), `category_ids`/`brand_id` set, and when
   MSF applies `channel_ids: [<target channel id>]` → verify via
   `catalog/products/variants/list` → `catalog/products/metafields/set`
   on the product.
2. **Customers** — `customers/groups/create` → `customers/create` in that
   group (when MSF applies, also set `origin_channel_id: <target channel id>`
   and `channel_ids: [<target channel id>]`) → `customers/addresses/create`
   → `customers/attributes/create` +
   `customers/attribute_values/upsert`.
   ⚠️ **Quirk:** `customers/addresses/create` requires the **full state
   name** (`"Texas"`), not an abbreviation (`"TX"` 422s) — see `FU-8`.
3. **Marketing** — `marketing/promotions/automatic/create` (a cart-value %
   discount) → `marketing/promotions/coupon/create` →
   `marketing/promotions/coupon/codes/create_single`.
4. **Inventory** — `inventory/locations/list` (note the existing default) →
   `inventory/locations/create` (needs `managed_by_external_source`,
   `time_zone`, and `address.email` + `address.geo_coordinates` — see
   `DEVELOPMENT.md` §2.5 / `FOLLOW-UPS.md` FU-6) →
   `inventory/adjustments/absolute` then `inventory/adjustments/relative` on
   a product variant → verify with `inventory/items/get`.
5. **Storefront** — `storefront/scripts/create` (note the scope warning if
   `visibility: all_pages`).
6. **Webhooks** — `webhooks/create` with an HTTPS destination.
7. **Carts/Checkout** — `carts/cart/create` (line item = the sample variant,
   `customer_id` set; when MSF applies, also `channel_id: <target channel id>`)
   → `carts/checkout/billing_address` →
   `carts/checkout/consignment_add` → `carts/checkout/get` to read
   `available_shipping_options` (should be non-empty for a real address if
   the store has shipping zones configured — if it comes back empty, don't
   assume the store is misconfigured; confirm the client is requesting
   `include=consignments.available_shipping_options`, see `FU-8`) →
   `carts/checkout/consignment_update` to select one → `carts/checkout/convert`.
8. **Orders** — the converted order lands in **Incomplete** status by design
   (no payment taken by `convert`); follow up with
   `orders/management/update_status` (e.g. → Awaiting Fulfillment) →
   `orders/management/metafields/set` → `orders/management/shipping_addresses/list`
   to get `order_address_id` → `orders/fulfillment/shipments/create`.
9. **Verify** — re-read each created record (list/get) and confirm the
   cross-domain interaction: the cart/order total should reflect the
   automatic promotion's discount from step 3. When MSF applies, also confirm
   the sample product is assigned to the target channel
   (`catalog/products/channel_summary` or `catalog/products/search` with
   `channel_ids`) and the sample customer is scoped to it (`customers/get`:
   `origin_channel_id` / `channel_ids`).
10. **Decision point** — ask the operator: *keep this sample data, or delete
    it now?* Only proceed to delete (reverse dependency order: shipments →
    order → cart/checkout already consumed → webhook → script → inventory
    adjustments are one-way (no delete, just note them) → location →
    coupon code → promotions → customer attribute value/attribute →
    address → customer → group → product → category → brand) on an explicit
    "delete" answer.
    - **Preview-first on destructive deletes:** R3 tools such as
      `catalog/products/delete` and `orders/management/delete` require a
      preview call (no `confirmed`) on the **same targeting** before
      `confirmed=true` — otherwise product delete returns *"no matching
      preview found"* and order delete may not execute as expected.
    - **Verify order cleanup:** after confirming
      `orders/management/delete`, re-fetch with `orders/management/get`.
      On at least one live store, delete returned `"status":"deleted"` while
      the order still existed (see `FOLLOW-UPS.md` FU-8) — note survivors
      rather than assuming cleanup succeeded.

### 10.3 B2B Surface Check (extends §10.2)

Naming: use `MCP Test Company` (parent) and `MCP Test Company - Subsidiary`
(child) for the two companies. The category name is **`Company Accounts`**
verbatim (not `MCP Test`-prefixed) — it's meant to read like a real
restricted-catalog label, not an obviously-disposable test artifact.

⚠️ **Correction (validated live, 2026-07-16):** an earlier draft of this
section assumed B2B Edition auto-provisions a native customer group per
company. That's only true for legacy **Dependent Companies** behavior. This
store — like all new B2B Edition stores since Oct 2024 — uses **Independent
Companies** behavior, where **no group is created automatically**; you create
the group yourself and assign it via a `customerGroupId` field at company
create/update time. Confirmed live: a company created via the Management API
sat at `bc_group_id: 0` for over an hour with no group appearing, while a
genuinely storefront-registered company on the same store had a real one —
there is no polling/waiting fix for API-created companies, because there was
never anything to wait for. This also surfaced a real tool gap — neither
`b2b/companies/create` nor `update` exposed a way to set this field at all —
fixed in the same pass (see `internal/bigcommerce/b2b_companies.go` /
`internal/tools/b2b/company_tools.go`); both tools now accept an optional
`customer_group_id` integer. The step order below reflects the corrected,
live-validated flow (group created *before* the company, not after).

1. **Confirm the target channel from §10.1, then validate B2B enablement** —
   the operator should already have chosen the storefront channel before §10.2
   began (see §10.1). Re-use that same `channel_id` here — do **not** pick a
   different channel mid-run. Confirm it also appears in `b2b/channels/list`
   (B2B-enabled storefronts only). On this POC store the live example target
   was `MSF-B2BE` = channel `1741970`.
2. **Category** — `catalog/categories/create` with `name: "Company Accounts"`
   and `channel_id: <target channel id>` so the category is created in that
   storefront's tree, not implicitly under the wrong storefront.
3. **Create the customer group, restricted to that category, first** —
   `customers/groups/create` with `category_access_type: "specific"` and
   `category_access_categories: [<Company Accounts category ID>]` set at
   creation time (no separate update step needed). This is the native
   BigCommerce mechanism: any storefront login mapped to this group sees
   only that one category and its products.
4. **Assign the D2C sample product to the channel and category** —
   ensure the sample product exists on the target storefront, not just in
   the global catalog. Use `catalog/products/create`/`update` with
   `channel_ids: [<target channel id>]` if needed, then
   `catalog/products/assign_categories` with the product from §10.2 step 1
   and this category's ID (additive; doesn't remove its existing category).
5. **Pre-create channel-scoped BC customers for every B2B user, then link
   them into B2B by `bc_customer_id`** — this is the key storefront-context
   fix. Do **not** rely on implicit BC-customer creation inside
   `b2b/companies/create` or `b2b/companies/users/bulk_create`, because that
   can land the identity on the wrong storefront in MSF stores. Instead:
   `customers/create` for the parent admin, subsidiary admin, and the later
   Senior/Junior buyers with `origin_channel_id: <target channel id>` and
   `channel_ids: [<target channel id>]`, plus the restricted
   `customer_group_id` from step 3. Then pass the resulting BC customer IDs
   into the B2B tools via `bc_customer_id`.
6. **Create (or update) the parent company with the group attached** —
   `b2b/companies/create` (`company_name`, `company_email`, `company_country`,
   **`company_phone`** — required though undocumented in the tool schema,
   see `FU-8` — plus `admin_first_name`/`admin_last_name`/`admin_email`, plus
   **`bc_customer_id`** = the channel-scoped BC customer from step 5, plus
   **`customer_group_id`** = the group from step 3) if creating fresh; use
   `b2b/companies/update` with the same `customer_group_id` if reassigning an
   existing company (Independent behavior allows reassignment — Dependent
   does not). Create also provisions the company's first user (role 0,
   admin) automatically **on the linked BC customer** rather than creating a
   new one in an arbitrary storefront context.
   ⚠️ **The `update` response is sparse** — even on a successful
   `customer_group_id` change, the returned company object can come back
   with `bc_group_id: 0` and most other fields blank. This is now fixed
   MCP-side (the tool re-fetches after update, mirroring what `create`
   already did for its own sparse-response quirk) — but if you're ever
   unsure, `b2b/companies/get` is the source of truth.
7. **Create the subsidiary company with the same group** — `b2b/companies/create`
   for `MCP Test Company - Subsidiary` with the **same** `bc_customer_id`
   pattern (subsidiary admin's BC customer from step 5) and the **same**
   `customer_group_id` from step 3 (per the "shared" scoping decision — do
   not create a second category or group), then
   `b2b/companies/hierarchy/attach_parent` with
   `company_id` = subsidiary, `parent_company_id` = parent.
8. **Create 3 users per company (Admin / Senior / Junior)** — each company
   already has 1 admin user (role 0) from its create call in steps 6/7. Add
   the other two via `b2b/companies/users/bulk_create` (`users_json`, max 10
   per call) with `role: 1` (senior buyer) and `role: 2` (junior buyer),
   and pass each row's **pre-created, channel-scoped** `bc_customer_id` from
   step 5. One bulk-create call can cover both companies at once
   (`company_id` varies per row), 6 users overall.
9. **Restrict payment methods to Offline-only** — `b2b/payments/list` for
   the store-wide method registry; inspect each entry's `code`/`title` to
   identify which are genuine Offline Payment Methods (e.g. `cheque`/Check,
   COD, bank deposit, in-store pickup, custom manual methods) as opposed to
   gateways/test providers **and** BigCommerce's built-in Gift
   Certificate/Store Credit (those are their own category, not "Offline,"
   even though they might look like a fit) — the endpoint has no explicit
   `is_offline` flag, so this is a manual read, not a filter. Then
   `b2b/companies/payments/list` per company to see current enabled state,
   and `b2b/companies/payments/update` with `updates_json` enabling
   (`isEnabled: true`) every identified Offline method and disabling
   (`isEnabled: false`) anything else currently enabled (in one live run:
   store had `cheque` + a test gateway both enabled by default — disabled
   the gateway, kept `cheque`). Repeat for the subsidiary.
10. **Quote → checkout → order** — exercise the commercial path on the
    **parent** company (one quote is enough for the surface check):
    1. `b2b/quotes/create` with `quote_json` for the sample product on the
       target `channelId`, buyer `contactInfo` (must be an **object**), and
       shipping address using **`state` / `stateCode`** (not
       `stateOrProvince`). **Always include `companyId`** (the B2B company
       id) — without it the quote appears in the Control Panel but **not**
       in the Buyer Portal; `contactInfo.email` / `companyName` alone do
       **not** link the quote (BC docs + live-confirmed 2026-07-16). Line
       items need **`productId`**, **`variantId`**, **`basePrice`**,
       **`offeredPrice`**, and **`discount`** as **numbers** (string prices
       and missing `variantId` have produced B2B 500s live — see FU-7). Set
       `expiredAt` as **`MM/DD/YYYY`**. Set `userEmail` to an existing B2B
       Control Panel system user / sales rep (e.g. the store admin email) —
       a buyer email 422s; omitting it can also fail depending on store
       config (see FU-8). Leave the quote **unordered** if you need to
       verify Buyer Portal visibility first — once ordered, `companyId`
       cannot be attached (`422 Quote has already been ordered`).
    2. `b2b/quotes/shipping/rates` then `b2b/quotes/shipping/select` so the
       quote has a shipping method before checkout (select returns a sparse
       body — re-`get` the quote if you need confirmation).
    3. `b2b/quotes/checkout` → capture `urls.cartId` (and the cart/checkout
       URLs).
    4. Complete checkout via the carts domain on that `cartId`:
       `carts/cart/update` to set the buyer's **`customer_id`** (quote
       checkout carts often arrive with `customer_id: 0` — required for B2B
       company indexing, see `docs/B2B.md` "Order lifecycle") →
       `carts/checkout/billing_address` → `carts/checkout/consignment_add` →
       `carts/checkout/consignment_update` (select a shipping option) →
       `carts/checkout/convert`. Record the BC `order_id`.
    5. `orders/management/update_status` off **Incomplete** (e.g. to
       **Awaiting Payment** / `status_id: 7`) — Incomplete orders are not
       reliably invoiceable / B2B-visible. Optionally `b2b/orders/get`
       with `bc_order_id` and wait briefly (~5–25s) until `companyId` is
       populated before invoicing.
    6. **`b2b/quotes/assign_to_order`** with `quote_id` + the BC
       `order_id` from step 4 — **required on the MCP/API surface-check
       path.** `b2b/quotes/checkout` only generates a cart + storefront
       checkout URLs; completing that cart via Management API
       `carts/checkout/convert` creates the BC order but does **not** mark
       the quote Ordered or write `bcOrderId` on the quote. Live-confirmed
       2026-07-22: after convert + status update the quote stayed status
       **In Process (2)** with empty `bcOrderId`/`orderId` until
       `POST /rfq/{quote_id}/ordered` (`assign_to_order`) ran. Buyer Portal
       / storefront checkout via the generated `checkoutUrl`
       (`isFromQuote=Y`) is the path that links natively (B2B frontend
       calls GraphQL `quoteOrdered`); that path is out of scope for this
       MCP-only checklist. After assign, re-`b2b/quotes/get` and confirm
       status **Ordered (4)** and `bcOrderId` = the BC order id.
11. **Invoice from order → offline payment** — continue the commercial path:
    1. `b2b/invoices/create_from_order` with the BC `order_id` from step 10
       (the tool resolves B2B Edition's internal order id automatically).
       Record the invoice id and its `openBalance` / `originalBalance`.
       (Fallback if `create_from_order` is unavailable for the order:
       `b2b/invoices/create` with a full `invoice_json` — requires
       `channelId`, and every address in `details.header` must include
       `street2` even as `""`; see FU-8.)
    2. `b2b/payment_records/create_offline` with `line_items_json` like
       `[{"invoiceId":<id>,"amount":"<partial or full>"}]`, plus
       `customer_id` = the **B2B company id** (string), `currency`, and a
       memo. Prefer a **partial** amount first so verification can show
       `status` flipping open → partially-paid and `openBalance`
       decreasing.
    3. Re-read: `b2b/invoices/get` (confirm balance/status) →
       `b2b/payment_records/get` (or `list`) → `b2b/receipts/list` /
       `b2b/receipts/lines/list_for_receipt` when a receipt appears for the
       payment.
12. **Verify** — `b2b/companies/hierarchy/get` on the parent (should list the
   subsidiary, and the subsidiary's own `b2b/companies/get` should show
   `parent_company_id`) → `customers/groups/get` on the group ID from step 3
   (confirm `category_access` shows `specific` + the one category) →
   `catalog/categories/products` on the category (confirm the sample product
   is listed) → `b2b/companies/users/list` per company (3 rows each, roles
   0/1/2) → `customers/get` on the linked BC customer IDs used in steps 5/8
   (confirm `origin_channel_id` / `channel_ids` point at the target
   storefront) →    `b2b/companies/payments/list` per company (only Offline
   methods enabled) → `b2b/quotes/get` on the quote from step 10 (status
   Ordered / `bcOrderId` set **after** `assign_to_order` on the API path) →
   `b2b/invoices/get` + payment/receipt reads from step 11 (openBalance
   reduced; status partially-paid or completed).
13. **Decision point** — same as §10.2 step 10, scoped to all B2B artifacts
   created here: reverse dependency order for commercial artifacts first
   (payment record → receipt lines/receipt if deletable → invoice → order
   if created here → quote, or archive the quote via `b2b/quotes/update`
   with `status=archived`) then users → companies (deleting a company
   cascades to its users and — by default — their linked BC customer
   accounts; see `b2b/companies/delete`'s `delete_bc_customers` flag) plus
   the customer group and the `Company Accounts` category/product assignment.
   Same preview-first and order-delete verification notes as §10.2 step 10.
