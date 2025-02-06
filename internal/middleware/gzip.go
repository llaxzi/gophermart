package middleware

import (
	"compress/gzip"
	"github.com/labstack/echo/v4"
	"io"
	"net/http"
	"strings"
)

type gzipWriter struct {
	http.ResponseWriter
	Writer io.Writer
}

func (w *gzipWriter) Write(p []byte) (int, error) {
	return w.Writer.Write(p)
}

func (m *middleware) Gzip(next echo.HandlerFunc) echo.HandlerFunc {
	return func(ctx echo.Context) error {
		// request - *http.Request
		request := ctx.Request()
		// response - *echo.Response
		response := ctx.Response()

		if strings.Contains(request.Header.Get("Content-Encoding"), "gzip") {
			gzipReader, err := gzip.NewReader(request.Body)
			if err != nil {
				return ctx.JSON(http.StatusBadRequest, map[string]string{"error": "invalid gzip request body"})
			}
			defer gzipReader.Close()
			request.Body = io.NopCloser(gzipReader)
		}

		if !strings.Contains(request.Header.Get("Accept-Encoding"), "gzip") {
			return next(ctx)
		}

		response.Header().Set("Content-Encoding", "gzip")

		gz := gzip.NewWriter(response.Writer)
		defer gz.Close()

		response.Writer = &gzipWriter{response.Writer, gz}

		return next(ctx)
	}
}
