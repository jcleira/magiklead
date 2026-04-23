package middleware

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const UserClerkIDKey contextKey = "userClerkID"

// ClerkAuth verifies the Clerk JWT from the Authorization header.
// For now this is a placeholder that extracts the bearer token.
// Full Clerk SDK verification will be wired in T16.
func ClerkAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"code":"unauthorized","message":"Missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader {
			http.Error(w, `{"code":"unauthorized","message":"Invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		// TODO(T16): Verify JWT with Clerk SDK and extract session claims
		// For now, pass token as clerk ID for development
		ctx := context.WithValue(r.Context(), UserClerkIDKey, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserClerkID extracts the Clerk user ID from the request context.
func GetUserClerkID(ctx context.Context) string {
	id, _ := ctx.Value(UserClerkIDKey).(string)
	return id
}
