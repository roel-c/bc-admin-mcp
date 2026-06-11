# BC-API-Reference — BigCommerce LLM Focused API Documentation

This file is the project’s canonical map of the BigCommerce REST Management API: endpoints by category, optimizations, and best practices. Use it with the MCP server (`internal/bigcommerce/client.go`) and the official [Management API documentation](https://developer.bigcommerce.com/docs/rest-management) for authoritative field-level specs.

**Project env vars:** `BC_STORE_HASH` maps to the `{store_hash}` segment in URLs below; `BC_AUTH_TOKEN` is the value for the `X-Auth-Token` header. See `AGENT.md` and `.env.example`.

**Agent / MCP tooling:** For read vs write tiers, batch caps, and concurrency defaults, see **`DEVELOPMENT.md`** (summarizes Section 9 below with project policy).

**BigCommerce API Reference** — Structured for Agentic / LLM Tool Use
Version: V3 (primary) / V2 (legacy where noted)
Base URL: https://api.bigcommerce.com/stores/{store_hash}/
 Auth: OAuth 2.0 via X-Auth-Token header
Format: JSON (all requests and responses)
________________


Table of Contents
1. API Architecture Overview
2. Authentication & Scopes
3. Rate Limits & Concurrency Rules
4. Threading & Bulk Request Best Practices
5. Pagination Patterns
6. API Categories & Endpoints
   * 6.1 Catalog — Products
   * 6.2 Catalog — Categories
   * 6.3 Catalog — Brands
   * 6.4 Catalog — Variants & Options
   * 6.5 Catalog — Metafields
   * 6.6 Price Lists
   * 6.7 Orders
   * 6.8 Order Shipments
   * 6.9 Customers
   * 6.10 Customer Segmentation
   * 6.11 Cart & Checkout (REST Storefront)
   * 6.12 Channels & Multi-Storefront
   * 6.13 Inventory & Locations
   * 6.14 Promotions & Coupons
   * 6.15 Shipping
   * 6.16 Tax
   * 6.17 Payments
   * 6.18 Store Settings
   * 6.19 Scripts & Content (Storefront)
   * 6.20 Themes
   * 6.21 Webhooks
   * 6.22 GraphQL APIs (Storefront + Admin)
7. Response Headers Reference
8. Error Codes Reference
9. LLM Tool Design Guidelines
________________


1. API Architecture Overview
BigCommerce exposes three distinct API surfaces:
Surface
	Protocol
	Primary Use
	REST Management API
	REST/JSON
	Server-side store management, automation, integrations
	REST Storefront API
	REST/JSON
	Client-side cart, checkout, customer actions on hosted storefronts
	GraphQL Storefront API
	GraphQL
	Headless storefronts, flexible data queries, Catalyst
	GraphQL Admin API
	GraphQL
	Admin mutations, catalog, orders (expanding)
	For agentic/LLM use: The REST Management API is the primary surface. It provides full CRUD access to all store data and is the correct choice for bulk operations, SEO updates, inventory changes, pricing, and order management.
________________


2. Authentication & Scopes
Credential Types
* V2/V3 API Account: Standard OAuth token for store management. Created in Store Control Panel → Advanced Settings → API Accounts.
* Stencil CLI Token: Theme development only — not relevant for agentic use.
Request Headers (required on all REST Management calls)
X-Auth-Token: {your_api_token}
Content-Type: application/json
Accept: application/json
OAuth Scopes — Grouped by Category
Each API token must be granted specific scopes. The following scopes are needed to enable full agentic operation:
Scope Name
	Permission Level
	Covers
	store_v2_products
	read/write
	Products, variants, images, videos
	store_v2_products_read_only
	read
	Products (read)
	store_catalog_categories
	read/write
	Category trees, category assignments
	store_v2_orders
	read/write
	Orders, order products, shipments
	store_v2_orders_read_only
	read
	Orders (read)
	store_v2_customers
	read/write
	Customer accounts, addresses
	store_v2_customers_read_only
	read
	Customers (read)
	store_v2_information
	read/write
	Store settings, general info
	store_inventory
	read/write
	Inventory locations, stock levels
	store_price_lists
	read/write
	Price lists and price records
	store_channel_settings
	read/write
	Channel configuration, MSF
	store_cart
	read/write
	Carts (server-side)
	store_checkouts
	read/write
	Checkout sessions
	store_marketing
	read/write
	Coupons, banners, gift certificates
	store_shipping
	read/write
	Shipping zones, methods, carriers
	store_payments_methods_read
	read
	Available payment methods
	store_themes_manage
	read/write
	Theme management
	store_content
	read/write
	Pages, scripts, widgets, redirects
	store_sites
	read/write
	Sites and routes (headless)
	LLM Tool Note: When defining tools for an agent, scope the token to only the permissions needed for that tool's action category. This limits blast radius from unintended mutations.
________________


3. Rate Limits & Concurrency Rules
Quota by Store Plan
Plan
	Requests/Hour
	Requests per 30 seconds
	Concurrent Requests
	Standard / Plus
	20,000
	150
	~400
	Pro
	60,000
	450
	~1,200
	Enterprise
	Custom (higher)
	Custom
	Custom
	Sandbox / Dev / Partner
	Varies by resource
	Varies
	Varies
	Rate Limit Response Headers
Every API response includes these headers. Always read them before sending the next request batch:
Header
	Description
	X-Rate-Limit-Requests-Left
	Remaining requests in current window
	X-Rate-Limit-Requests-Quota
	Total quota for the current window
	X-Rate-Limit-Time-Window-Ms
	Duration of the current rate limit window (ms)
	X-Rate-Limit-Time-Reset-Ms
	Time until quota resets (ms)
	HTTP 429 — Rate Limited Response
When quota is exhausted:
* Response code: 429 Too Many Requests
* Read X-Rate-Limit-Time-Reset-Ms and wait that duration before retrying
* Do not retry immediately — this compounds the problem
Per-Endpoint Concurrency Limits
Certain endpoints enforce their own concurrent request limits in addition to the global quota. Violating these returns a 429 regardless of remaining quota.
Endpoint / Resource
	Concurrent Request Limit
	Notes
	/v3/pricelists/{id}/records (upsert)
	1 (no parallelism)
	Serial only — parallel requests cause 429
	/v3/catalog/products (batch PUT)
	Recommend ≤ 3
	Batch up to 10 products per call
	/v3/catalog/variants (batch PUT)
	Recommend ≤ 3
	

	/v3/inventory/adjustments
	Recommend ≤ 5
	

	General REST Management endpoints
	Up to 10–20 concurrent
	Varies; monitor 429s
	REST Storefront Cart API
	Do not poll
	Never use interval polling from browser
	Critical: Per-endpoint limits are documented at the operation level in the BigCommerce API reference. Always check the specific endpoint's documentation for concurrent request notes before building bulk operations.
________________


4. Threading & Bulk Request Best Practices
Threading Model
BigCommerce explicitly supports threaded (concurrent) requests to improve throughput. <br> Key principle: send multiple requests simultaneously across open connections.
Recommended Concurrency Strategy by Operation Type
Operation
	Strategy
	Max Threads
	Batch Size
	Product reads (GET)
	Parallel
	10
	250/page
	Product bulk updates (PUT)
	Parallel
	3–5
	10 per request
	Variant updates
	Parallel
	3–5
	10 per request
	Category updates
	Parallel
	5
	Single per request
	Price list record upserts
	Serial only
	1
	Up to 1000/request
	Inventory adjustments
	Parallel
	5
	10 per request
	Order reads
	Parallel
	10
	250/page
	Order updates
	Parallel
	5
	Single per request
	Metafield writes
	Parallel
	5
	Single per request
	Webhook registration
	Serial
	1
	Single per request
	Self-Throttling Formula
Calculate a safe request rate from response headers:
safe_rate = X-Rate-Limit-Requests-Quota / (X-Rate-Limit-Time-Window-Ms / 1000)
Example: quota=150, window=30,000ms → safe_rate = 5 requests/second
Exponential Backoff Pattern (for 429 handling)
import time


def request_with_backoff(fn, max_retries=5):
    for attempt in range(max_retries):
        response = fn()
        if response.status_code == 429:
            wait_ms = int(response.headers.get('X-Rate-Limit-Time-Reset-Ms', 5000))
            wait_s = (wait_ms / 1000) * (2 ** attempt)  # exponential
            time.sleep(wait_s)
        else:
            return response
    raise Exception("Max retries exceeded")
Bulk Update Pattern (Recommended for Catalog Operations)
Instead of one request per product, batch products into groups of up to 10:
PUT /v3/catalog/products
Body: [ {product_1}, {product_2}, ... {product_10} ]
Send batches concurrently (3–5 threads) to maximize throughput while staying within concurrency limits.
ETags / Conditional Requests
Use If-None-Match with cached ETag values to skip re-downloading unchanged resources. Reduces quota consumption significantly for read-heavy workflows.
X-Correlation-Id Header (Headless / Multi-call Operations)
For workflows that chain multiple API calls (e.g., fetch product → update → verify), pass a shared X-Correlation-Id header. This helps BigCommerce infrastructure trace related requests end-to-end.
________________


5. Pagination Patterns
BigCommerce supports two pagination modes. Prefer cursor pagination where available.
Cursor Pagination (Preferred — Lower Complexity)
GET /v3/customers?limit=250&after={cursor_value}
* Use endCursor from response to get next page
* More efficient; no offset recalculation
* Available on: Customers V3, Catalog (select endpoints), GraphQL
Offset Pagination (Legacy — Still Widely Used)
GET /v3/catalog/products?page=1&limit=250
* Increment page until no results returned
* Standard across most V3 catalog endpoints
Pagination Best Practices for Bulk Operations
1. Always request maximum limit (usually 250) to minimize total request count
2. For large catalogs, run pagination reads in parallel across page ranges if total count is known
3. Cache pagination results locally before sending bulk update batches
________________


6. API Categories & Endpoints
________________


6.1 Catalog — Products
Base path: /v3/catalog/products
 Scope required: store_v2_products
Method
	Endpoint
	Description
	Batch Support
	GET
	/v3/catalog/products
	List all products (filter, paginate)
	—
	POST
	/v3/catalog/products
	Create a single product
	No
	PUT
	/v3/catalog/products
	Batch update products
	Yes — up to 10
	DELETE
	/v3/catalog/products
	Delete products (by ID array)
	Yes — up to 250
	GET
	/v3/catalog/products/{id}
	Get single product
	—
	PUT
	/v3/catalog/products/{id}
	Update single product
	—
	DELETE
	/v3/catalog/products/{id}
	Delete single product
	—
	GET
	/v3/catalog/products/{id}/images
	List product images
	—
	POST
	/v3/catalog/products/{id}/images
	Upload product image
	—
	DELETE
	/v3/catalog/products/{id}/images/{img_id}
	Delete product image
	—
	GET
	/v3/catalog/products/{id}/videos
	List product videos
	—
	POST
	/v3/catalog/products/{id}/videos
	Add product video
	—
	GET
	/v3/catalog/products/{id}/custom-fields
	List custom fields
	—
	POST
	/v3/catalog/products/{id}/custom-fields
	Create custom field
	—
	PUT
	/v3/catalog/products/{id}/custom-fields/{cf_id}
	Update custom field
	—
	DELETE
	/v3/catalog/products/{id}/custom-fields/{cf_id}
	Delete custom field
	—
	GET
	/v3/catalog/products/{id}/bulk-pricing-rules
	List bulk pricing rules
	—
	POST
	/v3/catalog/products/{id}/bulk-pricing-rules
	Create bulk pricing rule
	—
	GET
	/v3/catalog/products/{id}/reviews
	List product reviews
	—
	GET
	/v3/catalog/summary
	Get catalog-level summary stats
	—
	Key filterable fields on GET /products:
 keyword, sku, price, weight, condition, brand_id, date_created, date_modified, is_visible, is_featured, inventory_level, categories, channel_id
Important field notes:
* page_title → product SEO title
* meta_description → product SEO meta description
* search_keywords → comma-separated SEO search terms
* description → HTML body
* custom_url.url → product URL slug
* is_visible → boolean storefront visibility
________________


6.2 Catalog — Categories
Base path: /v3/catalog/trees
 Scope required: store_catalog_categories
V3 category trees are the current standard. The older /v3/catalog/categories endpoints are deprecated but still functional.
Method
	Endpoint
	Description
	GET
	/v3/catalog/trees
	List all category trees
	POST
	/v3/catalog/trees
	Create a category tree
	PUT
	/v3/catalog/trees
	Upsert category trees (batch)
	DELETE
	/v3/catalog/trees
	Delete category trees
	GET
	/v3/catalog/trees/{tree_id}/categories
	Get categories in a tree
	POST
	/v3/catalog/trees/categories
	Create categories (batch)
	PUT
	/v3/catalog/trees/categories
	Update categories (batch)
	DELETE
	/v3/catalog/trees/categories
	Delete categories
	GET
	/v3/catalog/categories/{id}/metafields
	List category metafields
	POST
	/v3/catalog/categories/{id}/metafields
	Create category metafield
	PUT
	/v3/catalog/categories/{id}/metafields/{mf_id}
	Update category metafield
	DELETE
	/v3/catalog/categories/{id}/metafields/{mf_id}
	Delete category metafield
	POST
	/v3/catalog/categories/{id}/image
	Upload category image
	DELETE
	/v3/catalog/categories/{id}/image
	Delete category image
	Key SEO fields on categories:
* page_title → category SEO title
* meta_description → category SEO meta description
* search_keywords → SEO keyword string
* custom_url.url → category URL slug
________________


6.3 Catalog — Brands
Base path: /v3/catalog/brands
 Scope required: store_v2_products
Method
	Endpoint
	Description
	GET
	/v3/catalog/brands
	List all brands
	POST
	/v3/catalog/brands
	Create a brand
	PUT
	/v3/catalog/brands/{id}
	Update a brand
	DELETE
	/v3/catalog/brands/{id}
	Delete a brand
	POST
	/v3/catalog/brands/{id}/image
	Upload brand image
	DELETE
	/v3/catalog/brands/{id}/image
	Delete brand image
	GET
	/v3/catalog/brands/{id}/metafields
	List brand metafields
	POST
	/v3/catalog/brands/{id}/metafields
	Create brand metafield
	Key fields: name, page_title, meta_description, search_keywords, image_url, custom_url
________________


6.4 Catalog — Variants & Options
Base path: /v3/catalog/products/{id}/variants
 Scope required: store_v2_products
Method
	Endpoint
	Description
	Batch
	GET
	/v3/catalog/products/{id}/variants
	List variants for a product
	—
	POST
	/v3/catalog/products/{id}/variants
	Create a variant
	No
	PUT
	/v3/catalog/variants
	Batch update variants
	Yes — up to 10
	GET
	/v3/catalog/products/{id}/variants/{variant_id}
	Get single variant
	—
	PUT
	/v3/catalog/products/{id}/variants/{variant_id}
	Update single variant
	—
	DELETE
	/v3/catalog/products/{id}/variants/{variant_id}
	Delete variant
	—
	POST
	/v3/catalog/products/{id}/variants/{variant_id}/image
	Upload variant image
	—
	GET
	/v3/catalog/products/{id}/options
	List product options
	—
	POST
	/v3/catalog/products/{id}/options
	Create option
	—
	PUT
	/v3/catalog/products/{id}/options/{opt_id}
	Update option
	—
	DELETE
	/v3/catalog/products/{id}/options/{opt_id}
	Delete option
	—
	GET
	/v3/catalog/products/{id}/modifiers
	List product modifiers
	—
	POST
	/v3/catalog/products/{id}/modifiers
	Create modifier
	—
	Key variant fields: sku, price, cost_price, sale_price, weight, inventory_level, image_url, purchasing_disabled
________________


6.5 Catalog — Metafields
Metafields store custom key-value data on catalog objects. Available on products, variants, categories, and brands.
Method
	Endpoint
	Description
	GET
	/v3/catalog/products/{id}/metafields
	List product metafields
	POST
	/v3/catalog/products/{id}/metafields
	Create product metafield
	PUT
	/v3/catalog/products/{id}/metafields/{mf_id}
	Update product metafield
	DELETE
	/v3/catalog/products/{id}/metafields/{mf_id}
	Delete product metafield
	GET
	/v3/catalog/products/{id}/variants/{v_id}/metafields
	List variant metafields
	POST
	/v3/catalog/products/{id}/variants/{v_id}/metafields
	Create variant metafield
	GET
	/v3/store/metafields
	List store-level metafields
	POST
	/v3/store/metafields
	Create store-level metafield
	Key fields: namespace, key, value, description, permission_set (app_only / write / read)
________________


6.6 Price Lists
Price lists allow customer-group or channel-specific pricing. The upsert endpoint is the primary bulk tool.
Base path: /v3/pricelists
 Scope required: store_price_lists
 ⚠️ Concurrency limit: Serial only — parallel upsert requests to /pricelists/{id}/records return 429.
Method
	Endpoint
	Description
	Notes
	GET
	/v3/pricelists
	List all price lists
	

	POST
	/v3/pricelists
	Create a price list
	

	PUT
	/v3/pricelists/{id}
	Update a price list
	

	DELETE
	/v3/pricelists/{id}
	Delete a price list
	

	GET
	/v3/pricelists/{id}/records
	Get all price records
	Paginated
	PUT
	/v3/pricelists/{id}/records
	Upsert price records (bulk)
	Serial — no parallel
	DELETE
	/v3/pricelists/{id}/records
	Delete price records
	

	GET
	/v3/pricelists/records
	Get records across all price lists
	(available in API, currently not surfaced in MCP)

	GET
	/v3/pricelists/assignments
	List price list assignments
	

	POST
	/v3/pricelists/assignments
	Create price list assignments (batch)
	Batch limit 25

	PUT
	/v3/pricelists/{price_list_id}/assignments
	Upsert one price list assignment
	Single assignment per call

	PUT
	/v3/pricelists/assignments
	Legacy/older docs variant (prefer explicit POST batch + /{price_list_id}/assignments upsert forms above)


	DELETE
	/v3/pricelists/assignments
	Delete price list assignments by filters

	**Implementation notes (catalog/pricelists subtree)** — MCP now ships:
	- `catalog/pricelists/*` (list/get/create/update/delete)
	- `catalog/pricelists/records/*` (list/upsert/delete)
	- `catalog/pricelists/assignments/*` (list/create_batch/upsert/delete)
	All write operations are preview→confirm, and `catalog/pricelists/records/upsert` is R2 with a conservative **100-row** cap per tool call and serial execution policy.

	________________


6.7 Orders
Base path: /v2/orders (V2 is primary for orders — V3 is expanding via GraphQL)
Scope required: store_v2_orders
Method
	Endpoint
	Description
	GET
	/v2/orders
	List all orders (filter, paginate)
	POST
	/v2/orders
	Create a manual order
	GET
	/v2/orders/{id}
	Get single order
	PUT
	/v2/orders/{id}
	Update an order
	DELETE
	/v2/orders/{id}
	Delete an order
	GET
	/v2/orders/{id}/products
	List products in an order
	GET
	/v2/orders/{id}/shipments
	List shipments for an order
	POST
	/v2/orders/{id}/shipments
	Create a shipment for an order
	GET
	/v2/orders/{id}/shipping_addresses
	List shipping addresses
	GET
	/v2/orders/{id}/messages
	Get order messages
	GET
	/v2/orders/{id}/refunds
	Get refund details
	POST
	/v3/orders/{id}/payment_actions/refund_quotes
	Create refund quote (recommended pre-step before refund)
	POST
	/v3/orders/{id}/payment_actions/refunds
	Issue a refund
	POST
	/v3/orders/{id}/payment_actions/capture
	Capture payment
	POST
	/v3/orders/{id}/payment_actions/void
	Void payment
	GET
	/v3/orders/{id}/transactions
	List transaction ledger entries for one order
	GET
	/v3/orders/{id}/metafields
	List order metafields
	POST
	/v3/orders/{id}/metafields
	Create order metafield
	PUT
	/v3/orders/{id}/metafields/{metafield_id}
	Update order metafield
	DELETE
	/v3/orders/{id}/metafields/{metafield_id}
	Delete order metafield
	GET
	/v2/order_statuses
	List all order statuses
	GET
	/v2/orders/count
	Get order count
	Key filterable fields: status_id, customer_id, email, min_date_created, max_date_created, is_deleted, payment_method, channel_id

**Implementation notes (`orders/**` subtree)** — MCP now ships:
- `orders/management/list`, `orders/management/get`, `orders/management/create`, `orders/management/update`, `orders/management/delete`, `orders/management/count`, `orders/management/statuses`, `orders/management/update_status`
- `orders/management/products/get`, `orders/management/metafields/list`, `orders/management/metafields/set`, `orders/management/metafields/delete`
- `orders/management/coupons/list`, `orders/management/shipping_addresses/list`, `orders/management/shipping_addresses/get`, `orders/management/shipping_addresses/update`, `orders/management/messages/list`, `orders/management/taxes/list`
- `orders/fulfillment/shipments/list`, `orders/fulfillment/shipments/get`, `orders/fulfillment/shipments/create`, `orders/fulfillment/shipments/update`, `orders/fulfillment/shipments/delete`
- `orders/payments/actions/list`, `orders/payments/transactions/list`, `orders/payments/capture`, `orders/payments/void`
- `orders/refunds/list`, `orders/refunds/legacy_list`, `orders/refunds/quote`, `orders/refunds/create`

All write operations in the orders subtree follow preview→confirm. Financially sensitive payment actions (`capture`, `void`, `refunds/create`) are R3 and require explicit per-order `confirmed=true`. Refund quotes are surfaced as a pre-step to reduce refund 422 failures.
________________


6.8 Order Shipments
Base path: /v2/orders/{id}/shipments
 Scope required: store_v2_orders
Method
	Endpoint
	Description
	GET
	/v2/orders/{id}/shipments
	List all shipments for an order
	POST
	/v2/orders/{id}/shipments
	Create a shipment
	PUT
	/v2/orders/{id}/shipments/{shipment_id}
	Update a shipment
	DELETE
	/v2/orders/{id}/shipments/{shipment_id}
	Delete a shipment
	GET
	/v2/shipping/shipments
	List shipments across all orders
	________________


6.9 Customers
Base path: /v3/customers
 Scope required: store_v2_customers
Method
	Endpoint
	Description
	Batch
	GET
	/v3/customers
	List all customers (cursor paginated)
	—
	POST
	/v3/customers
	Create customers
	Yes — array
	PUT
	/v3/customers
	Update customers
	Yes — array
	DELETE
	/v3/customers
	Delete customers
	Yes — id:in param
	GET
	/v3/customers/{id}
	Get single customer
	—
	GET
	/v3/customers/addresses
	List all customer addresses
	—
	POST
	/v3/customers/addresses
	Create customer addresses
	Yes — array
	PUT
	/v3/customers/addresses
	Update customer addresses
	Yes — array
	DELETE
	/v3/customers/addresses
	Delete customer addresses
	—
	GET
	/v3/customers/attributes
	List customer attribute definitions
	—
	POST
	/v3/customers/attributes
	Create attribute definition
	—
	GET
	/v3/customers/attribute-values
	Get customer attribute values
	—
	PUT
	/v3/customers/attribute-values
	Upsert customer attribute values
	Yes — array
	GET
	/v3/customers/{id}/metafields
	List customer metafields
	—
	Key fields: email, first_name, last_name, company, customer_group_id, notes, tax_exempt_category, authentication.force_password_reset
________________


6.9.1 Customer Groups (V2)
Base path: /v2/customer_groups
 Scope required: store_v2_customers
Customer Groups organize customers, control category access, and apply group-wide discount rules. The endpoint is V2-only; per-customer assignment is performed via V3 `PUT /v3/customers` using `customer_group_id` (see `customers/assign_group` and §6.9.2).
Method
	Endpoint
	Description
	Batch
	GET
	/v2/customer_groups
	List all customer groups (offset paginated)
	—
	POST
	/v2/customer_groups
	Create a customer group
	—
	GET
	/v2/customer_groups/{id}
	Get a single customer group
	—
	PUT
	/v2/customer_groups/{id}
	Update a customer group (discount_rules treated in bulk — sending the field overwrites all rules)
	—
	DELETE
	/v2/customer_groups/{id}
	Delete a customer group (members are unassigned automatically)
	—
	GET
	/v2/customer_groups/count
	Get total customer group count
	—
List filters (passed as query params): name, name:like, is_default, is_group_for_guests, date_created, date_created:min, date_created:max, date_modified, date_modified:min, date_modified:max.
Key fields:
* `name` (required on POST)
* `is_default` — auto-assigns new customers to this group
* `is_group_for_guests` — only one group can hold this flag
* `category_access`: `{type: "all"|"specific"|"none", categories: [int]}` (categories required only when type=specific)
* `discount_rules[]` — polymorphic. Two mutually exclusive modes:
  * Price-list mode: a single `{type: "price_list", price_list_id: int}` rule, no other rules allowed.
  * Discount mode: any combination of `{type: "category", method, amount, category_id}`, `{type: "product", method, amount, product_id}`, and at most one `{type: "all", method, amount}`.
* method ∈ {`percent`, `fixed`, `price`}; amount is a string-encoded float (e.g. `"5.0000"`).
Notes / quirks:
* The MCP server silently keeps the `price_list` rule and drops conflicting non-price_list rules with a warning surfaced in the tool response.
* Category discounts default to "this category and its subcategories"; the API has no toggle for "this category only" (control panel only).
* The MCP server's V2 client helpers (`GetV2`, `PostV2`, `PutV2`, `DeleteV2`) live in `internal/bigcommerce/client.go`; Customer Groups is the first V2 write surface.

6.9.2 Customer records and addresses (V3)

Base paths: `/v3/customers`, `/v3/customers/addresses`

Scope: `store_v2_customers` (read/write) or `store_v2_customers_read_only` (reads only).

Customers: GET list/search (filters such as id:in, email:in, name:like, customer_group_id:in, dates; include; sort; page/limit and cursors). POST and PUT accept arrays capped at **10** customers per call by BigCommerce. DELETE requires `id:in`. There is no GET-by-single-id; use `id:in` with one id.

Addresses: GET with filters (`customer_id:in`, `id:in`, etc.). POST and PUT accept JSON arrays. DELETE uses `id:in`.

Password writes use MCP tier **R2** with `set_password=true` and `confirmed=true` after preview.

Code: `internal/bigcommerce/customers.go`, `internal/tools/customers/customer_records.go`, `internal/tools/customers/customer_addresses_tools.go`.

6.9.3 Customer attributes, attribute values, and metafields (V3)

Base paths: `/v3/customers/attributes`, `/v3/customers/attribute-values`, `/v3/customers/{customerId}/metafields`, `/v3/customers/metafields`

Scope: `store_v2_customers` (read/write) or `store_v2_customers_read_only` (reads only).

Attributes (definitions): GET, POST, PUT, DELETE on `/v3/customers/attributes`. Each attribute carries a `name` and a `type` (one of `string`, `number`, `date`). `type` is fixed at create time — to change it, delete and recreate the attribute. Deleting an attribute cascades to every value of that attribute on every customer. POST/PUT bodies are arrays capped at **10** rows per call.

Attribute values (per customer): GET on `/v3/customers/attribute-values` (filters: `customer_id:in`, `attribute_id:in`, `attribute_value`, `attribute_value:in`). PUT upserts by `(customer_id, attribute_id)` (capped at 10 rows). DELETE uses `id:in`. BigCommerce coerces the supplied `value` to the attribute's declared type.

Metafields (per customer + cross-customer):
* `GET/POST/PUT/DELETE /v3/customers/{customerId}/metafields` — single-customer reads, upsert by namespace+key, delete by id.
* `GET/POST/PUT/DELETE /v3/customers/metafields` — list across customers (filters: `customer_id:in`, `namespace`, `namespace:in`, `namespace:like`, `key`, `key:in`), batched create/update (each row carries `resource_id`), and batch delete by `id:in`.

`permission_set` controls visibility (`app_only`, `read`, `write`, `read_and_sf_access`, `write_and_sf_access`); when omitted, BigCommerce defaults to `app_only` and the value is **not** exposed via the Storefront API.

Code: `internal/bigcommerce/customer_attributes.go`, `internal/tools/customers/attributes_tools.go`, `internal/tools/customers/attribute_values_tools.go`, `internal/tools/customers/metafields_tools.go`.

6.9.4 Customer settings, consent, stored instruments, and credential validation (V3)

**Settings** — Base paths: `/v3/customers/settings` (global) and `/v3/customers/settings/channels/{channel_id}` (per-channel overrides). Scope: `store_v2_customers` (or read-only variant for GET). Global settings cover `privacy_settings` (e.g. `ask_shopper_for_tracking_consent`, `policy_url`, `ask_shopper_for_tracking_consent_on_checkout`) and `customer_group_settings` (`guest_customer_group_id`, `default_customer_group_id`). Channel settings add **`allow_global_logins`**: when enabled on a storefront channel, customers without explicit `channel_ids` may use the same credentials across channels that also allow global logins (see BigCommerce channel-specific customers documentation). Channel `null` values inherit from global.

**Consent** — `GET` and `PUT /v3/customers/{customerId}/consent`. Request/response use `allow` and `deny` arrays whose entries are one of `essential`, `functional`, `analytics`, `targeting`.

**Stored instruments** — `GET /v3/customers/{customerId}/stored-instruments`. Response is a polymorphic list (`stored_card`, `stored_paypal_account`, `stored_bank_account`) including a **`token`** field. OAuth scopes **`store_stored_payment_instruments`** or **`store_stored_payment_instruments_read_only`** apply.

**Validate credentials** — `POST /v3/customers/validate-credentials` with JSON `{ "email", "password", "channel_id"? }`. Returns `{ "is_valid", "customer_id" }`. This endpoint has **stricter rate limiting** (HTTP 429 on abuse).

Code: `internal/bigcommerce/customer_settings_consent.go`, `internal/tools/customers/customer_settings_tools.go`, `internal/tools/customers/customer_consent_tools.go`, `internal/tools/customers/customer_stored_instruments_tools.go`, `internal/tools/customers/customer_validate_credentials_tools.go`.
________________


6.10 Customer Segmentation
Base paths: /v3/segments and /v3/shopper-profiles
 Scope required: store_v2_customers (GET endpoints accept store_v2_customers_read_only **except** the segment-membership GET noted below).
 Plan: **Enterprise-only** — non-enterprise stores receive 403 on every endpoint in this family.
 Generally Available (GA) as of late 2024.
Method
	Endpoint
	Description
	GET
	/v3/segments
	List all segments (paginated; supports id:in (UUIDs), page, limit)
	POST
	/v3/segments
	Bulk create segments — body is `[{name, description?}]`. Limits: 10 concurrent requests; max **1000** total segments per store.
	PUT
	/v3/segments
	Bulk update segments — body is `[{id, name?, description?}]`; 10 concurrent requests.
	DELETE
	/v3/segments?id:in=…
	Bulk delete segments. **Does not delete the associated shopper profiles** — only the segment metadata.
	GET
	/v3/segments/{segmentId}/shopper-profiles
	List shopper profiles in a segment. **Requires the modify Customers OAuth scope** (`store_v2_customers`) — the only GET in this family that does so.
	POST
	/v3/segments/{segmentId}/shopper-profiles
	Add profiles to a segment. Body is an array of shopper-profile UUIDs; max **50** per request; 10 concurrent.
	DELETE
	/v3/segments/{segmentId}/shopper-profiles?id:in=…
	Disassociate profiles from a segment without deleting the profiles themselves.
	GET
	/v3/shopper-profiles
	Paginated list of shopper profiles. **No id:in filter or single-profile GET** — to look up a profile by customer use `GET /v3/customers?id:in=…&include=shopper_profile_id`.
	POST
	/v3/shopper-profiles
	Bulk-create profiles. Body is `[{customer_id}]`. Each registered customer is 1:1 with a profile; duplicates 409.
	DELETE
	/v3/shopper-profiles?id:in=…
	Delete profiles and all of their segment memberships. Customer records themselves are unaffected.
	GET
	/v3/shopper-profiles/{shopperProfileId}/segments
	List the segments containing a profile.
	**Implementation notes** — Segment IDs are UUID strings; we expose an MCP `customers/segments/get` that wraps `id:in={uuid}` for parity with `customers/get`. The shopper-add tool accepts either `shopper_profile_ids` (UUIDs) or `customer_ids` (numeric, max 50/call) and resolves them via `customers?include=shopper_profile_id`; customers without an existing profile are surfaced under `missing_shopper_profiles` rather than silently dropped. Per-tool numeric caps: segments create/update **10**/call, segments delete **40**/call, shopper-add **50**/call, shopper-remove **40**/call, shopper-profile create **50**/call, shopper-profile delete **40**/call. The 403 scope hint additionally calls out the Enterprise plan requirement so non-enterprise stores get a clear failure mode.
	Code: `internal/bigcommerce/segments.go`, `internal/tools/customers/segments_tools.go`, `internal/tools/customers/shopper_profiles_tools.go`.
	________________


6.11 Cart & Checkout (REST Storefront)
Note: These are client-side APIs intended for use within browser-hosted storefronts. For server-side cart management, use the REST Management Cart API (/v3/carts).
REST Management — Carts
 Base path: /v3/carts
 Scope required: store_cart
Method
	Endpoint
	Description
	POST
	/v3/carts
	Create a cart
	GET
	/v3/carts/{id}
	Get a cart
	PUT
	/v3/carts/{id}
	Update a cart
	DELETE
	/v3/carts/{id}
	Delete a cart
	POST
	/v3/carts/{id}/items
	Add item(s) to cart
	PUT
	/v3/carts/{id}/items/{item_id}
	Update cart item
	DELETE
	/v3/carts/{id}/items/{item_id}
	Remove cart item
	POST
	/v3/carts/{id}/redirect_urls
	Create cart redirect URLs
	GET
	/v3/carts/{id}/metafields
	List cart metafields
	REST Management — Checkouts
 Base path: /v3/checkouts
 Scope required: store_checkouts
Method
	Endpoint
	Description
	GET
	/v3/checkouts/{id}
	Get checkout details
	PUT
	/v3/checkouts/{id}
	Update checkout
	POST
	/v3/checkouts/{id}/coupons
	Apply coupon
	DELETE
	/v3/checkouts/{id}/coupons/{code}
	Remove coupon
	POST
	/v3/checkouts/{id}/billing-address
	Set billing address
	PUT
	/v3/checkouts/{id}/billing-address/{addr_id}
	Update billing address
	POST
	/v3/checkouts/{id}/consignments
	Add consignment (shipping)
	PUT
	/v3/checkouts/{id}/consignments/{consign_id}
	Update consignment
	POST
	/v3/checkouts/{id}/orders
	Convert checkout to order
	________________


6.12 Channels & Multi-Storefront (MSF)
Base path: /v3/channels
 Scope required: store_channel_settings
Method
	Endpoint
	Description
	GET
	/v3/channels
	List all channels
	POST
	/v3/channels
	Create a channel
	GET
	/v3/channels/{id}
	Get a channel
	PUT
	/v3/channels/{id}
	Update a channel
	DELETE
	/v3/channels/{id}
	Delete a channel
	GET
	/v3/channels/{id}/site
	Get site attached to channel
	PUT
	/v3/channels/{id}/site
	Upsert channel site
	DELETE
	/v3/channels/{id}/site
	Delete channel site
	GET
	/v3/channels/{id}/currency-assignments
	Get currency assignments
	PUT
	/v3/channels/{id}/currency-assignments
	Update currency assignments
	GET
	/v3/channels/{id}/listings
	Get channel product listings
	PUT
	/v3/channels/{id}/listings
	Upsert channel listings
	GET
	/v3/catalog/products/{id}/channel-assignments
	Get product channel assignments
	PUT
	/v3/catalog/products/{id}/channel-assignments
	Update product channel assignments
	GET
	/v3/catalog/products/{id}/category-assignments
	Get product category assignments
	PUT
	/v3/catalog/products/{id}/category-assignments
	Upsert product category assignments
	________________


6.13 Inventory & Locations
Base path: /v3/inventory
 Scope required: store_inventory
Method
	Endpoint
	Description
	Batch
	GET
	/v3/inventory/locations
	List inventory locations
	—
	POST
	/v3/inventory/locations
	Create a location
	—
	PUT
	/v3/inventory/locations/{id}
	Update a location
	—
	DELETE
	/v3/inventory/locations/{id}
	Delete a location
	—
GET
	/v3/inventory/locations/{id}/metafields
	List location metafields
	—
POST
	/v3/inventory/locations/{id}/metafields
	Create location metafield
	—
PUT
	/v3/inventory/locations/{id}/metafields/{metafield_id}
	Update location metafield
	—
DELETE
	/v3/inventory/locations/{id}/metafields/{metafield_id}
	Delete location metafield
	—
	GET
	/v3/inventory/items
	List inventory items
	—
	PUT
	/v3/inventory/items
	Batch update inventory items
	Yes
	POST
	/v3/inventory/adjustments/absolute
	Set absolute inventory levels
	Yes — array
	POST
	/v3/inventory/adjustments/relative
	Adjust inventory by delta
	Yes — array
	GET
	/v3/inventory/items/{variant_id}
	Get inventory for a variant
	—
	Key fields on inventory items: location_id, product_id, variant_id, quantity, safety_stock, is_in_stock, bin_picking_number

**Implementation notes (`inventory/**` subtree)** — MCP now ships:
- `inventory/locations/list`
- `inventory/locations/create`
- `inventory/locations/update`
- `inventory/locations/delete`
- `inventory/locations/metafields/list`
- `inventory/locations/metafields/set`
- `inventory/locations/metafields/delete`
- `inventory/items/list`
- `inventory/items/get`
- `inventory/items/update_batch`
- `inventory/adjustments/absolute`
- `inventory/adjustments/relative`

Inventory location create/update and inventory item/adjustment writes are surfaced as **R2** preview→confirm operations. Location metafield set/delete are **R1** preview→confirm operations. `inventory/locations/delete` remains **R3** destructive (preview→confirm). Batch row caps remain 10 for `inventory/items/update_batch` and adjustment tools.
________________


6.14 Promotions & Coupons
Base path: /v3/promotions (and /v3/promotions/{id}/codes for coupon codes; /v2/coupons is the legacy V2 surface)
 Scope required: store_v2_marketing (writes); store_v2_marketing_read_only is sufficient for GETs.
 Default rate limit: 40 concurrent requests on most endpoints. Notable exceptions: `GET /v3/promotions/{id}/codes` and `POST /v3/promotions/{id}/codegen` default to 10 concurrent.
 Default `redemption_type`s: AUTOMATIC and COUPON. The field is **read-only after create** — PUT cannot flip it.
Method
	Endpoint
	Description
	GET
	/v3/promotions
	List promotions. Filters: `id`, `name`, `code`, `query` (matches name or code), `currency_code`, `redemption_type` (`automatic`|`coupon`), `status` (`ENABLED`|`DISABLED`|`INVALID`), `channels` (CSV of channel IDs). Sort: `id|name|priority|start_date` × `asc|desc`. Page/limit pagination (default limit 50).
	POST
	/v3/promotions
	Create one promotion. Single-record (not bulk). Body must include `rules[]` (one or more) plus `redemption_type` and other top-level fields.
	GET
	/v3/promotions/{id}
	Get a single promotion.
	PUT
	/v3/promotions/{id}
	Replace a promotion. **`redemption_type` is read-only.** PUT replaces the entire document; partial updates require fetch + merge on the caller side.
	DELETE
	/v3/promotions?id:in=…
	**Bulk delete** (max 50 ids/call). 422 if any promotion still has coupon codes attached — delete the codes first via `/v3/promotions/{id}/codes` before the promotion itself.
	GET
	/v3/promotions/{id}/codes
	List coupon codes attached to a promotion. **Cursor-paginated** via `before`/`after` (not page/limit); BigCommerce default rate limit is **10 concurrent** here (lower than other coupon endpoints).
	POST
	/v3/promotions/{id}/codes
	Create a single coupon code. Body: `code` (required, ≤50 chars; allowed: letters / numbers / spaces / underscores / hyphens), `max_uses` (0 = unlimited; **parent promotion's `max_uses` overrides**), `max_uses_per_customer` (0 = unlimited). **No PUT** — coupon codes are immutable; to change a code, delete and recreate.
	POST
	/v3/promotions/{id}/codegen
	Generate a batch of coupon codes for a `coupon_type=BULK` promotion. Body: `batch_size` (≤250), `prefix`, `suffix`, `length` (6..16, excludes prefix/suffix), `format` (`NUMBERS`|`LETTERS`|`ALPHANUMERIC`), `separator`. SINGLE-type promotions are rejected with 422.
	DELETE
	/v3/promotions/{id}/codes?id:in=…
	Bulk-delete coupon codes by id (max **50**/call per BC).
	GET
	/v2/coupons
	Legacy V2 coupons surface. Cursor-paginated as of Jan 2025; superseded by `/v3/promotions` + `/codes` for new builds.
	POST
	/v2/coupons
	Create a V2 coupon.
	PUT
	/v2/coupons/{id}
	Update a V2 coupon.
	DELETE
	/v2/coupons/{id}
	Delete a V2 coupon.
	GET
	/v3/promotions/settings
	Get store-wide promotion settings. Current live shape includes:
	`promotions_triggered_by_products_with_zero_product_price` (bool),
	`promotions_apply_on_products_with_custom_product_price` (bool),
	`number_of_coupons_allowed_at_checkout` (int 1..5; >1 is Enterprise-only),
	`promotions_applied_on_original_product_price` (bool).
	PUT
	/v3/promotions/settings
	Update store-wide promotion settings (full document replacement).
	**Rule engine shape** — each `rules[]` entry has exactly one of five `action` shapes: `cart_items` (per-line discount + `strategy ∈ LEAST_EXPENSIVE|MOST_EXPENSIVE`, `as_total`, `items` matcher), `cart_value` (order-level discount), `shipping` (`free_shipping: true`, optional `zone_ids`/`shipping_methods`), `gift_item` (`product_id`, `quantity`), `fixed_price_set` (`price` + `items`). `discount` accepts exactly one of `percentage_amount` | `fixed_amount` (string-encoded). `condition` is recursively polymorphic — `cart` (with optional `items` matcher, `minimum_quantity`, `minimum_spend`) plus `and|or|not` operators. `items` matchers accept the leaves `products|categories|brands|variants` (non-empty integer arrays) plus the same `and|or|not` operators. `customer.group_ids` and `customer.excluded_group_ids` are mutually exclusive (BC 422). `notifications[].type ∈ PROMOTION|UPSELL|ELIGIBLE|APPLIED`; `locations ∈ HOME_PAGE|PRODUCT_PAGE|CART_PAGE|CHECKOUT_PAGE`. **BigCommerce recommends ≤ 10 rules per promotion and < 100 active promotions per store** (above that the checkout slows / risks OOM).
	**Coupon-only outer fields** (only valid when `redemption_type=COUPON`): `coupon_type ∈ SINGLE | BULK` (default SINGLE), `coupon_overrides_other_promotions` (only valid when `can_be_used_with_other_promotions=false`; BC 422 otherwise), `multiple_codes` (BULK only), and the **deprecated** `coupon_overrides_automatic_when_offering_higher_discounts` (BC says use `coupon_overrides_other_promotions` instead).
	**Implementation notes (automatic subtree)** — The `marketing/promotions/automatic/*` tools hard-pin `redemption_type=AUTOMATIC`. The MCP `create` tool always overrides `redemption_type` to `AUTOMATIC` so coupon promotions can't be created through it; the `get` and `update` tools refuse to operate on a promotion whose stored `redemption_type` is `COUPON` and point at the coupon subtree. Per-tool numeric caps: delete **40** ids/call (under BC's 50). Soft-warn surfaces in `create` previews when the store already has ≥ 100 ENABLED promotions. Update merges top-level scalars and supports positional rule edits via `rules_patch=[{index, replace_with}]`; sending the entire `rules` array via `patch.rules` replaces it in full and emits a warning. The 403 scope hint additionally calls out `store_v2_marketing` so misscoped tokens get a clear failure mode.
	**Implementation notes (coupon subtree + coupon codes)** — The `marketing/promotions/coupon/*` tools hard-pin `redemption_type=COUPON`, with coupon-code lifecycle under `marketing/promotions/coupon/codes/*`. The validator extends the automatic subtree's deep-shape checks with redemption-type-aware cross-field rules: rejects the deprecated `coupon_overrides_automatic_when_offering_higher_discounts` outright (per design choice — REJECT mode), enforces the `coupon_overrides_other_promotions=true ⇒ can_be_used_with_other_promotions=false` constraint, validates `coupon_type ∈ SINGLE | BULK`, and rejects `multiple_codes` on SINGLE promotions. **Coupon codes are immutable** — there is no PUT on `/codes`; tools surface a "delete and recreate" message. `coupon/delete` accepts an optional `delete_codes_first=true` flag that walks each promotion's codes via cursor pagination and deletes them in chunks of **40** before deleting the promotion; the cascade is bounded at **1000 codes per promotion** to keep a single invocation reviewable. `codes/generate_bulk` pre-flights the parent's `coupon_type=BULK` and refuses on SINGLE; `batch_size` is hard-capped at **250** (BC's per-call limit). `codes/create_single` validates the `code` charset client-side (letters/numbers/spaces/underscores/hyphens, ≤50 chars) and surfaces a warning when the parent promotion's `max_uses` will override the code's. `codes/delete` caps at **40** ids/call (under BC's documented 50).
	**Implementation notes (settings subtree)** — `marketing/promotions/settings/*` adds `get` (R0) and `update` (R2). Update uses fetch-merge-PUT over the four live fields listed above, validates `number_of_coupons_allowed_at_checkout` to 1..5, type-checks booleans, soft-warns (but does not block) when setting coupon count > 1 (Enterprise-only), and returns `noop` without issuing PUT when the requested patch matches current settings.
	Code: `internal/bigcommerce/promotions.go`, `internal/bigcommerce/coupon_codes.go`, `internal/bigcommerce/types.go`, `internal/tools/promotions/automatic_tools.go`, `internal/tools/promotions/coupon_tools.go`, `internal/tools/promotions/coupon_codes_tools.go`, `internal/tools/promotions/settings_tools.go`, `internal/tools/promotions/validation.go`.
	________________


6.15 Shipping
Base path: /v2/shipping
 Scope required: store_shipping
Method
	Endpoint
	Description
	GET
	/v2/shipping/zones
	List shipping zones
	POST
	/v2/shipping/zones
	Create a shipping zone
	PUT
	/v2/shipping/zones/{id}
	Update a shipping zone
	DELETE
	/v2/shipping/zones/{id}
	Delete a shipping zone
	GET
	/v2/shipping/zones/{zone_id}/methods
	List methods in a zone
	POST
	/v2/shipping/zones/{zone_id}/methods
	Create a shipping method
	PUT
	/v2/shipping/zones/{zone_id}/methods/{method_id}
	Update a method
	DELETE
	/v2/shipping/zones/{zone_id}/methods/{method_id}
	Delete a method
	GET
	/v2/shipping/carriers
	List available carrier connections
	PUT
	/v2/shipping/settings
	Update shipping settings (origins)
	________________


6.16 Tax
Base path: /v3/tax
 Scope required: store_v2_information
Method
	Endpoint
	Description
	GET
	/v3/tax/classes
	List tax classes
	POST
	/v3/tax/classes
	Create a tax class
	GET
	/v3/tax/providers
	List connected tax providers
	GET
	/v3/pricing/products
	Get pricing with tax applied (for geolocation)
	POST
	/v3/tax-rates
	Create/update tax rates
	________________


6.17 Payments
Base path: /v2/payments and /v3/payments
 Scope required: store_payments_methods_read
Method
	Endpoint
	Description
	GET
	/v2/payments/methods
	List available payment methods
	POST
	/v3/payments/access_tokens
	Create a payment access token
	POST
	/payments
	Process payment (via payment gateway base URL)
	GET
	/v3/orders/{id}/payment_actions
	Get payment actions for order
	POST
	/v3/orders/{id}/payment_actions/capture
	Capture authorized payment
	POST
	/v3/orders/{id}/payment_actions/refund
	Issue order refund
	POST
	/v3/orders/{id}/payment_actions/void
	Void authorized payment
	________________


6.18 Store Settings
Base path: /v2/store and /v3/settings
 Scope required: store_v2_information
Method
	Endpoint
	Description
	GET
	/v2/store
	Get store information (name, address, etc.)
	GET
	/v3/settings/storefront
	Get storefront display settings
	PUT
	/v3/settings/storefront
	Update storefront settings
	GET
	/v3/settings/store/locale
	Get locale/currency settings
	PUT
	/v3/settings/store/locale
	Update locale settings
	GET
	/v3/settings/analytics
	Get analytics provider settings
	PUT
	/v3/settings/analytics
	Update analytics settings
	GET
	/v3/settings/SEO
	Get SEO settings
	PUT
	/v3/settings/SEO
	Update SEO settings
	GET
	/v3/settings/favicon
	Get favicon
	PUT
	/v3/settings/favicon
	Update favicon
	GET
	/v3/store/metafields
	Get store-level metafields
	POST
	/v3/store/metafields
	Create store-level metafield
	PUT
	/v3/store/metafields/{id}
	Update store-level metafield
	DELETE
	/v3/store/metafields/{id}
	Delete store-level metafield
	GET
	/v3/currencies
	List currencies
	POST
	/v3/currencies
	Create a currency
	PUT
	/v3/currencies/{code}
	Update a currency
	DELETE
	/v3/currencies/{code}
	Delete a currency
	________________


6.19 Scripts & Content (Storefront)
Base path: /v3/content
 Scope required: store_content
Method
	Endpoint
	Description
	GET
	/v3/content/scripts
	List all scripts
	POST
	/v3/content/scripts
	Create a script
	GET
	/v3/content/scripts/{uuid}
	Get a script
	PUT
	/v3/content/scripts/{uuid}
	Update a script
	DELETE
	/v3/content/scripts/{uuid}
	Delete a script
	GET
	/v3/content/pages
	List content pages
	POST
	/v3/content/pages
	Create a content page
	PUT
	/v3/content/pages
	Batch update pages
	GET
	/v3/content/pages/{id}
	Get a page
	PUT
	/v3/content/pages/{id}
	Update a page
	DELETE
	/v3/content/pages/{id}
	Delete a page
	GET
	/v3/content/redirects
	List URL redirects
	PUT
	/v3/content/redirects
	Upsert URL redirects (bulk)
	DELETE
	/v3/content/redirects
	Delete URL redirects
	GET
	/v3/widgets
	List widgets
	POST
	/v3/widgets
	Create a widget
	GET
	/v3/widgets/{uuid}
	Get a widget
	PUT
	/v3/widgets/{uuid}
	Update a widget
	DELETE
	/v3/widgets/{uuid}
	Delete a widget
	GET
	/v3/widget-templates
	List widget templates
	POST
	/v3/widget-templates
	Create a widget template
	GET
	/v3/placements
	List widget placements
	POST
	/v3/placements
	Create a widget placement
	________________


6.20 Themes
Base path: /v3/themes
 Scope required: store_themes_manage
Method
	Endpoint
	Description
	GET
	/v3/themes
	List installed themes
	POST
	/v3/themes
	Upload a theme (.zip)
	GET
	/v3/themes/{uuid}
	Get a theme
	DELETE
	/v3/themes/{uuid}
	Delete a theme
	POST
	/v3/themes/{uuid}/actions/activate
	Activate a theme
	POST
	/v3/themes/{uuid}/actions/download
	Download a theme
	GET
	/v3/themes/jobs/{job_id}
	Get theme job status (async)
	________________


6.21 Webhooks
Base path: /v3/hooks
 Scope required: store_v2_information (plus scopes for the events being subscribed to)
Method
	Endpoint
	Description
	GET
	/v3/hooks
	List all webhooks
	POST
	/v3/hooks
	Create a webhook (one at a time)
	GET
	/v3/hooks/{id}
	Get a webhook
	PUT
	/v3/hooks/{id}
	Update a webhook
	DELETE
	/v3/hooks/{id}
	Delete a webhook
	GET
	/v3/hooks/{id}/events
	Get recent events for a webhook
	Key webhook event scopes (subscribe to these for agentic triggers):
Event
	Fires When
	store/product/created
	Product created
	store/product/updated
	Product updated
	store/product/inventory/updated
	Inventory level changes
	store/category/created
	Category created
	store/order/created
	New order placed
	store/order/statusUpdated
	Order status changes
	store/order/archived
	Order archived
	store/customer/created
	New customer registered
	store/cart/created
	Cart created
	store/cart/itemAdded
	Item added to cart
	store/shipment/created
	Shipment created
	Webhook response requirement: Your endpoint must return HTTP 200 within 10 seconds or BigCommerce will retry the delivery.
________________


6.22 GraphQL APIs
GraphQL Storefront API
Endpoint: https://store-{store_hash}.mybigcommerce.com/graphql
 Auth: Customer JWT or anonymous storefront token
Best for: Headless storefronts, Catalyst framework, flexible product queries with fewer round trips
Key capabilities:
* Query products, categories, brands, variants in a single request
* Cart and checkout mutations
* Customer authentication and data
* Wishlist management
* Bi-directional cursor pagination
GraphQL Admin API (Expanding)
Endpoint: https://api.bigcommerce.com/stores/{store_hash}/graphql
 Auth: X-Auth-Token (same as REST)
Best for: Complex catalog queries, admin mutations where REST endpoints don't yet have batch support
Notable available mutations:
* createProductReview, updateProductReview, deleteProductReview
* Order-related queries and event hooks
* updateSettings, updateStorefrontSettings
LLM Note: GraphQL Admin API is actively expanding. Prefer REST V3 for bulk write operations; use GraphQL for reads that would otherwise require multiple REST calls.
________________


7. Response Headers Reference
Header
	Description
	Use In Agent
	X-Rate-Limit-Requests-Left
	Remaining requests in window
	Gate next batch; pause if < 20
	X-Rate-Limit-Requests-Quota
	Total quota for window
	Calculate safe request rate
	X-Rate-Limit-Time-Window-Ms
	Window size in ms
	Denominator for rate calculation
	X-Rate-Limit-Time-Reset-Ms
	Ms until quota reset
	Sleep duration on 429
	X-Retry-After
	Seconds to wait after 429
	Alternative retry signal
	Content-Type
	Response MIME type
	Should always be application/json
	Link
	Pagination cursor links
	Parse next cursor from this header
	X-Correlation-Id
	Request trace ID
	Pass in multi-step workflows
	________________


8. Error Codes Reference
HTTP Status
	Meaning
	Agent Action
	200 OK
	Success
	Continue
	201 Created
	Resource created
	Capture returned ID
	204 No Content
	Success, no body (DELETE)
	Continue
	400 Bad Request
	Validation failure
	Parse error body; fix payload
	401 Unauthorized
	Missing/invalid token
	Check X-Auth-Token; verify scopes
	403 Forbidden
	Insufficient scope
	Check token scopes for this resource
	404 Not Found
	Resource doesn't exist
	Verify ID; skip or flag
	405 Method Not Allowed
	Wrong HTTP method
	Check endpoint definition
	409 Conflict
	Duplicate resource (e.g. SKU)
	Handle collision; check existing
	422 Unprocessable Entity
	Semantic validation failure
	Parse errors array in response body
	429 Too Many Requests
	Rate/concurrency limit hit
	Wait X-Rate-Limit-Time-Reset-Ms; backoff
	500 Internal Server Error
	BigCommerce-side error
	Retry with backoff; log
	503 Service Unavailable
	Platform overload
	Retry with exponential backoff
	509 Bandwidth Limit Exceeded
	Legacy rate limit signal
	Same as 429 — pause and retry
	________________


9. LLM Tool Design Guidelines
This section describes how to translate BigCommerce API categories into tool definitions for an LLM agent.
Tool Naming Convention
Use the pattern: {action}_{resource} in snake_case.
Examples: get_products, bulk_update_products, upsert_price_records, create_order_shipment
Recommended Tool Groupings
Tool Group
	Underlying APIs
	Priority for Agentic Use
	Catalog Read
	GET /products, /categories, /brands
	High — used before any write operation
	Product SEO Update
	PUT /products (batch), metafields
	High — core use case
	Inventory Management
	/inventory/items, /inventory/adjustments
	High
	Price Management
	/pricelists, /pricelists/{id}/records
	High (serial only)
	Order Management
	/orders, /orders/{id}/shipments
	High
	Category Management
	/catalog/trees, /catalog/trees/categories
	Medium
	Customer Data
	/customers, /segments
	Medium
	Content & Redirects
	/content/pages, /content/redirects
	Medium
	Store Settings
	/settings/SEO, /settings/storefront
	Low-Medium
	Webhook Management
	/hooks
	Low (setup only)
	Theme Management
	/themes
	Low
	Tool Definition Template (for MCP or custom agent tool)
{
  "name": "bulk_update_products_seo",
  "description": "Updates SEO fields (page_title, meta_description, search_keywords, custom_url) for up to 10 products in a single API call.",
  "parameters": {
    "products": {
      "type": "array",
      "maxItems": 10,
      "items": {
        "type": "object",
        "required": ["id"],
        "properties": {
          "id": { "type": "integer" },
          "page_title": { "type": "string", "maxLength": 250 },
          "meta_description": { "type": "string", "maxLength": 650 },
          "search_keywords": { "type": "string" },
          "custom_url": {
            "type": "object",
            "properties": {
              "url": { "type": "string" },
              "is_customized": { "type": "boolean" }
            }
          }
        }
      }
    }
  }
}
Agent Safety Rules
1. Always GET before PUT/DELETE — never mutate data the agent hasn't first read and confirmed.
2. Surface diffs for review — for bulk updates, show a before/after diff to the operator before committing.
3. Hard limit on concurrent writes — cap write threads at 3–5 unless endpoint-specific documentation states higher is safe.
4. Never parallelize price list upserts — always serial.
5. Log all mutations — store request/response pairs with timestamps for audit and rollback.
6. Prefer soft deletes — set is_visible: false on products rather than hard deleting; deleted products are unrecoverable.
7. Paginate exhaustively before bulk writes — always pull all pages of a resource before deciding which records to modify.
8. Honor X-Rate-Limit-Requests-Left — when remaining requests drop below 20, pause the batch until quota resets.
________________


Document generated for use as a project reference file. To be uploaded as a knowledge artifact for LLM-assisted BigCommerce store management workflows.
Sources: BigCommerce Developer Center (developer.bigcommerce.com), BigCommerce API Best Practices docs, official rate limit documentation.