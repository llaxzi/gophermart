package tokens

import (
	"github.com/golang-jwt/jwt/v4"
	"github.com/llaxzi/gophermart/internal/models"
	"time"
)

type TokenBuilder interface {
	BuildJWTString(userLogin string) (string, error)
}

func NewTokenBuilder(secretKey []byte, tokenExp time.Duration) TokenBuilder {
	return &tokenBuilder{secretKey, tokenExp}
}

type tokenBuilder struct {
	secretKey []byte
	tokenExp  time.Duration
}

func (b *tokenBuilder) BuildJWTString(userLogin string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, models.UserClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(b.tokenExp)),
		},
		UserLogin: userLogin,
	})

	tokenString, err := token.SignedString(b.secretKey)
	if err != nil {
		return "", err
	}
	return tokenString, nil
}
