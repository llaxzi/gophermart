package tokens

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/llaxzi/gophermart/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestBuildJWTString(t *testing.T) {
	secretKey := []byte("test_secret")
	tokenExp := time.Minute * 1
	builder := NewTokenBuilder(secretKey, tokenExp)

	userLogin := "test_user"
	tokenString, err := builder.BuildJWTString(userLogin)

	assert.NoError(t, err)
	assert.NotEmpty(t, tokenString)

	token, err := jwt.ParseWithClaims(tokenString, &models.UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		return secretKey, nil
	})

	assert.NoError(t, err)
	assert.NotNil(t, token)
	assert.True(t, token.Valid)

	claims, ok := token.Claims.(*models.UserClaims)
	assert.True(t, ok)
	assert.Equal(t, userLogin, claims.UserLogin)

	assert.WithinDuration(t, time.Now().Add(tokenExp), claims.ExpiresAt.Time, time.Second*2)
}
