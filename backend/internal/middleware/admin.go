package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

const AdminUserIDKey contextKey = "adminUserID"

// AdminAuth gates admin-only endpoints. It assumes ClerkAuth has
// already populated UserClerkIDKey and rejects any caller whose
// resolved email isn't in `adminEmails`. The matched user's UUID is
// stashed in context so merge handlers can attribute merge_history
// rows.
//
// adminEmails is normalized lowercase at construction; the env-var
// reader (cmd/api/main.go) does the splitting on commas.
func AdminAuth(queries *repository.Queries, adminEmails []string) func(http.Handler) http.Handler {
	allow := make(map[string]struct{}, len(adminEmails))
	for _, e := range adminEmails {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" {
			allow[e] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allow) == 0 {
				http.Error(w, `{"code":"forbidden","message":"Admin access not configured"}`, http.StatusForbidden)
				return
			}
			clerkID := GetUserClerkID(r.Context())
			if clerkID == "" {
				http.Error(w, `{"code":"unauthorized","message":"Missing user"}`, http.StatusUnauthorized)
				return
			}

			user, err := queries.GetUserByClerkID(r.Context(), clerkID)
			if err != nil {
				http.Error(w, `{"code":"forbidden","message":"User not found"}`, http.StatusForbidden)
				return
			}
			if _, ok := allow[strings.ToLower(user.Email)]; !ok {
				http.Error(w, `{"code":"forbidden","message":"Admin access required"}`, http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), AdminUserIDKey, uuid.UUID(user.ID.Bytes))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetAdminUserID returns the UUID of the authenticated admin user.
func GetAdminUserID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(AdminUserIDKey).(uuid.UUID)
	return id
}
