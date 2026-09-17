package repositories

import (
	"strings"
	"testing"
)

func TestBoardQueryID(t *testing.T) {
	cases := map[string]int{
		"#42244": 42244,
		"42244":  42244,
		" #7 ":   7,
		"":       0,
		"#":      0,
		"Асқар":  0,
		"-3":     0,
		"0":      0,
	}
	for in, want := range cases {
		if got := boardQueryID(in); got != want {
			t.Fatalf("boardQueryID(%q) = %d, want %d", in, got, want)
		}
	}
}

// Поиск на канбане должен находить карточку по номеру («#42244» и «42244»),
// не теряя обычный текстовый поиск.
func TestBoardSearchCondition_AddsIDMatchForNumericQuery(t *testing.T) {
	args := []any{}
	cond := boardSearchCondition("#42244", &args, []string{"LOWER(l.title)"}, "l.id")
	if !strings.Contains(cond, "l.id = $2") {
		t.Fatalf("expected id match in condition, got %s", cond)
	}
	if !strings.Contains(cond, "LOWER(l.title) LIKE $1") {
		t.Fatalf("expected text match in condition, got %s", cond)
	}
	if len(args) != 2 || args[0] != "%#42244%" || args[1] != 42244 {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBoardSearchCondition_TextQueryStaysTextOnly(t *testing.T) {
	args := []any{}
	cond := boardSearchCondition("Асқар", &args, []string{"LOWER(l.title)"}, "l.id")
	if strings.Contains(cond, "l.id") {
		t.Fatalf("text query must not match by id, got %s", cond)
	}
	if len(args) != 1 {
		t.Fatalf("unexpected args: %#v", args)
	}
}

func TestBoardSearchCondition_EmptyQueryProducesNoCondition(t *testing.T) {
	args := []any{}
	if cond := boardSearchCondition("   ", &args, []string{"LOWER(l.title)"}, "l.id"); cond != "" {
		t.Fatalf("expected empty condition, got %s", cond)
	}
	if len(args) != 0 {
		t.Fatalf("empty query must not add args, got %#v", args)
	}
}
