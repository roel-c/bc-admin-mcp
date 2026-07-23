package bcserver

// Documentation-drift check.
//
// README.md's "Implemented Tools" table is documented (README.md itself,
// and docs/AGENT.md) as the human-browsable snapshot of every registered
// tool path. That claim can quietly go stale: a tool can ship in code
// without docs/WORKFLOW.md §7's "update docs as the batch lands" step
// actually happening. That is exactly how catalog/products/bulk_sku_update
// and catalog/products/custom_fields/create were found live on the server
// but absent from every doc during a manual audit — this test turns that
// one-off manual diff into something CI catches on every push.
//
// It is intentionally narrow: it parses README.md's own two documented
// shorthand conventions for collapsing tool-path alternatives (see the
// parser below) rather than trying to be a general markdown/table parser.
// If a new shorthand convention is introduced in README.md and this test
// starts failing spuriously, extend backtickSpans/documentedToolPaths to
// handle it — do not just delete the check.

import (
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/session"
	"github.com/stretchr/testify/require"
)

// splitTableRow splits a markdown table row into cells on unescaped "|"
// only. A naive strings.Split(line, "|") is wrong here: this table's own
// shorthand convention uses "\|" to render a literal pipe *within* a
// column (e.g. `b2b/companies/roles/list` \| `get`), and a naive split
// would incorrectly treat that escaped pipe as a real column boundary,
// silently truncating the first cell.
func splitTableRow(line string) []string {
	var cells []string
	var cur strings.Builder
	runes := []rune(line)
	for i, r := range runes {
		if r == '|' && (i == 0 || runes[i-1] != '\\') {
			cells = append(cells, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	cells = append(cells, cur.String())
	return cells
}

// backtickSpans returns the contents of every `...` span on a line, in order.
func backtickSpans(line string) []string {
	parts := strings.Split(line, "`")
	if len(parts) < 3 {
		return nil
	}
	spans := make([]string, 0, len(parts)/2)
	for i := 1; i < len(parts); i += 2 {
		spans = append(spans, parts[i])
	}
	return spans
}

// knownRoots are the always-registered domain roots plus b2b. Used to
// disambiguate a third shorthand: a second alternative that itself contains
// a "/" but is a *partial* suffix relative to the parent, not a standalone
// path, e.g. `b2b/receipts/delete` \| `lines/delete` documents
// b2b/receipts/lines/delete, not a bogus top-level "lines/delete".
var knownRoots = map[string]bool{
	"catalog": true, "orders": true, "customers": true, "marketing": true,
	"inventory": true, "storefront": true, "webhooks": true, "carts": true,
	"b2b": true,
}

// looksLikeToolPath filters out backtick spans in the Tool Path column that
// aren't actually paths (defensive; the column is expected to contain only
// paths or bare leaf alternatives, but this avoids false "documented"
// entries if that ever changes).
func looksLikeToolPath(tok string) bool {
	if tok == "" {
		return false
	}
	for _, r := range tok {
		if r == '/' || r == '_' || r == '-' || r == '*' ||
			(r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// documentedToolPaths scans README.md's tool tables for paths, expanding
// the two shorthand conventions the table actually uses:
//
//  1. One backtick span with escaped-pipe alternatives sharing a prefix,
//     e.g. `orders/management/list\|get\|create` documents
//     orders/management/list, orders/management/get, and
//     orders/management/create.
//  2. Multiple backtick spans in the same table cell joined by an escaped
//     pipe outside the backticks, e.g. `b2b/companies/roles/list` \| `get`
//     — the bare "get" inherits the parent path of the preceding full path
//     within that same cell.
func documentedToolPaths(t *testing.T, readmePath string) map[string]bool {
	t.Helper()

	raw, err := os.ReadFile(readmePath)
	require.NoError(t, err)

	documented := map[string]bool{}

	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := splitTableRow(trimmed)
		if len(cells) < 2 {
			continue
		}
		firstCell := cells[1] // cells[0] is empty (text before the leading '|')

		var currentParent string
		for _, span := range backtickSpans(firstCell) {
			for _, tok := range strings.Split(span, `\|`) {
				tok = strings.TrimSpace(tok)
				if !looksLikeToolPath(tok) {
					continue
				}

				firstSeg, hasSlash := tok, false
				if idx := strings.Index(tok, "/"); idx > 0 {
					firstSeg, hasSlash = tok[:idx], true
				}

				switch {
				case hasSlash && (currentParent == "" || knownRoots[firstSeg]):
					// A fresh absolute path, e.g. "b2b/receipts/delete".
					documented[tok] = true
					currentParent = tok[:strings.LastIndex(tok, "/")]
				case currentParent != "":
					// A bare leaf ("get") or partial suffix ("lines/delete")
					// relative to the most recent absolute path in this cell.
					documented[currentParent+"/"+tok] = true
				}
			}
		}
	}

	return documented
}

// TestDocsSync_ReadmeImplementedToolsTableCoversRegisteredPaths fails if a
// tool is registered in code but absent from README.md's Implemented Tools
// table, catching the exact drift class found manually in this project's
// documentation-efficiency audit (2026-07-16).
func TestDocsSync_ReadmeImplementedToolsTableCoversRegisteredPaths(t *testing.T) {
	reg := discovery.NewRegistry()
	registerCategories(reg, true) // include b2b — README documents it too

	cfg := testBigCommerceConfig()
	bc := bigcommerce.NewClient(cfg, slog.Default())
	t.Cleanup(func() { bc.Close() })
	b2bClient := bigcommerce.NewB2BClient(cfg.StoreHash, cfg.AuthToken, cfg.MaxRetries, slog.Default())
	t.Cleanup(func() { b2bClient.Close() })
	cache := session.NewStore(cfg.CacheTTL)
	registerTools(reg, bc, b2bClient, cache)

	readmePath := filepath.Join("..", "..", "README.md")
	documented := documentedToolPaths(t, readmePath)

	var missing []string
	for _, toolPath := range reg.ListToolPaths() {
		if !documented[toolPath] {
			missing = append(missing, toolPath)
		}
	}
	sort.Strings(missing)

	require.Empty(t, missing,
		"tool(s) registered in code but missing from README.md's Implemented Tools table — "+
			"add a row per docs/WORKFLOW.md §7 (\"update documentation as the batch lands\"):\n  %s",
		strings.Join(missing, "\n  "))
}
