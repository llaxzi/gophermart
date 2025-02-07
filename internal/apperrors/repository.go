package apperrors

import "errors"

var (
	ErrLoginTaken         = errors.New("login is already taken")
	ErrPgConnExc          = errors.New("pg connection exception")
	ErrInvalidLP          = errors.New("wrong login or password")
	ErrOrderInserted      = errors.New("you already loaded this order")
	ErrOrderInsertedLogin = errors.New("someone else already loaded this order")
	ErrNotEnoughFunds     = errors.New("not enough funds")
)
