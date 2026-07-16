# B2B Edition — API Research & Phased Implementation

BigCommerce B2B Edition (formerly BundleB2B) extends the storefront for business-to-business commerce. This document captures the full API surface, authentication architecture, and phased development plan.

---

## Authentication

**Unified Auth (September 30, 2025):** B2B Edition APIs now accept the same `X-Auth-Token` as the core Management API. A second `X-Store-Hash` header is required to route requests to the correct B2B Edition account.

| Header | Value | Source |
|--------|-------|--------|
| `X-Auth-Token` | Store-level API token | `BC_AUTH_TOKEN` (existing) |
| `X-Store-Hash` | Store hash | `BC_STORE_HASH` (existing) |

**No new credentials are needed.** The existing `BC_AUTH_TOKEN` + `BC_STORE_HASH` cover both APIs. The B2B Edition scope must be enabled on the store-level API account in BigCommerce Settings → Store-level API accounts.

**Gate:** Set `BC_B2B_ENABLED=true` in `.env` to enable the `b2b/` discovery root. When false (default), the domain is not registered — stores without B2B Edition will not see broken tools.

**Base URL:** `https://api-b2b.bigcommerce.com/api/v3/io/` for most resources (companies, users, addresses, roles, hierarchy, channels, orders, quotes, payments/credit/terms). **Exception:** Invoice Management (invoices, receipts, receipt-lines) is served from `https://api-b2b.bigcommerce.com/api/v3/io/ip/` — a distinct base with an `/ip` suffix, confirmed from each endpoint's OpenAPI `servers:` block (not obvious from the path alone; easy to miss).

Implementation: `internal/bigcommerce/b2b_client.go` (`B2BClient`)

---

## Full API Surface

BigCommerce documents 11 server-to-server resource families:

| Resource | Base Path | Description |
|---------|-----------|-------------|
| Companies | `/companies` | Company account CRUD + status + catalog/price-list assignment |
| Users | `/users` | Buyer portal user CRUD + role assignment |
| Addresses | `/addresses` | Company billing/shipping address management |
| Orders | `/orders` | Get B2B order info, assign historical orders to companies |
| Quotes (RFQ) | `/rfq` | Sales quote lifecycle, shipping rates, checkout/order conversion |
| Shopping Lists | `/shopping-list` | Repeat-purchase list management |
| Invoice Management | `/ip/invoices`, `/ip/receipts` | Invoice/receipt reads (distinct `/ip` base URL — see Authentication) |
| Payments | `/payments`, `/companies/{id}/payments`, `/companies/{id}/credit`, `/companies/{id}/payment-terms` | Payment methods, credit, net terms |
| Sales Staff | `/sales-staffs` | Backend sales rep company assignment |
| Super Admins | `/super-admins` | Frontend sales rep + masquerade session management |
| Channels | `/channels` | B2B channel information |

---

## Phased Implementation Plan

### Phase B1 — Company Administration ✅ Shipped

**Discovery tree:** `b2b/` → `b2b/companies/` with sub-trees `users/`, `addresses/`, `attachments/`, `roles/`, `permissions/`.

**Activation:** Set `BC_B2B_ENABLED=true` in `.env`.

**Companies**

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/companies/list` | R0 | List companies; filter by status/name/email |
| `b2b/companies/get` | R0 | Get company details by ID |
| `b2b/companies/create` | R1 | Create company + initial admin user (supports `extra_fields_json`, `customer_group_id`, and linking an existing BC customer via `bc_customer_id`) |
| `b2b/companies/update` | R1 | Update profile fields (supports `extra_fields_json`, `customer_group_id`); response is sparse — the tool re-fetches before returning |
| `b2b/companies/set_status` | R2 | Approve, reject, deactivate |
| `b2b/companies/delete` | R3 | Permanently delete company + all users; also deletes the users' linked BC customer accounts by default (`delete_bc_customers=false` to keep) |
| `b2b/companies/extra_fields` | R0 | List company extra-field (custom field) definitions |
| `b2b/companies/update_catalog` | R2 | Assign a price list/catalog (read-only on Independent-behavior stores) |

**Users**

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/companies/users/list` | R0 | List users; filter by company/role/email |
| `b2b/companies/users/get` | R0 | Get one user by B2B user ID (includes extra fields) |
| `b2b/companies/users/get_by_customer` | R0 | Resolve the B2B user from a BigCommerce customer ID |
| `b2b/companies/users/create` | R1 | Create buyer portal user (supports `extra_fields_json`) |
| `b2b/companies/users/bulk_create` | R1 | Create up to 10 users in one call (`users_json`) |
| `b2b/companies/users/update` | R1 | Update name, phone, role |
| `b2b/companies/users/delete` | R2 | Remove from buyer portal (BC customer preserved) |
| `b2b/companies/users/extra_fields` | R0 | List user extra-field definitions |

**Addresses**

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/companies/addresses/list` | R0 | List addresses; filter by company/billing/shipping/country |
| `b2b/companies/addresses/create` | R1 | Add address to a company |
| `b2b/companies/addresses/update` | R1 | Full PUT update of an address |
| `b2b/companies/addresses/delete` | R2 | Remove address (existing orders/quotes unaffected) |

**Attachments**

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/companies/attachments/list` | R0 | List a company's file attachments |
| `b2b/companies/attachments/add` | R1 | Upload a local file (≤10MB) to the company's Attachments tab |
| `b2b/companies/attachments/delete` | R2 | Delete an attachment by ID |

**Roles & permissions**

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/companies/roles/list` | R0 | List roles (predefined + custom) |
| `b2b/companies/roles/get` | R0 | Get a role and its permissions |
| `b2b/companies/roles/create` | R1 | Create a custom role (`permissions_json`) |
| `b2b/companies/roles/update` | R1 | Replace a custom role's name + full permission set |
| `b2b/companies/roles/delete` | R2 | Delete a custom role |
| `b2b/companies/permissions/list` | R0 | List permission definitions (discover codes) |
| `b2b/companies/permissions/create` | R1 | Create a custom permission |
| `b2b/companies/permissions/update` | R1 | Update a custom permission |
| `b2b/companies/permissions/delete` | R2 | Delete a custom permission |

**Account hierarchy** (requires Account Hierarchy enabled on the store)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/companies/hierarchy/get` | R0 | Full hierarchy (parents + nested subsidiaries) for a company |
| `b2b/companies/hierarchy/subsidiaries` | R0 | List subsidiary accounts beneath a company |
| `b2b/companies/hierarchy/attach_parent` | R1 | Set a parent above a company |
| `b2b/companies/hierarchy/detach_subsidiary` | R2 | Remove a subsidiary's parent link |

**Channels**

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/channels/list` | R0 | List storefront channels seen by B2B Edition |
| `b2b/channels/get` | R0 | Get a channel by BigCommerce channel ID |

**Orders** (B2B order metadata; `bc_order_id` = BigCommerce order ID)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/orders/get` | R0 | Get the B2B view of an order (PO number, company, extra fields) |
| `b2b/orders/update` | R1 | Set an order's PO number and/or extra fields |
| `b2b/orders/assign_customer_orders` | R2 | Attach a buyer's historical orders to their company |
| `b2b/orders/reassign` | R2 | Reassign orders by customer group — **Dependent-behavior stores only** |
| `b2b/orders/extra_fields` | R0 | List order extra-field definitions |

**Company status codes:** 0=pending, 1=approved, 2=rejected, 3=inactive

**User role codes:** 0=admin, 1=senior buyer, 2=junior buyer

**Permission levels:** 1=user, 2=company, 3=company and subsidiaries

**Extra fields:** Stores can require custom fields on companies/users. Use the `extra_fields` tools to discover definitions, and pass `extra_fields_json` (`[{"fieldName","fieldValue"}]`) on create/update.

**Customer group assignment (catalog/pricing visibility):** a company's buyers see the products/pricing determined by its linked BigCommerce customer group. Pass `customer_group_id` on `b2b/companies/create` or `update` to assign one — but this only takes effect on stores using **Independent Companies** behavior (the default for new stores since Oct 2024). On legacy **Dependent Companies** stores, BC Edition auto-creates and permanently 1:1-links a group per company instead, and `customer_group_id` is ignored. There is no MCP tool to detect which mode a store is in directly; infer it by creating a company and checking whether `bc_group_id` populates without you setting `customer_group_id` (Dependent) or stays `0` (Independent). To restrict a company to a specific catalog slice: create a category scoped to the intended storefront channel, create a customer group with `category_access_type: "specific"` scoped to that category (`customers/groups/create`), then assign that group's ID as `customer_group_id` on the company. Multiple companies (e.g. a parent and its subsidiaries) may share the same group/category restriction — live-validated in `WORKFLOW.md` §10.3.

**MSF storefront-channel scoping:** when the target storefront matters (for example a B2B buyer should belong to `MSF-B2BE`, not another storefront on the same store), do **not** rely on B2B Edition's implicit BC-customer creation. Instead, create the underlying BigCommerce customers first via `customers/create` with `origin_channel_id` and `channel_ids` set to the target storefront channel, then pass those IDs into `b2b/companies/create` or `b2b/companies/users/create` / `bulk_create` as `bc_customer_id`. Operationally, any future **D2C or B2B surface check** in an MSF store should begin with an explicit question: which storefront channel or channels should own the test data? `catalog/channels/list` is the reliable human-readable source for channel names; `b2b/channels/list` confirms which storefront channels B2B Edition sees (B2B runs only).

**Deferred (management API, needs a focused pass):** bulk-create companies (unusual `data.errors`+`meta[]` envelope), batch update `PUT /companies` (redundant with per-id update), and convert customer-group→company (legacy Dependent-behavior migration).

---

### Phase B2 — Quotes ✅ Shipped

Sales quote lifecycle: buyer requests quote → sales rep prices → buyer approves → converts to cart/order. Confirmed accessible via server-to-server auth despite the docs index nominally linking list/create/get/update under the Storefront section — the Management-section mirror pages (same `X-Auth-Token`/`X-Store-Hash` auth) work identically and were verified live.

**Discovery tree:** `b2b/quotes/` with a `shipping/` sub-tree.

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/quotes/list` | R0 | List quotes; filter by company/salesRep/status/date ranges |
| `b2b/quotes/get` | R0 | Full detail: line items, addresses, shipping method, message history |
| `b2b/quotes/create` | R1 | Create a quote (`quote_json`); **must include `companyId`** for Buyer Portal visibility (contact email/name alone are insufficient); visible to the buyer immediately unless `allowCheckout=false` |
| `b2b/quotes/update` | R1 | Partial update (`quote_json`); `productList` updates replace the full line-item set |
| `b2b/quotes/delete` | R3 | Permanently delete (use `update` with `status=archived` to hide instead) |
| `b2b/quotes/checkout` | R1 | Generate cart + checkout URLs (status New/In Process/Updated by Customer only) |
| `b2b/quotes/assign_to_order` | R2 | Associate an existing BC order with the quote |
| `b2b/quotes/pdf_export` | R0 | Backend-detail PDF download link (optional currency override) |
| `b2b/quotes/extra_fields` | R0 | List quote extra-field definitions |
| `b2b/quotes/shipping/rates` | R0 | Available static/real-time shipping rates (requires a shipping address on the quote) |
| `b2b/quotes/shipping/select` | R1 | Assign a rate (`shipping_method_id`, or `custom_name`+`custom_cost`) |
| `b2b/quotes/shipping/remove` | R2 | Clear the assigned shipping method |
| `b2b/quotes/shipping/custom_methods` | R0 | Store-wide custom shipping methods (requires the setting enabled in Quotes settings) |

**Quote status codes:** 0=new, 2=in process, 3=updated by customer, 4=ordered, 5=expired (others exist; see BigCommerce's Quote Statuses reference).

**API quirks confirmed live:**
- Quote IDs are **integers**; invoice/receipt IDs are strings.
- **`companyId` is required for Buyer Portal visibility.** Without it, quotes
  show in the Control Panel with empty `companyInfo: {}` but do not appear for
  company buyers. `contactInfo.email` / `companyName` alone do not link the
  quote. Ordered quotes cannot be patched with `companyId` afterward
  (`422 Quote has already been ordered`).
- `expiredAt` must be `MM/DD/YYYY` (BC's own 422 message has an unrendered `%D` template placeholder — cosmetic bug on their side).
- `POST /rfq` requires `discount` (top-level) and each `productList` item needs `basePrice` + `offeredPrice` + `discount` (prefer numbers; include `variantId`), none of which are marked required in the OpenAPI schema.
- `PUT /rfq/{id}/shipping-rate` (select) returns `data: []` on success, not the updated quote — don't expect quote detail back from that call.
- `/rfq/{id}/shipping-rates` (plural, GET) vs `/rfq/{id}/shipping-rate` (singular, PUT/DELETE) — mixing them returns BC's own 405.

---

### Phase B3 — Invoices, Receipts, Payments, Credit & Net Terms ✅ Shipped (read + write)

**Discovery tree:** `b2b/invoices/`, `b2b/receipts/` (+ `lines/`), `b2b/payment_records/`, `b2b/payments/`, `b2b/companies/payments/`, `b2b/companies/credit/`, `b2b/companies/payment_terms/`.

**Invoices** (served from the `/ip` base — see Authentication)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/invoices/list` | R0 | List invoices; filter/sort by company, status, PO number, dates |
| `b2b/invoices/get` | R0 | Full detail: line items, tax, billing address, balance |
| `b2b/invoices/download_pdf` | R0 | Download link for the invoice PDF |
| `b2b/invoices/extra_fields` | R0 | List invoice extra-field definitions |
| `b2b/invoices/create` | R2 | Create an invoice from a raw JSON body (`invoice_json`) |
| `b2b/invoices/create_from_order` | R2 | Generate an invoice from an existing order's data (`order_id` = BigCommerce order ID; the tool resolves B2B Edition's own internal order ID internally — see quirk below) |
| `b2b/invoices/update` | R2 | Update an invoice from a raw JSON body; `details` fully replaces rather than merging |
| `b2b/invoices/delete` | R3 | Permanently delete an invoice |

**Receipts** (same `/ip` base)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/receipts/list` | R0 | List payment receipts |
| `b2b/receipts/get` | R0 | Get a single receipt |
| `b2b/receipts/lines/list_all` | R0 | List line items across all receipts |
| `b2b/receipts/lines/list_for_receipt` | R0 | List line items on one receipt |
| `b2b/receipts/lines/get` | R0 | Get a single receipt line |
| `b2b/receipts/delete` | R3 | Permanently delete a receipt |
| `b2b/receipts/lines/delete` | R2 | Permanently delete a single receipt line |

**Payment records** (money logged against invoices; same `/ip` base)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/payment_records/list` | R0 | List payment records logged against invoices |
| `b2b/payment_records/get` | R0 | Get a payment record's detail |
| `b2b/payment_records/transactions` | R0 | List a payment record's transaction history |
| `b2b/payment_records/operations` | R0 | Get the operations currently allowed on a payment record |
| `b2b/payment_records/create_offline` | R2 | Log a new offline payment against one or more invoices |
| `b2b/payment_records/update_offline` | R2 | Update an existing offline payment record |
| `b2b/payment_records/perform_operation` | R2 | Perform a lifecycle operation (e.g. void) on a payment record |
| `b2b/payment_records/update_processing_status` | R2 | Directly set a payment record's processing status |
| `b2b/payment_records/delete` | R3 | Permanently delete a payment record |

**Payments, credit, and net terms** (standard base, not `/ip`)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/payments/list` | R0 | Store-wide payment method definitions |
| `b2b/payments/active_methods` | R0 | Currently-enabled methods across companies (filterable by `company_id`) |
| `b2b/companies/payments/list` | R0 | A company's payment methods + enabled state |
| `b2b/companies/credit/get` | R0 | Credit settings (fails if the store's Company Credit feature is off) |
| `b2b/companies/payment_terms/get` | R0 | Net-terms configuration (e.g. Net 45) |
| `b2b/companies/payments/update` | R2 | Enable or disable payment methods for a company |
| `b2b/companies/credit/update` | R2 | Update a company's credit settings |
| `b2b/companies/payment_terms/update` | R2 | Update a company's net-terms settings |

**API quirks confirmed live:**
- Invoice/receipt/receipt-line/payment-record IDs are **strings**; global `/payments` uses `id`/`paymentCode` while `/companies/{id}/payments` uses `paymentId`/`code` for the same concepts — different field names for the same data, not a documentation error.
- **`POST /orders/{orderId}/invoices` (`create_from_order`) takes B2B Edition's own internal order ID, not the BigCommerce order ID** — they are different numbers (`GetB2BOrder`'s `id` field vs. its `bcOrderId` field). Passing the BC order ID returns a 404 "Order does not exist" even for a real, existing order. The tool resolves this automatically via a `b2b/orders/get`-equivalent lookup before calling the endpoint, so callers only ever need to supply the familiar BC order ID.

---

### Order lifecycle: from checkout to an invoiceable B2B order

Confirmed live against a POC store while validating the quote → order → invoice pipeline. This is platform behavior, not specific to this MCP, but it's easy to trip over:

1. **`carts/checkout/convert`** (`POST /v3/checkouts/{id}/orders`) always creates the order in **Incomplete** status by BigCommerce design — this endpoint takes no payment. Incomplete orders sit in a "limbo" state: hidden from both the native Orders dashboard and the B2B Orders panel unless explicitly filtered to show Incomplete orders.
2. A **real payment method must be applied** to move the order out of Incomplete:
   - **Offline methods** (e.g. "Submit for invoicing") cannot be selected via any REST API — BigCommerce's Payments API explicitly does not support offline methods; only a real storefront checkout session can choose one. The resulting order lands in **Awaiting Payment**.
   - **Gateway methods** (credit card) *can* be processed via API using a Payment Access Token against the separate `payments.bigcommerce.com` server — a materially different, more involved flow than anything in this MCP today.
   - The practical path for orders created through `carts/checkout/convert` is to move them out of Incomplete via `orders/management/update_status` (mirrors what a merchant does manually in the admin panel).
3. **B2B-panel visibility is driven by the cart/order's `customer_id`.** If the checkout's customer belongs to a B2B company user, the resulting order gets a `companyId` (after a short async indexing delay — seen up to ~25s) and appears in **both** the native BigCommerce Orders dashboard and the B2B Admin Panel's Orders section. Orders placed with `customer_id: 0` (guest) only ever appear in the native dashboard, never in the B2B panel — confirmed by comparing a guest order, a guest-but-company-linked order, and a real admin-buyer order side by side.
4. Once an order has both a real status (not Incomplete) and, for B2B invoicing, a `companyId`, `b2b/invoices/create_from_order` succeeds.

---

### Phase B4 — Shopping Lists ✅ Shipped

Repeat-purchase list management for buyers.

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/shopping_lists/list` | R0 | List lists visible to a buyer; provide exactly one of `user_id` (B2B buyer user ID) or `customer_id` (BC customer ID) |
| `b2b/shopping_lists/get` | R0 | Full detail including items |
| `b2b/shopping_lists/create` | R1 | Create, optionally with initial `items_json` |
| `b2b/shopping_lists/update` | R1 | Partial update; item `quantity=0` removes that item; omitted existing items are otherwise left alone unless replaced by ID |
| `b2b/shopping_lists/delete` | R3 | Permanently delete |
| `b2b/shopping_lists/items/remove` | R2 | Remove a single item by ID |

**API quirk confirmed live:** `POST /shopping-list` rejects a null `status` with a 422, even though the OpenAPI schema marks only `name` as required. `b2b/shopping_lists/create` now defaults `status` to `"0"` (approved) when omitted.

---

### Phase B5 — Sales Operations ✅ Shipped

Backend sales rep (Sales Staff) and frontend sales rep / masquerade (Super Admin) company assignment.

**Sales Staff**

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/sales_staff/list` | R0 | List B2B users with a Sales Staff role; filterable by `company_id` |
| `b2b/sales_staff/get` | R0 | Get a sales staff account's company assignments |
| `b2b/sales_staff/update_assignments` | R1 | Assign/unassign companies (`assignments_json`: `[{"companyId","assignStatus"}]`); non-destructive |

**Super Admins** (both the super-admin- and company-perspective views)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/super_admins/list` | R0 | List Super Admins with assigned-company counts |
| `b2b/super_admins/companies_overview` | R0 | List companies with assigned-Super-Admin counts (inverse view) |
| `b2b/super_admins/get` | R0 | Get a Super Admin's account details |
| `b2b/super_admins/companies` | R0 | List the companies assigned to one Super Admin |
| `b2b/super_admins/create` | R1 | Create (or convert an existing BC customer into) a Super Admin |
| `b2b/super_admins/bulk_create` | R1 | Create up to 10 Super Admins in one call |
| `b2b/super_admins/update` | R1 | Update name/phone/uuid/extra fields (email is read-only) |
| `b2b/super_admins/update_assignments` | R1 | Assign/unassign companies (`assignments_json`: `[{"companyId","isAssigned"}]`); non-destructive |
| `b2b/companies/super_admins/list` | R0 | List the Super Admins assigned to a company |
| `b2b/companies/super_admins/update_assignments` | R1 | Assign/unassign Super Admins for a company (`assignments_json`: `[{"superAdminId","isAssigned"}]`); non-destructive |

**API quirk confirmed live:** the two assignment directions use **different field names** for the same boolean concept — Sales Staff uses `assignStatus`, Super Admins use `isAssigned`. Confirmed from each endpoint's OpenAPI schema, not an assumption.

---

## Setup Instructions

1. In BigCommerce control panel: **Settings → Store-level API accounts → Create API Account**
2. Enable the **B2B Edition** scope on the account
3. Use the generated token as `BC_AUTH_TOKEN` (replaces or supplements your existing token)
4. Add `BC_B2B_ENABLED=true` to your `.env`
5. Restart the MCP server — `b2b/` will appear in `discover_tools("")`

## References

- [B2B Edition API Overview](https://docs.bigcommerce.com/developer/api-reference/rest/b2b/overview)
- [B2B Authentication (Unified)](https://docs.bigcommerce.com/developer/docs/b2b-edition/getting-started/authentication)
- [B2B APIs — Full Resource Table](https://docs.bigcommerce.com/developer/docs/b2b-edition/getting-started/about-our-apis)
- `internal/bigcommerce/b2b_client.go` — B2B HTTP client
- `internal/bigcommerce/b2b_companies.go` — Company/User/Address types and methods
- `internal/tools/b2b/company_tools.go` — Phase B1 tool handlers
- `docs/b2be-page-detection.md` — Storefront/buyer portal injection research (Script Manager)
