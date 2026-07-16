package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/roel-c/bc-admin-mcp/internal/middleware"
	"github.com/stretchr/testify/suite"
)

type BearerAuthSuite struct {
	suite.Suite
	handler http.Handler
}

func TestBearerAuthSuite(t *testing.T) {
	suite.Run(t, new(BearerAuthSuite))
}

func (s *BearerAuthSuite) SetupTest() {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	s.handler = middleware.BearerAuth("test-secret-token")(inner)
}

func (s *BearerAuthSuite) TestValidToken() {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer test-secret-token")
	rec := httptest.NewRecorder()

	s.handler.ServeHTTP(rec, req)
	s.Equal(http.StatusOK, rec.Code)
}

func (s *BearerAuthSuite) TestMissingAuthHeader() {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	s.handler.ServeHTTP(rec, req)
	s.Equal(http.StatusUnauthorized, rec.Code)
}

func (s *BearerAuthSuite) TestWrongScheme() {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	s.handler.ServeHTTP(rec, req)
	s.Equal(http.StatusUnauthorized, rec.Code)
}

func (s *BearerAuthSuite) TestInvalidToken() {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()

	s.handler.ServeHTTP(rec, req)
	s.Equal(http.StatusForbidden, rec.Code)
}

func (s *BearerAuthSuite) TestEmptyBearerValue() {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()

	s.handler.ServeHTTP(rec, req)
	s.Equal(http.StatusForbidden, rec.Code)
}
