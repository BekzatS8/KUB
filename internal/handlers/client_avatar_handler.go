package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type ClientAvatarHandler struct {
	service    *services.ClientService
	clientRepo *repositories.ClientRepository
	filesRoot  string
}

func NewClientAvatarHandler(service *services.ClientService, clientRepo *repositories.ClientRepository, filesRoot string) *ClientAvatarHandler {
	return &ClientAvatarHandler{service: service, clientRepo: clientRepo, filesRoot: filesRoot}
}

// POST /clients/:id/avatar
func (h *ClientAvatarHandler) Upload(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	userID, roleID := getUserAndRole(c)

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)

	fileHeader, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите файл для загрузки")
		return
	}
	if fileHeader.Size > 10<<20 {
		badRequest(c, "Файл слишком большой (максимум 10 МБ)")
		return
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !allowedAvatarExt(ext) {
		badRequest(c, "Поддерживаются JPG, PNG, WebP и PDF")
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		badRequest(c, "Не удалось открыть файл")
		return
	}
	defer src.Close()

	if err := validateAvatarMime(src, ext); err != nil {
		badRequest(c, "Файл не является изображением")
		return
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		badRequest(c, "Ошибка чтения файла")
		return
	}

	// Get current client to check access and get old avatar path
	current, err := h.service.GetByID(clientID, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "У вас нет доступа к этому клиенту")
		} else {
			notFound(c, ClientNotFoundCode, "Клиент не найден")
		}
		return
	}
	if current == nil {
		notFound(c, ClientNotFoundCode, "Клиент не найден")
		return
	}
	var oldAvatarPath string
	if current.AvatarPath != nil {
		oldAvatarPath = *current.AvatarPath
	}

	// Generate storage key
	name, _ := randomHex(16)
	key := filepath.ToSlash(filepath.Join(
		"avatars", "clients", strconv.Itoa(clientID),
		name+ext,
	))

	if err := h.saveFile(src, key); err != nil {
		internalError(c, "Не удалось сохранить файл")
		return
	}

	avatarURL := fmt.Sprintf("/clients/%d/avatar/content", clientID)
	if err := h.clientRepo.UpdateAvatar(clientID, avatarURL, key); err != nil {
		h.removeStoredFile(key)
		internalError(c, "Не удалось обновить аватар")
		return
	}

	// Remove old file
	if oldAvatarPath != "" {
		h.removeStoredFile(oldAvatarPath)
	}

	// Re-fetch client for response
	updated, err := h.service.GetByID(clientID, userID, roleID)
	if err != nil {
		internalError(c, "Не удалось загрузить данные клиента")
		return
	}
	c.JSON(http.StatusOK, updated)
}

// PATCH /clients/:id/avatar/crop
func (h *ClientAvatarHandler) UpdateCrop(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	userID, roleID := getUserAndRole(c)

	var req avatarCropRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректные данные")
		return
	}

	// Validate access
	if _, err := h.service.GetByID(clientID, userID, roleID); err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "У вас нет доступа к этому клиенту")
		} else {
			notFound(c, ClientNotFoundCode, "Клиент не найден")
		}
		return
	}

	if req.CropScale != nil && *req.CropScale <= 0 {
		badRequest(c, "Масштаб должен быть положительным")
		return
	}
	if req.CropSize != nil && *req.CropSize <= 0 {
		badRequest(c, "Размер должен быть положительным")
		return
	}

	if err := h.clientRepo.UpdateAvatarCrop(clientID, req.CropX, req.CropY, req.CropScale, req.CropSize); err != nil {
		internalError(c, "Не удалось обновить кроп")
		return
	}

	updated, err := h.service.GetByID(clientID, userID, roleID)
	if err != nil {
		internalError(c, "Не удалось загрузить данные клиента")
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DELETE /clients/:id/avatar
func (h *ClientAvatarHandler) Delete(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	userID, roleID := getUserAndRole(c)

	// Get current client to check access and get old avatar path
	current, err := h.service.GetByID(clientID, userID, roleID)
	if err != nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "У вас нет доступа к этому клиенту")
		} else {
			notFound(c, ClientNotFoundCode, "Клиент не найден")
		}
		return
	}
	if current == nil {
		notFound(c, ClientNotFoundCode, "Клиент не найден")
		return
	}
	var oldAvatarPath string
	if current.AvatarPath != nil {
		oldAvatarPath = *current.AvatarPath
	}

	if err := h.clientRepo.DeleteAvatar(clientID); err != nil {
		internalError(c, "Не удалось удалить аватар")
		return
	}

	if oldAvatarPath != "" {
		h.removeStoredFile(oldAvatarPath)
	}

	updated, err := h.service.GetByID(clientID, userID, roleID)
	if err != nil {
		internalError(c, "Не удалось загрузить данные клиента")
		return
	}
	c.JSON(http.StatusOK, updated)
}

// GET /clients/:id/avatar/content
func (h *ClientAvatarHandler) Serve(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	userID, roleID := getUserAndRole(c)

	client, err := h.service.GetByID(clientID, userID, roleID)
	if err != nil || client == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "У вас нет доступа к этому клиенту")
		} else {
			notFound(c, ClientNotFoundCode, "Клиент не найден")
		}
		return
	}
	if client.AvatarPath == nil || *client.AvatarPath == "" {
		notFound(c, NotFoundCode, "Аватар не найден")
		return
	}

	absPath, err := h.resolveFilePath(*client.AvatarPath)
	if err != nil {
		notFound(c, NotFoundCode, "Аватар не найден")
		return
	}

	f, err := os.Open(absPath)
	if err != nil {
		notFound(c, NotFoundCode, "Аватар не найден")
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		notFound(c, NotFoundCode, "Аватар не найден")
		return
	}

	c.Header("Content-Type", avatarContentType(filepath.Ext(*client.AvatarPath)))
	c.Header("Content-Disposition", "inline")
	http.ServeContent(c.Writer, c.Request, filepath.Base(*client.AvatarPath), stat.ModTime(), f)
}

// --- File helpers ---

func (h *ClientAvatarHandler) saveFile(reader io.Reader, key string) error {
	absPath, err := h.resolveFilePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	dst, err := os.Create(absPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, reader); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (h *ClientAvatarHandler) resolveFilePath(key string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(key))
	if clean == "." || clean == "" || filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid file key")
	}
	rootAbs, err := filepath.Abs(h.filesRoot)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(rootAbs, clean)
	relToRoot, err := filepath.Rel(rootAbs, joined)
	if err != nil || strings.HasPrefix(relToRoot, "..") {
		return "", fmt.Errorf("path traversal detected")
	}
	return joined, nil
}

func (h *ClientAvatarHandler) removeStoredFile(key string) {
	if key == "" {
		return
	}
	absPath, err := h.resolveFilePath(key)
	if err != nil {
		return
	}
	_ = os.Remove(absPath)
}
