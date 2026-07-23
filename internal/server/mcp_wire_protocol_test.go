package bcserver

// Wire-protocol integration tests.
//
// The registration audit tests (registration_audit_test.go) call
// discovery.Registry methods directly — they never go through the actual
// MCP request/response machinery (JSON-RPC method routing, argument
// marshaling, content serialization). That leaves a real gap: a bug in how
// discover_tools/execute_tool are wired to the mcp-go transport would not be
// caught by any automated test, only by a human manually driving the built
// binary (see docs/ARCHITECTURE.md §9, "Integration Tests (gap)").
//
// These tests close that gap using mcp-go's in-process client transport
// (client.NewInProcessClient), which talks to a real *server.MCPServer
// through the SDK's actual tool-call machinery — no stdio/network transport
// needed, and no BigCommerce credentials required, because every case here
// either returns before the handler would reach the BigCommerce client, or
// only exercises client-side validation that runs before any HTTP call.
// Keeping it hermetic is intentional: this suite runs on every `go test
// ./...`, including CI, so it must never depend on live credentials or
// network access — that is what the manual/live Full Surface Check
// (docs/WORKFLOW.md §10) is for.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/stretchr/testify/require"
)

// newWireTestClient builds the real MCP server (same constructor the binary
// uses) with a fake, non-networked BigCommerce config, wraps it in an
// in-process MCP client, and completes the initialize handshake.
func newWireTestClient(t *testing.T) *client.Client {
	t.Helper()

	cfg := &config.Config{
		BigCommerce: testBigCommerceConfig(),
		Server: config.ServerConfig{
			Name:    "bigcommerce-mcp-test",
			Version: "test",
		},
	}

	mcpServer := New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	c, err := client.NewInProcessClient(mcpServer)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	require.NoError(t, c.Start(context.Background()))

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "wire-protocol-test-client", Version: "1.0.0"}
	_, err = c.Initialize(context.Background(), initReq)
	require.NoError(t, err)

	return c
}

// callDiscoverTools drives discover_tools over the real wire protocol and
// decodes the JSON stub list exactly as a real MCP client would.
func callDiscoverTools(t *testing.T, c *client.Client, path string) []map[string]any {
	t.Helper()

	req := mcp.CallToolRequest{}
	req.Params.Name = "discover_tools"
	req.Params.Arguments = map[string]any{"path": path}

	result, err := c.CallTool(context.Background(), req)
	require.NoError(t, err, "discover_tools(%q) transport-level call failed", path)
	require.False(t, result.IsError, "discover_tools(%q) returned a tool-level error: %v", path, result.Content)
	require.Len(t, result.Content, 1)

	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok, "discover_tools(%q) did not return text content", path)

	var entries []map[string]any
	require.NoError(t, json.Unmarshal([]byte(text.Text), &entries), "discover_tools(%q) did not return valid JSON", path)
	return entries
}

// TestMCPWireProtocol_DiscoverToolsRootMatchesDocumentedRoots exercises the
// exact call README.md's Quick Start and docs/AGENT.md tell an operator/agent
// to make first: discover_tools(""). If the documented root set and the real
// wire response ever diverge, this fails — the same class of drift found
// manually during the doc-efficiency audit, now caught automatically.
func TestMCPWireProtocol_DiscoverToolsRootMatchesDocumentedRoots(t *testing.T) {
	c := newWireTestClient(t)
	entries := callDiscoverTools(t, c, "")

	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		require.Equal(t, "category", e["type"], "root entry %v must be a category", e["path"])
		path, ok := e["path"].(string)
		require.True(t, ok)
		got[path] = true
	}

	documentedRoots := []string{
		"catalog", "orders", "customers", "marketing",
		"inventory", "storefront", "webhooks", "carts",
	}
	for _, root := range documentedRoots {
		require.True(t, got[root], "discover_tools(\"\") over the real wire protocol is missing documented root %q", root)
	}
	// b2b is gated by BC_B2B_ENABLED (false in this test's fake config) and
	// must stay hidden — mirrors TestFullRegistrationActiveRoots but through
	// the actual transport rather than the registry API directly.
	require.False(t, got["b2b"], "b2b must stay hidden when BC_B2B_ENABLED is false")
	require.Len(t, got, len(documentedRoots),
		"discover_tools(\"\") returned an unexpected number of roots over the wire — "+
			"update README.md/docs/AGENT.md if a new always-on domain was intentionally added")
}

// TestMCPWireProtocol_DiscoverToolsDrillDown exercises the two-level drill
// documented in README.md's Quick Start and docs/ARCHITECTURE.md §9's
// "Manual Drill — Discovery": category -> subcategory -> tool stubs with a
// tier, over the real transport.
func TestMCPWireProtocol_DiscoverToolsDrillDown(t *testing.T) {
	c := newWireTestClient(t)

	subEntries := callDiscoverTools(t, c, "catalog")
	require.NotEmpty(t, subEntries, "discover_tools(\"catalog\") must not be empty over the wire")

	leafEntries := callDiscoverTools(t, c, "catalog/products")
	require.NotEmpty(t, leafEntries, "discover_tools(\"catalog/products\") must not be empty over the wire")

	sawTool := false
	for _, e := range leafEntries {
		if e["type"] == "tool" {
			sawTool = true
			tier, _ := e["tier"].(string)
			require.NotEmpty(t, tier, "tool %v must expose a non-empty tier over the wire", e["path"])
		}
	}
	require.True(t, sawTool, "catalog/products should contain at least one leaf tool over the wire")
}

// TestMCPWireProtocol_ExecuteToolRequiresToolPath exercises the meta-tool's
// own early validation (before any tool handler — and therefore before any
// BigCommerce call — runs), over the real transport.
func TestMCPWireProtocol_ExecuteToolRequiresToolPath(t *testing.T) {
	c := newWireTestClient(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "execute_tool"
	req.Params.Arguments = map[string]any{
		"arguments": map[string]any{"name_like": "test"},
	}

	result, err := c.CallTool(context.Background(), req)
	require.NoError(t, err, "execute_tool transport-level call failed")
	require.True(t, result.IsError, "execute_tool without tool_path must return a tool-level error")
}

// TestMCPWireProtocol_ExecuteToolUnknownPathReturnsClearError confirms an
// unrecognized tool_path fails fast, over the wire, with no network call —
// the registry rejects it before ever reaching a handler.
func TestMCPWireProtocol_ExecuteToolUnknownPathReturnsClearError(t *testing.T) {
	c := newWireTestClient(t)

	req := mcp.CallToolRequest{}
	req.Params.Name = "execute_tool"
	req.Params.Arguments = map[string]any{
		"tool_path": "catalog/products/does_not_exist",
		"arguments": map[string]any{},
	}

	result, err := c.CallTool(context.Background(), req)
	require.NoError(t, err, "execute_tool transport-level call failed")
	require.True(t, result.IsError, "execute_tool with an unknown tool_path must return a tool-level error")
}

// TestMCPWireProtocol_ExecuteToolRunsRealHandlerValidation drives a real,
// registered R1 tool handler over the wire — not just the meta-tool's own
// early-exit branches — to prove handler dispatch and client-side validation
// work end-to-end. It stays hermetic by tripping a cap that DEVELOPMENT.md
// §2.5 documents as enforced "before any BigCommerce request fires"
// (product_ids × category_ids <= 500 pairs), so the assertion below is
// reached without any network call.
func TestMCPWireProtocol_ExecuteToolRunsRealHandlerValidation(t *testing.T) {
	c := newWireTestClient(t)

	productIDs := make([]any, 100)
	for i := range productIDs {
		productIDs[i] = i + 1
	}
	categoryIDs := make([]any, 6) // 100 * 6 = 600 pairs > the documented 500 cap
	for i := range categoryIDs {
		categoryIDs[i] = i + 1
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "execute_tool"
	req.Params.Arguments = map[string]any{
		"tool_path": "catalog/products/assign_categories",
		"arguments": map[string]any{
			"product_ids":  productIDs,
			"category_ids": categoryIDs,
		},
	}

	result, err := c.CallTool(context.Background(), req)
	require.NoError(t, err, "execute_tool transport-level call failed")
	require.True(t, result.IsError, "exceeding the documented pairs cap must surface as a tool-level error")
	require.Len(t, result.Content, 1)
	text, ok := mcp.AsTextContent(result.Content[0])
	require.True(t, ok)
	require.Contains(t, text.Text, "pairs", "cap error should explain the (product, category) pairs limit")
}
