package storefront

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/roel-c/bc-admin-mcp/internal/tools/shared"
)

func toolError(format string, args ...any) *mcp.CallToolResult {
	return shared.ToolError(format, args...)
}

func toolJSON(data any) (*mcp.CallToolResult, error) {
	return shared.ToolJSON(data)
}
