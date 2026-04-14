package repositories

import (
	"reflect"
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
