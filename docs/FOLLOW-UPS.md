# Follow-ups & Tracked Technical Debt

Actionable items deferred from the architecture audit. Each entry lists the
problem, affected code, and a proposed fix so it can be picked up independently.

---

## FU-1 — `id:in` query chunking gaps (HIGH)

**Problem:** BigCommerce's `id:in` filter has a practical per-request limit
(~40–50 IDs; documented as 40 for several V3 resources). Several tools accept
more IDs than a single `id:in` query can safely carry, or pass the full list in
one query without chunking. On large inputs this can silently truncate results
or return a 4xx.

The BigCommerce client already chunks correctly in a few places (e.g.
`GetCustomersByIDs`, `GetCategoriesByIDs` chunk at 100; `DeletePromotionsByIDs`
chunks at 40). The gaps below bypass that pattern by building `id:in` inline.

### Affected tools / call sites

| Tool | Location | Issue |
|------|----------|-------|
| `customers/list` (by `customer_ids`) | `internal/tools/customers/customer_records.go` (`handleList`, ~L190) | Joins all `customer_ids` into a single `id:in` via `SearchCustomers`; `SearchCustomers` does not chunk (only `GetCustomersByIDs` does). |
| `customers/segments/shoppers/add` | `internal/tools/customers/segments_tools.go` (~L363) | Resolves up to **50** `customer_ids` through one `SearchCustomers` `id:in`, exceeding the 40-ID limit (`maxCustomerIDInQuery`). |
| `customers/segments/list` (by `segment_ids`) | `internal/tools/customers/segments_tools.go` (`handleList`, ~L176) | No cap on `segment_ids` before the `id:in` join; `docs/DEVELOPMENT.md` documents ≤ 40. |
| `customers/addresses/list` | `internal/tools/customers/customer_addresses_tools.go` (~L120) | `address_ids` / `customer_id` list filters joined into `id:in` with no max-count check. |
| `customers/attributes/delete` | `internal/bigcommerce/customer_attributes.go` (`DeleteCustomerAttributes`) | Sends all IDs (tool cap 50) in one `id:in`; no client-side chunking to 40. |
| `customers/delete`, `customers/addresses/delete` | `internal/tools/customers/*` + `internal/bigcommerce/customers.go` | DELETE by `id:in` with tool cap 50, no chunking. |
| `catalog` category assignments upsert | `internal/bigcommerce/metafields.go` (`UpsertCategoryAssignments`, ~L270) | Sends the entire assignment slice in one PUT; large lists may 422 / hit body limits. |

### Proposed fix

1. Add a shared chunking helper in `internal/bigcommerce` (e.g.
   `chunkIntIDs(ids []int, size int) [][]int`) and a small `foreachIDChunk`
   read/delete wrapper that aggregates results across chunks.
2. Route the read paths above through a chunked variant (mirror
   `GetCustomersByIDs`) so `SearchCustomers`-by-ID and segment/address list
   filters chunk at the documented limit (40) transparently.
3. Chunk the DELETE-by-`id:in` paths the same way `DeletePromotionsByIDs`
   already does.
4. Reconcile tool caps (currently 50 in several places) with the effective
   `id:in` limit, or rely on transparent chunking so the caps become about
   payload sanity rather than the query limit.
5. Add unit tests that pass > chunk-size IDs and assert multiple client calls
   (via gomock) with correctly partitioned ID sets.

---

## FU-2 — Related deferred items (MEDIUM/LOW)

Surfaced by the same audit; lower priority than FU-1.

- **`B2BGetAll` has no pagination ceiling** (`internal/bigcommerce/b2b_client.go`):
  unbounded memory on very large company/user/address lists. Apply a
  `MaxTotalRecords`-style ceiling as the core client's `GetAll` does.
- **Order sub-resource pagination ignores `MaxTotalRecords`**
  (`internal/bigcommerce/orders.go`, e.g. `ListOrderProducts`,
  `ListOrderShipments`): unbounded fetch for very large orders.
- **`Cache.Evict` is never called on a timer** (`internal/session/cache.go`):
  wire a background goroutine (e.g. every 60s) so expired entries are reclaimed
  proactively rather than only on write. (See ARCHITECTURE §6.5.)
- **Config knobs `BC_MAX_WRITE_CONCURRENCY` and `BC_INVENTORY_BATCH_SIZE` are
  accepted but not wired** (`internal/config/config.go`): either implement them
  (`BatchPut` concurrency; inventory batch sizing) or remove them and update the
  docs. Left in place pending a product decision because both are documented as
  reserved/forward-looking.

---

## FU-4 — Findings from the live read/write test pass (MSF-B2BE)

Surfaced while exercising real writes against the POC store. Two were fixed
immediately (see below); the rest are tracked here.

**Fixed in this pass:**
- ✅ `catalog/products/create` now accepts an inline `variants` array so a
  product + all variants are created in a single `POST /v3/catalog/products`
  (BigCommerce V3 best practice). Previously product-with-variants required
  N+2 calls (product → option → each variant). See
  [Create Product](https://docs.bigcommerce.com/developer/api-reference/rest/admin/catalog/products/create-product.mdx).
- ✅ `b2b/companies/create` now requires `company_email` and `company_country`
  (plus admin name), matching the B2B API's documented required fields — a
  valid-looking call previously returned an opaque 422. See
  [B2B Company Management](https://docs.bigcommerce.com/developer/learn/courses/b2b-core/company/rest-company-management).
- ✅ `catalog/products/options/create` description corrected (it wrongly claimed
  variants auto-generate from options — they do not via the API).

**Still open:**
- **Opaque 4xx errors** — API errors surface as `"BigCommerce API returned status 422"`
  with no detail (`SafeError` drops the body). For 400/422, surface the BC error
  `title`/`detail`/`errors` map (still redacting anything sensitive) so callers can
  self-correct. This cost several guess-and-retry cycles during testing
  (`internal/bigcommerce/types.go` `APIError.SafeError`).
- **Standalone `catalog/products/variants/create` contract is confusing** — the
  parser requires `label`, but BC requires numeric `id` + `option_id`; label-only
  → BC 422, ids-only → client rejects. Either resolve names → value IDs server-side
  (look up the option by display name + label) so names alone work, or document the
  id+option_id+label requirement explicitly. The new inline-variants path avoids this.
- **Misnamed/unknown tool params are silently ignored** — e.g. passing `values`
  instead of `option_values` to `options/create` was dropped with no error, so the
  option was created with no values. Consider warning on unrecognized argument keys.
- **`b2b/companies/list` `name` filter does not filter server-side** — a `name`
  query returned every company (including non-matching ones). Verify the B2B list
  query param name/semantics and either apply it correctly or filter client-side.

---

## FU-5 — Widespread live read/write test: fixes shipped

A broad read/write pass across every domain against the live POC store (MSF-B2BE
channel) drove these code fixes. All are in the current binary with tests green
and were verified live except where noted.

- ✅ **BC structured errors surfaced** (`internal/bigcommerce/types.go` `APIError.SafeError`) — 4xx now include BC's `title`/`detail`/`errors`. This unblocked diagnosing every finding below.
- ✅ **Modifier `required` dropped `false`** — `ProductModifierCreate.Required` was `omitempty`; BC requires the field always. Now always serialized.
- ✅ **Coupon `generate_bulk` false failure** — BC `/codegen` returns `data` as a batch **object**, not `[]CouponCode`; client now tolerates it (`CodeGenResult`) and the tool reports the requested count + points to `codes/list`. (Codes were being minted while the tool reported failure — a double-generate hazard.)
- ✅ **Inventory location create sent an object; BC wants a batch array** (`error.expected.jsarray`). Handler now wraps `[location]`. Verified live (BC then required `managed_by_external_source`, `time_zone`, and `address{email, geo_coordinates}`, with `state` as a 2-letter code — see FU-6).
- ✅ **Cart metafields** — `Metafield.ResourceID` was `int`, but a cart's `resource_id` is its UUID string. Changed to `json.RawMessage`. Verified live (set + list).
- ✅ **Checkout billing address** — `CheckoutAddress.ID` was `int`; BC returns string address IDs. Changed to string. Verified live.

---

## FU-6 — Widespread live test: open findings

Discovered during the same pass.

- ✅ **FIXED — Inventory location update & delete used single-resource paths that 403 (HIGH).** BigCommerce's location update/delete are batch operations. `UpdateInventoryLocation` now `PUT /v3/inventory/locations` with an array body carrying the immutable `id`; `DeleteInventoryLocation` now `DELETE /v3/inventory/locations?location_id:in=…`. Verified against BC docs + live.
- ✅ **RESOLVED — No brand delete/image tools.** Added `catalog/brands/delete` (R3), `catalog/brands/image/set` (R1, URL-based via update), and `catalog/brands/image/delete` (R2). (BC's direct image *upload* is multipart-only and not exposed via MCP; use a public `image_url`.)
- ✅ **FIXED — `inventory/items/get` returned 403.** There is no single-item GET; `GetInventoryItem` now uses the list endpoint `GET /v3/inventory/items?variant_id:in={id}&limit=1` and returns the single row. Verified live.
- ✅ **FIXED — Product image `image_url` extension check too strict.** Now only rejects an explicit *non-image* extension; extension-less/CDN/query URLs are allowed (BC validates by MIME). Verified live.
- ✅ **FIXED — `categories/bulk_update` `set_*` fields non-obvious.** Tool description now spells out the `category_ids` + `set_*` shape and notes there is no per-row `updates` array.
- ✅ **FIXED — `consent/update` required both `allow` and `deny`.** Request type no longer `omitempty` and the handler sends `[]` for the omitted side, so a one-sided update no longer 422s.
- ✅ **FIXED — `webhooks/events` 404 on a hook with no history.** Now returns an empty events list with a clear note instead of a misleading "not found" error.
- ✅ **FIXED — Checkout billing-address update-by-id used an int.** `UpdateBillingAddress` and the `billing_address_id` param are now strings (BC checkout address IDs are strings).
- ✅ **PARTIALLY ADDRESSED — Sparse B2B responses.** `b2b/companies/create` now re-fetches the full company record after create for a useful confirmation. (User/address creates still return sparse bodies without an id to re-fetch — a BC-API limitation.)
- **`inventory/locations/create` required fields undocumented** — BC requires `managed_by_external_source`, `time_zone`, and `address` with `email` + `geo_coordinates`, and `state` as a 2-letter code. Now diagnosable via surfaced errors, but the tool should still document/validate these to avoid 422 iteration. (OPEN)
- **Inconsistent batch/id parameter names across tools** — `customer_batch`, `address_batch`, `attribute_batch`, `value_batch`, `updates`, `category_ids`; and `promotion` (create) vs `promotion_id` (update/set_status) vs `promotion_ids` (delete); and inventory `location` object. A naming convention (or per-tool doc clarity) would cut trial-and-error. (OPEN — design decision)

---

## FU-7 — B2B Phase B: deferred writes and open items (2026-07-15)

Surfaced while shipping B2B Management API Phase B (Quotes; Invoices/Receipts;
Payments/Credit/Net Terms) and live-validating against a POC store.

**Deferred by product decision (financial writes, read-only-first):**
- `PUT /companies/{id}/payments` — update which payment methods are enabled
  for a company.
- `PUT /companies/{id}/credit` — update a company's credit settings.
- `PUT /companies/{id}/payment-terms` — update a company's net-terms config.
- `POST /invoices`, `POST /orders/{id}/invoices`, `PUT /invoices/{id}`,
  `DELETE /invoices/{id}` — invoice generation/update/delete.
- `POST /payments/offline`, `PUT /payments/offline/{id}`,
  `POST /payments/{id}/operations`, `PUT /payments/{id}/processing-status`,
  `DELETE /payments/{id}` — logging/managing payments against invoices.
- `DELETE /receipts/{id}`, `DELETE /receipts/{id}/lines/{lineId}` — receipt
  and receipt-line deletion.

**Open — quote `productList` write shape is underdocumented.** The OpenAPI
schema for `POST /rfq` and `PUT /rfq/{id}` only documents `options` on
`productList` items; live testing showed `basePrice` and `discount` are also
required per-item (plus top-level `discount`), none of which are marked
required in the schema. `internal/tools/b2b/quote_tools.go` accepts a raw
`quote_json` body for this reason rather than modeling individual fields —
revisit if BC's docs are corrected, or document the full required shape
directly in the tool description from more live testing.

**Open — `/rfq/{id}/shipping-rate` (select) response is inconsistent with
other quote write endpoints.** Returns `{"data": []}` on success instead of
the updated quote (every other quote write endpoint returns the quote or a
result object). `SelectB2BQuoteShippingRate` tolerates this; flag if BC
changes it, since the current behavior means callers can't see the applied
result without a follow-up `get`.

**Note for future work — `/ip` base URL is easy to miss.** Invoice Management
(invoices, receipts, receipt-lines) is served from
`https://api-b2b.bigcommerce.com/api/v3/io/ip`, not the standard `.../io` base
every other B2B endpoint uses. This is only visible in each endpoint's OpenAPI
`servers:` block, not in the path itself or in the top-level docs overview.
Confirm the `servers:` block for any new B2B endpoint group before assuming
the standard base.

**Note for future work — non-destructive assignment endpoints use
inconsistent field names.** Sales Staff company assignments use
`assignStatus`; Super Admin company/company-super-admin assignments use
`isAssigned`. Both are booleans meaning the same thing. Always confirm the
exact field name from the endpoint's own OpenAPI schema rather than reusing
another resource's naming.

**Note for future work — shopping list create requires `status` despite the
schema marking only `name` as required.** `POST /shopping-list` 422s with a
null `status` field. `b2b/shopping_lists/create` now defaults it to `"0"`
(approved) when the caller omits it — a pattern worth checking for on any new
B2B create endpoint where the documented "required" list looks suspiciously
short (see also the quote `productList` issue above).

---

## FU-3 — Environment read/write test pass (COMPLETED 2026-07-15)

Completed a broad read/write pass across all domains against the live POC store
(MSF-B2BE channel), creating single + bulk sample data (products, variants via
the single-call inline path, categories, brands, price lists, customers,
attributes, orders, promotions + coupon codes, inventory, storefront scripts,
webhooks, carts/checkout, B2B companies/users/addresses) and tearing it down
after each domain. Findings and fixes are captured in FU-5 (shipped) and FU-6
(open). Every domain's write path works except: inventory location
update/delete (FU-6, batch-shape 403), and checkout convert-to-order (blocked by
store shipping config, not the MCP). Remaining opportunity: extend
`scripts/smoke_all_domains.sh` with the storefront/webhooks/carts/b2b R0 reads
exercised here, and add a guarded write-path smoke.
