package services

import "errors"

var (
	ErrForbidden     = errors.New("forbidden")
	ErrReadOnly      = errors.New("read-only role")
	ErrNotChatMember = errors.New("user is not a member of this chat")
	ErrChatNotFound  = errors.New("chat not found")
)
