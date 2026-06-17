// Package middleware provides HTTP middleware for the fyom server.
package middleware

import (
	"context"
	"net/http"

	"github.com/fyom/fyom/pkg/presign"
)

const (
	ctxKeyPresignedAccess contextKey = "presigned_access"
	ctxKeyPresignedPath   contextKey = "presigned_path"
)

// RequireValidPresign returns middleware that validates presigned URL
// query parameters on each request.
//
// Required query parameters:
//   - exp
//   - sig
//
// Valid presigned requests are marked in request context using
// ctxKeyPresignedAccess. Media handlers can then treat the request as
// authorized for the signed resource path without requiring JWT headers.
//
// This middleware is used for media endpoints accessed directly by <img> and
// <video> tags, where Authorization headers are not reliably available.
func RequireValidPresign(signer *presign.Signer) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if signer == nil {
				http.Error(w, "presign signer unavailable", http.StatusInternalServerError)
				return
			}

			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}

			path := r.URL.Path
			exp := r.URL.Query().Get("exp")
			sig := r.URL.Query().Get("sig")

			if !signer.ValidateMethod(r.Method, path, exp, sig) {
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			w.Header().Set("Cache-Control", "public, max-age=3600, immutable")

			ctx := context.WithValue(r.Context(), ctxKeyPresignedAccess, true)
			ctx = context.WithValue(ctx, ctxKeyPresignedPath, path)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsPresignedAccess reports whether the request has passed presigned URL
// validation.
func IsPresignedAccess(r *http.Request) bool {
	if r == nil {
		return false
	}

	ok, _ := r.Context().Value(ctxKeyPresignedAccess).(bool)

	return ok
}

// GetPresignedPath returns the request path that was validated by presign
// middleware.
func GetPresignedPath(r *http.Request) string {
	if r == nil {
		return ""
	}

	path, _ := r.Context().Value(ctxKeyPresignedPath).(string)

	return path
}
