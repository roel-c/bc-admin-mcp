package bigcommerce_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/bigcommerce"
	"github.com/stretchr/testify/suite"
)

type APIErrorScopeHintSuite struct {
	suite.Suite
}

func TestAPIErrorScopeHintSuite(t *testing.T) {
	suite.Run(t, new(APIErrorScopeHintSuite))
}

func (s *APIErrorScopeHintSuite) TestForbiddenChannelListingsRead() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodGet,
		Path:       "channels/3/listings",
	}
	s.Contains(e.Error(), "store_channel_listings_read_only")
	s.Contains(e.SafeError(), "store_channel_listings_read_only")
}

func (s *APIErrorScopeHintSuite) TestForbiddenChannelListingsWrite() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodPut,
		Path:       "channels/3/listings",
	}
	s.Contains(e.Error(), "store_channel_listings")
	s.False(strings.Contains(e.Error(), "read_only"))
}

func (s *APIErrorScopeHintSuite) TestForbiddenChannelsList() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodGet,
		Path:       "channels",
	}
	s.Contains(e.Error(), "store_channel_settings_read_only")
}

func (s *APIErrorScopeHintSuite) TestForbiddenChannelAssignmentsRead() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodGet,
		Path:       "catalog/products/channel-assignments",
	}
	s.Contains(e.Error(), "store_v2_products_read_only")
}

func (s *APIErrorScopeHintSuite) TestForbiddenCatalogProductsWrite() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodPut,
		Path:       "catalog/products/123",
	}
	s.Contains(e.Error(), "store_v2_products")
	s.False(strings.Contains(e.Error(), "read_only"))
}

func (s *APIErrorScopeHintSuite) TestUnauthorizedHasGenericHint() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusUnauthorized,
		Method:     http.MethodGet,
		Path:       "catalog/products",
	}
	s.Contains(e.Error(), "X-Auth-Token")
}

func (s *APIErrorScopeHintSuite) TestEmptyPathFallsBack() {
	e := &bigcommerce.APIError{
		StatusCode: 422,
		Body:       []byte("{}"),
	}
	s.Contains(e.Error(), "BigCommerce API error 422")
}
