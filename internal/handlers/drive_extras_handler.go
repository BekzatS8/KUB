package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Хранилище, ТЗ «доработки CRM КУБ», п.8: пароли, загрузка папками, ZIP,
// архивировать/разархивировать.

type driveArchiveRequest struct {
	IDs  []int64 `json:"ids"`
	Name string  `json:"name"`
}

// POST /api/v1/drive/nodes/:id/unlock {password}
func (h *DriveHandler) Unlock(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	_ = c.ShouldBindJSON(&body)
	token, expires, err := h.svc.Unlock(c.Request.Context(), driveActor(c), id, body.Password)
	if err != nil {
		writeDriveError(c, err, "Не удалось проверить пароль")
		return
	}
	c.JSON(http.StatusOK, gin.H{"node_id": id, "token": token, "expires_at": expires})
}

// PUT /api/v1/drive/nodes/:id/password {current_password, new_password} —
// поставить или сменить; пустой new_password — снять.
func (h *DriveHandler) SetPassword(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	var body struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	if err := h.svc.SetPassword(c.Request.Context(), driveActor(c), id, body.Current, body.New); err != nil {
		writeDriveError(c, err, "Не удалось изменить пароль")
		return
	}
	c.JSON(http.StatusOK, gin.H{"node_id": id, "protected": body.New != ""})
}

// POST /api/v1/drive/folders/path {parent_id, path} — папки для загрузки папкой.
func (h *DriveHandler) EnsureFolderPath(c *gin.Context) {
	var body struct {
		ParentID *int64 `json:"parent_id"`
		Path     string `json:"path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	node, err := h.svc.EnsureFolderPath(c.Request.Context(), driveActor(c), body.ParentID, body.Path)
	if err != nil {
		writeDriveError(c, err, "Не удалось создать папки")
		return
	}
	c.JSON(http.StatusOK, node)
}

// POST /api/v1/drive/zip-link {ids} — ссылка на ZIP выбранного.
func (h *DriveHandler) ZipLink(c *gin.Context) {
	var body driveArchiveRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	link, err := h.svc.ZipLink(c.Request.Context(), driveActor(c), body.IDs)
	if err != nil {
		writeDriveError(c, err, "Не удалось подготовить архив")
		return
	}
	c.JSON(http.StatusOK, link)
}

// GET /api/v1/drive/zip/:token — сам архив (публичный путь, токен подписан).
func (h *DriveHandler) Zip(c *gin.Context) {
	started := false
	err := h.svc.OpenZip(c.Request.Context(), c.Param("token"), c.Writer, func(name string) {
		started = true
		c.Header("Content-Type", "application/zip")
		c.Header("Content-Disposition", contentDisposition("attachment", name))
		c.Status(http.StatusOK)
	})
	if err != nil && !started {
		writeDriveError(c, err, "Не удалось собрать архив")
	}
}

// POST /api/v1/drive/archive {ids, name}
func (h *DriveHandler) Archive(c *gin.Context) {
	var body driveArchiveRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Некорректный запрос")
		return
	}
	node, err := h.svc.Archive(c.Request.Context(), driveActor(c), body.IDs, body.Name)
	if err != nil {
		writeDriveError(c, err, "Не удалось создать архив")
		return
	}
	c.JSON(http.StatusCreated, node)
}

// POST /api/v1/drive/nodes/:id/unarchive
func (h *DriveHandler) Unarchive(c *gin.Context) {
	id, ok := parsePathID(c, "id")
	if !ok {
		return
	}
	node, err := h.svc.Unarchive(c.Request.Context(), driveActor(c), id)
	if err != nil {
		writeDriveError(c, err, "Не удалось распаковать архив")
		return
	}
	c.JSON(http.StatusCreated, node)
}
