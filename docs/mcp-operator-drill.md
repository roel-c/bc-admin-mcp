# MCP discovery and preview drills

This server uses **progressive disclosure**: clients call **`discover_tools`** to walk a category tree, then **`execute_tool`** with a **`tool_path`** and nested **`arguments`**. Writes at tier **R1+** return a **preview** until **`confirmed: true`**.

## Automated checks (CI-friendly)

From the repository root:

```bash
go test ./internal/server/... -count=1 -run 'TestFullRegistration'
```

This suite (see `internal/server/registration_audit_test.go`) verifies that:

1. **`discover_tools("")`** exposes only implemented roots (**`catalog`**, **`customers`**, **`marketing`**).
2. **Every registered category** has a non-empty child list (no dead-end category nodes).
3. **Every tool’s parent category path** exists in the registry (no orphaned tools).
4. **Breadth-first discovery from the root** reaches **every** category and **every** tool path (no hierarchy wiring mistakes).
5. **Every R1–R3 tool** declares a **`confirmed`** boolean in its input schema (enforced at registration time; the test is a regression guard).

Run the full test suite after tool-registration changes:

```bash
go test ./... -count=1
```

## Manual drill — discovery

Use your MCP host’s “call tool” UI or JSON-RPC. Meta-tools are named exactly:

- `discover_tools` — argument **`path`**: string, use `""` for root.

Suggested sequence:

1. `discover_tools` with `path: ""` → expect **`catalog`**, **`customers`**, **`marketing`**.
2. `discover_tools` with `path: "catalog"` (or `"customers"` / `"marketing"`) → expect subcategories under that domain.
3. Drill into one leaf area (e.g. `catalog/products/channel_assignments`) until you see **tool** stubs with **`tier`** fields.

## Manual drill — preview then confirm

Pick any **R1** tool (for example `catalog/categories/bulk_update` or `catalog/products/channel_assignments/assign`).

1. **First call:** same **`tool_path`**, include the real payload but set **`"confirmed": false`** or omit **`confirmed`** (handlers treat missing confirmation as preview where applicable).
2. Inspect the JSON: expect **`status": "preview"`** or **`pending_confirmation"`** (wording varies by tool), and no mutation on the store.
3. **Second call:** identical **`arguments`** with **`"confirmed": true`** to execute.

**Shape reminder:**

```json
{
  "tool_path": "catalog/products/metafields/set",
  "arguments": {
    "product_id": 123,
    "namespace": "example",
    "key": "flag",
    "value": "1"
  }
}
```

Do not place `product_id` next to `tool_path` at the top level; only **`arguments`** is forwarded to the inner tool.

## Live Management API parity

For REST-only verification (OAuth token, no MCP), use **`make smoke-msf`**, which exercises products, channel-assignments, trees, category filters aligned with this server, and listings.
