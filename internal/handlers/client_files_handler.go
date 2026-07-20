package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/repositories"
	"turcompany/internal/services"
	"turcompany/internal/storage"
)

type ClientFilesHandler struct {
	Service *services.ClientFilesService
	Store   storage.Storage
}

func NewClientFilesHandler(service *services.ClientFilesService, store storage.Storage) *ClientFilesHandler {
	return &ClientFilesHandler{Service: service, Store: store}
}

// POST /clients/:id/files
func (h *ClientFilesHandler) Upload(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	category := c.PostForm("category")
	fileHeader, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите файл для загрузки")
		return
	}
	userID, roleID := getUserAndRole(c)

	rec, err := h.Service.UploadPrimary(c.Request.Context(), userID, roleID, clientID, category, fileHeader)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden), errors.Is(err, services.ErrReadOnly):
			forbidden(c, "У вас нет права загружать файлы этого клиента")
		case errors.Is(err, repositories.ErrClientNotFound):
			notFound(c, ClientNotFoundCode, "Клиент не найден")
		case errors.Is(err, services.ErrUnsupportedClientFileCategory):
			badRequest(c, "Эта категория файла не подходит для выбранного типа клиента")
		case errors.Is(err, services.ErrUnsupportedClientFileExtension):
			badRequest(c, "Этот формат файла не поддерживается для выбранной категории")
		case errors.Is(err, services.ErrFileRequired):
			badRequest(c, "Выберите файл для загрузки")
		default:
			internalError(c, "Не удалось загрузить файл клиента")
		}
		return
	}

	c.JSON(http.StatusOK, rec)
}

// GET /clients/:id/files/primary
func (h *ClientFilesHandler) ServePrimaryInline(c *gin.Context) {
	h.servePrimary(c, false)
}

// GET /clients/:id/files/primary/download
func (h *ClientFilesHandler) ServePrimaryDownload(c *gin.Context) {
	h.servePrimary(c, true)
}

func (h *ClientFilesHandler) servePrimary(c *gin.Context, download bool) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	category := c.DefaultQuery("category", "")
	if category == "" {
		badRequest(c, "Укажите категорию файла")
		return
	}
	userID, roleID := getUserAndRole(c)

	key, fileName, mimeType, err := h.Service.ResolvePrimaryPath(c.Request.Context(), userID, roleID, clientID, category)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden), errors.Is(err, services.ErrReadOnly):
			forbidden(c, "У вас нет доступа к файлу этого клиента")
		case errors.Is(err, repositories.ErrClientNotFound):
			notFound(c, ClientNotFoundCode, "Клиент не найден")
		case errors.Is(err, repositories.ErrClientFileNotFound), errors.Is(err, os.ErrNotExist):
			notFound(c, NotFoundCode, "Файл клиента не найден")
		case errors.Is(err, services.ErrClientFilePathTraversal):
			badRequest(c, "Некорректный путь к файлу")
		default:
			internalError(c, "Не удалось открыть файл клиента")
		}
		return
	}

	reader, size, err := h.Store.Open(c.Request.Context(), key)
	if err != nil {
		notFound(c, NotFoundCode, "Файл клиента не найден")
		return
	}
	defer reader.Close()

	if mimeType != "" {
		c.Header("Content-Type", mimeType)
	}
	if download {
		c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	} else {
		c.Header("Content-Disposition", "inline")
	}
	c.Header("Content-Length", http.CanonicalHeaderKey(itoa(size)))
	http.ServeContent(c.Writer, c.Request, fileName, time.Time{}, reader)
}

// GET /clients/:id/attachments — список файлов-вложений клиента.
func (h *ClientFilesHandler) ListAttachments(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	userID, roleID := getUserAndRole(c)
	files, err := h.Service.ListFiles(c.Request.Context(), userID, roleID, clientID)
	if err != nil {
		h.mapClientFileError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": files, "count": len(files)})
}

// POST /clients/:id/attachments — загрузить файл-вложение (несколько на категорию).
func (h *ClientFilesHandler) UploadAttachment(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	category := c.PostForm("category")
	fileHeader, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите файл для загрузки")
		return
	}
	userID, roleID := getUserAndRole(c)
	rec, err := h.Service.UploadAttachment(c.Request.Context(), userID, roleID, clientID, category, fileHeader)
	if err != nil {
		h.mapClientFileError(c, err)
		return
	}
	c.JSON(http.StatusOK, rec)
}

// GET /clients/:id/attachments/:fileId — просмотр (inline, для вьюера/зума).
func (h *ClientFilesHandler) ServeAttachmentInline(c *gin.Context) {
	h.serveAttachment(c, false)
}

// GET /clients/:id/attachments/:fileId/download — скачивание.
func (h *ClientFilesHandler) ServeAttachmentDownload(c *gin.Context) {
	h.serveAttachment(c, true)
}

func (h *ClientFilesHandler) serveAttachment(c *gin.Context, download bool) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	fileID, err := strconv.ParseInt(c.Param("fileId"), 10, 64)
	if err != nil || fileID <= 0 {
		badRequest(c, "Некорректный ID файла")
		return
	}
	userID, roleID := getUserAndRole(c)
	key, fileName, mimeType, err := h.Service.ResolveFileByID(c.Request.Context(), userID, roleID, clientID, fileID)
	if err != nil {
		h.mapClientFileError(c, err)
		return
	}
	reader, _, err := h.Store.Open(c.Request.Context(), key)
	if err != nil {
		notFound(c, NotFoundCode, "Файл клиента не найден")
		return
	}
	defer reader.Close()
	if mimeType != "" {
		c.Header("Content-Type", mimeType)
	}
	if download {
		c.Header("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	} else {
		c.Header("Content-Disposition", "inline")
	}
	http.ServeContent(c.Writer, c.Request, fileName, time.Time{}, reader)
}

// DELETE /clients/:id/attachments/:fileId — удалить файл клиента.
func (h *ClientFilesHandler) DeleteAttachment(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	fileID, err := strconv.ParseInt(c.Param("fileId"), 10, 64)
	if err != nil || fileID <= 0 {
		badRequest(c, "Некорректный ID файла")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.DeleteFile(c.Request.Context(), userID, roleID, clientID, fileID); err != nil {
		h.mapClientFileError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// mapClientFileError — общий маппинг ошибок работы с файлами клиента.
func (h *ClientFilesHandler) mapClientFileError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrForbidden), errors.Is(err, services.ErrReadOnly):
		forbidden(c, "У вас нет прав на файлы этого клиента")
	case errors.Is(err, repositories.ErrClientNotFound):
		notFound(c, ClientNotFoundCode, "Клиент не найден")
	case errors.Is(err, repositories.ErrClientFileNotFound), errors.Is(err, os.ErrNotExist):
		notFound(c, NotFoundCode, "Файл клиента не найден")
	case errors.Is(err, services.ErrUnsupportedClientFileCategory):
		badRequest(c, "Недопустимая категория файла")
	case errors.Is(err, services.ErrUnsupportedClientFileExtension):
		badRequest(c, "Формат файла не поддерживается")
	case errors.Is(err, services.ErrFileRequired):
		badRequest(c, "Выберите файл для загрузки")
	default:
		internalError(c, "Не удалось выполнить операцию с файлом клиента")
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
