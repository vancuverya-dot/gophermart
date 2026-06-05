package server

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/vancuverya-dot/gophermart/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID string `json:"user_id"`
	Login  string `json:"login"`
	jwt.RegisteredClaims
}

var ErrInvalidToken = errors.New("bad token")

func AuthMiddleware(next http.Handler) http.Handler {
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
			return []byte(config.TokenKey), nil
		})

		if err != nil || !token.Valid {
			log.Printf("token validation failed: err=%v valid=%v tokenStr=%s", err, token.Valid, tokenStr) // ← сюда
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if err != nil || !token.Valid {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		claims := token.Claims.(*Claims)
		ctx := context.WithValue(r.Context(), "uid", claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
