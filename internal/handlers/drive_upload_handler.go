package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

// Загрузка файла частями (большие файлы): начало → куски → завершение.
// Каждый запрос меньше лимитов nginx и прокси Next.js, поэтому размер файла
// ограничен только хранилищем (у администратора хранилища — ничем).

type driveStartUploadRequest struct {
	ParentID    *int64 `json:"parent_id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}

func writeDriveUploadError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, services.ErrDriveUploadNotFound):
		notFound(c, "DRIVE_UPLOAD_NOT_FOUND", "Загрузка не найдена или устарела — начните заново")
	case errors.Is(err, services.ErrDriveUploadOffset):
		conflict(c, "DRIVE_UPLOAD_OFFSET", "Часть файла пришла не по порядку")
	case errors.Is(err, services.ErrDriveUploadIncomplete):
		badRequestWithCode(c, "DRIVE_UPLOAD_INCOMPLETE", "Файл загружен не полностью")
	default:
		writeDriveError(c, err, fallback)
	}
}

// POST /api/v1/drive/uploads {parent_id, name, size, content_type}
func (h *DriveHandler) StartUpload(c *gin.Context) {
	var req driveStartUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		badRequest(c, "Некорректный запрос")
		return
	}
	session, err := h.svc.StartUpload(c.Request.Context(), driveActor(c), req.ParentID, req.Name, req.Size, req.ContentType)
	if err != nil {
		writeDriveUploadError(c, err, "Не удалось начать загрузку")
		return
	}
	c.JSON(http.StatusCreated, session)
}

// PUT /api/v1/drive/uploads/:id?offset=N — тело запроса = кусок файла.
func (h *DriveHandler) UploadChunk(c *gin.Context) {
	offset, err := strconv.ParseInt(c.Query("offset"), 10, 64)
	if err != nil || offset < 0 {
		badRequest(c, "Некорректный offset")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, services.DriveMaxChunkBytes+1)
	received, err := h.svc.WriteChunk(driveActor(c), c.Param("id"), offset, c.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			err = services.ErrDriveTooLarge
		}
		if errors.Is(err, services.ErrDriveUploadOffset) {
			c.JSON(http.StatusConflict, gin.H{"error_code": "DRIVE_UPLOAD_OFFSET", "message": "Часть файла пришла не по порядку", "received": received})
			return
		}
		writeDriveUploadError(c, err, "Не удалось сохранить часть файла")
		return
	}
	c.JSON(http.StatusOK, gin.H{"received": received})
}

// POST /api/v1/drive/uploads/:id/complete
func (h *DriveHandler) CompleteUpload(c *gin.Context) {
	node, err := h.svc.CompleteUpload(c.Request.Context(), driveActor(c), c.Param("id"))
	if err != nil {
		writeDriveUploadError(c, err, "Не удалось сохранить файл")
		return
	}
	c.JSON(http.StatusCreated, node)
}

// DELETE /api/v1/drive/uploads/:id — отмена.
func (h *DriveHandler) CancelUpload(c *gin.Context) {
	if err := h.svc.CancelUpload(driveActor(c), c.Param("id")); err != nil {
		writeDriveUploadError(c, err, "Не удалось отменить загрузку")
		return
	}
	c.Status(http.StatusNoContent)
}
