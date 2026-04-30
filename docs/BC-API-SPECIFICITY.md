# BigCommerce API Specificity

Field-level quirks, undocumented behaviors, and response shape differences discovered during development and testing. This file is the living reference for BigCommerce API behavior that deviates from what the official documentation suggests or leaves ambiguous.

---

## Table of Contents

1. [Category Tree vs Legacy Categories](#1-category-tree-vs-legacy-categories)
2. [Variant Price Inheritance (price: 0)](#2-variant-price-inheritance-price-0)
3. [CustomURL Object Pattern](#3-customurl-object-pattern)
4. [Product price vs calculated_price](#4-product-price-vs-calculated_price)
5. [Query Parameter Filter Operators](#5-query-parameter-filter-operators)
6. [Category Trees Batch Update Endpoint (PUT = Updates Only)](#6-category-trees-batch-update-endpoint-put--updates-only)
7. [Category Creation: POST, Not PUT](#7-category-creation-post-not-put)
8. [Category Deletion: Cascade Behavior & Product Impact](#8-category-deletion-cascade-behavior--product-impact)
9. [Parent Name Resolution Pattern](#9-parent-name-resolution-pattern)
10. [Empty Filter Query Parameter Bug](#10-empty-filter-query-parameter-bug)

---

## 1. Category Tree vs Legacy Categories

**Discovered:** 2026-04-13 during initial MCP tool testing
**Affected files:** `internal/bigcommerce/types.go`, `internal/bigcommerce/products.go`

BigCommerce has two category API families with **different response shapes**. The legacy endpoints are deprecated but still functional. Our server uses the Category Tree endpoints exclusively.

### Endpoint Comparison

| | Category Tree (current) | Legacy Categories (deprecated) |
|---|---|---|
| **Base path** | `/v3/catalog/trees/categories` | `/v3/catalog/categories` |
| **ID field** | `category_id` | `id` |
| **URL field** | Object: `{"url": "/path/", "is_customized": false}` | String: `"/path/"` |
| **Structure** | Hierarchical within named trees | Flat list with `parent_id` |
| **Multi-storefront** | Yes — categories belong to trees | No |

### The `category_id` vs `id` Problem

The Category Tree endpoint returns `category_id` as the primary identifier, not `id`. However, every other BigCommerce endpoint that references categories (e.g., product `categories` array, filter params like `categories:in=`) uses the **numeric value** from `category_id` but calls it by different names depending on context.

**Our fix:** Custom `UnmarshalJSON` on the `Category` struct that reads both `category_id` and `id`, preferring `category_id` when present.

```go
// From internal/bigcommerce/types.go
func (c *Category) UnmarshalJSON(data []byte) error {
    type alias Category
    aux := &struct {
        *alias
        AltID int `json:"id,omitempty"`
    }{alias: (*alias)(c)}
    if err := json.Unmarshal(data, aux); err != nil {
        return err
    }
    if c.ID == 0 && aux.AltID != 0 {
        c.ID = aux.AltID
    }
    return nil
}
```

### The `url` Field Shape

The Category Tree endpoint returns `url` as a JSON object, not a string:

```json
{
  "category_id": 408,
  "name": "Shop All",
  "url": {
    "url": "/shop-all/",
    "is_customized": false
  }
}
```

The legacy endpoint returned it as a plain string: `"url": "/shop-all/"`.

**Our fix:** Changed the `Category.URL` field from `string` to `*CustomURL` (the same struct already used by `Product`).

---

## 2. Variant Price Inheritance (price: 0)

**Discovered:** 2026-04-13 during product/variant traversal testing
**Affected files:** `internal/tools/catalog/products.go`

### Behavior

When a BigCommerce variant has `price: 0`, it does **not** mean the product variant is free. It means the variant **inherits the product-level price**. The actual selling price is determined by a separate `calculated_price` field.

### Example from Live Data

```json
{
  "product": {
    "id": 19186,
    "name": "BigCommerce Super Soft T-Shirt",
    "price": 22.00
  },
  "variants": [
    {"id": 24559, "sku": "SKU-BBA08BA0", "price": 0},
    {"id": 24560, "sku": "SKU-856D7E8B", "price": 0}
  ]
}
```

These variants sell for $22.00 (the product price), not $0.00. The `price: 0` is BigCommerce's way of saying "no variant-level override."

### Impact on Our Code

**Status:** Fixed (2026-04-13)

The original `has_variant_pricing` detection in `handleGet` (products.go) checked `v.Price != product.Price`, which incorrectly flagged `price: 0` variants as having different pricing when they're actually inheriting. This was fixed across three locations:

| Location | Function | Change |
|---|---|---|
| `handleGet` | Variant pricing detection | `v.Price != product.Price` → `v.Price != 0` |
| `previewBulkPriceUpdate` | Variant counting for preview | `v.Price != prod.Price` → `v.Price != 0` |
| `updateVariantPrices` | Variant update filtering | `v.Price != prod.Price` → `v.Price != 0` |

The fix uses `v.Price != 0` (not `v.Price != 0 && v.Price != product.Price`) because:
- `price: 0` always means "inherit" — these variants should never be directly updated
- When updating a product's base price, `price: 0` variants automatically inherit the new price
- Only variants with an explicit merchant-set price (`> $0`) receive the adjustment

### Fields to Be Aware Of

| Field | Meaning |
|---|---|
| `price` | The explicit variant price override. `0` = inherit from product. |
| `calculated_price` | The actual selling price after inheritance, rules, and sale logic. Not currently fetched by our client. |
| `sale_price` | Explicit sale price override at variant level. Also supports `0` = inherit. |
| `retail_price` | MSRP/compare-at price at variant level. |
| `map_price` | Minimum advertised price. |

### Recommendation

For operations that only need to know the selling price (e.g., reporting), request `calculated_price` via `include_fields` to get the true selling price. For mutation operations (price adjustments), our current approach of checking `v.Price != 0` is correct — it cleanly identifies which variants have explicit overrides vs inherited pricing.

---

## 3. CustomURL Object Pattern

**Discovered:** 2026-04-13 during category deserialization
**Affected files:** `internal/bigcommerce/types.go`

### Behavior

Any BigCommerce entity with a storefront-facing URL returns a `url` or `custom_url` field as a **JSON object**, not a string:

```json
{
  "url": "/some-path/",
  "is_customized": true
}
```

The `is_customized` field indicates whether the merchant manually set the URL slug or BigCommerce auto-generated it from the entity name.

### Affected Entity Types

| Entity | Field Name | Endpoint |
|---|---|---|
| Products | `custom_url` | `/v3/catalog/products` |
| Categories | `url` | `/v3/catalog/trees/categories` |
| Brands | `custom_url` | `/v3/catalog/brands` |
| Pages | `url` | `/v3/content/pages` |

Note the inconsistency: Products and Brands call it `custom_url`, while Categories and Pages call it `url`.

Additionally, the **inner field name** differs:
- Products/Brands: `"url": "/some-path/"`
- Categories/Pages: `"path": "/some-path/"`

### Our Struct

**Status:** Fixed (2026-04-13)

```go
type CustomURL struct {
    URL          string `json:"url"`
    Path         string `json:"path"`
    IsCustomized bool   `json:"is_customized"`
}

func (u *CustomURL) GetPath() string {
    if u.Path != "" {
        return u.Path
    }
    return u.URL
}
```

The struct captures both `url` and `path` fields. The `GetPath()` helper returns whichever is populated, so callers don't need to know which API shape they're dealing with. Reused across `Product.CustomURL` and `Category.URL`. When adding new entity types (Brands, Pages), use this same struct.

---

## 4. Product `price` vs `calculated_price`

**Discovered:** 2026-04-13 during bulk price update testing
**Affected files:** `internal/bigcommerce/types.go`, `internal/tools/catalog/products.go`

### Behavior

BigCommerce products have multiple price fields with distinct meanings:

| Field | Meaning |
|---|---|
| `price` | Base catalog price — the raw value set on the product |
| `calculated_price` | Actual selling price after price lists, customer group rules, sale pricing, and bulk rules |
| `sale_price` | Explicit sale override (`0` = no sale) |
| `retail_price` | MSRP / compare-at price (`0` = none) |
| `map_price` | Minimum advertised price (`0` = none) |
| `cost_price` | Wholesale / cost basis (`0` = none) |

For stores without price lists or customer group pricing, `price` and `calculated_price` are typically identical. When they differ, `calculated_price` is what the storefront displays and what the merchant sees in the admin panel.

### Example from Live Data

```json
{
  "id": 19538,
  "name": "Custom Mug",
  "price": 29.99,
  "calculated_price": 29.99,
  "sale_price": 0,
  "retail_price": 0,
  "map_price": 0,
  "cost_price": 0
}
```

### Impact on Our Code

- The `Product` struct now includes `CalculatedPrice`, `RetailPrice`, and `MapPrice`
- `ListProductsByCategory` requests `calculated_price` via `include_fields`
- Preview responses include `calculated_price` when it differs from `price`, so the merchant sees the price they recognize
- Mutations (PUT) target the `price` field, which is the correct field for the BigCommerce update API

### Default Product Sort Order

The `GET /v3/catalog/products?categories:in=N` endpoint returns products sorted by **`id` ascending** by default (creation order). This does not match the order shown in the BigCommerce admin panel, which may use `sort_order`, `name`, or `date_modified`. The `sort` and `direction` query parameters control ordering:

```
GET /v3/catalog/products?categories:in=408&sort=name&direction=asc
```

Supported sort fields: `id`, `name`, `sku`, `price`, `date_modified`, `date_last_imported`, `inventory_level`, `is_visible`, `total_sold`.

---

## 5. Query Parameter Filter Operators

**Discovered:** 2026-04-13 during search tool enhancement
**Affected files:** `internal/tools/catalog/products.go`

### Behavior

BigCommerce `GET` endpoints support colon-delimited filter operators on query parameters. These are not separate parameters — they are **suffixes** appended to a field name with a colon separator.

### Supported Operators

| Operator | Syntax | Meaning | Example |
|---|---|---|---|
| Equals (default) | `field=value` | Exact match | `name=My Product` |
| LIKE | `field:like=value` | SQL LIKE partial match | `name:like=Testing` |
| IN | `field:in=csv` | Matches any in comma-separated list | `categories:in=23,45` |
| NOT IN | `field:not_in=csv` | Excludes comma-separated list | `categories:not_in=23` |
| Min (>=) | `field:min=value` | Greater than or equal to | `price:min=10` |
| Max (<=) | `field:max=value` | Less than or equal to | `price:max=100` |
| Greater | `field:greater=value` | Strictly greater than | `price:greater=9.99` |
| Less | `field:less=value` | Strictly less than | `price:less=100.01` |

### Available Filter Fields for Products (`GET /v3/catalog/products`)

| Field | Supported Operators | Notes |
|---|---|---|
| `name` | `=`, `:like` | Case-insensitive for `:like` |
| `sku` | `=`, `:like`, `:in` | |
| `price` | `=`, `:min`, `:max`, `:greater`, `:less` | Filters on base `price`, not `calculated_price` |
| `categories` | `:in`, `:not_in` | Product must belong to at least one listed category |
| `brand_id` | `=`, `:in` | |
| `is_visible` | `=` | Boolean: `true` or `false` |
| `keyword` | `=` | Full-text search across name, SKU, description |
| `id` | `=`, `:in`, `:not_in`, `:min`, `:max` | |
| `date_modified` | `:min`, `:max` | ISO 8601 format |
| `date_last_imported` | `:min`, `:max` | ISO 8601 format |
| `availability` | `=` | `available`, `disabled`, `preorder` |
| `condition` | `=` | `New`, `Used`, `Refurbished` |
| `type` | `=` | `physical`, `digital` |
| `inventory_level` | `:min`, `:max` | Integer |
| `total_sold` | `:min`, `:max` | Integer |
| `weight` | `:min`, `:max` | Decimal |

### Pagination & Sorting Parameters

| Parameter | Values | Default | Notes |
|---|---|---|---|
| `page` | Integer >= 1 | 1 | Our client auto-paginates via `GetAll` |
| `limit` | 1–250 | 50 | Max per request |
| `sort` | `id`, `name`, `sku`, `price`, `date_modified`, `date_last_imported`, `inventory_level`, `is_visible`, `total_sold` | `id` | |
| `direction` | `asc`, `desc` | `asc` | |
| `include_fields` | Comma-separated field names | All fields | Reduces payload size |
| `exclude_fields` | Comma-separated field names | None | |
| `include` | `variants`, `images`, `custom_fields`, `bulk_pricing_rules`, `primary_image`, `modifiers`, `options`, `videos` | None | Sub-resource expansion |

### Pattern for Other Endpoints

The same operator syntax applies to other `GET` endpoints. When adding search tools for new domains:

- **Orders:** `status_id:in`, `date_created:min`, `date_created:max`, `customer_id`
- **Customers:** `name:like`, `email:like`, `company:like`, `date_created:min`
- **Brands:** `name`, `name:like`

Refer to each endpoint's documentation for the specific fields and supported operators.

### Our Implementation

We use a declarative `SearchFilter` table to map tool parameters to BigCommerce query parameters. Adding a new filter requires:
1. One entry in the `ProductSearchFilters` slice (or equivalent for the new domain)
2. One `mcp.With*` call in the tool schema with an LLM-guiding description

The `ExtractFilters` helper is reusable across all search tools.

---

## 6. Category Trees Batch Update Endpoint (PUT = Updates Only)

**Discovered:** 2026-04-13 during category management domain build
**Affected files:** `internal/bigcommerce/types.go`, `internal/bigcommerce/products.go`, `internal/tools/catalog/categories.go`

### Endpoint

`PUT /v3/catalog/trees/categories` — batch **updates** existing categories. Despite the official documentation suggesting this endpoint can create or update, our live testing revealed that using `PUT` to create new categories returns a **422** error requiring `category_id` (i.e., the category must already exist). Category **creation** must use `POST` — see [Section 7](#7-category-creation-post-not-put).

### Key Constraints

| Constraint | Value |
|---|---|
| Max categories per request | **50** |
| Required field for updates | `category_id` |
| HTTP method for creates | **POST** (not PUT — see Section 7) |

### Writable Fields

| Field | Type | Notes |
|---|---|---|
| `category_id` | int | Required for updates |
| `name` | string | Category display name |
| `description` | string | Supports HTML |
| `page_title` | string | SEO title tag |
| `meta_description` | string | SEO meta description |
| `search_keywords` | string | Comma-separated, for internal store search |
| `is_visible` | bool | Storefront visibility |
| `sort_order` | int | Display priority |
| `default_product_sort` | string | One of: `best_selling`, `price_desc`, `price_asc`, `avg_customer_review`, `alpha_asc`, `alpha_desc`, `featured`, `newest`, `use_store_settings` |
| `image_url` | string | Category image |
| `parent_id` | int | Parent category |
| `tree_id` | int | Category tree (multi-storefront) |
| `url` | object | `{"path": "/slug/", "is_customized": bool}` |

### Category Tree Search Filters

The `GET /v3/catalog/trees/categories` endpoint supports these filters:

| Filter | Syntax | Notes |
|---|---|---|
| Name exact | `name=value` | |
| Name partial | `name:like=value` | Case-insensitive |
| Parent ID | `parent_id:in=N` | Direct children of that parent. Plain `parent_id=N` is rejected with **422** on this endpoint; use `:in` even for a single ID (including `0` for root-level categories). |
| Tree ID | `tree_id:in=N` | Multi-storefront filter. Plain `tree_id=N` is rejected with **422** (“not valid filter parameter”); use `:in` even for a single tree ID. |
| Visibility | `is_visible=true/false` | |
| Keyword | `keyword=value` | Full-text search |
| ID in | `category_id:in=1,2,3` | Fetch specific IDs |

### Our Implementation

- `CategorySearchFilters` in `categories.go` maps tool params **`tree_id` → `tree_id:in`** and **`parent_id` → `parent_id:in`** (and `channel_id` resolution sets `tree_id:in` the same way)
- `CategoryUpdate` struct uses pointer fields to distinguish "not included" from "set to empty"
- `BatchUpdateCategories` uses `BatchPut` with a batch size of 50
- The `bulk_update` tool follows the same preview-then-confirm pattern as product bulk updates

---

## 7. Category Creation: POST, Not PUT

**Discovered:** 2026-04-13 during category create tool implementation
**Affected files:** `internal/bigcommerce/products.go`, `internal/bigcommerce/types.go`, `internal/tools/catalog/categories.go`

### The Problem

The BigCommerce documentation for `PUT /v3/catalog/trees/categories` describes it as an "upsert" endpoint that can create or update categories. In practice, sending a category payload **without** a `category_id` via `PUT` returns a **422** error:

```json
{
  "status": 422,
  "errors": {
    "0.tree_id": "Tree Id is required and can't be empty",
    "0.category_id": "Category does not exist."
  }
}
```

### The Fix

Category creation requires `POST /v3/catalog/trees/categories`, not `PUT`. The payload is an array of category objects:

```json
POST /v3/catalog/trees/categories
[
  {
    "name": "New Category",
    "tree_id": 1,
    "parent_id": 0
  }
]
```

### `tree_id` vs `parent_id` (anyOf Constraint)

The `POST` endpoint enforces an `anyOf` constraint: each category must include **either** `tree_id` (for root-level categories) **or** `parent_id` (for subcategories), but not necessarily both. When both are sent and `parent_id > 0`, the API ignores `tree_id`. When `parent_id` is `0` (root-level), `tree_id` is required.

Our `CategoryCreate` struct uses `omitempty` on both fields to avoid sending `0` values:

```go
type CategoryCreate struct {
    Name     string `json:"name"`
    TreeID   int    `json:"tree_id,omitempty"`
    ParentID int    `json:"parent_id,omitempty"`
    // ...
}
```

This ensures:
- Root-level categories send `tree_id` only (via `GetDefaultTreeID`)
- Subcategories send `parent_id` only (the tree is inherited from the parent)

### Our Implementation

- `CreateCategory()` in `products.go` uses `c.Post()`, not `c.Put()`
- The `handleCreate` handler resolves `parent_name` to `parent_id` server-side (see [Section 9](#9-parent-name-resolution-pattern))
- Default tree ID is fetched via `GET /v3/catalog/trees` and cached

---

## 8. Category Deletion: Cascade Behavior & Product Impact

**Discovered:** 2026-04-13 during category delete tool implementation
**Affected files:** `internal/bigcommerce/products.go`, `internal/tools/catalog/categories.go`

### Endpoint

`DELETE /v3/catalog/trees/categories` — deletes categories by ID. Unlike many BigCommerce endpoints, this uses **query parameters** rather than a path parameter:

```
DELETE /v3/catalog/trees/categories?category_id:in=42,43,44
```

### Key Behaviors

| Behavior | Detail |
|---|---|
| **Response code** | `204 No Content` on success |
| **Cascade** | Deleting a parent category **automatically deletes all subcategories** in the subtree |
| **Products** | Products assigned to a deleted category are **NOT deleted** — they simply lose the category assignment |
| **Multiple IDs** | The `category_id:in` filter accepts comma-separated IDs for batch deletion |

### Product Impact (Verified)

This is a critical detail for merchant-facing tools. When a category is deleted:
- Products remain in the store with all other data intact
- Products lose the deleted category assignment (removed from `categories` array)
- Products with no remaining category assignments still exist but may not appear on the storefront

This behavior is beneficial for merchants who want to reorganize categories without losing product data.

### Our Safety Implementation

The `catalog/categories/delete` and `catalog/categories/bulk_delete` tools (both Tier R3) implement a three-phase safety flow:

1. **Child Detection**: Before any action, the tool queries `GET /v3/catalog/trees/categories?parent_id:in=<id>` to check for subcategories
2. **Blocked Gate**: If children exist and `include_children` is not `true`, the tool **blocks** the operation and returns a list of affected children, requiring explicit acknowledgment
3. **Preview + Confirm**: Standard preview showing all categories to be deleted, explicit note about product impact, then `confirmed=true` to execute

---

## 9. Parent Name Resolution Pattern

**Discovered:** 2026-04-13 during category create and delete tool implementation
**Affected files:** `internal/tools/catalog/categories.go`

### The Problem

BigCommerce category APIs require numeric `category_id` and `parent_id` values. However, the BigCommerce admin UI does not prominently display category IDs, making it impractical for users (or LLMs) to know them. A merchant will say "create a subcategory under Electronics," not "create a subcategory with parent_id 42."

### The Pattern

Our category tools accept a `parent_name` (or `category_name`) string parameter as an alternative to the numeric ID. The server resolves the name to an ID via `SearchCategories`:

```go
func (c *Categories) resolveParentName(ctx context.Context, name string) (int, error) {
    results, err := c.bc.SearchCategories(ctx, map[string]string{
        "name": name,
    })
    if len(results) == 0 {
        return 0, fmt.Errorf("no category found with name %q", name)
    }
    if len(results) > 1 {
        return 0, fmt.Errorf("multiple categories match name %q (found %d); use category_id instead", name, len(results))
    }
    return results[0].ID, nil
}
```

### Ambiguity Handling

If the name matches **multiple** categories, the tool returns an error listing the count and directing the user to use the numeric ID instead. This prevents accidental operations on the wrong category.

### Tools Using This Pattern

| Tool | Name Parameter | ID Parameter | Usage |
|---|---|---|---|
| `catalog/categories/create` | `parent_name` | `parent_id` | Resolve parent for subcategory creation |
| `catalog/categories/delete` | `category_name` | `category_id` | Resolve target for deletion |

The two parameters are mutually exclusive in all cases — providing both is a validation error.

---

## 10. Empty Filter Query Parameter Bug

**Discovered:** 2026-04-13 during category list `list_all` implementation
**Affected files:** `internal/bigcommerce/products.go`

### The Problem

When `SearchCategories` or `SearchProducts` was called with no filters (e.g., listing all categories), the URL construction always appended a `?` to the base path:

```go
path := "catalog/trees/categories?"  // Always had trailing '?'
vals := url.Values{}
// ... no filters added ...
path += vals.Encode()  // Produces: "catalog/trees/categories?"
```

When pagination was added later (e.g., by `GetAll`), the final URL became:
```
catalog/trees/categories?&page=2&limit=250
```

The `?&` sequence produced a 422 error from BigCommerce:
```json
{"status": 422, "title": "The filter(s):  are not valid filter parameter(s)."}
```

### The Fix

Both `SearchCategories` and `SearchProducts` now only append `?` when there are actual query parameters:

```go
path := "catalog/trees/categories"
vals := url.Values{}
for k, v := range params {
    vals.Set(k, v)
}
if encoded := vals.Encode(); encoded != "" {
    path += "?" + encoded
}
```

This ensures clean URLs regardless of whether filters are present.

---

## 11. Category Tree GET Filter Uses `category_id:in`, Not `id:in`

**Discovered:** 2026-04-14 during `update_categories` tool testing
**Affected files:** `internal/bigcommerce/products.go`

### The Problem

The `GET /v3/catalog/trees/categories` endpoint uses `category_id:in` as the filter parameter for fetching specific categories by ID. Using `id:in` instead returns a **422** error:

```json
{
  "status": 422,
  "title": "The filter(s): id:in are not valid filter parameter(s).",
  "type": "https://developer.bigcommerce.com/api-docs/getting-started/api-status-codes"
}
```

This is consistent with how the same endpoint uses `category_id` in the response body (rather than `id`), and matches the `DELETE` endpoint which also requires `category_id:in`.

### Fix Applied

Changed `GetCategory` from `?id:in=` to `?category_id:in=`:

```go
body, err := c.Get(ctx, "catalog/trees/categories?category_id:in="+strconv.Itoa(categoryID))
```

### Key Takeaway

All filter/query parameters on the Category Tree endpoint use `category_id` as the field name, not `id`. This applies to GET, PUT, and DELETE operations. The `SearchCategories` and `DeleteCategories` methods were already correct; only `GetCategory` used the wrong parameter.

---

## 12. Category Metafields Use Legacy Path, Not Tree Path

**Discovered:** 2026-04-14
**Affected files:** `internal/bigcommerce/metafields.go`

### Issue

Category metafield CRUD endpoints use the **legacy** category path:
```
GET/POST    /v3/catalog/categories/{id}/metafields
PUT/DELETE  /v3/catalog/categories/{id}/metafields/{metafield_id}
```

These do **not** use the newer tree-based path (`/v3/catalog/trees/categories/...`). Attempting to use the tree path for metafields results in 404 errors.

### Key Takeaway

When working with category metafields, always use the legacy `/v3/catalog/categories/{id}/metafields` path. The Category Tree endpoints only handle category CRUD and hierarchy operations (list, get, create/update, delete), not sub-resources like metafields.

---

## 13. Dedicated Category-Assignments Endpoints Mirror the Catalog-Side Pattern

**Discovered:** 2026-04-14 (PUT) / 2026-04-28 (DELETE)
**Affected files:** `internal/bigcommerce/metafields.go`, `internal/tools/catalog/categories_assignments.go`, `internal/tools/catalog/products.go`

### Behavior

`PUT /v3/catalog/products/category-assignments` accepts `[{product_id, category_id}]` pairs and performs an **upsert**:

- If the assignment doesn't exist, it creates it
- If it already exists, the call succeeds silently (204 No Content)
- It **never removes** existing assignments — it's purely additive

`DELETE /v3/catalog/products/category-assignments?product_id:in=…&category_id:in=…` is the matching tear-down: it removes only the (product, category) pairs in the Cartesian intersection of the two filter lists, leaving every other category assignment on those products intact.

Both endpoints contrast with the batch `PUT /v3/catalog/products` approach where you replace the entire `categories` array on a product (which is destructive — it silently drops any category not in the request body).

### Tool Mapping

| Scenario | Tool | Underlying call |
|----------|------|------------------|
| Add products to categories (no removal) | `catalog/products/assign_categories` (R1) | `PUT /v3/catalog/products/category-assignments` |
| Remove specific (product, category) links **without** clobbering other assignments | `catalog/products/unassign_categories` (R2, preview → confirm) | `DELETE /v3/catalog/products/category-assignments?product_id:in=…&category_id:in=…` |
| Replace the **entire** category set on a product | `catalog/products/update` with `category_ids` (R1) | Batch `PUT /v3/catalog/products` with new `categories` array |

`unassign_categories` is preferred over `products/update categories=[…]` for partial removals because the latter is a full-array replacement and silently drops any category you forget to include.

### Caps (enforced in handlers)

| Tool | `product_ids` | `category_ids` | Pairs |
|------|---------------|----------------|-------|
| `assign_categories` | ≤ 100 | ≤ 50 | ≤ 500 |
| `unassign_categories` | ≤ 100 | ≤ 50 | (same intersection) |

---

## Adding New Entries

When you discover a new API quirk during development:

1. Add a new numbered section to this file
2. Include the discovery date and affected source files
3. Document the expected vs actual behavior with concrete JSON examples from live data
4. Note the impact on existing code and any fix applied or recommended
