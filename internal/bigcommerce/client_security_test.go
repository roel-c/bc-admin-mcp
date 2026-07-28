package bigcommerce_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/roel-c/bc-admin-mcp/internal/config"
	"github.com/stretchr/testify/suite"
)

type ClientSecuritySuite struct {
	suite.Suite
	logger *slog.Logger
}

func TestClientSecuritySuite(t *testing.T) {
	suite.Run(t, new(ClientSecuritySuite))
}

func (s *ClientSecuritySuite) SetupTest() {
	s.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
}

type securityRoundTripFunc func(*http.Request) (*http.Response, error)

func (f securityRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func (s *ClientSecuritySuite) failingHTTPClient(cancel context.CancelFunc, secret string) *http.Client {
	return &http.Client{Transport: securityRoundTripFunc(func(_ *http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"debug":"` + secret + `"}`)),
		}, nil
	})}
}

func (s *ClientSecuritySuite) TestManagementFinalRetryErrorOmitsResponseBody() {
	const secret = "management-internal-secret"
	ctx, cancel := context.WithCancel(context.Background())
	client := bigcommerce.NewClientWithHTTPClient(config.BigCommerceConfig{
		RequestsPerSecond: 1000,
		MaxRetries:        1,
	}, s.logger, s.failingHTTPClient(cancel, secret))
	s.T().Cleanup(client.Close)

	_, _, err := client.Do(ctx, http.MethodGet, "https://example.invalid/v3/catalog/products", nil)
	s.Require().Error(err)
	s.Contains(err.Error(), "502")
	s.NotContains(err.Error(), secret)
	s.NotContains(err.Error(), `"debug"`)
}

func (s *ClientSecuritySuite) TestB2BFinalRetryErrorOmitsResponseBody() {
	const secret = "b2b-internal-secret"
	ctx, cancel := context.WithCancel(context.Background())
	client := bigcommerce.NewB2BClientWithHTTPClient(
		"hash", "token", 1, s.logger, s.failingHTTPClient(cancel, secret),
	)
	s.T().Cleanup(client.Close)

	_, err := client.Do(ctx, http.MethodGet, "https://example.invalid/companies", nil)
	s.Require().Error(err)
	s.Contains(err.Error(), "502")
	s.NotContains(err.Error(), secret)
	s.NotContains(err.Error(), `"debug"`)
}

func (s *ClientSecuritySuite) TestB2BMultipartFinalRetryErrorOmitsResponseBody() {
	const secret = "b2b-multipart-internal-secret"
	ctx, cancel := context.WithCancel(context.Background())
	client := bigcommerce.NewB2BClientWithHTTPClient(
		"hash", "token", 1, s.logger, s.failingHTTPClient(cancel, secret),
	)
	s.T().Cleanup(client.Close)

	_, err := client.B2BPostMultipart(ctx, "companies/42/attachments", "attachmentFile", "file.txt", []byte("file"))
	s.Require().Error(err)
	s.Contains(err.Error(), "502")
	s.NotContains(err.Error(), secret)
	s.NotContains(err.Error(), `"debug"`)
}
