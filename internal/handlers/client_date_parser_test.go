package handlers

import "testing"

func TestParseFlexibleDateFormats(t *testing.T) {
	cases := []string{"2024-01-31", "31.01.2024", "2024-01-31T12:13:14Z"}
	for _, in := range cases {
		v, err := parseFlexibleDate("birth_date", in, false)
		if err != nil || v == nil {
			t.Fatalf("expected parse for %s, err=%v", in, err)
		}
	}
}

func TestParseFlexibleDateClearOnEmpty(t *testing.T) {
	v, err := parseFlexibleDate("birth_date", "", false)
	if err != nil || v != nil {
		t.Fatalf("expected nil,nil got %v,%v", v, err)
	}
}

func TestParseFlexibleDateInvalid(t *testing.T) {
	_, err := parseFlexibleDate("birth_date", "31/01/2024", false)
	if err == nil {
		t.Fatal("expected error")
	}
}
