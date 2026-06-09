// Package middleware provides HTTP middleware for the fyom server.
package middleware

import (
	"net/http"

	"github.com/fyom/fyom/pkg/presign"
)

// RequireValidPresign returns middleware that validates presigned URL
// query parameters (exp + sig) on each request. Requests without valid
// signatures are rejected with 403 Forbidden.
//
// On success, sets Cache-Control: public, max-age=3600, immutable.
// This middleware does NOT check JWT — it is used on media endpoints
// that are accessed directly by <img> and <video> tags.
func RequireValidPresign(signer *presign.Signer) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			exp := r.URL.Query().Get("exp")
			sig := r.URL.Query().Get("sig")

			if !signer.Validate(path, exp, sig) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			w.Header().Set("Cache-Control", "public, max-age=3600, immutable")
			next.ServeHTTP(w, r)
		})
	}
}
