// Package middleware содержит HTTP-middleware сервиса.
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/zyablitskiy/team-manager/internal/pkg/httpx"
	"github.com/zyablitskiy/team-manager/internal/pkg/jwt"
)

type ctxKey string

const userIDKey ctxKey = "user_id"

// TokenParser проверяет токен доступа.
type TokenParser interface {
	Parse(token string) (*jwt.Claims, error)
}

// Auth — middleware проверки JWT. Кладёт user_id в контекст запроса.
func Auth(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				httpx.Fail(w, httpx.Unauthorized("authorization header required"))
				return
			}

			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				httpx.Fail(w, httpx.Unauthorized("invalid authorization header"))
				return
			}

			claims, err := parser.Parse(strings.TrimSpace(parts[1]))
			if err != nil {
				httpx.Fail(w, httpx.Unauthorized("invalid or expired token"))
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserID извлекает идентификатор пользователя из контекста.
func UserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)

	return id, ok
}
