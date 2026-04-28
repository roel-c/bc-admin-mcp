package middleware

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// WithLogging wraps tool handlers with structured logging for observability.
func WithLogging(logger *slog.Logger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()

			logger.Info("tool call started",
				"tool", request.Params.Name,
			)

			result, err := next(ctx, request)
			elapsed := time.Since(start)

			if err != nil {
				logger.Error("tool call failed",
					"tool", request.Params.Name,
					"duration_ms", elapsed.Milliseconds(),
					"error", err,
				)
				return result, err
			}

			if result != nil && result.IsError {
				logger.Warn("tool call returned error",
					"tool", request.Params.Name,
					"duration_ms", elapsed.Milliseconds(),
				)
			} else {
				logger.Info("tool call completed",
					"tool", request.Params.Name,
					"duration_ms", elapsed.Milliseconds(),
				)
			}

			return result, nil
		}
	}
}
