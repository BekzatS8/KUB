package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
)

// Перенос файлов, лежащих в корне бакета, под префикс компании.
//
// До введения префиксов KUB писал всё в корень: pdf/…, clients/…, drive/….
// Чтобы в тот же бакет можно было пустить другие компании, эти объекты нужно
// переложить в kub/pdf/…, kub/clients/… и т.д.
//
// Правила безопасности:
//   - переносятся только известные системные папки (LegacyTopLevelFolders и
//     явно добавленные): всё остальное в корне может принадлежать кому-то ещё
//     и остаётся на месте, попадая в отчёт;
//   - копирование серверное (без выкачивания через наш сервер) и повторяемое:
//     уже скопированный объект того же размера пропускается, поэтому запуск
//     можно повторять сколько угодно — например, после переключения
//     инсталляции на префикс, чтобы догнать файлы, созданные в промежутке;
//   - исходники удаляются только по явному флагу и только после проверки, что
//     копия на месте и совпадает по размеру.

// PrefixMigrationOptions — параметры переноса.
type PrefixMigrationOptions struct {
	// To — префикс компании, куда переносим (например «kub»).
	To string
	// ExtraFolders — корневые папки сверх LegacyTopLevelFolders, которые тоже
	// принадлежат этой инсталляции.
	ExtraFolders []string
	// Apply — выполнить копирование. false — только посчитать план.
	Apply bool
	// DeleteSource — удалить исходные объекты после проверенного копирования.
	DeleteSource bool
	// Progress вызывается на каждый обработанный объект (может быть nil).
	Progress func(action, src, dst string)
}

// PrefixMigrationReport — итог переноса.
type PrefixMigrationReport struct {
	Scanned       int
	Planned       int
	PlannedBytes  int64
	Copied        int
	AlreadyCopied int
	Deleted       int
	Failed        int
	// AlreadyPrefixed — объекты, уже лежащие под целевым префиксом.
	AlreadyPrefixed int
	// Untouched — объекты вне известных папок: корневой сегмент → количество.
	// Пустая строка — файлы прямо в корне бакета.
	Untouched map[string]int
	Errors    []string
}

// prefixMoveDecision — что делать с одним ключом.
type prefixMoveDecision struct {
	Move    bool
	Dest    string
	Top     string // корневой сегмент ключа
	Already bool   // уже под целевым префиксом
}

// planPrefixMove решает судьбу одного ключа. Чистая функция — вся логика
// «что трогать, а что нет» проверяется без бакета.
func planPrefixMove(key, to string, folders map[string]bool) prefixMoveDecision {
	norm := strings.TrimLeft(key, "/")
	top, _, hasSlash := strings.Cut(norm, "/")
	if !hasSlash {
		// Файл прямо в корне бакета: системе такие не принадлежат.
		return prefixMoveDecision{Top: ""}
	}
	if top == to {
		return prefixMoveDecision{Top: top, Already: true}
	}
	if folders[top] {
		return prefixMoveDecision{Move: true, Dest: prefixedKey(to, norm), Top: top}
	}
	return prefixMoveDecision{Top: top}
}

// MigrateToPrefix переносит объекты из корня бакета под префикс компании.
// Работает с бакетом напрямую, собственный префикс хранилища не учитывается.
func (s *S3Storage) MigrateToPrefix(ctx context.Context, opts PrefixMigrationOptions) (*PrefixMigrationReport, error) {
	to := NormalizeCompanyPrefix(opts.To)
	if to == "" {
		return nil, fmt.Errorf("не указан префикс компании, куда переносить файлы")
	}
	if err := ValidateCompanyPrefix(to); err != nil {
		return nil, err
	}
	if opts.DeleteSource && !opts.Apply {
		return nil, fmt.Errorf("удаление исходников возможно только вместе с копированием (Apply)")
	}

	folders := make(map[string]bool, len(LegacyTopLevelFolders)+len(opts.ExtraFolders))
	for f := range LegacyTopLevelFolders {
		folders[f] = true
	}
	for _, f := range opts.ExtraFolders {
		if f = strings.Trim(strings.TrimSpace(f), "/"); f != "" {
			folders[f] = true
		}
	}

	report := &PrefixMigrationReport{Untouched: map[string]int{}}
	progress := opts.Progress
	if progress == nil {
		progress = func(string, string, string) {}
	}
	fail := func(format string, args ...any) {
		report.Failed++
		report.Errors = append(report.Errors, fmt.Sprintf(format, args...))
	}

	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Recursive: true}) {
		if obj.Err != nil {
			return report, fmt.Errorf("список объектов бакета: %w", obj.Err)
		}
		report.Scanned++
		d := planPrefixMove(obj.Key, to, folders)
		switch {
		case d.Already:
			report.AlreadyPrefixed++
			continue
		case !d.Move:
			report.Untouched[d.Top]++
			continue
		}
		report.Planned++
		report.PlannedBytes += obj.Size
		if !opts.Apply {
			progress("plan", obj.Key, d.Dest)
			continue
		}

		// Повторный запуск: копия того же размера уже есть — не копируем.
		if st, err := s.client.StatObject(ctx, s.bucket, d.Dest, minio.StatObjectOptions{}); err == nil && st.Size == obj.Size {
			report.AlreadyCopied++
			progress("exists", obj.Key, d.Dest)
		} else {
			if err := s.copyObject(ctx, obj.Key, d.Dest, obj.Size); err != nil {
				fail("копирование %s → %s: %v", obj.Key, d.Dest, err)
				continue
			}
			st, err := s.client.StatObject(ctx, s.bucket, d.Dest, minio.StatObjectOptions{})
			if err != nil || st.Size != obj.Size {
				fail("проверка копии %s: размер %d, ожидался %d (%v)", d.Dest, st.Size, obj.Size, err)
				continue
			}
			report.Copied++
			progress("copied", obj.Key, d.Dest)
		}

		if opts.DeleteSource {
			if err := s.client.RemoveObject(ctx, s.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
				fail("удаление исходника %s: %v", obj.Key, err)
				continue
			}
			report.Deleted++
			progress("deleted", obj.Key, d.Dest)
		}
	}
	return report, nil
}

// copyObject — серверное копирование внутри бакета. Одиночный CopyObject в
// S3 ограничен 5 ГБ; объекты крупнее копируются составным запросом.
func (s *S3Storage) copyObject(ctx context.Context, src, dst string, size int64) error {
	source := minio.CopySrcOptions{Bucket: s.bucket, Object: src}
	dest := minio.CopyDestOptions{Bucket: s.bucket, Object: dst}
	const singleCopyLimit = 5 << 30
	if size > singleCopyLimit {
		_, err := s.client.ComposeObject(ctx, dest, source)
		return err
	}
	_, err := s.client.CopyObject(ctx, dest, source)
	return err
}
