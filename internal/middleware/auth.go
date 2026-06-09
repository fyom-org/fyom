// Package middleware provides HTTP middleware for the fyom server.
package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

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
// On success, it injects user_id, username, and role into the request context.
func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
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

			ctx := context.WithValue(r.Context(), keyUserID, claims["sub"])
			ctx = context.WithValue(ctx, keyUsername, claims["username"])
			ctx = context.WithValue(ctx, keyRole, claims["role"])
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
