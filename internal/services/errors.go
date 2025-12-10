package services

import "errors"

var (
	ErrForbidden = errors.New("forbidden")
	ErrReadOnly  = errors.New("read-only role")
)
