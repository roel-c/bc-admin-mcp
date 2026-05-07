#!/usr/bin/env bash
# Pre-session live smoke test: one safe R0 read per MCP domain.
#
# Verifies BC_AUTH_TOKEN has the required OAuth scopes for every domain
# and that the API is reachable before starting a manual test session.
#
# Usage:
#   ./scripts/smoke_all_domains.sh          (reads .env automatically)
#   make smoke                              (alias added to Makefile)
#
# Exit codes:
#   0  — all required domains passed
#   1  — one or more required domains failed

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ── credentials ──────────────────────────────────────────────────────────────
if [[ -f .env ]]; then
    set -a
    # shellcheck disable=SC1091
    source ./.env
    set +a
fi

if [[ -z "${BC_STORE_HASH:-}" || -z "${BC_AUTH_TOKEN:-}" ]]; then
    echo "FAIL: BC_STORE_HASH and BC_AUTH_TOKEN must be set (via .env or environment)"
    exit 1
fi

V3="https://api.bigcommerce.com/stores/${BC_STORE_HASH}/v3"
V2="https://api.bigcommerce.com/stores/${BC_STORE_HASH}/v2"
AUTH=(-H "X-Auth-Token: ${BC_AUTH_TOKEN}" -H "Accept: application/json")

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# ── helpers ───────────────────────────────────────────────────────────────────
PASS=0
WARN=0
FAIL=0

# check <label> <url> [required=true|false]
#   required=true  → failure increments FAIL and contributes to exit code
#   required=false → failure increments WARN only (scope gap, Enterprise gate, etc.)
check() {
    local label="$1"
    local url="$2"
    local required="${3:-true}"
    local out="${tmp}/$(echo "$label" | tr '/ ' '__').json"

    local code
    code=$(curl -sS -o "$out" -w "%{http_code}" "${AUTH[@]}" "$url")

    if [[ "$code" == "200" ]]; then
        # Extract a meaningful count from the response for context
        local count
        count=$(python3 -c "
import json, sys
try:
    d = json.load(open('$out'))
    data = d.get('data') if isinstance(d, dict) else d
    if isinstance(data, list):
        print(len(data))
    else:
        print('ok')
except Exception:
    print('ok')
" 2>/dev/null || echo "ok")
        printf "  %-55s \033[32mPASS\033[0m  HTTP 200  (rows/items: %s)\n" "$label" "$count"
        (( PASS++ )) || true
    else
        local body_snippet
        body_snippet=$(head -c 200 "$out" 2>/dev/null | tr '\n' ' ' || true)
        if [[ "$required" == "true" ]]; then
            printf "  %-55s \033[31mFAIL\033[0m  HTTP %s  %s\n" "$label" "$code" "$body_snippet"
            (( FAIL++ )) || true
        else
            printf "  %-55s \033[33mWARN\033[0m  HTTP %s  %s\n" "$label" "$code" "$body_snippet"
            (( WARN++ )) || true
        fi
    fi
}

# ── run ───────────────────────────────────────────────────────────────────────
echo ""
echo "BigCommerce MCP Server — pre-session domain smoke test"
echo "store: ${BC_STORE_HASH}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

echo ""
echo "▸ catalog/products  (scope: store_v2_products_read_only)"
check "catalog/products/search"          "${V3}/catalog/products?limit=3"
check "catalog/products/get (by id)"     "${V3}/catalog/products?limit=1"

echo ""
echo "▸ catalog/categories  (scope: store_v2_products_read_only)"
check "catalog/categories/list"          "${V3}/catalog/trees/categories?limit=3"

echo ""
echo "▸ catalog/brands  (scope: store_v2_products_read_only)"
check "catalog/brands/list"              "${V3}/catalog/brands?limit=3"

echo ""
echo "▸ catalog/variants  (scope: store_v2_products_read_only)"
check "catalog/variants/list"            "${V3}/catalog/variants?limit=3"

echo ""
echo "▸ catalog/pricelists  (scope: store_price_lists)"
check "catalog/pricelists/list"          "${V3}/pricelists?limit=3" false

echo ""
echo "▸ catalog/channels  (scope: store_channel_settings_read_only)"
check "catalog/channels/list"            "${V3}/channels?limit=10" false
check "catalog/channels/category_trees"  "${V3}/catalog/trees?limit=5" false

echo ""
echo "▸ orders/management  (scope: store_v2_orders_read_only)"
check "orders/management/list"           "${V2}/orders?limit=3"
check "orders/management/count"          "${V2}/orders/count"
check "orders/management/statuses"       "${V2}/order_statuses"

echo ""
echo "▸ customers  (scope: store_v2_customers_read_only)"
check "customers/list"                   "${V3}/customers?limit=3"
check "customers/groups/list"            "${V2}/customer_groups?limit=3"
check "customers/attributes/list"        "${V3}/customers/attributes?limit=3"

echo ""
echo "▸ customers/settings  (scope: store_v2_customers_read_only)"
check "customers/settings/global/get"    "${V3}/customers/settings"

echo ""
echo "▸ customers/segments  (Enterprise; scope: store_v2_customers_read_only)"
check "customers/segments/list"          "${V3}/segments?limit=3" false
check "customers/shopper_profiles/list"  "${V3}/shopper-profiles?limit=3" false

echo ""
echo "▸ marketing/promotions  (scope: store_v2_marketing_read_only)"
check "marketing/promotions/automatic/list"  "${V3}/promotions?redemption_type=automatic&limit=3" false
check "marketing/promotions/coupon/list"     "${V3}/promotions?redemption_type=coupon&limit=3" false
check "marketing/promotions/settings/get"    "${V3}/promotions/settings" false

echo ""
echo "▸ inventory  (scope: store_inventory)"
check "inventory/locations/list"         "${V3}/inventory/locations?limit=3" false
check "inventory/items/list"             "${V3}/inventory/items?limit=3" false

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
printf "Results: \033[32m%d PASS\033[0m  \033[33m%d WARN\033[0m  \033[31m%d FAIL\033[0m\n" "$PASS" "$WARN" "$FAIL"

if [[ "$WARN" -gt 0 ]]; then
    echo ""
    echo "WARN = missing OAuth scope or Enterprise-only endpoint."
    echo "Add the required scope to your API account in the BC control panel"
    echo "and re-run, or accept the limitation for this session."
fi

echo ""
if [[ "$FAIL" -gt 0 ]]; then
    echo "FAIL: $FAIL required domain(s) did not return HTTP 200."
    echo "Fix the credential/scope issues above before starting a manual session."
    exit 1
else
    echo "OK: all required domains are reachable and returning data."
fi
