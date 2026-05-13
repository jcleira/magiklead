package middleware

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jcleira/magiklead/backend/internal/bootstrap"
	"github.com/jcleira/magiklead/backend/internal/repository"
)

const TenantIDKey contextKey = "tenantID"

// TenantLookupFunc resolves the authenticated user's tenant. It must
// return (tenantID, true, nil) when the user has a tenant,
// (uuid.Nil, false, nil) when no user row exists for clerkID, and a
// non-nil error for unexpected database failures.
type TenantLookupFunc func(ctx context.Context, clerkID string) (uuid.UUID, bool, error)

// BootstrapFunc provisions a fresh user + tenant + role + free
// subscription for a Clerk identity. The signature is the same shape
// as bootstrap.Bootstrap, minus the pool — wire the pool at
// construction time.
type BootstrapFunc func(ctx context.Context, clerkID, email, displayName string) (uuid.UUID, error)

// EnsureTenant guarantees a tenant ID in the request context for any
// authenticated request, lazy-bootstrapping one on the first request
// for a fresh Clerk identity.
//
// Contract:
//   - No clerk ID in context → pass through (the auth layer will have
//     already returned 401 for protected routes; for public routes we
//     don't need a tenant).
//   - Lookup succeeds with a tenant → tenant lands in context.
//   - Lookup says "no user row" → call bootstrap. On success, tenant
//     lands in context. On bootstrap.ErrNoEmail, return 503 with a
//     body that names the JWT-template misconfiguration. On any other
//     bootstrap error, return 500.
//   - Lookup itself errors (DB connectivity, etc.) → return 500.
func EnsureTenant(lookup TenantLookupFunc, boot BootstrapFunc) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clerkID := GetUserClerkID(r.Context())
			if clerkID == "" {
				next.ServeHTTP(w, r)
				return
			}

			tenantID, found, err := lookup(r.Context(), clerkID)
			if err != nil {
				log.Printf("ensure tenant: lookup failed for clerk_id=%s: %v", clerkID, err)
				http.Error(w, `{"code":"internal","message":"Tenant lookup failed"}`, http.StatusInternalServerError)
				return
			}
			if !found {
				email := GetUserEmail(r.Context())
				tenantID, err = boot(r.Context(), clerkID, email, "")
				if err != nil {
					if errors.Is(err, bootstrap.ErrNoEmail) {
						log.Printf("ensure tenant: bootstrap aborted for clerk_id=%s: %v", clerkID, err)
						http.Error(w, `{"code":"jwt_template_misconfigured","message":"Bootstrap requires the `+"`email`"+` claim. Configure the `+"`magiklead-backend`"+` Clerk JWT template to include {\"email\": \"{{user.primary_email_address}}\"}."}`, http.StatusServiceUnavailable)
						return
					}
					log.Printf("ensure tenant: bootstrap failed for clerk_id=%s: %v", clerkID, err)
					http.Error(w, `{"code":"internal","message":"Tenant bootstrap failed"}`, http.StatusInternalServerError)
					return
				}
			}

			ctx := context.WithValue(r.Context(), TenantIDKey, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// NewLookup builds a TenantLookupFunc backed by sqlc queries. Returns
// (uuid.Nil, false, nil) when the user row is missing — the
// pgx.ErrNoRows case is the lazy-bootstrap signal, not an error.
func NewLookup(queries *repository.Queries) TenantLookupFunc {
	return func(ctx context.Context, clerkID string) (uuid.UUID, bool, error) {
		user, err := queries.GetUserByClerkID(ctx, clerkID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return uuid.Nil, false, nil
			}
			return uuid.Nil, false, err
		}
		tenants, err := queries.ListTenantsByUser(ctx, pgtype.UUID{Bytes: user.ID.Bytes, Valid: true})
		if err != nil {
			return uuid.Nil, false, err
		}
		if len(tenants) == 0 {
			// User row exists but has no tenant — treat as
			// not-found so bootstrap re-creates the tenant.
			return uuid.Nil, false, nil
		}
		return uuid.UUID(tenants[0].ID.Bytes), true, nil
	}
}

// NewBootstrap is a thin adapter from bootstrap.Bootstrap to the
// BootstrapFunc shape used by EnsureTenant. The pool is captured here
// so the middleware itself doesn't need to know about pgx.
func NewBootstrap(pool *pgxpool.Pool) BootstrapFunc {
	return func(ctx context.Context, clerkID, email, displayName string) (uuid.UUID, error) {
		return bootstrap.Bootstrap(ctx, pool, clerkID, email, displayName)
	}
}
