package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type APIError struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
}

const (
	BadRequestCode    = "BAD_REQUEST"
	UnauthorizedCode  = "UNAUTHORIZED"
	ForbiddenCode     = "FORBIDDEN"
	NotFoundCode      = "NOT_FOUND"
	ConflictCode      = "CONFLICT"
	InternalErrorCode = "INTERNAL_ERROR"

	DealNotFoundCode       = "DEAL_NOT_FOUND"
	LeadNotFoundCode       = "LEAD_NOT_FOUND"
	DocumentNotFound       = "DOCUMENT_NOT_FOUND"
	ClientNotFoundCode     = "CLIENT_NOT_FOUND"
	ReadOnlyRoleCode       = "READ_ONLY_ROLE"
	InvalidEmailCode       = "INVALID_EMAIL"
	InvalidDateFormatCode  = "INVALID_DATE_FORMAT"
	EmailAlreadyUsedCode   = "EMAIL_ALREADY_USED"
	UnsupportedDocType     = "UNSUPPORTED_DOC_TYPE"
	InvalidStatusCode      = "INVALID_STATUS"
	ValidationFailed       = "VALIDATION_FAILED"
	ExpiredCode            = "EXPIRED"
	DealAlreadyExistsCode  = "DEAL_ALREADY_EXISTS_FOR_LEAD"
	ClientAlreadyExists    = "CLIENT_ALREADY_EXISTS"
	ClientInUseCode        = "CLIENT_IN_USE"
	ChatNotFoundCode       = "CHAT_NOT_FOUND"
	ChatNotMemberCode      = "CHAT_NOT_MEMBER"
	ChatForbiddenCode      = "CHAT_FORBIDDEN"
	ChatUserNotFoundCode   = "CHAT_USER_NOT_FOUND"
	ChatUserInactiveCode   = "CHAT_USER_INACTIVE"
	DirectChatWithSelfCode = "DIRECT_CHAT_WITH_SELF"
	ChatInvalidPayloadCode = "CHAT_INVALID_PAYLOAD"
	ChatConflictCode       = "CHAT_CONFLICT"
)

func writeError(c *gin.Context, status int, code string, msg string) {
	c.JSON(status, APIError{
		ErrorCode: code,
		Message:   msg,
	})
}

func writeErrorWithDetails(c *gin.Context, status int, code string, msg string, details any) {
	c.JSON(status, APIError{
		ErrorCode: code,
		Message:   msg,
		Details:   details,
	})
}

func badRequest(c *gin.Context, msg string) {
	writeError(c, http.StatusBadRequest, BadRequestCode, msg)
}

func unauthorized(c *gin.Context, msg string) {
	writeError(c, http.StatusUnauthorized, UnauthorizedCode, msg)
}

func forbidden(c *gin.Context, msg string) {
	writeError(c, http.StatusForbidden, ForbiddenCode, msg)
}

func notFound(c *gin.Context, domainCode string, msg string) {
	writeError(c, http.StatusNotFound, domainCode, msg)
}

func conflict(c *gin.Context, domainCode string, msg string) {
	writeError(c, http.StatusConflict, domainCode, msg)
}

func internalError(c *gin.Context, msg string) {
	writeError(c, http.StatusInternalServerError, InternalErrorCode, msg)
}

func gone(c *gin.Context, code string, msg string) {
	if code == "" {
		code = ExpiredCode
	}
	writeError(c, http.StatusGone, code, msg)
}

func badRequestWithCode(c *gin.Context, code string, msg string) {
	writeError(c, http.StatusBadRequest, code, msg)
}
