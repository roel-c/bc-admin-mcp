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
| `b2b/companies/create` | R1 | Create company + initial admin user (supports `extra_fields_json`) |
| `b2b/companies/update` | R1 | Update profile fields (supports `extra_fields_json`) |
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

**Deferred (management API, needs a focused pass):** bulk-create companies (unusual `data.errors`+`meta[]` envelope), batch update `PUT /companies` (redundant with per-id update), and convert customer-group→company (legacy Dependent-behavior migration).

---

### Phase B2 — Quotes ✅ Shipped

Sales quote lifecycle: buyer requests quote → sales rep prices → buyer approves → converts to cart/order. Confirmed accessible via server-to-server auth despite the docs index nominally linking list/create/get/update under the Storefront section — the Management-section mirror pages (same `X-Auth-Token`/`X-Store-Hash` auth) work identically and were verified live.

**Discovery tree:** `b2b/quotes/` with a `shipping/` sub-tree.

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/quotes/list` | R0 | List quotes; filter by company/salesRep/status/date ranges |
| `b2b/quotes/get` | R0 | Full detail: line items, addresses, shipping method, message history |
| `b2b/quotes/create` | R1 | Create a quote (`quote_json`); visible to the buyer immediately unless `allowCheckout=false` |
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
- `expiredAt` must be `MM/DD/YYYY` (BC's own 422 message has an unrendered `%D` template placeholder — cosmetic bug on their side).
- `POST /rfq` requires `discount` (top-level) and each `productList` item needs `basePrice` + `discount`, none of which are marked required in the OpenAPI schema.
- `PUT /rfq/{id}/shipping-rate` (select) returns `data: []` on success, not the updated quote — don't expect quote detail back from that call.
- `/rfq/{id}/shipping-rates` (plural, GET) vs `/rfq/{id}/shipping-rate` (singular, PUT/DELETE) — mixing them returns BC's own 405.

---

### Phase B3 — Invoices, Receipts, Payments, Credit & Net Terms ✅ Shipped (read-only)

Read-only per product decision — write operations (create/update invoices, log payments, update company payment methods/credit/terms) are deferred; see Follow-ups.

**Discovery tree:** `b2b/invoices/`, `b2b/receipts/` (+ `lines/`), `b2b/payments/`, `b2b/companies/payments/`, `b2b/companies/credit/`, `b2b/companies/payment_terms/`.

**Invoices** (served from the `/ip` base — see Authentication)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/invoices/list` | R0 | List invoices; filter/sort by company, status, PO number, dates |
| `b2b/invoices/get` | R0 | Full detail: line items, tax, billing address, balance |
| `b2b/invoices/download_pdf` | R0 | Download link for the invoice PDF |
| `b2b/invoices/extra_fields` | R0 | List invoice extra-field definitions |

**Receipts** (same `/ip` base)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/receipts/list` | R0 | List payment receipts |
| `b2b/receipts/get` | R0 | Get a single receipt |
| `b2b/receipts/lines/list_all` | R0 | List line items across all receipts |
| `b2b/receipts/lines/list_for_receipt` | R0 | List line items on one receipt |
| `b2b/receipts/lines/get` | R0 | Get a single receipt line |

**Payments, credit, and net terms** (standard base, not `/ip`)

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/payments/list` | R0 | Store-wide payment method definitions |
| `b2b/payments/active_methods` | R0 | Currently-enabled methods across companies (filterable by `company_id`) |
| `b2b/companies/payments/list` | R0 | A company's payment methods + enabled state |
| `b2b/companies/credit/get` | R0 | Credit settings (fails if the store's Company Credit feature is off) |
| `b2b/companies/payment_terms/get` | R0 | Net-terms configuration (e.g. Net 45) |

**API quirks confirmed live:** invoice/receipt/receipt-line IDs are **strings**; global `/payments` uses `id`/`paymentCode` while `/companies/{id}/payments` uses `paymentId`/`code` for the same concepts — different field names for the same data, not a documentation error.

---

### Phase B4 — Shopping Lists *(planned)*

Repeat-purchase list management for buyers and sales reps.

| Tool | Tier | Endpoint |
|------|------|---------|
| `b2b/shopping_lists/list` | R0 | `GET /shopping-list` — filter by userId/companyId |
| `b2b/shopping_lists/get` | R0 | `GET /shopping-list/{id}` |
| `b2b/shopping_lists/create` | R1 | `POST /shopping-list` |
| `b2b/shopping_lists/update` | R1 | `PUT /shopping-list/{id}` — name, description, items array |
| `b2b/shopping_lists/delete` | R3 | `DELETE /shopping-list/{id}` |
| `b2b/shopping_lists/items/remove` | R2 | `DELETE /shopping-list/{id}/items/{itemId}` |

---

### Phase B5 — Sales Operations *(planned)*

Super admin masquerade and sales rep assignment.

| Tool | Tier | Description |
|------|------|------------|
| `b2b/super_admins/list` | R0 | List frontend sales reps with company assignments |
| `b2b/super_admins/assign` | R1 | Assign super admin to a company |
| `b2b/super_admins/remove` | R2 | Remove super admin from a company |
| `b2b/sales_staff/list` | R0 | List backend sales reps |
| `b2b/sales_staff/assign` | R1 | Assign sales rep to a company |

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
