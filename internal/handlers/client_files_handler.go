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
		badRequest(c, "Invalid client ID")
		return
	}
	category := c.PostForm("category")
	fileHeader, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "file is required")
		return
	}
	userID, roleID := getUserAndRole(c)

	rec, err := h.Service.UploadPrimary(c.Request.Context(), userID, roleID, clientID, category, fileHeader)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden), errors.Is(err, services.ErrReadOnly):
			forbidden(c, "Forbidden")
		case errors.Is(err, repositories.ErrClientNotFound):
			notFound(c, ClientNotFoundCode, "Client not found")
		case errors.Is(err, services.ErrUnsupportedClientFileCategory):
			badRequest(c, "unsupported category for this client type")
		case errors.Is(err, services.ErrUnsupportedClientFileExtension):
			badRequest(c, "unsupported file extension for selected category")
		case errors.Is(err, services.ErrFileRequired):
			badRequest(c, err.Error())
		default:
			internalError(c, "Failed to upload client file")
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
		badRequest(c, "Invalid client ID")
		return
	}
	category := c.DefaultQuery("category", "")
	if category == "" {
		badRequest(c, "category is required")
		return
	}
	userID, roleID := getUserAndRole(c)

	absPath, fileName, mimeType, err := h.Service.ResolvePrimaryPath(c.Request.Context(), userID, roleID, clientID, category)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrForbidden), errors.Is(err, services.ErrReadOnly):
			forbidden(c, "Forbidden")
		case errors.Is(err, repositories.ErrClientNotFound):
			notFound(c, ClientNotFoundCode, "Client not found")
		case errors.Is(err, repositories.ErrClientFileNotFound), errors.Is(err, os.ErrNotExist):
			notFound(c, NotFoundCode, "Client file not found")
		case errors.Is(err, services.ErrClientFilePathTraversal):
			badRequest(c, "Invalid file path")
		default:
			internalError(c, "Failed to resolve client file")
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
