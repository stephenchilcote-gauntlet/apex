package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

// APIKeyAuth returns middleware requiring a valid API key on every request.
// The key may be supplied as:
//
//	Authorization: Bearer <key>
//	X-API-Key: <key>
//
// If apiKey is empty the middleware is a no-op (dev / test mode).
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if apiKey == "" {
			return next // auth disabled
		}
		expected := []byte(apiKey)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var provided string
			if v := r.Header.Get("X-API-Key"); v != "" {
				provided = v
			} else if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				provided = strings.TrimPrefix(auth, "Bearer ")
			}

			if subtle.ConstantTimeCompare([]byte(provided), expected) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
