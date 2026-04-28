package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	bcserver "github.com/roel-c/bc-admin-mcp/internal/server"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	mcpServer := bcserver.New(cfg, logger)

	logger.Info("starting BigCommerce MCP server",
		"name", cfg.Server.Name,
		"version", cfg.Server.Version,
		"transport", cfg.Server.Transport,
	)

	switch cfg.Server.Transport {
	case config.TransportStdio:
		if err := server.ServeStdio(mcpServer); err != nil {
			log.Fatalf("stdio server error: %v", err)
		}

	case config.TransportStreamableHTTP:
		addr := fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
		authMw := middleware.BearerAuth(cfg.Server.AuthToken)
		httpTransport := server.NewStreamableHTTPServer(mcpServer)
		handler := authMw(httpTransport)
		logger.Info("listening on "+addr, "auth", "bearer-token")
		srv := &http.Server{Addr: addr, Handler: handler}
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}

	case config.TransportSSE:
		addr := fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port)
		authMw := middleware.BearerAuth(cfg.Server.AuthToken)
		sseTransport := server.NewSSEServer(mcpServer)
		handler := authMw(sseTransport)
		logger.Info("listening on "+addr, "auth", "bearer-token")
		srv := &http.Server{Addr: addr, Handler: handler}
		if err := srv.ListenAndServe(); err != nil {
			log.Fatalf("SSE server error: %v", err)
		}

	default:
		log.Fatalf("unsupported transport: %s", cfg.Server.Transport)
	}
}
