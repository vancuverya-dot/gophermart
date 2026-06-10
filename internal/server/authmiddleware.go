package server

import (
	"context"
	"errors"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const uidKey contextKey = "uid"

// GetUID — возвращает uid пользователя из контекста.
func GetUID(ctx context.Context) string {
	uid, _ := ctx.Value(uidKey).(string)
	return uid
}

type Claims struct {
	UserID string `json:"user_id"`
	Login  string `json:"login"`
	jwt.RegisteredClaims
}

var ErrInvalidToken = errors.New("bad token")

func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tokenStr string

		cookie, err := r.Cookie("token")
		if err == nil {
			tokenStr = cookie.Value
		}

		if tokenStr == "" {
			tokenStr = r.Header.Get("Authorization")
		}

		if tokenStr == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidToken
			}
			return []byte(s.cfg.TokenKey), nil
		})

		if err != nil || !token.Valid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		claims := token.Claims.(*Claims)
		ctx := context.WithValue(r.Context(), uidKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
