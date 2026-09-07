package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"github.com/dchote/livestream-viewer/internal/model"
)

type ctxKey int

const userKey ctxKey = 1

func UserFromContext(ctx context.Context) *model.User {
	u, _ := ctx.Value(userKey).(*model.User)
	return u
}

func Auth(db *gorm.DB, secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			raw := ""
			if strings.HasPrefix(header, "Bearer ") {
				raw = strings.TrimPrefix(header, "Bearer ")
			} else if q := r.URL.Query().Get("access_token"); q != "" {
				raw = q
			}
			if raw == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
				return
			}
			tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
				if t.Method != jwt.SigningMethodHS256 {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})
			if err != nil || !tok.Valid {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token", nil)
				return
			}
			claims, ok := tok.Claims.(jwt.MapClaims)
			if !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token", nil)
				return
			}
			sub, _ := claims["sub"].(string)
			id, err := strconv.ParseUint(sub, 10, 64)
			if err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token", nil)
				return
			}
			var user model.User
			if err := db.First(&user, id).Error; err != nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token", nil)
				return
			}
			ctx := context.WithValue(r.Context(), userKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole must run after Auth.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFromContext(r.Context())
			if u == nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required", nil)
				return
			}
			if !allowed[u.Role] {
				WriteError(w, http.StatusForbidden, "forbidden", "insufficient permissions", nil)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// EnforcePasswordChange must run after Auth. Users with MustChangePassword may only
// hit /auth/me and /auth/change-password until they set a new password.
func EnforcePasswordChange(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFromContext(r.Context())
		if u != nil && u.MustChangePassword {
			switch r.URL.Path {
			case "/api/v1/auth/me", "/api/v1/auth/change-password":
			default:
				WriteError(w, http.StatusForbidden, "password_change_required", "password must be changed before continuing", nil)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
