package repositories

import "errors"

var (
	ErrDealAlreadyExists  = errors.New("deal already exists")
	ErrClientNotFound     = errors.New("client not found")
	ErrClientFileNotFound = errors.New("client file not found")
	ErrAuditSchemaMissing = errors.New("audit_logs table is missing")
)
