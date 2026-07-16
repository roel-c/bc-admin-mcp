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

**Base URL:** `https://api-b2b.bigcommerce.com/api/v3/io/`

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
| Quotes (RFQ) | `/rfq` (v2) | Sales quote lifecycle + cart conversion |
| Shopping Lists | `/shopping-list` | Repeat-purchase list management |
| Invoice Management | `/ip/invoices` | Invoice generation + external payment logging |
| Payments | (payments resource) | Payment methods, available credit, net terms |
| Sales Staff | `/sales-staff` | Backend sales rep company assignment |
| Super Admins | `/super-admins` | Frontend sales rep + masquerade session management |
| Channels | `/channels` | B2B channel information |

---

## Phased Implementation Plan

### Phase B1 — Company & User Management ✅ Shipped

**Discovery tree:** `b2b/` → `b2b/companies/` + `b2b/companies/users/` + `b2b/companies/addresses/`

**Activation:** Set `BC_B2B_ENABLED=true` in `.env`.

| Tool | Tier | Description |
|------|------|-------------|
| `b2b/companies/list` | R0 | List companies; filter by status/name/email |
| `b2b/companies/get` | R0 | Get company details by ID |
| `b2b/companies/create` | R1 | Create company + initial admin user |
| `b2b/companies/update` | R1 | Update profile fields |
| `b2b/companies/set_status` | R2 | Approve, reject, deactivate |
| `b2b/companies/delete` | R3 | Permanently delete company + all users; also deletes the users' linked BC customer accounts by default (`delete_bc_customers=false` to keep) |
| `b2b/companies/users/list` | R0 | List users; filter by company/role/email |
| `b2b/companies/users/create` | R1 | Create buyer portal user (0=admin, 1=senior, 2=junior) |
| `b2b/companies/users/update` | R1 | Update name, phone, role |
| `b2b/companies/users/delete` | R2 | Remove from buyer portal (BC customer preserved) |
| `b2b/companies/addresses/list` | R0 | List addresses; filter by company/billing/shipping/country |
| `b2b/companies/addresses/create` | R1 | Add address to a company |
| `b2b/companies/addresses/update` | R1 | Full PUT update of an address |
| `b2b/companies/addresses/delete` | R2 | Remove address (existing orders/quotes unaffected) |

**Company status codes:** 0=pending, 1=approved, 2=rejected, 3=inactive

**User role codes:** 0=admin, 1=senior buyer, 2=junior buyer

---

### Phase B2 — Quotes *(planned)*

Sales quote lifecycle: buyer requests quote → sales rep prices → buyer approves → converts to cart/order.

| Tool | Tier | Endpoint |
|------|------|---------|
| `b2b/quotes/list` | R0 | `GET /rfq` — list with status/company/date filters |
| `b2b/quotes/get` | R0 | `GET /rfq/{id}` — full line items, pricing, messages |
| `b2b/quotes/update_status` | R2 | Approve, reject, or expire a quote |
| `b2b/quotes/convert_to_cart` | R2 | `POST /rfq/{id}/checkout` — returns cart/checkout URL |
| `b2b/quotes/assign_to_order` | R2 | `POST /rfq/{id}/ordered` — link quote to placed order |
| `b2b/quotes/export_pdf` | R0 | `POST /rfq/{id}/pdf-export` — returns PDF URL |

---

### Phase B3 — Shopping Lists *(planned)*

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

### Phase B4 — Invoicing & Payments *(planned)*

Net terms / PO-based purchasing and invoice management.

| Tool | Tier | Endpoint |
|------|------|---------|
| `b2b/invoices/list` | R0 | `GET /ip/invoices` — filter by company/status/date/PO |
| `b2b/invoices/get` | R0 | Single invoice with line items and balance |
| `b2b/invoices/create` | R2 | Generate invoice for a purchase order |
| `b2b/invoices/log_payment` | R2 | Log an external payment against an invoice |
| `b2b/companies/payments/list` | R0 | View payment methods and credit balance |
| `b2b/companies/payments/net_terms` | R1 | View/update net terms for a company |

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
