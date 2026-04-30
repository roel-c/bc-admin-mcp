#!/usr/bin/env bash
# Automated smoke slice: MSF-aligned reads matching MCP tools
#   catalog/products/channel_assignments/list
#   catalog/channels/category_trees (GET /v3/catalog/trees?channel_id:in=)
#   GET /v3/catalog/trees/categories?tree_id:in=… (& optional parent_id:in=…)
#   catalog/channels/listings/list (GET /v3/channels/{id}/listings)
#
# Assignments vs listings (when to use which): docs/channel-assignments-vs-listings.md
#
# Usage: from repo root, with .env containing BC_STORE_HASH and BC_AUTH_TOKEN:
#   ./scripts/smoke_msf_slice.sh
# Or: make smoke-msf
#
# Does not print secrets. Non-200 on assignments/trees/listings is reported but
# does not fail the script if catalog/products still works (often OAuth scope gaps).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ ! -f .env ]]; then
	echo "FAIL: .env not found in ${ROOT}"
	exit 1
fi
set -a
# shellcheck disable=SC1091
source ./.env
set +a

if [[ -z "${BC_STORE_HASH:-}" || -z "${BC_AUTH_TOKEN:-}" ]]; then
	echo "FAIL: BC_STORE_HASH and BC_AUTH_TOKEN must be set (e.g. via .env)"
	exit 1
fi

BASE="https://api.bigcommerce.com/stores/${BC_STORE_HASH}/v3"
AUTH=( -H "X-Auth-Token: ${BC_AUTH_TOKEN}" -H "Accept: application/json" )

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl_json() {
	local url="$1"
	local out="$2"
	local code
	code=$(curl -sS -o "$out" -w "%{http_code}" "${AUTH[@]}" "$url")
	echo "$code"
}

echo "=== GET /v3/catalog/products?limit=3 (sample IDs for downstream calls) ==="
code=$(curl_json "${BASE}/catalog/products?limit=3" "${tmp}/products.json")
echo "HTTP ${code}"
if [[ "${code}" != "200" ]]; then
	head -c 600 "${tmp}/products.json"
	echo
	exit 1
fi
PIDS=$(python3 -c "import json; d=json.load(open('${tmp}/products.json')); print(','.join(str(x['id']) for x in d.get('data') or []))")
if [[ -z "${PIDS}" ]]; then
	echo "FAIL: no products returned"
	exit 1
fi
echo "product_id:in sample: ${PIDS}"

echo ""
echo "=== GET /v3/channels?limit=50 (pick first active storefront) ==="
code=$(curl_json "${BASE}/channels?limit=50" "${tmp}/channels.json")
echo "HTTP ${code}"
if [[ "${code}" != "200" ]]; then
	head -c 600 "${tmp}/channels.json"
	echo
	exit 1
fi
CH=$(python3 <<PY
import json
d=json.load(open("${tmp}/channels.json"))
for x in d.get("data") or []:
    if x.get("type") == "storefront" and x.get("status") in ("active", "prelaunch"):
        print(x["id"])
        break
else:
    print("")
PY
)
if [[ -z "${CH}" ]]; then
	CH=1
	echo "No active/prelaunch storefront in first page; fallback channel_id=${CH}"
else
	echo "channel_id=${CH} (active/prelaunch storefront)"
fi

echo ""
echo "=== GET /v3/catalog/products/channel-assignments?product_id:in=... ==="
code=$(curl_json "${BASE}/catalog/products/channel-assignments?product_id:in=${PIDS}" "${tmp}/assign.json")
echo "HTTP ${code}"
if [[ "${code}" == "200" ]]; then
	python3 -c "
import json
d=json.load(open('${tmp}/assign.json'))
data=d.get('data') or []
print('assignment_rows:', len(data))
wanted = {int(x) for x in '${PIDS}'.split(',') if x}
seen = set()
for r in data[:50]:
    pid = r.get('product_id')
    cid = r.get('channel_id')
    if pid is not None:
        seen.add(int(pid))
    print(' ', pid, '-> channel', cid)
missing = sorted(wanted - seen)
if missing:
    print('products_with_no_assignment_rows:', missing, '(normal if product has no channel assignments)')
"
else
	echo "WARN: assignments call failed (often missing Products write/read scope or API change). Body:"
	head -c 800 "${tmp}/assign.json"
	echo
fi

echo ""
echo "=== GET /v3/catalog/trees?channel_id:in=${CH} (category_trees tool) ==="
code=$(curl_json "${BASE}/catalog/trees?channel_id:in=${CH}" "${tmp}/trees.json")
echo "HTTP ${code}"
if [[ "${code}" == "200" ]]; then
	python3 -c "
import json
d=json.load(open('${tmp}/trees.json'))
data=d.get('data') or []
print('trees:', len(data))
for t in data[:8]:
    print(' ', 'tree_id', t.get('id'), '|', (t.get('name') or '')[:60], '| channels', t.get('channels'))
"
else
	echo "WARN: trees filtered by channel failed. Body:"
	head -c 800 "${tmp}/trees.json"
	echo
fi

echo ""
echo "=== GET /v3/catalog/trees/categories?tree_id:in=…&limit=5 (matches catalog/categories/list MSF filter) ==="
TREE_ID=$(python3 <<PY
import json
try:
    d=json.load(open("${tmp}/trees.json"))
    data=d.get("data") or []
    if data and data[0].get("id") is not None:
        print(int(data[0]["id"]))
except Exception:
    pass
PY
)
if [[ -z "${TREE_ID}" ]]; then
	echo "SKIP: no tree id (trees call non-200 or empty data)"
else
	code=$(curl_json "${BASE}/catalog/trees/categories?tree_id:in=${TREE_ID}&limit=5" "${tmp}/tree_cats.json")
	echo "tree_id:in=${TREE_ID} HTTP ${code}"
	if [[ "${code}" == "200" ]]; then
		python3 -c "
import json
d=json.load(open('${tmp}/tree_cats.json'))
data=d.get('data') or []
print('categories:', len(data))
for c in data[:5]:
    print(' ', c.get('id'), c.get('name'))
"
		PARENT_FOR_PROBE=$(python3 <<PY
import json
try:
    d=json.load(open("${tmp}/tree_cats.json"))
    data=d.get("data") or []
    if not data:
        print("")
    else:
        p=data[0].get("parent_id")
        if p is None:
            print("")
        else:
            print(int(p))
except Exception:
    print("")
PY
)
		if [[ -n "${PARENT_FOR_PROBE}" ]]; then
			code2=$(curl_json "${BASE}/catalog/trees/categories?tree_id:in=${TREE_ID}&parent_id:in=${PARENT_FOR_PROBE}&limit=5" "${tmp}/tree_cats_by_parent.json")
			echo "combined tree_id:in=${TREE_ID} parent_id:in=${PARENT_FOR_PROBE} HTTP ${code2}"
			if [[ "${code2}" != "200" ]]; then
				echo "WARN: parent_id:in combined filter failed. Body:"
				head -c 500 "${tmp}/tree_cats_by_parent.json"
				echo
			fi
		fi
	else
		echo "WARN: trees/categories with tree_id:in failed. Body:"
		head -c 600 "${tmp}/tree_cats.json"
		echo
	fi
fi

CHANNELS_FOR_LISTINGS=$(python3 <<PY
import json
ch=set()
try:
    d=json.load(open("${tmp}/assign.json"))
    for r in d.get("data") or []:
        cid = r.get("channel_id")
        if cid is not None:
            ch.add(int(cid))
except Exception:
    pass
if not ch:
    ch.add(int("${CH}"))
print(",".join(str(x) for x in sorted(ch)[:3]))
PY
)

echo ""
echo "=== GET /v3/channels/{id}/listings?limit=10&product_id:in=... (listings tool, first page) ==="
IFS=',' read -ra ARR <<< "${CHANNELS_FOR_LISTINGS}"
for cid in "${ARR[@]}"; do
	[[ -z "${cid}" ]] && continue
	code=$(curl_json "${BASE}/channels/${cid}/listings?limit=10&product_id:in=${PIDS}" "${tmp}/list_${cid}.json")
	echo "--- channel ${cid} HTTP ${code} ---"
	if [[ "${code}" == "200" ]]; then
		python3 -c "
import json
d=json.load(open('${tmp}/list_${cid}.json'))
data=d.get('data') or []
print('listing_rows:', len(data))
for L in data[:10]:
    print(' ', 'product', L.get('product_id'), 'state', L.get('state'), 'listing_id', L.get('listing_id'))
"
	else
		echo "WARN: listings failed for channel ${cid} (scope or no listings). Body:"
		head -c 500 "${tmp}/list_${cid}.json"
		echo
	fi
done

echo ""
echo "=== smoke_msf_slice complete ==="
