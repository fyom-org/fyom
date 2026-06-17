package middleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/response"
)

const (
	ctxKeyAllowedLibraries    contextKey = "allowed_libraries"
	ctxKeyPermissionsResolved contextKey = "permissions_resolved"
)

// GetAllowedLibraryIDs returns the list of library IDs the current request can access.
//
// Semantics:
//   - nil means unrestricted access, typically admin or owner.
//   - empty slice means no library access.
//   - non-empty slice means access is restricted to those library IDs.
//
// Important security behavior:
// If ResolvePermissions was not executed, non-privileged users fail closed and
// receive an empty slice. This prevents accidentally treating missing middleware
// as unrestricted access.
func GetAllowedLibraryIDs(r *http.Request) []string {
	if r == nil {
		return []string{}
	}

	if ArePermissionsResolved(r) {
		value := r.Context().Value(ctxKeyAllowedLibraries)
		if value == nil {
			return nil
		}

		ids, ok := value.([]string)
		if !ok {
			return []string{}
		}

		return ids
	}

	// Compatibility fallback for routes that have not yet been wired through
	// ResolvePermissions. Non-privileged users fail closed.
	if isPrivilegedRole(getRoleString(r)) {
		return nil
	}

	return []string{}
}

// ArePermissionsResolved reports whether ResolvePermissions has run for this request.
func ArePermissionsResolved(r *http.Request) bool {
	if r == nil {
		return false
	}

	resolved, ok := r.Context().Value(ctxKeyPermissionsResolved).(bool)

	return ok && resolved
}

// IsUnrestrictedLibraryAccess reports whether the request has unrestricted library access.
//
// This returns true only when GetAllowedLibraryIDs returns nil.
func IsUnrestrictedLibraryAccess(r *http.Request) bool {
	return GetAllowedLibraryIDs(r) == nil
}

// IsLibraryAllowed reports whether the request can access the given library ID.
func IsLibraryAllowed(r *http.Request, libraryID string) bool {
	if libraryID == "" {
		return false
	}

	allowedIDs := GetAllowedLibraryIDs(r)

	// nil means unrestricted access.
	if allowedIDs == nil {
		return true
	}

	for _, allowedID := range allowedIDs {
		if allowedID == libraryID {
			return true
		}
	}

	return false
}

// ResolvePermissions loads the current user's library permissions and stores
// them in request context.
//
// Admins and owners receive nil allowed IDs, which means unrestricted access.
// Regular users receive an explicit slice. If they have no library permissions,
// that slice is empty.
func ResolvePermissions(libPermRepo *repository.LibraryPermissionRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := getRoleString(r)

			if isPrivilegedRole(role) {
				ctx := withResolvedLibraryPermissions(r.Context(), nil)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			userID := GetUserID(r)
			userIDStr, ok := userID.(string)
			if !ok || userIDStr == "" {
				response.Error(w, 401, "unauthorized")
				return
			}

			ids, err := libPermRepo.GetUserLibraries(r.Context(), userIDStr)
			if err != nil {
				slog.Error(
					"failed to resolve library permissions",
					"user_id", userIDStr,
					"error", err,
				)
				response.Error(w, 500, "internal server error")
				return
			}

			if ids == nil {
				ids = []string{}
			}

			ctx := withResolvedLibraryPermissions(r.Context(), ids)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func withResolvedLibraryPermissions(ctx context.Context, allowedIDs []string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyPermissionsResolved, true)
	ctx = context.WithValue(ctx, ctxKeyAllowedLibraries, allowedIDs)

	return ctx
}

func getRoleString(r *http.Request) string {
	if r == nil {
		return ""
	}

	role := GetRole(r)
	roleStr, ok := role.(string)
	if !ok {
		return ""
	}

	return roleStr
}

func isPrivilegedRole(role string) bool {
	return role == "admin" || role == "owner"
}
