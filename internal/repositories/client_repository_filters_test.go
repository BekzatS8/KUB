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
	// текст + цифры для сверки с телефоном без пробелов
	if len(args) != 2 || args[0] != "%7701%" || args[1] != "%7701%" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

// Каждое слово ищется отдельно: «Иван Петров» находит «Петров Иван».
func TestBuildClientListWhere_QueryWordsAnyOrder(t *testing.T) {
	where, args := buildClientListWhere(nil, "", ClientListFilter{Query: "  Иван   Петров "}, ArchiveScopeActiveOnly, 1)
	if len(args) != 2 || args[0] != "%иван%" || args[1] != "%петров%" {
		t.Fatalf("unexpected args: %#v", args)
	}
	if strings.Count(where, "ip.last_name") != 2 || strings.Contains(where, "regexp_replace") {
		t.Fatalf("expected two word conditions without phone digits: %s", where)
	}
}

func TestClientSearchPhoneDigits(t *testing.T) {
	cases := map[string]string{
		"+7(701)123-45-67": "77011234567",
		"87011234567":      "87011234567|77011234567",
		"8701123":          "8701123|7701123",
		"8045":             "8045",
		"701":              "701",
		"12":               "",
		"Иван":             "",
	}
	for in, want := range cases {
		if got := strings.Join(clientSearchPhoneDigits(in), "|"); got != want {
			t.Fatalf("%q: want %q, got %q", in, want, got)
		}
	}
}

// Телефон с пробелами — одно слово, а не «8», «701», «45»…
func TestClientSearchWords_PhoneStaysWhole(t *testing.T) {
	cases := map[string]string{
		"8 701 123 45 67":    "87011234567",
		"+7 (701) 123-45-67": "+7(701)123-45-67",
		"Петров 8 701":       "Петров|8|701",
		"  Иван   Петров  ":  "Иван|Петров",
	}
	for in, want := range cases {
		if got := strings.Join(clientSearchWords(in), "|"); got != want {
			t.Fatalf("%q: want %q, got %q", in, want, got)
		}
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

func TestBuildClientListWhere_BranchScope(t *testing.T) {
	branchID := 7
	where, args := buildClientListWhere(nil, "", ClientListFilter{BranchID: &branchID}, ArchiveScopeActiveOnly, 1)
	if !strings.Contains(where, "c.branch_id = $1") {
		t.Fatalf("expected branch condition in where, got: %s", where)
	}
	if len(args) != 1 || args[0] != branchID {
		t.Fatalf("unexpected args for branch scope: %#v", args)
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
