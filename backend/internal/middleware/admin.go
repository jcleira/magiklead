package middleware

import (
	"net/http"
	"strings"
)

// AdminAuth gates admin-only endpoints. It assumes ClerkAuth has
// already populated UserClerkIDKey + UserEmailKey, and rejects any
// caller whose email isn't in `adminEmails`. The check is performed
// entirely from JWT claims — no DB round-trip — so denied requests
// don't touch the database.
//
// adminEmails is normalized lowercase at construction; the env-var
// reader (cmd/api/main.go) does the splitting on commas.
func AdminAuth(adminEmails []string) func(http.Handler) http.Handler {
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
			if GetUserClerkID(r.Context()) == "" {
				http.Error(w, `{"code":"unauthorized","message":"Missing user"}`, http.StatusUnauthorized)
				return
			}
			email := GetUserEmail(r.Context())
			if email == "" {
				http.Error(w, `{"code":"forbidden","message":"Missing email claim"}`, http.StatusForbidden)
				return
			}
			if _, ok := allow[email]; !ok {
				http.Error(w, `{"code":"forbidden","message":"Admin access required"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}
}
