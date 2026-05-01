package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/clerk/clerk-sdk-go/v2/jwt"
)

type contextKey string

const (
	UserClerkIDKey contextKey = "userClerkID"
	UserEmailKey   contextKey = "userEmail"
)

// sessionCustomClaims mirrors the claims injected by the Clerk
// "magiklead-backend" JWT template. The template must include an
// `email` claim resolving to `{{user.primary_email_address}}` so
// AdminAuth can gate callers without a DB lookup.
type sessionCustomClaims struct {
	Email string `json:"email"`
}

// ClerkAuth verifies the Clerk session JWT carried in the
// Authorization header, extracts the user's clerk id + primary email,
// and stashes both in context. Invalid/expired tokens yield 401 before
// any DB query.
//
// Requires clerk.SetKey() to have been called at startup (see
// cmd/api/main.go).
func ClerkAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"code":"unauthorized","message":"Missing authorization header"}`, http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == authHeader || token == "" {
			http.Error(w, `{"code":"unauthorized","message":"Invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		claims, err := jwt.Verify(r.Context(), &jwt.VerifyParams{
			Token: token,
			CustomClaimsConstructor: func(context.Context) any {
				return &sessionCustomClaims{}
			},
		})
		if err != nil {
			http.Error(w, `{"code":"unauthorized","message":"Invalid or expired token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserClerkIDKey, claims.Subject)
		if custom, ok := claims.Custom.(*sessionCustomClaims); ok && custom.Email != "" {
			ctx = context.WithValue(ctx, UserEmailKey, strings.ToLower(custom.Email))
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserClerkID extracts the Clerk user ID from the request context.
func GetUserClerkID(ctx context.Context) string {
	id, _ := ctx.Value(UserClerkIDKey).(string)
	return id
}

// GetUserEmail extracts the Clerk-verified primary email from context.
// Returns empty string when the JWT lacked the custom `email` claim.
func GetUserEmail(ctx context.Context) string {
	email, _ := ctx.Value(UserEmailKey).(string)
	return email
}
