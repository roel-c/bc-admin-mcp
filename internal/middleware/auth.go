package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// BearerAuth returns HTTP middleware that validates a Bearer token in the
// Authorization header. It uses constant-time comparison to prevent timing
// side-channels against the expected token.
func BearerAuth(expectedToken string) func(http.Handler) http.Handler {
	expected := []byte(expectedToken)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, `{"error":"missing Authorization header"}`, http.StatusUnauthorized)
				return
			}

			const prefix = "Bearer "
			if !strings.HasPrefix(auth, prefix) {
				http.Error(w, `{"error":"Authorization header must use Bearer scheme"}`, http.StatusUnauthorized)
				return
			}

			token := []byte(strings.TrimPrefix(auth, prefix))
			if subtle.ConstantTimeCompare(token, expected) != 1 {
				http.Error(w, `{"error":"invalid bearer token"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
