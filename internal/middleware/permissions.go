package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/response"
)

const ctxKeyAllowedLibraries contextKey = "allowed_libraries"

// GetAllowedLibraryIDs returns the list of library IDs the user is allowed to see.
// nil means no filter (admin sees all).
func GetAllowedLibraryIDs(r *http.Request) []string {
	v := r.Context().Value(ctxKeyAllowedLibraries)
	if v == nil {
		return nil
	}
	ids, _ := v.([]string)
	return ids
}

// ResolvePermissions is middleware that loads the user's library permissions
// and stores them in the request context. Admins get nil (no filter).
func ResolvePermissions(libPermRepo *repository.LibraryPermissionRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := GetUserID(r)
			role := GetRole(r)
			var allowedIDs []string
			if role == "admin" {
				allowedIDs = nil // nil means no filter (admin sees all)
			} else {
				userIDStr, ok := userID.(string)
				if !ok {
					response.Error(w, 401, "unauthorized")
					return
				}
				ids, err := libPermRepo.GetUserLibraries(r.Context(), userIDStr)
				if err != nil {
					slog.Error("failed to resolve permissions", "error", err)
					response.Error(w, 500, "internal server error")
					return
				}
				allowedIDs = ids
			}
			ctx := context.WithValue(r.Context(), ctxKeyAllowedLibraries, allowedIDs)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
