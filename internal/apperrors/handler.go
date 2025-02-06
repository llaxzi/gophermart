package apperrors

import "errors"

var (
	ErrServer      = errors.New("server error")
	ErrInvalidJSON = errors.New("invalid JSON")
)
