package repositories

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildDealListWhere_SearchQueryAddsAllExpectedFields(t *testing.T) {
	where, args := buildDealListWhere(DealListFilter{Query: "7701"}, 1)
	if where == "" {
		t.Fatal("expected non-empty where clause")
	}
	patterns := []string{"display_name", "bin_iin", "primary_phone", "primary_email", "CAST(d.amount AS TEXT)", "d.currency"}
	for _, p := range patterns {
		if !contains(where, p) {
			t.Fatalf("expected where clause to contain %q, got: %s", p, where)
		}
	}
	if len(args) != 1 {
		t.Fatalf("expected one search arg, got %d", len(args))
	}
	if args[0] != "%7701%" {
		t.Fatalf("unexpected search arg: %#v", args[0])
	}
}

func TestBuildDealListWhere_StatusGroupAndStatusPriority(t *testing.T) {
	whereGroup, argsGroup := buildDealListWhere(DealListFilter{StatusGroup: "active"}, 3)
	if !contains(whereGroup, "= ANY($3)") {
		t.Fatalf("expected status group ANY clause, got: %s", whereGroup)
	}
	if len(argsGroup) != 1 {
		t.Fatalf("expected one status group arg, got %d", len(argsGroup))
	}

	whereStatus, _ := buildDealListWhere(DealListFilter{Status: "won", StatusGroup: "active"}, 3)
	if !contains(whereStatus, "COALESCE(d.status, 'new') = $3") {
		t.Fatalf("expected exact status clause, got: %s", whereStatus)
	}
	if contains(whereStatus, "= ANY(") {
		t.Fatalf("status group must not be applied when exact status is set, got: %s", whereStatus)
	}
}

func TestDealStatusesFromGroup(t *testing.T) {
	tests := []struct {
		group string
		want  []string
	}{
		{group: "active", want: []string{"new", "in_progress", "negotiation"}},
		{group: "completed", want: []string{"won"}},
		{group: "closed", want: []string{"lost", "cancelled"}},
		{group: "all", want: nil},
	}
	for _, tc := range tests {
		got := dealStatusesFromGroup(tc.group)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("group=%s got=%v want=%v", tc.group, got, tc.want)
		}
	}
}

func TestDealSortExpression(t *testing.T) {
	tests := []struct {
		name    string
		filter  DealListFilter
		wantBy  string
		wantOrd string
	}{
		{name: "default", filter: DealListFilter{}, wantBy: "d.created_at", wantOrd: "DESC"},
		{name: "amount asc", filter: DealListFilter{SortBy: "amount", Order: "asc"}, wantBy: "d.amount", wantOrd: "ASC"},
		{name: "status desc", filter: DealListFilter{SortBy: "status", Order: "desc"}, wantBy: "COALESCE(d.status, 'new')", wantOrd: "DESC"},
		{name: "client name", filter: DealListFilter{SortBy: "client_name", Order: "asc"}, wantBy: "LOWER(COALESCE(c.display_name, c.name, ''))", wantOrd: "ASC"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotBy, gotOrd := dealSortExpression(tc.filter)
			if gotBy != tc.wantBy || gotOrd != tc.wantOrd {
				t.Fatalf("got (%s,%s) want (%s,%s)", gotBy, gotOrd, tc.wantBy, tc.wantOrd)
			}
		})
	}
}

func contains(s, needle string) bool { return strings.Contains(s, needle) }
