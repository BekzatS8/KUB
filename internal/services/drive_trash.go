package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// DriveActivityNotifier — Лента (FeedEventService): туда уходит событие, когда
// сотрудник удаляет файлы в хранилище.
type DriveActivityNotifier interface {
	Create(ctx context.Context, requesterID int, eventType string, payload json.RawMessage, resourceID *int) (*models.FeedEvent, error)
}

func (s *DriveService) SetNotifier(n DriveActivityNotifier) { s.notifier = n }

// driveDeletePayload — событие Ленты «удаление в хранилище».
type driveDeletePayload struct {
	Items  []driveDeletedItem `json:"items"`
	Folder string             `json:"folder"`
	Count  int                `json:"count"`
}

type driveDeletedItem struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// Корзина хранилища: «Удалить» переносит в корзину, откуда элемент можно
// восстановить или удалить навсегда. Только администратор.

// DriveTrash — содержимое корзины.
type DriveTrash struct {
	Items      []repositories.DriveTrashItem `json:"items"`
	TotalBytes int64                         `json:"total_bytes"`
}

// Delete переносит файл или папку со всем содержимым в корзину.
func (s *DriveService) Delete(ctx context.Context, actor DriveActor, id int64) (int, error) {
	return s.DeleteMany(ctx, actor, []int64{id})
}

// DeleteMany переносит элементы в корзину. Сотрудник удаляет только внутри
// папки с доступом edit; его удаление попадает в Ленту — администратор
// оставляет его в корзине или восстанавливает. Возвращает, сколько элементов
// перенесено в корзину.
func (s *DriveService) DeleteMany(ctx context.Context, actor DriveActor, ids []int64) (int, error) {
	ids, err := normalizeDriveIDs(ids)
	if err != nil {
		return 0, err
	}
	nodes := make([]*models.DriveNode, 0, len(ids))
	for _, id := range ids {
		node, err := s.getNode(ctx, id)
		if err != nil {
			return 0, err
		}
		if err := s.canModify(ctx, actor, node); err != nil {
			return 0, err
		}
		nodes = append(nodes, node)
	}
	folder := s.folderPath(ctx, actor, nodes[0].ParentID)

	payload := driveDeletePayload{Folder: folder}
	for _, node := range nodes {
		if _, err := s.repo.TrashNode(ctx, node.ID, actor.UserID); err != nil {
			// Уже в корзине (удалили вместе с папкой выше) — не ошибка.
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return len(payload.Items), mapDriveRepoErr(err)
		}
		payload.Items = append(payload.Items, driveDeletedItem{ID: node.ID, Name: node.Name, Kind: node.Kind})
	}
	payload.Count = len(payload.Items)
	if payload.Count > 0 && !actor.canManage() && s.notifier != nil {
		raw, _ := json.Marshal(payload)
		rid := int(payload.Items[0].ID)
		if _, err := s.notifier.Create(ctx, actor.UserID, models.FeedEventTypeDriveDelete, raw, &rid); err != nil {
			// Удаление уже в корзине — событие Ленты вторично, не откатываем.
			log.Printf("[drive] feed event failed user=%d items=%d err=%v", actor.UserID, payload.Count, err)
		}
	}
	return payload.Count, nil
}

// folderPath — путь папки словами для Ленты: «Хранилище / КУБ Алматы / …».
func (s *DriveService) folderPath(ctx context.Context, actor DriveActor, parentID *int64) string {
	parts := []string{"Хранилище"}
	if parentID != nil {
		if chain, err := s.repo.Ancestors(ctx, *parentID, actor.UserID); err == nil {
			for _, a := range chain {
				parts = append(parts, a.Name)
			}
		}
	}
	return strings.Join(parts, " / ")
}

// RestoreFromFeed — администратор отклонил в Ленте удаление сотрудника:
// возвращаем удалённое из корзины. То, что уже восстановили или удалили
// навсегда вручную, пропускаем.
func (s *DriveService) RestoreFromFeed(ctx context.Context, reviewerID int, payload json.RawMessage) error {
	var p driveDeletePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return err
	}
	admin := DriveActor{UserID: reviewerID, RoleID: authz.RoleSystemAdmin}
	restored := 0
	for _, it := range p.Items {
		n, err := s.Restore(ctx, admin, []int64{it.ID})
		if errors.Is(err, ErrDriveNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		restored += n
	}
	if restored == 0 && len(p.Items) > 0 {
		return errors.New("удалённое уже восстановлено или удалено навсегда")
	}
	return nil
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
