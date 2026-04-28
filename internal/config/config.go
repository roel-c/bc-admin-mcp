package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportStreamableHTTP Transport = "streamable-http"
	TransportSSE            Transport = "sse"
)

type Config struct {
	BigCommerce BigCommerceConfig
	Server      ServerConfig
}

type BigCommerceConfig struct {
	StoreHash string
	AuthToken string

	RequestsPerSecond    float64
	QuotaSafetyBuffer    int
	MaxRetries           int
	ProductBatchSize     int
	VariantBatchSize     int
	InventoryBatchSize   int
	DefaultPageLimit     int
	MaxTotalRecords      int
	DelayBetweenChunks   time.Duration
	MaxWriteConcurrency  int
	CacheTTL             time.Duration
}

type ServerConfig struct {
	Name      string
	Version   string
	Transport Transport
	Address   string
	Port      int
	AuthToken string // Bearer token required for HTTP/SSE transports; empty disables auth
}

func Load() (*Config, error) {
	storeHash := os.Getenv("BC_STORE_HASH")
	if storeHash == "" {
		return nil, fmt.Errorf("BC_STORE_HASH environment variable is required")
	}

	authToken := os.Getenv("BC_AUTH_TOKEN")
	if authToken == "" {
		return nil, fmt.Errorf("BC_AUTH_TOKEN environment variable is required")
	}

	cfg := &Config{
		BigCommerce: BigCommerceConfig{
			StoreHash:           storeHash,
			AuthToken:           authToken,
			RequestsPerSecond:   envFloat("BC_REQUESTS_PER_SECOND", 2.0),
			QuotaSafetyBuffer:   envInt("BC_QUOTA_SAFETY_BUFFER", 25),
			MaxRetries:          envInt("BC_MAX_RETRIES", 6),
			ProductBatchSize:    envInt("BC_PRODUCT_BATCH_SIZE", 10),
			VariantBatchSize:    envInt("BC_VARIANT_BATCH_SIZE", 10),
			InventoryBatchSize:  envInt("BC_INVENTORY_BATCH_SIZE", 10),
			DefaultPageLimit:    envInt("BC_DEFAULT_PAGE_LIMIT", 250),
			MaxTotalRecords:     envInt("BC_MAX_TOTAL_RECORDS", 10000),
			DelayBetweenChunks:  time.Duration(envInt("BC_DELAY_BETWEEN_CHUNKS_MS", 500)) * time.Millisecond,
			MaxWriteConcurrency: envInt("BC_MAX_WRITE_CONCURRENCY", 1),
			CacheTTL:            time.Duration(envInt("BC_CACHE_TTL_SECONDS", 60)) * time.Second,
		},
		Server: ServerConfig{
			Name:      envStr("MCP_SERVER_NAME", "bigcommerce-mcp"),
			Version:   envStr("MCP_SERVER_VERSION", "0.1.0"),
			Transport: Transport(envStr("MCP_TRANSPORT", "stdio")),
			Address:   envStr("MCP_ADDRESS", "127.0.0.1"),
			Port:      envInt("MCP_PORT", 8080),
			AuthToken: os.Getenv("MCP_AUTH_TOKEN"),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.BigCommerce.RequestsPerSecond <= 0 {
		return fmt.Errorf("requests per second must be > 0, got %f", c.BigCommerce.RequestsPerSecond)
	}
	if c.BigCommerce.RequestsPerSecond > 30 {
		return fmt.Errorf("requests per second exceeds BigCommerce safe limit of 30, got %f", c.BigCommerce.RequestsPerSecond)
	}
	if c.BigCommerce.MaxRetries < 1 || c.BigCommerce.MaxRetries > 20 {
		return fmt.Errorf("max retries must be 1-20, got %d", c.BigCommerce.MaxRetries)
	}
	if c.BigCommerce.ProductBatchSize < 1 || c.BigCommerce.ProductBatchSize > 10 {
		return fmt.Errorf("product batch size must be 1-10, got %d", c.BigCommerce.ProductBatchSize)
	}
	if c.BigCommerce.VariantBatchSize < 1 || c.BigCommerce.VariantBatchSize > 10 {
		return fmt.Errorf("variant batch size must be 1-10, got %d", c.BigCommerce.VariantBatchSize)
	}
	if c.BigCommerce.DefaultPageLimit < 1 || c.BigCommerce.DefaultPageLimit > 250 {
		return fmt.Errorf("default page limit must be 1-250, got %d", c.BigCommerce.DefaultPageLimit)
	}
	if c.BigCommerce.MaxTotalRecords < 0 {
		return fmt.Errorf("max total records must be >= 0 (0 = unlimited), got %d", c.BigCommerce.MaxTotalRecords)
	}
	if c.BigCommerce.CacheTTL < 0 {
		return fmt.Errorf("cache TTL must be >= 0, got %v", c.BigCommerce.CacheTTL)
	}
	switch c.Server.Transport {
	case TransportStdio:
		// stdio is inherently process-local; no auth needed
	case TransportStreamableHTTP, TransportSSE:
		if c.Server.AuthToken == "" {
			return fmt.Errorf(
				"MCP_AUTH_TOKEN is required for %s transport — "+
					"set it to a strong random secret to authenticate clients",
				c.Server.Transport,
			)
		}
	default:
		return fmt.Errorf("unsupported transport: %s", c.Server.Transport)
	}
	return nil
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
