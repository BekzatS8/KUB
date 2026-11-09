package handlers

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/services"
)

// EmployeeHandler — раздел «Сотрудники» и кадровые документы (ТЗ «доработки
// CRM КУБ», п.1–2).
type EmployeeHandler struct {
	svc *services.EmployeeService
}

func NewEmployeeHandler(svc *services.EmployeeService) *EmployeeHandler {
	return &EmployeeHandler{svc: svc}
}

func writeEmployeeError(c *gin.Context, err error, fallback string) {
	if msg, ok := services.IsEmployeeValidation(err); ok {
		badRequestWithCode(c, "employee_invalid", msg)
		return
	}
	switch {
	case errors.Is(err, services.ErrForbidden):
		forbidden(c, "Нет доступа")
	case errors.Is(err, services.ErrNotFound):
		notFound(c, "employee_not_found", "Не найдено")
	default:
		log.Printf("[employees] %s %s: %v", c.Request.Method, c.FullPath(), err)
		internalError(c, fallback)
	}
}

func employeeID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		badRequest(c, "Invalid id")
		return 0, false
	}
	return id, true
}

func employeeDocID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("docId"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(c, "Invalid document id")
		return 0, false
	}
	return id, true
}

// GET /employees?q=&type=&position=&include_fired=&sort_by=&order=&page=&size=
func (h *EmployeeHandler) List(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	page, size := normalizedPageAndSize(c)
	items, total, err := h.svc.List(c.Request.Context(), models.EmployeeFilter{
		Query:        c.Query("q"),
		Type:         strings.TrimSpace(c.Query("type")),
		Position:     c.Query("position"),
		IncludeFired: c.Query("include_fired") == "true" || c.Query("include_fired") == "1",
		SortBy:       c.Query("sort_by"),
		Order:        c.Query("order"),
		Limit:        size,
		Offset:       offsetFromPage(page, size),
	}, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to list employees")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "size": size})
}

// GET /employees/positions
func (h *EmployeeHandler) Positions(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	items, err := h.svc.Positions(c.Request.Context(), roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to list positions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// GET /employees/:id
func (h *EmployeeHandler) Get(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	_, roleID := getUserAndRole(c)
	e, err := h.svc.Get(c.Request.Context(), id, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to get employee")
		return
	}
	c.JSON(http.StatusOK, e)
}

// POST /employees
func (h *EmployeeHandler) Create(c *gin.Context) {
	var body models.Employee
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	e, err := h.svc.Create(c.Request.Context(), &body, userID, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to create employee")
		return
	}
	c.JSON(http.StatusCreated, e)
}

// PUT /employees/:id
func (h *EmployeeHandler) Update(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	var body models.Employee
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	body.ID = id
	_, roleID := getUserAndRole(c)
	e, err := h.svc.Update(c.Request.Context(), &body, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to update employee")
		return
	}
	c.JSON(http.StatusOK, e)
}

// POST /employees/:id/photo (multipart, поле file)
func (h *EmployeeHandler) UploadPhoto(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	fh, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Файл не передан")
		return
	}
	if fh.Size > 5<<20 {
		badRequestWithCode(c, "employee_invalid", "Фото — не больше 5 МБ")
		return
	}
	f, err := fh.Open()
	if err != nil {
		badRequest(c, "Не удалось прочитать файл")
		return
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, 5<<20+1))
	if err != nil {
		badRequest(c, "Не удалось прочитать файл")
		return
	}
	_, roleID := getUserAndRole(c)
	if err := h.svc.SetPhoto(c.Request.Context(), id, content, roleID); err != nil {
		writeEmployeeError(c, err, "Failed to save photo")
		return
	}
	e, _ := h.svc.Get(c.Request.Context(), id, roleID)
	c.JSON(http.StatusOK, e)
}

// GET /employees/:id/photo
func (h *EmployeeHandler) Photo(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	_, roleID := getUserAndRole(c)
	r, ct, err := h.svc.OpenPhoto(c.Request.Context(), id, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to open photo")
		return
	}
	defer r.Close()
	c.Header("Cache-Control", "private, max-age=300")
	c.DataFromReader(http.StatusOK, -1, ct, r, nil)
}

// GET /employees/templates
func (h *EmployeeHandler) ListTemplates(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	items, err := h.svc.ListTemplates(c.Request.Context(), roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to list templates")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// POST /employees/templates (multipart: file, name, kind)
func (h *EmployeeHandler) UploadTemplate(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Файл не передан")
		return
	}
	f, err := fh.Open()
	if err != nil {
		badRequest(c, "Не удалось прочитать файл")
		return
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, 20<<20+1))
	if err != nil {
		badRequest(c, "Не удалось прочитать файл")
		return
	}
	userID, roleID := getUserAndRole(c)
	t, err := h.svc.UploadTemplate(c.Request.Context(), c.PostForm("name"), c.PostForm("kind"), fh.Filename, content, userID, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to upload template")
		return
	}
	c.JSON(http.StatusCreated, t)
}

// GET /employees/templates/:id/file
func (h *EmployeeHandler) TemplateFile(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	_, roleID := getUserAndRole(c)
	r, name, err := h.svc.OpenTemplate(c.Request.Context(), id, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to open template")
		return
	}
	defer r.Close()
	c.DataFromReader(http.StatusOK, -1, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", r,
		map[string]string{"Content-Disposition": contentDisposition("attachment", name)})
}

// DELETE /employees/templates/:id
func (h *EmployeeHandler) DeleteTemplate(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	_, roleID := getUserAndRole(c)
	if err := h.svc.DeleteTemplate(c.Request.Context(), id, roleID); err != nil {
		writeEmployeeError(c, err, "Failed to delete template")
		return
	}
	c.Status(http.StatusNoContent)
}

// GET /employees/:id/documents
func (h *EmployeeHandler) ListDocuments(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	_, roleID := getUserAndRole(c)
	items, err := h.svc.ListDocuments(c.Request.Context(), id, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to list documents")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// POST /employees/:id/documents {template_ids: [...]} — «Выписать документ».
func (h *EmployeeHandler) IssueDocuments(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	var body struct {
		TemplateIDs []int `json:"template_ids"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	items, err := h.svc.IssueDocuments(c.Request.Context(), id, body.TemplateIDs, userID, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to issue documents")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"items": items})
}

// POST /employees/:id/documents/:docId/send {channel: sms|email}
func (h *EmployeeHandler) SendDocument(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	docID, ok := employeeDocID(c)
	if !ok {
		return
	}
	var body struct {
		Channel string `json:"channel"`
	}
	_ = c.ShouldBindJSON(&body)
	userID, roleID := getUserAndRole(c)
	d, err := h.svc.SendDocument(c.Request.Context(), id, docID, body.Channel, userID, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to send document")
		return
	}
	c.JSON(http.StatusOK, d)
}

// POST /employees/:id/documents/:docId/revoke
func (h *EmployeeHandler) RevokeDocument(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	docID, ok := employeeDocID(c)
	if !ok {
		return
	}
	_, roleID := getUserAndRole(c)
	d, err := h.svc.RevokeDocument(c.Request.Context(), id, docID, roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to revoke document")
		return
	}
	c.JSON(http.StatusOK, d)
}

// GET /employees/:id/documents/:docId/file?variant=pdf|signed|docx&download=1
func (h *EmployeeHandler) DocumentFile(c *gin.Context) {
	id, ok := employeeID(c)
	if !ok {
		return
	}
	docID, ok := employeeDocID(c)
	if !ok {
		return
	}
	_, roleID := getUserAndRole(c)
	r, ct, name, err := h.svc.OpenDocumentFile(c.Request.Context(), id, docID, c.Query("variant"), roleID)
	if err != nil {
		writeEmployeeError(c, err, "Failed to open document")
		return
	}
	defer r.Close()
	disp := "inline"
	if c.Query("download") == "1" {
		disp = "attachment"
	}
	c.DataFromReader(http.StatusOK, -1, ct, r, map[string]string{"Content-Disposition": contentDisposition(disp, name)})
}

// contentDisposition — заголовок с именем файла, в том числе кириллическим
// (RFC 5987: filename* в UTF-8 и ASCII-запасной вариант).
func contentDisposition(disposition, name string) string {
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	return disposition + `; filename="` + ascii + `"; filename*=UTF-8''` + url.PathEscape(name)
}
