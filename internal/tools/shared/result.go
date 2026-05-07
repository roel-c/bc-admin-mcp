package shared

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// ToolError returns a CallToolResult flagged as an error with a formatted message.
func ToolError(format string, args ...any) *mcp.CallToolResult {
	msg := fmt.Sprintf(format, args...)
	if len(msg) > 1000 {
		msg = msg[:1000] + "... (truncated)"
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: msg},
		},
	}
}

// ToolJSON marshals data and returns it as a CallToolResult text payload.
func ToolJSON(data any) (*mcp.CallToolResult, error) {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return ToolError("failed to marshal response: %v", err), nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: string(raw)},
		},
	}, nil
}
