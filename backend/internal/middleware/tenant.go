package middleware

import (
	"context"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

const TenantIDKey contextKey = "tenantID"

// WithTenant looks up the authenticated user's default tenant and puts it in context.
func WithTenant(queries *repository.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clerkID := GetUserClerkID(r.Context())
			if clerkID == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Look up user by clerk ID
			user, err := queries.GetUserByClerkID(r.Context(), clerkID)
			if err != nil {
				log.Printf("tenant middleware: user not found for clerk_id=%s: %v", clerkID, err)
				next.ServeHTTP(w, r)
				return
			}

			// Get user's first tenant
			tenants, err := queries.ListTenantsByUser(r.Context(), pgtype.UUID{Bytes: user.ID.Bytes, Valid: true})
			if err != nil || len(tenants) == 0 {
				log.Printf("tenant middleware: no tenants for user %s", user.ID.Bytes)
				next.ServeHTTP(w, r)
				return
			}

			tenantID := uuid.UUID(tenants[0].ID.Bytes)
			ctx := context.WithValue(r.Context(), TenantIDKey, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
