package middleware

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"github.com/labstack/echo/v4"
	"github.com/llaxzi/gophermart/internal/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddleware_Auth(t *testing.T) {

	tokenB := tokens.NewTokenBuilder([]byte("test"), time.Minute)
	mw := NewMiddleware([]byte("test"))

	e := echo.New()
	nextHandler := func(ctx echo.Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"message": "Success"})
	}

	t.Run("Missing Authorization Header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)

		handler := mw.Auth(nextHandler)
		err := handler(ctx)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var resp map[string]string
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Authorization header required", resp["error"])
	})

	t.Run("Invalid Token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid_token")
		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)

		handler := mw.Auth(nextHandler)
		err := handler(ctx)

		require.NoError(t, err)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)

		var resp map[string]string
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Invalid token", resp["error"])
	})

	t.Run("Valid Token", func(t *testing.T) {
		login := "test_user"
		validToken, err := tokenB.BuildJWTString(login)
		assert.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+validToken)
		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)

		handler := mw.Auth(nextHandler)
		err = handler(ctx)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)

		userLogin := ctx.Get("user_login")
		assert.Equal(t, login, userLogin)
	})

}

func TestMiddleware_Gzip(t *testing.T) {
	e := echo.New()
	mw := NewMiddleware([]byte("test_secret"))

	nextHandler := func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"message": "Success"})
	}

	t.Run("Request with Gzip-Encoded Body", func(t *testing.T) {
		requestBody := `{"data":"test"}`
		compressedBody, err := gzipCompress([]byte(requestBody))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(compressedBody))
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)

		handler := mw.Gzip(nextHandler)
		err = handler(ctx)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("Invalid Gzip Request Body", func(t *testing.T) {
		invalidGzipBody := []byte("not_gzip_data")

		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(invalidGzipBody))
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)

		handler := mw.Gzip(nextHandler)
		err := handler(ctx)

		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp map[string]string
		err = json.Unmarshal(rec.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "invalid gzip request body", resp["error"])
	})

	t.Run("Response with Gzip Encoding", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")

		rec := httptest.NewRecorder()
		ctx := e.NewContext(req, rec)

		handler := mw.Gzip(nextHandler)
		err := handler(ctx)

		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))

		gzReader, err := gzip.NewReader(rec.Body)
		require.NoError(t, err)
		defer gzReader.Close()

		uncompressedBody, err := io.ReadAll(gzReader)
		require.NoError(t, err)

		var resp map[string]string
		err = json.Unmarshal(uncompressedBody, &resp)
		require.NoError(t, err)
		assert.Equal(t, "Success", resp["message"])
	})
}

func gzipCompress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, err := gz.Write(data)
	if err != nil {
		return nil, err
	}
	gz.Close()
	return buf.Bytes(), nil
}
