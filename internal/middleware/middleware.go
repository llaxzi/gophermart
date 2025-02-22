package middleware

import (
	"github.com/labstack/echo/v4"
)

type Middleware interface {
	Auth(next echo.HandlerFunc) echo.HandlerFunc
	Gzip(next echo.HandlerFunc) echo.HandlerFunc
}

func NewMiddleware(secretKey []byte) Middleware {
	return &middleware{secretKey}
}

type middleware struct {
	secretKey []byte
}
