package config_test

import (
	"os"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/stretchr/testify/suite"
)

type ConfigValidationSuite struct {
	suite.Suite
	origEnv map[string]string
}

func TestConfigValidationSuite(t *testing.T) {
	suite.Run(t, new(ConfigValidationSuite))
}

var envKeys = []string{
	"BC_STORE_HASH", "BC_AUTH_TOKEN", "BC_REQUESTS_PER_SECOND",
	"BC_MAX_RETRIES", "BC_PRODUCT_BATCH_SIZE", "BC_VARIANT_BATCH_SIZE",
	"BC_DEFAULT_PAGE_LIMIT", "BC_MAX_TOTAL_RECORDS", "BC_CACHE_TTL_SECONDS",
	"MCP_TRANSPORT", "MCP_AUTH_TOKEN", "BC_QUOTA_SAFETY_BUFFER",
	"BC_INVENTORY_BATCH_SIZE", "BC_DELAY_BETWEEN_CHUNKS_MS",
	"BC_MAX_WRITE_CONCURRENCY", "MCP_SERVER_NAME", "MCP_SERVER_VERSION",
	"MCP_ADDRESS", "MCP_PORT",
}

func (s *ConfigValidationSuite) SetupTest() {
	s.origEnv = make(map[string]string)
	for _, k := range envKeys {
		s.origEnv[k] = os.Getenv(k)
	}
	s.setDefaults()
}

func (s *ConfigValidationSuite) TearDownTest() {
	for k, v := range s.origEnv {
		if v == "" {
			os.Unsetenv(k)
		} else {
			os.Setenv(k, v)
		}
	}
}

func (s *ConfigValidationSuite) setDefaults() {
	os.Setenv("BC_STORE_HASH", "test-hash")
	os.Setenv("BC_AUTH_TOKEN", "test-token")
	os.Setenv("MCP_TRANSPORT", "stdio")
	for _, k := range envKeys {
		if k != "BC_STORE_HASH" && k != "BC_AUTH_TOKEN" && k != "MCP_TRANSPORT" {
			os.Unsetenv(k)
		}
	}
}

func (s *ConfigValidationSuite) TestValidDefaultConfig() {
	cfg, err := config.Load()
	s.NoError(err)
	s.NotNil(cfg)
}

func (s *ConfigValidationSuite) TestMissingStoreHash() {
	os.Unsetenv("BC_STORE_HASH")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "BC_STORE_HASH")
}

func (s *ConfigValidationSuite) TestMissingAuthToken() {
	os.Unsetenv("BC_AUTH_TOKEN")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "BC_AUTH_TOKEN")
}

func (s *ConfigValidationSuite) TestZeroRequestsPerSecond() {
	os.Setenv("BC_REQUESTS_PER_SECOND", "0")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "requests per second")
}

func (s *ConfigValidationSuite) TestExcessiveRequestsPerSecond() {
	os.Setenv("BC_REQUESTS_PER_SECOND", "31")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "requests per second")
}

func (s *ConfigValidationSuite) TestMaxRetriesTooLow() {
	os.Setenv("BC_MAX_RETRIES", "0")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "max retries")
}

func (s *ConfigValidationSuite) TestMaxRetriesTooHigh() {
	os.Setenv("BC_MAX_RETRIES", "21")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "max retries")
}

func (s *ConfigValidationSuite) TestProductBatchSizeTooLow() {
	os.Setenv("BC_PRODUCT_BATCH_SIZE", "0")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "product batch size")
}

func (s *ConfigValidationSuite) TestProductBatchSizeTooHigh() {
	os.Setenv("BC_PRODUCT_BATCH_SIZE", "11")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "product batch size")
}

func (s *ConfigValidationSuite) TestPageLimitTooLow() {
	os.Setenv("BC_DEFAULT_PAGE_LIMIT", "0")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "default page limit")
}

func (s *ConfigValidationSuite) TestPageLimitTooHigh() {
	os.Setenv("BC_DEFAULT_PAGE_LIMIT", "251")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "default page limit")
}

func (s *ConfigValidationSuite) TestNegativeMaxTotalRecords() {
	os.Setenv("BC_MAX_TOTAL_RECORDS", "-1")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "max total records")
}

func (s *ConfigValidationSuite) TestHTTPTransportRequiresAuthToken() {
	os.Setenv("MCP_TRANSPORT", "streamable-http")
	os.Unsetenv("MCP_AUTH_TOKEN")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "MCP_AUTH_TOKEN")
}

func (s *ConfigValidationSuite) TestSSETransportRequiresAuthToken() {
	os.Setenv("MCP_TRANSPORT", "sse")
	os.Unsetenv("MCP_AUTH_TOKEN")
	_, err := config.Load()
	s.Error(err)
	s.Contains(err.Error(), "MCP_AUTH_TOKEN")
}

func (s *ConfigValidationSuite) TestStdioTransportNoAuthRequired() {
	os.Setenv("MCP_TRANSPORT", "stdio")
	os.Unsetenv("MCP_AUTH_TOKEN")
	cfg, err := config.Load()
	s.NoError(err)
	s.NotNil(cfg)
}

func (s *ConfigValidationSuite) TestHTTPTransportWithAuthSucceeds() {
	os.Setenv("MCP_TRANSPORT", "streamable-http")
	os.Setenv("MCP_AUTH_TOKEN", "my-secret")
	cfg, err := config.Load()
	s.NoError(err)
	s.Equal("my-secret", cfg.Server.AuthToken)
}
