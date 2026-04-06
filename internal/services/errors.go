package services

import "errors"

var (
	ErrForbidden         = errors.New("forbidden")
	ErrReadOnly          = errors.New("read-only role")
	ErrNotChatMember     = errors.New("user is not a member of this chat")
	ErrChatNotFound      = errors.New("chat not found")
	ErrDealAlreadyExists = errors.New("deal already exists for lead")

	// Deal validation / domain errors
	ErrLeadIDRequired      = errors.New("lead_id is required")
	ErrClientIDRequired    = errors.New("client_id is required")
	ErrAmountInvalid       = errors.New("amount must be greater than 0")
	ErrDealNotFound        = errors.New("deal not found")
	ErrClientNotFound      = errors.New("client not found")
	ErrClientTypeRequired  = errors.New("client_type is required")
	ErrClientTypeMismatch  = errors.New("client_type does not match stored client type")
	ErrClientTypeImmutable = errors.New("client_type is immutable")
	ErrInvalidEmail        = errors.New("invalid email")
	ErrEmailAlreadyUsed    = errors.New("email already used")

	ErrUnsupportedClientFileCategory  = errors.New("unsupported client file category")
	ErrUnsupportedClientFileExtension = errors.New("unsupported client file extension")
	ErrClientFilePathTraversal        = errors.New("invalid client file path")
	ErrFileRequired                   = errors.New("file is required")
)
