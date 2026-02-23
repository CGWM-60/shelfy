package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
)

type authContextKey string

const authUserContextKey authContextKey = "auth_user"

type APIAuthManager interface {
	Enabled() bool
	AuthenticateRequest(r *http.Request) (string, bool)
}

func RequireAPIAuth(manager APIAuthManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if manager == nil || !manager.Enabled() {
				next.ServeHTTP(w, r)
				return
			}
			username, ok := manager.AuthenticateRequest(r)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			ctx := context.WithValue(r.Context(), authUserContextKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func AuthUserFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if username, ok := ctx.Value(authUserContextKey).(string); ok {
		return username
	}
	return ""
}
