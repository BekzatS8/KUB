package repositories

import "errors"

var (
	ErrDealAlreadyExists = errors.New("deal already exists")
	ErrClientNotFound    = errors.New("client not found")
)
