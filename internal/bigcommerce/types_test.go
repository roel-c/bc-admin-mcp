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

func (s *APIErrorScopeHintSuite) TestForbiddenPriceListsScopeHint() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodGet,
		Path:       "pricelists",
	}
	s.Contains(e.Error(), "store_price_lists")
	s.Contains(e.SafeError(), "store_price_lists")
}

func (s *APIErrorScopeHintSuite) TestForbiddenOrderPaymentActionsScopeHint() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodPost,
		Path:       "orders/44/payment_actions/refunds",
	}
	s.Contains(e.Error(), "store_v2_orders")
	s.Contains(e.Error(), "store_v2_transactions")
}

func (s *APIErrorScopeHintSuite) TestForbiddenOrderTransactionsScopeHint() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodGet,
		Path:       "orders/44/transactions",
	}
	s.Contains(e.Error(), "store_v2_orders")
	s.Contains(e.Error(), "store_v2_transactions")
}

func (s *APIErrorScopeHintSuite) TestForbiddenInventoryScopeHint() {
	e := &bigcommerce.APIError{
		StatusCode: http.StatusForbidden,
		Method:     http.MethodGet,
		Path:       "inventory/items",
	}
	s.Contains(e.Error(), "store_inventory")
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
	s.Contains(e.Error(), "BigCommerce API returned status 422")
}

// BigCommerce V3 structured error bodies (title + field errors) must be
// surfaced so callers can diagnose 4xx failures.
func (s *APIErrorScopeHintSuite) TestV3ErrorTitleAndFieldErrorsSurfaced() {
	e := &bigcommerce.APIError{
		StatusCode: 422,
		Method:     http.MethodPost,
		Path:       "catalog/products/506/modifiers",
		Body: []byte(`{"status":422,"title":"JSON data is missing or invalid",` +
			`"type":"https://developer.bigcommerce.com/api-docs/getting-started/api-status-codes",` +
			`"errors":{"type":"required field is missing"}}`),
	}
	msg := e.SafeError()
	s.Contains(msg, "422")
	s.Contains(msg, "JSON data is missing or invalid")
	s.Contains(msg, "type: required field is missing")
}

// V2-style array error bodies ([{status, message}]) must also be surfaced.
func (s *APIErrorScopeHintSuite) TestV2ArrayErrorMessageSurfaced() {
	e := &bigcommerce.APIError{
		StatusCode: 400,
		Body:       []byte(`[{"status":400,"message":"The field 'email' is invalid."}]`),
	}
	s.Contains(e.SafeError(), "The field 'email' is invalid.")
}

// B2B Edition errors use {"code":422,"data":{"errMsg":"..."},"meta":{"message":"..."}}
// rather than the core BC V3 {title,detail,errors} shape.
func (s *APIErrorScopeHintSuite) TestB2BErrMsgSurfaced() {
	e := &bigcommerce.APIError{
		StatusCode: 422,
		Body:       []byte(`{"code":422,"data":{"errMsg":"Custom shipping is not enabled"},"meta":{"message":"Custom shipping is not enabled"}}`),
	}
	msg := e.SafeError()
	s.Contains(msg, "422")
	s.Contains(msg, "Custom shipping is not enabled")
}

// B2B field-validation errors use {"data":{"firstName":["This field may not be null."]}}.
func (s *APIErrorScopeHintSuite) TestB2BFieldErrorsSurfaced() {
	e := &bigcommerce.APIError{
		StatusCode: 422,
		Body:       []byte(`{"code":422,"data":{"firstName":["This field may not be null."]},"meta":{"message":"VALIDATION"}}`),
	}
	msg := e.SafeError()
	s.Contains(msg, "VALIDATION")
	s.Contains(msg, "firstName")
	s.Contains(msg, "This field may not be null.")
}

// meta.message of "SUCCESS" (seen on some 2xx-shaped error bodies) should not
// be surfaced as if it were an error detail.
func (s *APIErrorScopeHintSuite) TestB2BSuccessMetaMessageNotSurfacedAsError() {
	e := &bigcommerce.APIError{
		StatusCode: 404,
		Body:       []byte(`{"code":404,"data":{},"meta":{"message":"SUCCESS"}}`),
	}
	msg := e.SafeError()
	s.NotContains(msg, "SUCCESS")
}
