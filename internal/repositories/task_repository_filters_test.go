package repositories

import (
	"strings"
	"testing"

	"turcompany/internal/models"
)

func TestTaskStatusesFromGroup(t *testing.T) {
	if got := taskStatusesFromGroup("active"); len(got) != 2 || got[0] != "new" || got[1] != "in_progress" {
		t.Fatalf("unexpected active statuses: %v", got)
	}
	if got := taskStatusesFromGroup("closed"); len(got) != 2 || got[0] != "done" || got[1] != "cancelled" {
		t.Fatalf("unexpected closed statuses: %v", got)
	}
	if got := taskStatusesFromGroup("all"); got != nil {
		t.Fatalf("expected nil for all, got %v", got)
	}
}

func TestTaskSortExpressionWhitelist(t *testing.T) {
	tests := []struct {
		sortBy  string
		order   string
		wantBy  string
		wantOrd string
	}{
		{"", "", "created_at", "DESC"},
		{"due_date", "asc", "due_date", "ASC"},
		{"priority", "desc", "priority", "DESC"},
		{"status", "asc", "status", "ASC"},
		{"title", "desc", "LOWER(COALESCE(title,''))", "DESC"},
	}
	for _, tc := range tests {
		gotBy, gotOrd := taskSortExpression(tc.sortBy, tc.order)
		if gotBy != tc.wantBy || gotOrd != tc.wantOrd {
			t.Fatalf("got (%s,%s) want (%s,%s)", gotBy, gotOrd, tc.wantBy, tc.wantOrd)
		}
	}
}

func TestTaskFilterQueryAndStatusPriority(t *testing.T) {
	status := models.TaskStatus("done")
	filter := models.TaskFilter{Status: &status, StatusGroup: "active", Query: "archive"}
	conditions := []string{}
	args := []interface{}{}
	argID := 1

	if filter.Status != nil {
		conditions = append(conditions, "status = $")
		args = append(args, *filter.Status)
		argID++
	} else {
		_ = taskStatusesFromGroup(filter.StatusGroup)
	}
	if strings.TrimSpace(filter.Query) != "" {
		conditions = append(conditions, "q")
		args = append(args, "%"+strings.ToLower(strings.TrimSpace(filter.Query))+"%")
		argID++
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args (status + q), got %d", len(args))
	}
	if args[0] != models.StatusDone {
		t.Fatalf("expected exact status priority, got %v", args[0])
	}
}
