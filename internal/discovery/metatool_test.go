package discovery_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/discovery"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/stretchr/testify/suite"
)

type MetaToolSuite struct {
	suite.Suite
	registry   *discovery.Registry
	enforcer   *middleware.TierEnforcer
	discoverFn func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
	executeFn  func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

func TestMetaToolSuite(t *testing.T) {
	suite.Run(t, new(MetaToolSuite))
}

func (s *MetaToolSuite) SetupTest() {
	s.registry = discovery.NewRegistry()
	s.enforcer = middleware.NewTierEnforcer()

	s.registry.RegisterCategory("catalog", "Product catalog")
	s.registry.RegisterCategory("catalog/products", "Product operations")
	s.registry.RegisterCategory("orders", "Order management")

	handlerCalled := false
	s.registry.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/search",
		Tier:        middleware.TierR0,
		Summary:     "Search products",
		Description: "Search products",
		Tool:        toolWithoutConfirmed("search", "Search"),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			handlerCalled = true
			_ = handlerCalled
			args := req.GetArguments()
			resp := map[string]any{"received_args": args, "handler": "search"}
			data, _ := json.Marshal(resp)
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(data)}},
			}, nil
		},
	})

	s.registry.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/update",
		Tier:        middleware.TierR1,
		Summary:     "Update products",
		Description: "Update products",
		Tool:        toolWithConfirmed("update", "Update"),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "update called"}},
			}, nil
		},
	})

	metaTools := s.registry.MetaTools(s.enforcer)
	for _, st := range metaTools {
		if st.Tool.Name == "discover_tools" {
			s.discoverFn = st.Handler
		} else if st.Tool.Name == "execute_tool" {
			s.executeFn = st.Handler
		}
	}
	s.Require().NotNil(s.discoverFn)
	s.Require().NotNil(s.executeFn)
}

func (s *MetaToolSuite) callDiscover(path string) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "discover_tools",
			Arguments: map[string]any{"path": path},
		},
	}
	return s.discoverFn(context.Background(), req)
}

func (s *MetaToolSuite) callExecute(toolPath string, args map[string]any) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "execute_tool",
			Arguments: map[string]any{
				"tool_path": toolPath,
				"arguments": args,
			},
		},
	}
	return s.executeFn(context.Background(), req)
}

func (s *MetaToolSuite) parseJSON(result *mcp.CallToolResult) any {
	s.Require().NotEmpty(result.Content)
	text := result.Content[0].(mcp.TextContent).Text
	var data any
	s.Require().NoError(json.Unmarshal([]byte(text), &data))
	return data
}

// --- discover_tools tests ---

func (s *MetaToolSuite) TestDiscoverRootReturnsCategories() {
	result, err := s.callDiscover("")
	s.NoError(err)
	s.False(result.IsError)

	entries := s.parseJSON(result).([]any)
	s.Len(entries, 2)

	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.(map[string]any)["path"].(string)
	}
	s.Contains(paths, "catalog")
	s.Contains(paths, "orders")
}

func (s *MetaToolSuite) TestDiscoverDrillIntoCategory() {
	result, err := s.callDiscover("catalog")
	s.NoError(err)

	entries := s.parseJSON(result).([]any)
	s.Len(entries, 1)
	s.Equal("catalog/products", entries[0].(map[string]any)["path"])
	s.Equal("category", entries[0].(map[string]any)["type"])
}

func (s *MetaToolSuite) TestDiscoverShowsTools() {
	result, err := s.callDiscover("catalog/products")
	s.NoError(err)

	entries := s.parseJSON(result).([]any)
	s.Len(entries, 2)

	for _, e := range entries {
		entry := e.(map[string]any)
		s.Equal("tool", entry["type"])
	}
}

func (s *MetaToolSuite) TestDiscoverNonexistentReturnsError() {
	result, err := s.callDiscover("nonexistent")
	s.NoError(err)
	s.True(result.IsError)
}

// --- execute_tool tests ---

func (s *MetaToolSuite) TestExecuteDispatchesToHandler() {
	result, err := s.callExecute("catalog/products/search", map[string]any{
		"keyword": "shoes",
	})
	s.NoError(err)
	s.False(result.IsError)

	data := s.parseJSON(result).(map[string]any)
	s.Equal("search", data["handler"])
}

func (s *MetaToolSuite) TestExecutePassesArguments() {
	result, err := s.callExecute("catalog/products/search", map[string]any{
		"keyword": "hats",
		"limit":   float64(10),
	})
	s.NoError(err)

	data := s.parseJSON(result).(map[string]any)
	received := data["received_args"].(map[string]any)
	s.Equal("hats", received["keyword"])
	s.Equal(float64(10), received["limit"])
}

func (s *MetaToolSuite) TestExecuteMissingToolPathReturnsError() {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "execute_tool",
			Arguments: map[string]any{},
		},
	}
	result, err := s.executeFn(context.Background(), req)
	s.NoError(err)
	s.True(result.IsError)
}

func (s *MetaToolSuite) TestExecuteUnknownToolReturnsError() {
	result, err := s.callExecute("nonexistent/tool", nil)
	s.NoError(err)
	s.True(result.IsError)
}

func (s *MetaToolSuite) TestExecuteNilArgumentsHandledGracefully() {
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "execute_tool",
			Arguments: map[string]any{
				"tool_path": "catalog/products/search",
			},
		},
	}
	result, err := s.executeFn(context.Background(), req)
	s.NoError(err)
	s.False(result.IsError)
}

func (s *MetaToolSuite) TestExecuteR4ToolBlocked() {
	s.registry.RegisterTool(&discovery.ToolDef{
		Path:        "catalog/products/forbidden",
		Tier:        middleware.TierR4,
		Summary:     "Forbidden op",
		Description: "Blocked",
		Tool:        mcp.NewTool("forbidden", mcp.WithDescription("forbidden")),
		Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "should not reach"}},
			}, nil
		},
	})

	result, err := s.callExecute("catalog/products/forbidden", nil)
	s.NoError(err)
	s.True(result.IsError)
	text := result.Content[0].(mcp.TextContent).Text
	s.Contains(text, "forbidden")
}
