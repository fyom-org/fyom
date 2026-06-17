// Package middleware provides HTTP middleware for the fyom server.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/response"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
)

type contextKey string

const (
	keyUserID   contextKey = "user_id"
	keyUsername contextKey = "username"
	keyRole     contextKey = "role"
)

// GetUserID returns the user_id from the request context.
func GetUserID(r *http.Request) interface{} {
	return r.Context().Value(keyUserID)
}

// GetUsername returns the username from the request context.
func GetUsername(r *http.Request) interface{} {
	return r.Context().Value(keyUsername)
}

// GetRole returns the role from the request context.
func GetRole(r *http.Request) interface{} {
	return r.Context().Value(keyRole)
}

// AuthMiddleware validates JWT tokens from the Authorization header.
// Deprecated: use AuthMiddlewareWithUserRepo for protected routes that must reject deleted/downgraded users.
func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return AuthMiddlewareWithUserRepo(jwtSecret, nil)
}

// AuthMiddlewareWithUserRepo validates JWT tokens and rehydrates the user from the current DB.
// Protected handlers should use this form so deleted/downgraded users are rejected immediately.
func AuthMiddlewareWithUserRepo(jwtSecret string, userRepo *repository.UserRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				response.Error(w, 401, "missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				response.Error(w, 401, "invalid authorization header format")
				return
			}

			claims, err := parseAndValidateToken(parts[1], jwtSecret)
			if err != nil {
				response.Error(w, 401, err.Error())
				return
			}

			userID, _ := claims["sub"].(string)
			if userID == "" {
				response.Error(w, 401, "token missing subject claim")
				return
			}

			if userRepo != nil {
				user, err := userRepo.GetByID(r.Context(), userID)
				if err != nil || user == nil {
					response.Error(w, 401, "unauthorized")
					return
				}
				ctx := context.WithValue(r.Context(), keyUserID, user.ID)
				ctx = context.WithValue(ctx, keyUsername, user.Username)
				ctx = context.WithValue(ctx, keyRole, user.Role)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			ctx := context.WithValue(r.Context(), keyUserID, claims["sub"])
			ctx = context.WithValue(ctx, keyUsername, claims["username"])
			ctx = context.WithValue(ctx, keyRole, claims["role"])
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin is a middleware that rejects requests from non-admin users.
// Must be used after AuthMiddleware so that the role is already in the context.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawRole := GetRole(r)
		roleStr, ok := rawRole.(string)
		if !ok {
			roleStr = fmt.Sprintf("%v", rawRole)
		}
		if roleStr != "admin" {
			slog.Warn("rbac_rejected", "role", roleStr, "path", r.URL.Path)
			response.Error(w, 403, "admin role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// parseAndValidateToken parses a JWT string and returns its claims.
func parseAndValidateToken(tokenString string, secret string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}))

	if err != nil {
		return nil, fmt.Errorf("invalid or expired token")
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	if exp, ok := claims["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}

	if _, ok := claims["sub"]; !ok {
		return nil, fmt.Errorf("token missing subject claim")
	}

	return claims, nil
}

// AllowLocalOnly is a middleware that only allows requests from localhost/loopback.
// This is used to restrict internal endpoints like the desktop bootstrap session
// bridge.
//
// Security note: this check deliberately inspects ONLY r.RemoteAddr and ignores
// X-Forwarded-For / X-Real-IP. The bootstrap session endpoint must remain
// reachable exclusively from the loopback interface; honoring forwarding
// headers would let any client spoof a localhost origin. Do not "fix" this by
// adding XFF handling without a trusted-proxy model.
func AllowLocalOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRemoteAddr(r.RemoteAddr) {
			response.Error(w, 403, "forbidden: localhost only")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isLoopbackRemoteAddr reports whether the given r.RemoteAddr value identifies
// a loopback origin.
//
// Go's net/http server always populates RemoteAddr in "host:port" form —
// "127.0.0.1:54321" for IPv4 and "[::1]:54321" for IPv6 — so net.SplitHostPort
// is the correct parser. The previous strings.LastIndex(":") implementation
// mis-split IPv6 addresses: "[::1]:54321" became "[::1]" (brackets kept, never
// matching the "::1" branch) and a bare "::1" became "::" (split at the last
// colon), both causing legitimate loopback requests to be rejected.
//
// The fallback handles the rare no-port forms ("127.0.0.1", "::1", "[::1]",
// "localhost") seen in some test harnesses and proxies, stripping IPv6
// brackets so the comparison is against the canonical host.
func isLoopbackRemoteAddr(remoteAddr string) bool {
	if remoteAddr == "" {
		return false
	}

	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		// SplitHostPort already strips the brackets from "[::1]:54321" -> "::1".
		host = h
	} else {
		// No port present. Strip IPv6 brackets from a bare "[::1]" form so the
		// comparison below matches "::1".
		host = strings.Trim(host, "[]")
	}

	switch host {
	case "127.0.0.1", "::1", "localhost":
		return true
	default:
		// Also accept any 127.0.0.0/8 address (e.g., 127.1.2.3), which is
		// loopback per RFC 5735 but not the literal "127.0.0.1".
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return true
		}
		return false
	}
}
