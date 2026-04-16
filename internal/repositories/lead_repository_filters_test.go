package repositories

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestBuildLeadListWhere_SearchAcrossTitleDescriptionPhone(t *testing.T) {
	where, args := buildLeadListWhere(LeadListFilter{Query: "7701"}, 1)
	if where == "" {
		t.Fatal("expected where clause")
	}
	for _, p := range []string{"title", "description", "phone"} {
		if !strings.Contains(where, p) {
			t.Fatalf("expected %q in where: %s", p, where)
		}
	}
	if len(args) != 1 || args[0] != "%7701%" {
		t.Fatalf("unexpected args: %#v", args)
	}
	for _, expected := range []string{"l.title::text", "l.description::text", "l.phone::text"} {
		if !strings.Contains(where, expected) {
			t.Fatalf("expected explicit text cast %q in where: %s", expected, where)
		}
	}
}

func TestBuildLeadListWhere_QueryAndBranchIDUseDifferentPlaceholders(t *testing.T) {
	branchID := 12
	where, args := buildLeadListWhere(LeadListFilter{Query: "7701", BranchID: &branchID}, 1)
	if !strings.Contains(where, "LIKE $1") {
		t.Fatalf("expected query placeholder at $1, got where=%s", where)
	}
	if !strings.Contains(where, "l.branch_id = $2") {
		t.Fatalf("expected branch placeholder at $2, got where=%s", where)
	}
	if len(args) != 2 || args[0] != "%7701%" || args[1] != branchID {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildLeadListWhere_StatusGroupQueryAndBranchID(t *testing.T) {
	branchID := 17
	where, args := buildLeadListWhere(LeadListFilter{StatusGroup: "active", Query: "ana", BranchID: &branchID}, 1)
	for _, expected := range []string{"ANY($1)", "LIKE $2", "l.branch_id = $3"} {
		if !strings.Contains(where, expected) {
			t.Fatalf("expected %q in where, got %s", expected, where)
		}
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %#v", args)
	}
	if args[1] != "%ana%" || args[2] != branchID {
		t.Fatalf("unexpected args ordering: %#v", args)
	}
}

func TestBuildLeadListWhere_PlaceholderNumbersAreUniqueAndSequential(t *testing.T) {
	branchID := 17
	where, args := buildLeadListWhere(LeadListFilter{StatusGroup: "active", Query: "ana", BranchID: &branchID}, 1)
	re := regexp.MustCompile(`\$\d+`)
	matches := re.FindAllString(where, -1)
	seen := map[string]bool{}
	for _, m := range matches {
		if seen[m] {
			continue
		}
		seen[m] = true
	}
	if !seen["$1"] || !seen["$2"] || !seen["$3"] {
		t.Fatalf("expected placeholders $1,$2,$3 in where: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
}

func TestBuildLeadListWhere_OwnerScopeQueryAndBranchIDStartAt2(t *testing.T) {
	branchID := 99
	where, args := buildLeadListWhere(LeadListFilter{Query: "silk", BranchID: &branchID}, 2)
	if !strings.Contains(where, "LIKE $2") {
		t.Fatalf("expected query placeholder at $2, got where=%s", where)
	}
	if !strings.Contains(where, "l.branch_id = $3") {
		t.Fatalf("expected branch placeholder at $3, got where=%s", where)
	}
	if len(args) != 2 || args[0] != "%silk%" || args[1] != branchID {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildLeadListWhere_StatusPriorityOverStatusGroup(t *testing.T) {
	whereGroup, _ := buildLeadListWhere(LeadListFilter{StatusGroup: "active"}, 3)
	if !strings.Contains(whereGroup, "= ANY($3)") {
		t.Fatalf("expected ANY clause for group: %s", whereGroup)
	}

	whereExact, _ := buildLeadListWhere(LeadListFilter{Status: "converted", StatusGroup: "active"}, 3)
	if !strings.Contains(whereExact, "COALESCE(status, 'new') = $3") {
		t.Fatalf("expected exact status clause: %s", whereExact)
	}
	if strings.Contains(whereExact, "= ANY(") {
		t.Fatalf("status group must be ignored when status set: %s", whereExact)
	}
}

func TestLeadStatusesFromGroup(t *testing.T) {
	tests := []struct {
		group string
		want  []string
	}{
		{group: "active", want: []string{"new", "in_progress", "confirmed"}},
		{group: "closed", want: []string{"converted", "cancelled"}},
		{group: "all", want: nil},
	}
	for _, tc := range tests {
		if got := leadStatusesFromGroup(tc.group); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("group=%s got=%v want=%v", tc.group, got, tc.want)
		}
	}
}

func TestLeadSortExpressionWhitelist(t *testing.T) {
	tests := []struct {
		filter  LeadListFilter
		wantBy  string
		wantOrd string
	}{
		{filter: LeadListFilter{}, wantBy: "created_at", wantOrd: "DESC"},
		{filter: LeadListFilter{SortBy: "status", Order: "asc"}, wantBy: "COALESCE(status, 'new')", wantOrd: "ASC"},
		{filter: LeadListFilter{SortBy: "title", Order: "desc"}, wantBy: "LOWER(COALESCE(title, ''))", wantOrd: "DESC"},
	}
	for _, tc := range tests {
		gotBy, gotOrd := leadSortExpression(tc.filter)
		if gotBy != tc.wantBy || gotOrd != tc.wantOrd {
			t.Fatalf("got (%s,%s) want (%s,%s)", gotBy, gotOrd, tc.wantBy, tc.wantOrd)
		}
	}
}
