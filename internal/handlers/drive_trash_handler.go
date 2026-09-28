package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Корзина хранилища — только администратор (группа drive.manage).

type driveIDsRequest struct {
	IDs []int64 `json:"ids"`
}

// GET /api/v1/drive/trash
func (h *DriveHandler) Trash(c *gin.Context) {
	trash, err := h.svc.ListTrash(c.Request.Context(), driveActor(c))
	if err != nil {
		writeDriveOpsError(c, err, "Не удалось загрузить корзину")
		return
	}
	c.JSON(http.StatusOK, trash)
}

// POST /api/v1/drive/trash/restore {ids}
func (h *DriveHandler) Restore(c *gin.Context) {
	var req driveIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	n, err := h.svc.Restore(c.Request.Context(), driveActor(c), req.IDs)
	if err != nil {
		writeDriveOpsError(c, err, "Не удалось восстановить")
		return
	}
	c.JSON(http.StatusOK, gin.H{"restored": n})
}

// POST /api/v1/drive/trash/purge {ids} — удалить навсегда.
func (h *DriveHandler) Purge(c *gin.Context) {
	var req driveIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	// Объекты в хранилище удаляются после записей — не обрываем, если
	// вкладку закрыли.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Minute)
	defer cancel()
	n, err := h.svc.Purge(ctx, driveActor(c), req.IDs)
	if err != nil {
		writeDriveOpsError(c, err, "Не удалось удалить навсегда")
		return
	}
	c.JSON(http.StatusOK, gin.H{"files_removed": n})
}

// DELETE /api/v1/drive/trash — очистить корзину.
func (h *DriveHandler) EmptyTrash(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 10*time.Minute)
	defer cancel()
	n, err := h.svc.EmptyTrash(ctx, driveActor(c))
	if err != nil {
		writeDriveOpsError(c, err, "Не удалось очистить корзину")
		return
	}
	c.JSON(http.StatusOK, gin.H{"files_removed": n})
}
