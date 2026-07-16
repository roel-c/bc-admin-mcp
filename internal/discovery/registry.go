package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
)

// ToolDef is a registered tool with its handler and metadata, kept internal
// to the registry. Only stubs are exposed to the LLM via the meta-tools.
type ToolDef struct {
	Path        string          // e.g. "catalog/products/search"
	Tier        middleware.Tier // R0-R4 from BC-Tool-Boundaries.md
	Summary     string          // <=150 chars, shown in discover_tools
	Description string          // Full description, shown on execute
	Tool        mcp.Tool        // Full MCP tool definition with schema
	Handler     server.ToolHandlerFunc
}

// CategoryDef is a node in the tool hierarchy tree.
type CategoryDef struct {
	Path     string
	Summary  string
	Children []string // child category paths or tool paths
}

// Registry holds the two-tier progressive disclosure hierarchy.
// Tier 1: lightweight category/tool stubs navigated via discover_tools.
// Tier 2: full tool schemas + handlers invoked via execute_tool.
type Registry struct {
	mu         sync.RWMutex
	tools      map[string]*ToolDef     // keyed by full path
	categories map[string]*CategoryDef // keyed by category path
}

func NewRegistry() *Registry {
	return &Registry{
		tools:      make(map[string]*ToolDef),
		categories: make(map[string]*CategoryDef),
	}
}

// RegisterCategory adds a category node to the hierarchy.
func (r *Registry) RegisterCategory(path, summary string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.categories[path] = &CategoryDef{
		Path:    path,
		Summary: summary,
	}

	parent := parentPath(path)
	if parent != "" {
		if pCat, ok := r.categories[parent]; ok {
			pCat.Children = append(pCat.Children, path)
		}
	}
}

// RegisterTool adds a tool to the registry and links it into its parent category.
// It panics at startup if an R1+ tool is missing the required "confirmed"
// boolean parameter — catching developer mistakes before any request is served.
func (r *Registry) RegisterTool(def *ToolDef) {
	if middleware.RequiresConfirmation(def.Tier) {
		r.validateConfirmedParam(def)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.Path] = def

	parent := parentPath(def.Path)
	if parent != "" {
		if pCat, ok := r.categories[parent]; ok {
			pCat.Children = append(pCat.Children, def.Path)
		}
	}
}

// validateConfirmedParam ensures R1+ tools declare a "confirmed" boolean
// input property, preventing developers from accidentally skipping the
// preview-then-confirm pattern.
func (r *Registry) validateConfirmedParam(def *ToolDef) {
	props := def.Tool.InputSchema.Properties
	if len(props) == 0 {
		panic(fmt.Sprintf(
			"tool %q (tier %s) has no input properties — "+
				"R1+ tools must declare a 'confirmed' boolean parameter",
			def.Path, def.Tier,
		))
	}
	if _, hasConfirmed := props["confirmed"]; !hasConfirmed {
		panic(fmt.Sprintf(
			"tool %q (tier %s) is missing required 'confirmed' boolean parameter — "+
				"all R1+ tools must support the preview-then-confirm pattern",
			def.Path, def.Tier,
		))
	}
}

// Discover returns the children of a category path as lightweight stubs.
// If path is empty, returns root-level categories.
func (r *Registry) Discover(path string) ([]DiscoveryEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if path == "" {
		return r.rootEntries(), nil
	}

	cat, ok := r.categories[path]
	if !ok {
		return nil, fmt.Errorf("category %q not found", path)
	}

	entries := make([]DiscoveryEntry, 0, len(cat.Children))
	for _, childPath := range cat.Children {
		if childCat, ok := r.categories[childPath]; ok {
			entries = append(entries, DiscoveryEntry{
				Path:    childCat.Path,
				Type:    "category",
				Summary: childCat.Summary,
			})
		} else if tool, ok := r.tools[childPath]; ok {
			entries = append(entries, DiscoveryEntry{
				Path:    tool.Path,
				Type:    "tool",
				Summary: tool.Summary,
				Tier:    string(tool.Tier),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

// GetTool returns a tool definition by path, or nil if not found.
func (r *Registry) GetTool(path string) *ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[path]
}

// ListCategoryPaths returns every registered category path, sorted.
func (r *Registry) ListCategoryPaths() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.categories))
	for p := range r.categories {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// ListToolPaths returns every registered tool path, sorted.
func (r *Registry) ListToolPaths() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for p := range r.tools {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// GetCategory returns the CategoryDef for path, or nil if not found.
func (r *Registry) GetCategory(path string) *CategoryDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.categories[path]
}

// HasCategory reports whether a category path is registered.
func (r *Registry) HasCategory(path string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.categories[path]
	return ok
}

func (r *Registry) rootEntries() []DiscoveryEntry {
	var entries []DiscoveryEntry
	for _, cat := range r.categories {
		if !strings.Contains(cat.Path, "/") {
			entries = append(entries, DiscoveryEntry{
				Path:    cat.Path,
				Type:    "category",
				Summary: cat.Summary,
			})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries
}

func parentPath(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx < 0 {
		return ""
	}
	return path[:idx]
}

// DiscoveryEntry is the lightweight stub returned by discover_tools.
type DiscoveryEntry struct {
	Path    string `json:"path"`
	Type    string `json:"type"`
	Summary string `json:"summary"`
	Tier    string `json:"tier,omitempty"`
}

// MetaTools returns the two MCP tools that implement progressive disclosure:
// discover_tools and execute_tool.
func (r *Registry) MetaTools(tierEnforcer *middleware.TierEnforcer) []server.ServerTool {
	return []server.ServerTool{
		{
			Tool: mcp.NewTool("discover_tools",
				mcp.WithDescription(
					"Navigate the BigCommerce tool hierarchy. "+
						"Pass an empty path to see top-level categories, "+
						"or a category path like 'catalog' or 'catalog/products' to see its children.",
				),
				mcp.WithString("path",
					mcp.Description("Category path to explore. Empty string for root."),
					mcp.DefaultString(""),
				),
			),
			Handler: r.handleDiscover,
		},
		{
			Tool: mcp.NewTool("execute_tool",
				mcp.WithDescription(
					"Execute a BigCommerce tool by its full path. "+
						"Use discover_tools first to find available tools and their paths.",
				),
				mcp.WithString("tool_path",
					mcp.Description("Full tool path, e.g. 'catalog/products/search'"),
					mcp.Required(),
				),
				mcp.WithObject("arguments",
					mcp.Description("Arguments to pass to the tool"),
				),
			),
			Handler: r.handleExecute(tierEnforcer),
		},
	}
}

func (r *Registry) handleDiscover(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := request.GetString("path", "")

	entries, err := r.Discover(path)
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: err.Error()}},
		}, nil
	}

	// Compact JSON (no indentation) keeps discovery responses token-cheap —
	// this is the single most frequent response shape the LLM sees.
	data, _ := json.Marshal(entries)
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
	}, nil
}

func (r *Registry) handleExecute(tierEnforcer *middleware.TierEnforcer) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		toolPath := request.GetString("tool_path", "")
		if toolPath == "" {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "tool_path is required"}},
			}, nil
		}

		def := r.GetTool(toolPath)
		if def == nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: fmt.Sprintf("tool %q not found", toolPath)}},
			}, nil
		}

		if err := tierEnforcer.Check(def.Tier, request); err != nil {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: err.Error()}},
			}, nil
		}

		args := request.GetArguments()
		rawInner := args["arguments"]

		// arguments must be a JSON object (map[string]any) or absent.
		// If the LLM passes a non-object value (e.g. a string or array),
		// reject early with a clear message rather than letting each tool
		// handler emit a confusing "X is required" error from a nil args map.
		var innerArgs map[string]any
		if rawInner != nil {
			var ok bool
			innerArgs, ok = rawInner.(map[string]any)
			if !ok {
				return &mcp.CallToolResult{
					IsError: true,
					Content: []mcp.Content{mcp.TextContent{
						Type: "text",
						Text: fmt.Sprintf(
							"execute_tool: 'arguments' must be a JSON object (got %T) — "+
								"pass tool parameters as a nested object, e.g. {\"tool_path\": %q, \"arguments\": {\"param\": \"value\"}}",
							rawInner, toolPath,
						),
					}},
				}, nil
			}
		}

		innerRequest := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      def.Path,
				Arguments: innerArgs,
			},
		}

		// The logging middleware only sees the meta-tool name (execute_tool);
		// log the resolved inner tool path + latency here so observability
		// reflects the real operation rather than the dispatcher.
		start := time.Now()
		result, err := def.Handler(ctx, innerRequest)
		isErr := err != nil || (result != nil && result.IsError)
		slog.Default().Info("tool_executed",
			"tool_path", def.Path,
			"tier", string(def.Tier),
			"duration_ms", time.Since(start).Milliseconds(),
			"is_error", isErr,
		)
		return result, err
	}
}
