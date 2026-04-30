# Multi-storefront (MSF) — API review & insertion points (initial pass)

This document summarizes **public BigCommerce documentation** and **repo-local API notes** relevant to detecting multi-storefront behavior and wiring channel-aware catalog operations. Use it when designing implementation; verify endpoint shapes against the live [Developer Center](https://developer.bigcommerce.com/) before coding.

**Phased delivery (this app):** see [`docs/channels-msf-implementation-roadmap.md`](./channels-msf-implementation-roadmap.md) for shipped vs planned MCP tools and scopes.

**Terminology:** BigCommerce uses **channel** in REST paths; merchants often say **storefront**. A storefront channel is typically `type: "storefront"` on `GET /v3/channels`.

---

## 1. Is “MSF enabled” directly exposed?

**Finding:** There is **no single documented Management REST flag** read in this pass that states “multi-storefront is on” for arbitrary API clients. MSF is a **plan / product capability**; runtime behavior is inferred from **channel and catalog shape**.

**Practical detection heuristics** (to validate per-store in implementation):

| Signal | API | Interpretation |
|--------|-----|----------------|
| Multiple active storefront channels | `GET /v3/channels` | Filter `type == storefront` and `status` in storefront lifecycle (`active`, `prelaunch`, etc.). **More than one** active storefront channel ⇒ MSF-style operations matter. |
| Multiple category trees or tree ↔ channel mapping | `GET /v3/catalog/trees` (optionally `channel_id:in=…`) | Per BC MSF guides, trees tie to channels; multiple trees or distinct `channel_id` values ⇒ channel-scoped navigation. |
| Product listings per channel | `GET /v3/channels/{channel_id}/listings` | Presence of listing records / ability to vary by channel supports “listed on storefront A but not B.” |

**Caveats:**

- Every store has at least **channel id `1`** (default Stencil storefront); **counting ≥2 storefront channels** is a stronger signal than “MSF SKU on contract.”
- **OAuth scopes:** Channel reads/updates typically need **`store_channel_settings`** (see `BC-API-Reference.md` §6.12). Today’s MCP token must include whatever scopes the merchant granted; missing scope ⇒ **404/403** — treat as “cannot evaluate MSF” not “disabled.”
- **Store Information (`GET /v2/store`)** — useful for name, currency, features; **do not assume** a stable `multi_storefront` boolean without confirming current V2 response schema for your store type.

**Optional follow-up:** GraphQL Admin **store** / feature introspection (if your integration already uses GraphQL) — out of scope for this REST-focused pass.

---

## 2. Channels API (foundation)

| Method | Path (v3) | Role |
|--------|------------|------|
| GET | `/v3/channels` | List channels: `id`, `name`, `type`, `status`, `platform`, etc. |
| GET | `/v3/channels/{id}` | Single channel |
| PUT/PATCH | `/v3/channels/{id}` | Update channel (admin; not always needed for catalog tools) |
| GET | `/v3/channels/{id}/site` | Site URL / routing context for storefront |
| GET/PUT | `/v3/channels/{id}/currency-assignments` | Channel currency |
| GET/PUT | `/v3/channels/{id}/listings` | **Channel product listings** (visibility/listing state per channel) |

**Docs:** [Channels API](https://developer.bigcommerce.com/api-reference/store-management/channels-api), [Multi-Storefront overview](https://developer.bigcommerce.com/docs/storefront/multi-storefront), [MSF API guide (BigCommerce Docs)](https://docs.bigcommerce.com/developer/docs/admin/multi-storefront/api-guide).

**Implemented in this repo:** Channel data for the **merchant’s connected store** (same store hash and OAuth access token as the rest of this integration) comes from Store Management **`GET /v3/channels`**: `internal/bigcommerce/channels.go` — `ListStoreChannels`. MCP tool **`catalog/channels/list`** (R0) — optional `type` / `status` query passthrough; response adds `active_storefront_channel_count` and `multi_storefront_likely` (heuristic: >1 storefront with `active` or `prelaunch` status).

**Category trees (MSF navigation):** `internal/bigcommerce/category_trees.go` — `ListCategoryTrees`, `GetTreeIDForChannel`. MCP tool **`catalog/channels/category_trees`** (R0) — optional `channel_id` → `channel_id:in` on **`GET /v3/catalog/trees`**.

---

## 3. Products: listing / delisting on channels

Two complementary surfaces appear in documentation; **confirm which your store uses** for a given workflow:

### 3a. Catalog — product channel assignments (bulk-friendly)

Public docs describe:

- `GET /v3/catalog/products/channel-assignments` — query filters such as `product_id`, `channel_id`
- `PUT /v3/catalog/products/channel-assignments` — body array `{ product_id, channel_id }` to assign
- `DELETE` — with required filter params to remove assignments

**Docs:** [Channel assignments (REST catalog)](https://developer.bigcommerce.com/docs/rest-catalog/products/channel-assignments).

**BC-API-Reference.md** (this repo) also lists **per-product** paths:

- `GET/PUT /v3/catalog/products/{id}/channel-assignments`

Prefer **one** canonical approach in implementation to avoid duplicate logic; align with the **current** Developer Center OpenAPI for query/body rules and **parallelism** warnings (BC discourages parallel assignment calls for the same product IDs).

### 3b. Channels — listings under `/v3/channels/{id}/listings`

Useful when the operation is **“listing”** in the channel-product sense (MSF product lists / visibility on a storefront). **GET** for audit; **POST** create; **PUT** update (per BigCommerce **channels.v3** — `listing_id` required on PUT).

**Implemented MCP tools:** **`catalog/channels/listings/list`**, **`create`**, **`update`** — see [`channels-msf-implementation-roadmap.md`](./channels-msf-implementation-roadmap.md) Phase 3.

**Insertion points (future):**

- `internal/tools/catalog/products.go` — create/update/search: optional `channel_id` or “effective channel” for visibility semantics.
- **Shipped:** `catalog/products/channel_assignments/list`, `assign`, `remove` — Phase 2 in roadmap.
- **`catalog/channels/list`** returns channels from `GET /v3/channels`.

---

## 4. Categories & category trees (storefront-specific trees)

**Current code (single-tree assumption):**

```257:274:internal/bigcommerce/products.go
// GetDefaultTreeID fetches the first category tree's ID. For single-storefront
// stores there is typically one tree.
func (c *Client) GetDefaultTreeID(ctx context.Context) (int, error) {
	raw, err := c.GetAll(ctx, "catalog/trees")
	// ... returns first tree ID only
}
```

**BC MSF pattern (documented):** resolve the tree for a storefront using **`GET /v3/catalog/trees?channel_id:in={channel_id}`** (and related tree metadata / bulk tree updates as documented). A tree is associated with **at most one channel** in current BC messaging.

**Insertion points (future):**

- Replace or augment **`GetDefaultTreeID`** with `GetTreeIDForChannel(ctx, channelID)` or `ResolveDefaultChannelAndTree(ctx)` caching.
- **Category create / move / SEO / list** — any path that uses `tree_id` today should accept optional **`channel_id`** (resolve `tree_id` server-side) or explicit `tree_id` with validation that it belongs to the intended channel.
- **Product category assignment** — `PUT .../catalog/products/{id}/category-assignments` (per `BC-API-Reference.md`) may need channel or tree context for MSF-correct assignments; cross-check with category tree for that channel.

---

## 5. Storefront settings (category UX per channel)

**Storefront Category** settings (sort depth, visibility behavior, overrides) may be channel-specific via settings APIs (see Developer Center **settings / storefront category** paths). Useful for **parity with Control Panel**, not strictly required for “list products on channel 2.”

**Docs:** [Storefront category (management)](https://developer.bigcommerce.com/docs/rest-management/settings/storefront-category) (verify exact path/version in OpenAPI).

---

## 6. International / locale overlays (MSF+)

Enterprise-style **locale overrides per channel** (names, SEO, options) often involve **GraphQL Admin** and “international enhancements” docs — separate from minimal “which channel is this tree?” work. Defer unless product scope explicitly includes locale matrices.

**Docs:** [MSF international enhancements (overview)](https://developer.bigcommerce.com/docs/store-operations/catalog/msf-international-enhancements/overview).

---

## 7. Suggested MCP / server insertion map (implementation phase)

| Layer | What to add |
|-------|-------------|
| **Config** | Optional `BC_DEFAULT_CHANNEL_ID` (or resolve from `GET /v3/channels` default storefront); never hard-code beyond dev defaults. |
| **`internal/bigcommerce`** | `channels.go`: list/get channels; optional listings GET; channel-assignments GET/PUT/DELETE with shared rate-limit discipline. |
| **`BigCommerceAPI` interface** (`internal/tools/catalog/interfaces.go`) | Extend with channel methods; regenerate mock. |
| **Catalog tools** | Thread `channel_id` (or resolved tree) into category + product flows; document in tool descriptions when omitted = default channel behavior. |
| **Discovery** | `catalog/channels` registered with **`catalog/channels/list`**. |
| **Docs / prompts** | `README`, `bc_system_prompt.md`, `BC-Tool-Boundaries.md` — caps, scopes, “single-channel default” vs explicit `channel_id`. |

---

## 8. Verification checklist (before coding MSF tools)

1. Call **`GET /v3/channels`** with production token; record `type`, `status`, `id` for storefront rows.
2. Call **`GET /v3/catalog/trees`** unfiltered and with `channel_id:in=` for each storefront id; compare tree ids.
3. Call **`GET /v3/catalog/products/channel-assignments`** with a small `product_id:in` sample; confirm filter syntax.
4. Confirm token has **`store_channel_settings`** (and catalog scopes) — note failures for docs.
5. Re-read OpenAPI for **`/v3/channels/{id}/listings`** vs **catalog channel-assignments** for your target merchant workflow.

---

## 9. References (external)

- [Introduction to Multi-Storefront](https://developer.bigcommerce.com/docs/storefront/multi-storefront)
- [Multi-Storefront API Guide](https://docs.bigcommerce.com/developer/docs/admin/multi-storefront/api-guide)
- [Channels API](https://developer.bigcommerce.com/api-reference/store-management/channels-api)
- [Product channel assignments](https://developer.bigcommerce.com/docs/rest-catalog/products/channel-assignments)
- [Category trees (docs)](https://docs.bigcommerce.com/developer/api-reference/rest/admin/catalog/category-trees)

---

## 10. References (this repo)

- [`docs/channels-msf-implementation-roadmap.md`](./channels-msf-implementation-roadmap.md) — phased MCP features for MSF
- `BC-API-Reference.md` — §6.12 Channels & MSF, catalog channel/category assignments
- `internal/bigcommerce/products.go` — `GetDefaultTreeID`, tree/category CRUD
- `docs/catalog-completion-checklist.md` — MSF checklist row
- `docs/discovery-registration-audit.md` — how to register new `catalog/channels` subtree when tools exist
