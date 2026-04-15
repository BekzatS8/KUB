package repositories

import (
	"strings"
	"testing"
)

func TestBuildDocumentListWhere_SearchFields(t *testing.T) {
	where, args := buildDocumentListWhere(DocumentListFilter{Query: "25"}, ArchiveScopeActiveOnly, 1)
	for _, s := range []string{"dcm.doc_type", "file_path_docx", "CAST(dcm.deal_id AS TEXT)", "c.display_name"} {
		if !strings.Contains(where, s) {
			t.Fatalf("expected %q in where: %s", s, where)
		}
	}
	if len(args) != 1 || args[0] != "%25%" {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBuildDocumentListWhere_ExactFilters(t *testing.T) {
	dealID := int64(25)
	clientID := int64(16)
	where, args := buildDocumentListWhere(DocumentListFilter{Status: "signed", DocType: "contract", DealID: &dealID, ClientID: &clientID, ClientType: "individual"}, ArchiveScopeAll, 3)
	for _, s := range []string{"dcm.status = $3", "dcm.doc_type = $4", "dcm.deal_id = $5", "d.client_id = $6", "c.client_type = $7"} {
		if !strings.Contains(where, s) {
			t.Fatalf("expected %q in where: %s", s, where)
		}
	}
	if len(args) != 5 {
		t.Fatalf("expected 5 args, got %d", len(args))
	}
}

func TestDocumentSortExpressionWhitelist(t *testing.T) {
	tests := []struct {
		f       DocumentListFilter
		wantBy  string
		wantOrd string
	}{
		{f: DocumentListFilter{}, wantBy: "dcm.id", wantOrd: "DESC"},
		{f: DocumentListFilter{SortBy: "created_at", Order: "asc"}, wantBy: "dcm.created_at", wantOrd: "ASC"},
		{f: DocumentListFilter{SortBy: "status", Order: "desc"}, wantBy: "dcm.status", wantOrd: "DESC"},
		{f: DocumentListFilter{SortBy: "doc_type", Order: "asc"}, wantBy: "dcm.doc_type", wantOrd: "ASC"},
	}
	for _, tc := range tests {
		by, ord := documentSortExpression(tc.f)
		if by != tc.wantBy || ord != tc.wantOrd {
			t.Fatalf("got (%s,%s) want (%s,%s)", by, ord, tc.wantBy, tc.wantOrd)
		}
	}
}

func TestDocumentBaseSelect_IncludesCompanyID(t *testing.T) {
	if !strings.Contains(documentBaseSelect, "dcm.company_id") {
		t.Fatalf("documentBaseSelect must include dcm.company_id, got: %s", documentBaseSelect)
	}
}
