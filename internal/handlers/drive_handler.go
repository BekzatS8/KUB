package handlers

import (
	"context"
	"errors"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/services"
)

// DriveHandler — раздел «Хранилище»: папки, файлы, доступы и предпросмотр.
type DriveHandler struct {
	svc *services.DriveService
}

func NewDriveHandler(svc *services.DriveService) *DriveHandler {
	return &DriveHandler{svc: svc}
}

func driveActor(c *gin.Context) services.DriveActor {
	userID, roleID := getUserAndRole(c)
	return services.DriveActor{UserID: userID, RoleID: roleID}
}

// parseOptionalID читает необязательный id: пусто или «root» — корень.
func parseOptionalID(raw string) (*int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "root" || raw == "null" {
		return nil, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, false
	}
	return &id, true
}

func parsePathID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(c.Param(name)), 10, 64)
	if err != nil || id <= 0 {
		badRequest(c, "Некорректный идентификатор")
		return 0, false
	}
	return id, true
}

func writeDriveError(c *gin.Context, err error, fallback string) {
	switch {
	case errors.Is(err, services.ErrDriveForbidden):
		forbidden(c, "Недостаточно прав для работы с хранилищем")
	case errors.Is(err, services.ErrDriveNotFound):
		notFound(c, NotFoundCode, "Файл или папка не найдены")
	case errors.Is(err, services.ErrDriveNameTaken):
		conflict(c, "DRIVE_NAME_TAKEN", "В этой папке уже есть элемент с таким именем")
	case errors.Is(err, services.ErrDriveBadName):
		badRequestWithCode(c, "DRIVE_BAD_NAME", "Недопустимое имя: оно не должно быть пустым, содержать / или \\ и быть длиннее 255 символов")
	case errors.Is(err, services.ErrDriveNotFolder):
		badRequestWithCode(c, "DRIVE_NOT_FOLDER", "Выбранный элемент не является папкой")
	case errors.Is(err, services.ErrDriveNotFile):
		badRequestWithCode(c, "DRIVE_NOT_FILE", "Выбранный элемент не является файлом")
	case errors.Is(err, services.ErrDriveTooLarge):
		writeError(c, http.StatusRequestEntityTooLarge, "DRIVE_TOO_LARGE", "Файл превышает допустимый размер")
	case errors.Is(err, services.ErrDrivePreviewUnavailable):
		writeError(c, http.StatusUnprocessableEntity, "DRIVE_PREVIEW_UNAVAILABLE", "Предпросмотр этого файла недоступен — скачайте его")
	case errors.Is(err, services.ErrDriveBadExpiry):
		badRequestWithCode(c, "DRIVE_BAD_EXPIRY", "Срок доступа должен быть в будущем")
	case errors.Is(err, services.ErrDriveNoUsers):
		badRequestWithCode(c, "DRIVE_NO_USERS", "Выберите хотя бы одного пользователя")
	default:
		log.Printf("[drive] %s: %v", fallback, err)
		internalError(c, fallback)
	}
}

// GET /api/v1/drive/nodes?parent_id=
func (h *DriveHandler) List(c *gin.Context) {
	parentID, ok := parseOptionalID(c.Query("parent_id"))
	if !ok {
		badRequest(c, "Некорректный parent_id")
		return
	}
	listing, err := h.svc.List(c.Request.Context(), driveActor(c), parentID)
	if err != nil {
		writeDriveError(c, err, "Не удалось загрузить содержимое папки")
		return
	}
	c.JSON(http.StatusOK, listing)
}

type driveCreateFolderRequest struct {
	ParentID *int64 `json:"parent_id"`
	Name     string `json:"name"`
}

// POST /api/v1/drive/folders
func (h *DriveHandler) CreateFolder(c *gin.Context) {
	var req driveCreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	node, err := h.svc.CreateFolder(c.Request.Context(), driveActor(c), req.ParentID, req.Name)
	if err != nil {
		writeDriveError(c, err, "Не удалось создать папку")
		return
	}
	c.JSON(http.StatusCreated, node)
}

// POST /api/v1/drive/files (multipart: file, parent_id)
func (h *DriveHandler) Upload(c *gin.Context) {
	// Лимит на тело ставим до разбора multipart: иначе огромный файл сначала
	// целиком ляжет на диск сервера и только потом будет отклонён.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.svc.MaxUploadBytes()+(8<<20))

	fileHeader, err := c.FormFile("file")
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeDriveError(c, services.ErrDriveTooLarge, "")
			return
		}
		badRequest(c, "Файл не передан")
		return
	}
	parentID, ok := parseOptionalID(c.PostForm("parent_id"))
	if !ok {
		badRequest(c, "Некорректный parent_id")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		internalError(c, "Не удалось прочитать файл")
		return
	}
	defer file.Close()

	node, err := h.svc.Upload(c.Request.Context(), driveActor(c), parentID,
		fileHeader.Filename, fileHeader.Size, fileHeader.Header.Get("Content-Type"), file)
	if err != nil {
		writeDriveError(c, err, "Не удалось загрузить файл")
		return
	}
	c.JSON(http.StatusCreated, node)
}

type driveRenameRequest struct {
	Name string `json:"name"`
}

// PATCH /api/v1/drive/nodes/:id
func (h *DriveHandler) Rename(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var req driveRenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	node, err := h.svc.Rename(c.Request.Context(), driveActor(c), id, req.Name)
	if err != nil {
		writeDriveError(c, err, "Не удалось переименовать")
		return
	}
	c.JSON(http.StatusOK, node)
}

// DELETE /api/v1/drive/nodes/:id
func (h *DriveHandler) Delete(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	// Удаление объектов из S3 не должно обрываться, если пользователь закрыл
	// вкладку: записи в базе к этому моменту уже удалены.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 2*time.Minute)
	defer cancel()
	removed, err := h.svc.Delete(ctx, driveActor(c), id)
	if err != nil {
		writeDriveError(c, err, "Не удалось удалить")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok", "files_removed": removed})
}

// GET /api/v1/drive/nodes/:id/link?variant=original|pdf&disposition=inline|attachment
//
// GET, а не POST: ссылка ничего не меняет, а у ролей «только чтение» (ОКК)
// ReadOnlyGuard блокирует любые POST — они не смогли бы даже открыть файл.
func (h *DriveHandler) Link(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	inline := c.DefaultQuery("disposition", "inline") != "attachment"
	link, err := h.svc.Link(c.Request.Context(), driveActor(c), id, c.Query("variant"), inline)
	if err != nil {
		writeDriveError(c, err, "Не удалось получить ссылку на файл")
		return
	}
	c.JSON(http.StatusOK, link)
}

// GET /api/v1/drive/nodes/:id/shares
func (h *DriveHandler) ListShares(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	shares, err := h.svc.ListShares(c.Request.Context(), driveActor(c), id)
	if err != nil {
		writeDriveError(c, err, "Не удалось загрузить доступы")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": shares})
}

type driveShareRequest struct {
	UserIDs []int `json:"user_ids"`
	// ExpiresAt — RFC 3339; null или отсутствует — бессрочно.
	ExpiresAt *time.Time `json:"expires_at"`
}

// POST /api/v1/drive/nodes/:id/shares
func (h *DriveHandler) Share(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var req driveShareRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректный запрос: expires_at должен быть в формате RFC 3339")
		return
	}
	shares, err := h.svc.Share(c.Request.Context(), driveActor(c), id, req.UserIDs, req.ExpiresAt)
	if err != nil {
		writeDriveError(c, err, "Не удалось выдать доступ")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": shares})
}

// DELETE /api/v1/drive/shares/:id
func (h *DriveHandler) Unshare(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	if err := h.svc.Unshare(c.Request.Context(), driveActor(c), id); err != nil {
		writeDriveError(c, err, "Не удалось отозвать доступ")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GET /api/v1/drive/users — кому можно выдать доступ.
func (h *DriveHandler) Users(c *gin.Context) {
	users, err := h.svc.Users(c.Request.Context(), driveActor(c))
	if err != nil {
		writeDriveError(c, err, "Не удалось загрузить пользователей")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": users})
}

// GET|HEAD /api/v1/drive/raw/:token — содержимое по временной ссылке.
//
// Маршрут публичный (без JWT): авторизацией служит подпись ссылки, а доступ
// пользователя перепроверяется на каждый запрос. Отдача через ServeContent
// даёт Range-запросы — видео перематывается, не скачиваясь целиком.
func (h *DriveHandler) Raw(c *gin.Context) {
	content, err := h.svc.Open(c.Request.Context(), c.Param("token"))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrDriveLinkExpired):
			c.String(http.StatusGone, "Ссылка устарела — откройте файл заново")
		case errors.Is(err, services.ErrDriveLinkInvalid):
			c.String(http.StatusNotFound, "Ссылка недействительна")
		case errors.Is(err, services.ErrDriveForbidden), errors.Is(err, services.ErrDriveNotFound):
			c.String(http.StatusNotFound, "Файл не найден или доступ к нему закрыт")
		case errors.Is(err, services.ErrDrivePreviewUnavailable):
			c.String(http.StatusUnprocessableEntity, "Предпросмотр этого файла недоступен — скачайте его")
		default:
			log.Printf("[drive] raw: %v", err)
			c.String(http.StatusInternalServerError, "Не удалось открыть файл")
		}
		return
	}
	defer content.Reader.Close()

	disposition := "attachment"
	if content.Inline {
		disposition = "inline"
	}
	// FormatMediaType кодирует кириллицу по RFC 5987 (filename*=utf-8''…);
	// без этого русские имена превращались бы в кракозябры при скачивании.
	if header := mime.FormatMediaType(disposition, map[string]string{"filename": content.Name}); header != "" {
		c.Header("Content-Disposition", header)
	} else {
		c.Header("Content-Disposition", disposition)
	}
	c.Header("Content-Type", content.ContentType)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "private, max-age=300")
	// Всё, кроме PDF, показывается в «песочнице»: даже если под видом картинки
	// лежит документ со скриптами, он не исполнится на домене API. PDF
	// исключение — встроенный просмотрщик браузера в песочнице не работает.
	if !strings.HasPrefix(content.ContentType, "application/pdf") {
		c.Header("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'")
	}

	http.ServeContent(c.Writer, c.Request, "", content.ModTime, content.Reader)
}
