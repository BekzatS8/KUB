package repositories

import (
	"strings"
	"testing"
)

// Обратная связь заказчика 18.09.2026: удалённая тестовая сделка продолжала
// висеть в аналитике («Воронка продаж», «Выручка по периодам», «Топ клиентов»).
// Отчёты обязаны исключать удалённые записи и при этом учитывать архивные —
// выигранная сделка уезжает в архив автоматически.
func TestReportStatsExcludeDeletedKeepArchived(t *testing.T) {
	deals := dealArchiveWhere(ArchiveScopeAll, "")
	if deals != "deleted_at IS NULL" {
		t.Fatalf("report scope for deals must only exclude deleted rows, got %q", deals)
	}
	if strings.Contains(deals, "is_archived") {
		t.Fatalf("report scope must not drop archived deals, got %q", deals)
	}

	aliased := dealArchiveWhere(ArchiveScopeAll, "d")
	if aliased != "d.deleted_at IS NULL" {
		t.Fatalf("aliased report scope mismatch, got %q", aliased)
	}

	leads := leadArchiveWhere(ArchiveScopeAll)
	if leads != "deleted_at IS NULL" {
		t.Fatalf("report scope for leads must only exclude deleted rows, got %q", leads)
	}
}
