package repositories

import (
	"strings"
	"testing"
)

// TestBuildDocumentListWhere_HiddenVisibilityFilter checks that when HiddenVisibilityUserID
// is set, the generated WHERE clause contains the is_hidden visibility guard AND the
// user ID is passed as an argument.
func TestBuildDocumentListWhere_HiddenVisibilityFilter(t *testing.T) {
	userID := 5
	filter := DocumentListFilter{HiddenVisibilityUserID: &userID}
	where, args := buildDocumentListWhere(filter, ArchiveScopeActiveOnly, 1)

	if !strings.Contains(where, "dcm.is_hidden = FALSE") {
		t.Errorf("WHERE must contain is_hidden guard; got: %s", where)
	}
	if !strings.Contains(where, "dcm.created_by") {
		t.Errorf("WHERE must contain created_by guard; got: %s", where)
	}

	found := false
	for _, arg := range args {
		if v, ok := arg.(int); ok && v == userID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("viewer user ID %d must be in query args; got %v", userID, args)
	}
}

// TestBuildDocumentListWhere_AdminBypassNoHiddenFilter verifies that nil HiddenVisibilityUserID
// (admin path) produces no is_hidden condition in the WHERE clause.
func TestBuildDocumentListWhere_AdminBypassNoHiddenFilter(t *testing.T) {
	filter := DocumentListFilter{} // nil HiddenVisibilityUserID = admin
	where, _ := buildDocumentListWhere(filter, ArchiveScopeActiveOnly, 1)

	if strings.Contains(where, "is_hidden") {
		t.Errorf("admin path must have no is_hidden guard in WHERE; got: %s", where)
	}
}

// TestBuildDocumentListWhere_HiddenFilterWithOtherConditions verifies the hidden filter
// composes correctly with other active filters (Status, BranchID).
func TestBuildDocumentListWhere_HiddenFilterWithOtherConditions(t *testing.T) {
	userID := 3
	filter := DocumentListFilter{
		Status:                "draft",
		HiddenVisibilityUserID: &userID,
	}
	where, args := buildDocumentListWhere(filter, ArchiveScopeActiveOnly, 1)

	if !strings.Contains(where, "dcm.status") {
		t.Errorf("WHERE must contain status condition; got: %s", where)
	}
	if !strings.Contains(where, "is_hidden") {
		t.Errorf("WHERE must contain is_hidden guard; got: %s", where)
	}
	// args: status value + viewer user ID
	if len(args) != 2 {
		t.Errorf("expected 2 args (status + viewer), got %d: %v", len(args), args)
	}
}
