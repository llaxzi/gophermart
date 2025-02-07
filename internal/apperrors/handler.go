package apperrors

import "errors"

var (
	ErrServer       = errors.New("server error")
	ErrInvalidJSON  = errors.New("invalid JSON")
	ErrNoData       = errors.New("no data")
	ErrInvalidOrder = errors.New("invalid order number")
)
