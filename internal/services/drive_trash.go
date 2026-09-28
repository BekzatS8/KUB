package services

import (
	"context"
	"database/sql"
	"errors"
	"log"

	"turcompany/internal/repositories"
)

// Корзина хранилища: «Удалить» переносит в корзину, откуда элемент можно
// восстановить или удалить навсегда. Только администратор.

// DriveTrash — содержимое корзины.
type DriveTrash struct {
	Items      []repositories.DriveTrashItem `json:"items"`
	TotalBytes int64                         `json:"total_bytes"`
}

// Delete переносит файл или папку со всем содержимым в корзину. Возвращает,
// сколько элементов перенесено.
func (s *DriveService) Delete(ctx context.Context, actor DriveActor, id int64) (int, error) {
	if !actor.canManage() {
		return 0, ErrDriveForbidden
	}
	n, err := s.repo.TrashNode(ctx, id, actor.UserID)
	if err != nil {
		return 0, mapDriveRepoErr(err)
	}
	return int(n), nil
}

func (s *DriveService) ListTrash(ctx context.Context, actor DriveActor) (*DriveTrash, error) {
	if !actor.canManage() {
		return nil, ErrDriveForbidden
	}
	items, err := s.repo.ListTrash(ctx)
	if err != nil {
		return nil, err
	}
	trash := &DriveTrash{Items: items}
	for _, it := range items {
		trash.TotalBytes += it.SizeBytes
	}
	return trash, nil
}

// Restore возвращает записи корзины на прежнее место. Если исходную папку
// тоже удалили — в корень хранилища. При совпадении имени — «Договор (2).pdf».
func (s *DriveService) Restore(ctx context.Context, actor DriveActor, ids []int64) (int, error) {
	if !actor.canManage() {
		return 0, ErrDriveForbidden
	}
	ids, err := normalizeDriveIDs(ids)
	if err != nil {
		return 0, err
	}
	restored := 0
	for _, id := range ids {
		name, parentID, parentLive, err := s.repo.GetTrashRoot(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return restored, ErrDriveNotFound
		}
		if err != nil {
			return restored, err
		}
		if !parentLive {
			parentID = nil
		}
		for attempt := 1; attempt <= 50; attempt++ {
			err = s.repo.RestoreNode(ctx, id, parentID, numberedDriveName(name, attempt))
			if !errors.Is(err, repositories.ErrDriveNameTaken) {
				break
			}
		}
		if err != nil {
			return restored, mapDriveRepoErr(err)
		}
		restored++
	}
	return restored, nil
}

// Purge удаляет записи корзины навсегда: записи в базе и объекты в хранилище.
// Возвращает число удалённых файлов.
func (s *DriveService) Purge(ctx context.Context, actor DriveActor, ids []int64) (int, error) {
	if !actor.canManage() {
		return 0, ErrDriveForbidden
	}
	ids, err := normalizeDriveIDs(ids)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, id := range ids {
		// Навсегда удаляется только то, что уже в корзине.
		if _, _, _, err := s.repo.GetTrashRoot(ctx, id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return removed, ErrDriveNotFound
			}
			return removed, err
		}
		n, err := s.purgeNode(ctx, id)
		if err != nil {
			return removed, err
		}
		removed += n
	}
	return removed, nil
}

// EmptyTrash удаляет навсегда всё содержимое корзины.
func (s *DriveService) EmptyTrash(ctx context.Context, actor DriveActor) (int, error) {
	if !actor.canManage() {
		return 0, ErrDriveForbidden
	}
	ids, err := s.repo.ListTrashRootIDs(ctx)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, id := range ids {
		n, err := s.purgeNode(ctx, id)
		// Запись внутри удалённой ранее папки уйдёт вместе с ней каскадом.
		if errors.Is(err, ErrDriveNotFound) {
			continue
		}
		if err != nil {
			return removed, err
		}
		removed += n
	}
	return removed, nil
}

// purgeNode стирает узел со всем содержимым и его объекты в хранилище.
func (s *DriveService) purgeNode(ctx context.Context, id int64) (int, error) {
	objects, err := s.repo.DeleteNode(ctx, id)
	if err != nil {
		return 0, mapDriveRepoErr(err)
	}
	// Записи уже удалены — объекты чистим «лучшим усилием». Оставшийся в
	// бакете объект недоступен никому и только занимает место, это видно в логе.
	for _, o := range objects {
		if o.StorageKey != "" {
			if err := s.store.Delete(context.Background(), o.StorageKey); err != nil {
				log.Printf("[drive] delete object failed key=%s err=%v", o.StorageKey, err)
			}
		}
		_ = s.store.Delete(context.Background(), drivePreviewKey(o.NodeID))
	}
	return len(objects), nil
}
