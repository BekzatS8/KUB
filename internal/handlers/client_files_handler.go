package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"

	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type ClientFilesHandler struct {
	Service *services.ClientFilesService
}

func NewClientFilesHandler(service *services.ClientFilesService) *ClientFilesHandler {
	return &ClientFilesHandler{Service: service}
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

	absPath, fileName, mimeType, err := h.Service.ResolvePrimaryPath(c.Request.Context(), userID, roleID, clientID, category)
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
	if mimeType != "" {
		c.Header("Content-Type", mimeType)
	}
	if download {
		c.FileAttachment(absPath, fileName)
		return
	}
	c.Header("Content-Disposition", "inline")
	c.File(absPath)
}
