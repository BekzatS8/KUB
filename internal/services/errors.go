package services

import "errors"

var (
	ErrForbidden         = errors.New("forbidden")
	ErrReadOnly          = errors.New("read-only role")
	ErrNotChatMember     = errors.New("user is not a member of this chat")
	ErrChatNotFound      = errors.New("chat not found")
	ErrDealAlreadyExists = errors.New("deal already exists for lead")

	// Deal validation / domain errors
	ErrLeadIDRequired          = errors.New("lead_id is required")
	ErrClientIDRequired        = errors.New("client_id is required")
	ErrAmountInvalid           = errors.New("amount must be greater than 0")
	ErrDealNotFound            = errors.New("deal not found")
	ErrLeadNotFound            = errors.New("lead not found")
	ErrClientNotFound          = errors.New("client not found")
	ErrClientTypeRequired      = errors.New("client_type is required")
	ErrInvalidClientType       = errors.New("invalid client_type")
	ErrClientTypeMismatch      = errors.New("client_type does not match stored client type")
	ErrClientTypeImmutable     = errors.New("client_type is immutable")
	ErrClientRepoNotConfigured = errors.New("client repository not configured")
	ErrInvalidEmail            = errors.New("invalid email")
	ErrEmailAlreadyUsed        = errors.New("email already used")
	ErrClientAlreadyExists     = errors.New("client already exists")
	ErrIndividualIINExists     = errors.New("individual profile with this IIN already exists")
	ErrLegalBINExists          = errors.New("legal profile with this BIN already exists")
	ErrClientFilePrimaryExists = errors.New("primary file for this category already exists")
	ErrClientInUse             = errors.New("client has linked entities")
	ErrPublicLinkAlreadyUsed   = errors.New("public link already used")
	ErrResetTokenAlreadyUsed   = errors.New("password reset token already used")
	ErrTelegramLinkAlreadyUsed = errors.New("telegram link already used")
	ErrWazzupIntegrationExists = errors.New("wazzup integration already exists")
	ErrSchemaMismatch          = errors.New("database schema mismatch")
	ErrInvalidState            = errors.New("invalid state")
	ErrIllegalStatusTransition = errors.New("illegal status transition")

	ErrUnsupportedClientFileCategory  = errors.New("unsupported client file category")
	ErrUnsupportedClientFileExtension = errors.New("unsupported client file extension")
	ErrClientFilePathTraversal        = errors.New("invalid client file path")
	ErrFileRequired                   = errors.New("file is required")
)

type DealAlreadyExistsError struct {
	LeadID         int
	ExistingDealID int
}

func (e *DealAlreadyExistsError) Error() string { return ErrDealAlreadyExists.Error() }
func (e *DealAlreadyExistsError) Unwrap() error { return ErrDealAlreadyExists }
