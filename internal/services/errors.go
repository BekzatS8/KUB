package services

import "errors"

var (
	ErrForbidden         = errors.New("forbidden")
	ErrReadOnly          = errors.New("read-only role")
	ErrNotChatMember     = errors.New("user is not a member of this chat")
	ErrChatNotFound      = errors.New("chat not found")
	ErrDealAlreadyExists = errors.New("deal already exists for lead")

	// Deal validation / domain errors
	ErrLeadIDRequired   = errors.New("lead_id is required")
	ErrClientIDRequired = errors.New("client_id is required")
	ErrAmountInvalid    = errors.New("amount must be greater than 0")
	ErrDealNotFound     = errors.New("deal not found")
)
