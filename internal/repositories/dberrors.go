package repositories

import (
	"errors"

	"github.com/lib/pq"
)

const (
	SQLStateUniqueViolation = "23505"
	SQLStateForeignKey      = "23503"
	SQLStateCheckViolation  = "23514"
	SQLStateUndefinedTable  = "42P01"
)

func AsPQError(err error) (*pq.Error, bool) {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return nil, false
	}
	return pqErr, true
}

func IsSQLState(err error, state string) bool {
	pqErr, ok := AsPQError(err)
	return ok && string(pqErr.Code) == state
}

func ConstraintName(err error) string {
	pqErr, ok := AsPQError(err)
	if !ok {
		return ""
	}
	return string(pqErr.Constraint)
}
