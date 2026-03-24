// Package middleware contains HTTP middleware for the VoidDB API server.
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/voiddb/void/internal/auth"
	"go.uber.org/zap"
)

// contextKey is a private type for context values to avoid collisions.
type contextKey string

const (
	// ClaimsKey is the context key for the authenticated user's JWT claims.
	ClaimsKey contextKey = "void_claims"
)

// ClaimsFromContext retrieves the JWT Claims stored in the request context
// by the Auth middleware.  Returns nil if no claims are present.
func ClaimsFromContext(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(ClaimsKey).(*auth.Claims)
	return c
}

// Auth returns HTTP middleware that validates the Bearer token in the
// Authorization header.  Requests without a valid token receive 401.
func Auth(svc *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				http.Error(w, `{"error":"missing authorization token"}`, http.StatusUnauthorized)
				return
			}
			claims, err := svc.ValidateAccessToken(token)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole returns HTTP middleware that rejects requests whose JWT role is
// below the required level.  Must be used after Auth middleware.
func RequireRole(required auth.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthenticated"}`, http.StatusUnauthorized)
				return
			}
			if !roleAtLeast(claims.Role, required) {
				http.Error(w, `{"error":"insufficient permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORS adds permissive CORS headers for development and cross-origin frontends.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	wildcard := false
	for _, o := range allowedOrigins {
		if o == "*" {
			wildcard = true
		}
		originSet[o] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := wildcard
			if !allowed {
				_, allowed = originSet[origin]
			}
			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Void-Token")
			w.Header().Set("Access-Control-Max-Age", "86400")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// JSON sets the Content-Type response header to application/json.
func JSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

// RequestLogger writes one structured log entry per HTTP request.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.Method == http.MethodOptions {
			return
		}
		zap.L().Info("http request",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.Int("status", rec.status),
			zap.Duration("duration", time.Since(start)),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// extractBearerToken pulls the token string from "Authorization: Bearer <tok>".
func extractBearerToken(r *http.Request) string {
	hdr := r.Header.Get("Authorization")
	if hdr == "" {
		// Also accept the token as a query parameter for WebSocket clients.
		return r.URL.Query().Get("token")
	}
	if strings.HasPrefix(hdr, "Bearer ") {
		return strings.TrimPrefix(hdr, "Bearer ")
	}
	return ""
}

// roleAtLeast returns true if have ≥ required in the role hierarchy.
func roleAtLeast(have, required auth.Role) bool {
	order := map[auth.Role]int{
		auth.RoleReadOnly:  1,
		auth.RoleReadWrite: 2,
		auth.RoleAdmin:     3,
	}
	return order[have] >= order[required]
}
