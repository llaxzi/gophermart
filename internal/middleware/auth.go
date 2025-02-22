package middleware

import (
	"fmt"
	"github.com/golang-jwt/jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/llaxzi/gophermart/internal/models"
	"net/http"
	"strings"
)

func (m *middleware) Auth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		authHeader := ctx.Request().Header.Get("Authorization")
		if authHeader == "" {
			return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "Authorization header required"})
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

		claims := &models.UserClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return m.secretKey, nil
		})
		if err != nil || !token.Valid {
			return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": "Invalid token"})
		}
		ctx.Set("user_login", claims.UserLogin)
		return next(ctx)
	}
}
