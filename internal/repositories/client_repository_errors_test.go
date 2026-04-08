package repositories

import (
	"errors"
	"testing"

	"github.com/lib/pq"
)

func TestIsProfileSplitTableMissing(t *testing.T) {
	err := &pq.Error{Code: pq.ErrorCode(SQLStateUndefinedTable), Message: `relation "client_individual_profiles" does not exist`}
	if !isProfileSplitTableMissing(err) {
		t.Fatal("expected true for undefined client_individual_profiles")
	}

	err = &pq.Error{Code: pq.ErrorCode(SQLStateUndefinedTable), Message: `relation "client_legal_profiles" does not exist`}
	if !isProfileSplitTableMissing(err) {
		t.Fatal("expected true for undefined client_legal_profiles")
	}

	err = &pq.Error{Code: pq.ErrorCode(SQLStateUndefinedTable), Message: `relation "users" does not exist`}
	if isProfileSplitTableMissing(err) {
		t.Fatal("expected false for unrelated undefined table")
	}

	if isProfileSplitTableMissing(errors.New("plain error")) {
		t.Fatal("expected false for non-pq error")
	}
}
