package auth

import (
	"context"
	"net/http"
	"strings"

	"cpal/internal/domain"
)

type ctxKey int

const claimsKey ctxKey = 0

// FromContext returns the authenticated claims, if any.
func FromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(claimsKey).(Claims)
	return c, ok
}

// WriteUnauthorized / WriteForbidden are injected by the api package so this
// package stays free of any specific error-envelope dependency.
type ErrorWriter func(w http.ResponseWriter, status int, code, msg string)

// Middleware authenticates requests via the Bearer access token. On success it
// stores the Claims in the request context; otherwise it writes 401.
func (m *Manager) Middleware(writeErr ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authz := r.Header.Get("Authorization")
			raw, ok := strings.CutPrefix(authz, "Bearer ")
			if !ok || raw == "" {
				writeErr(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
				return
			}
			claims, err := m.ParseAccess(strings.TrimSpace(raw))
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns middleware that rejects principals lacking the given role.
func RequireRole(role domain.Role, writeErr ErrorWriter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, ok := FromContext(r.Context())
			if !ok {
				writeErr(w, http.StatusUnauthorized, "unauthorized", "not authenticated")
				return
			}
			if c.Role != role {
				writeErr(w, http.StatusForbidden, "forbidden", "insufficient role")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
