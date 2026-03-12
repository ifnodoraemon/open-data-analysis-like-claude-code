package auth

import (
	"context"
	"net/http"
)

type Identity struct {
	UserID      string `json:"userId"`
	UserName    string `json:"userName"`
	UserEmail   string `json:"userEmail"`
	WorkspaceID string `json:"workspaceId"`
	Workspace   string `json:"workspaceName"`
}

type contextKey string

const identityKey contextKey = "identity"

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey, identity)
}

func FromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey).(Identity)
	return identity, ok
}

func Middleware(defaultIdentity Identity) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithIdentity(r.Context(), defaultIdentity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
