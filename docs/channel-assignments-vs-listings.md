# Channel assignments vs channel listings

BigCommerce separates **which channels may sell a product** from **how that product appears on each channel’s storefront**. The MCP server exposes both; choosing the right surface avoids “the API says it’s assigned but the storefront doesn’t show it” confusion.

## Channel catalog assignments

**REST:** `GET|PUT|DELETE /v3/catalog/products/channel-assignments`

**Meaning:** The product is **linked to one or more sales channels** at the catalog level. If a product has no row for a channel, it is not part of that channel’s sellable catalog.

**MCP (examples):**

- `catalog/products/channel_assignments/list`
- `catalog/products/channel_assignments/assign`
- `catalog/products/channel_assignments/remove`

**Typical use:** “Put this SKU on the AU storefront channel” or “Remove this product from the wholesale channel.”

## Channel listings

**REST:** `GET|POST|PUT /v3/channels/{channel_id}/listings`

**Meaning:** A **listing** row controls channel-specific presentation and **state** (e.g. active vs draft), overrides, and visibility rules that depend on the Listings API, not only the assignment table.

**MCP (examples):**

- `catalog/channels/listings/list`
- `catalog/channels/listings/create`
- `catalog/channels/listings/update`

**Typical use:** “This product is assigned to the channel but doesn’t appear on the storefront” — check **listings** for that `channel_id` and `product_id`, not only channel-assignments.

## Combined read: `catalog/products/channel_summary`

For a small batch of products (caps in the tool schema), **`catalog/products/channel_summary`** aggregates:

- Channel assignments (`channel-assignments`)
- Listing state per channel (`/v3/channels/{id}/listings`)

Use it when debugging MSF visibility without hand-correlating two APIs.

## Smoke check

`make smoke-msf` (see `scripts/smoke_msf_slice.sh`) hits both **channel-assignments** and **listings** with live credentials so you can compare HTTP status and row counts in one run.
