package repositories

import (
	"strings"
	"testing"
)

func TestBuildClientListWhere_QueryFields(t *testing.T) {
	where, args := buildClientListWhere(nil, "", ClientListFilter{Query: "7701"}, ArchiveScopeActiveOnly, 1)
	for _, part := range []string{"c.name", "c.display_name", "ip.last_name", "lp.company_name", "c.bin_iin", "ip.iin", "primary_phone", "primary_email"} {
		if !strings.Contains(where, part) {
			t.Fatalf("expected %q in where: %s", part, where)
		}
	}
	if len(args) != 1 || args[0] != "%7701%" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildClientListWhere_HasDealsAndStatusGroup(t *testing.T) {
	v := true
	where, args := buildClientListWhere(nil, "", ClientListFilter{HasDeals: &v, DealStatusGroup: "active"}, ArchiveScopeActiveOnly, 1)
	if !strings.Contains(where, "EXISTS (SELECT 1 FROM deals d") {
		t.Fatalf("expected exists clause: %s", where)
	}
	if !strings.Contains(where, "d.is_archived = FALSE") {
		t.Fatalf("expected non-archived deals filter: %s", where)
	}
	if len(args) != 1 {
		t.Fatalf("expected one status-group arg, got %d", len(args))
	}

	f := false
	whereNoDeals, _ := buildClientListWhere(nil, "", ClientListFilter{HasDeals: &f}, ArchiveScopeActiveOnly, 1)
	if !strings.Contains(whereNoDeals, "NOT EXISTS") {
		t.Fatalf("expected NOT EXISTS for has_deals=false: %s", whereNoDeals)
	}
}

func TestClientSortExpressionWhitelist(t *testing.T) {
	tests := []struct {
		filter  ClientListFilter
		wantBy  string
		wantOrd string
	}{
		{filter: ClientListFilter{}, wantBy: "c.created_at", wantOrd: "DESC"},
		{filter: ClientListFilter{SortBy: "client_type", Order: "asc"}, wantBy: "COALESCE(c.client_type, 'individual')", wantOrd: "ASC"},
		{filter: ClientListFilter{SortBy: "display_name", Order: "desc"}, wantBy: "LOWER(COALESCE(NULLIF(c.display_name, ''), NULLIF(c.name, ''), ''))", wantOrd: "DESC"},
	}
	for _, tc := range tests {
		by, ord := clientSortExpression(tc.filter)
		if by != tc.wantBy || ord != tc.wantOrd {
			t.Fatalf("got (%s,%s) want (%s,%s)", by, ord, tc.wantBy, tc.wantOrd)
		}
	}
}
