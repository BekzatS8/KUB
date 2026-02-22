package handlers

import (
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type DocumentHandler struct {
	Service *services.DocumentService
}
type createFromClientRequest struct {
	ClientID int               `json:"client_id" binding:"required"`
	DealID   int               `json:"deal_id"` // можно 0, тогда возьмём последнюю сделку клиента
	DocType  string            `json:"doc_type" binding:"required"`
	Extra    map[string]string `json:"extra"` // сумма, причина и т.п.
}

func NewDocumentHandler(service *services.DocumentService) *DocumentHandler {
	return &DocumentHandler{Service: service}
}

// ===== CRUD =====

// POST /documents
func (h *DocumentHandler) CreateDocument(c *gin.Context) {
	var doc models.Document
	if err := c.ShouldBindJSON(&doc); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	id, err := h.Service.CreateDocument(&doc, userID, roleID)
	if err != nil {
		switch err.Error() {
		case "read-only role", "forbidden":
			forbidden(c, "Read-only role")
			return
		case "deal not found":
			notFound(c, DealNotFoundCode, "Deal not found")
			return
		case "lead not found":
			notFound(c, LeadNotFoundCode, "Lead not found")
			return
		case "unsupported doc_type":
			writeError(c, http.StatusBadRequest, UnsupportedDocType, "Unsupported document type")
			return
		case "pdf generator not configured":
			internalError(c, "Failed to create document")
			return
		}
		internalError(c, "Failed to create document")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// POST /documents/upload
func (h *DocumentHandler) Upload(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	dealID, err := strconv.ParseInt(c.PostForm("deal_id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid deal id")
		return
	}
	docType := c.PostForm("doc_type")
	file, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "File is required")
		return
	}
	doc, saveErr := h.Service.UploadDocument(dealID, docType, file, userID, roleID)
	if saveErr != nil {
		switch saveErr.Error() {
		case "forbidden", "read-only role":
			forbidden(c, "Read-only role")
			return
		case "deal not found":
			notFound(c, DealNotFoundCode, "Deal not found")
			return
		case "doc_type is required", "invalid filename":
			badRequest(c, "Invalid payload")
			return
		case "unsupported doc_type":
			writeError(c, http.StatusBadRequest, UnsupportedDocType, "Unsupported document type")
			return
		}
		internalError(c, "Failed to upload document")
		return
	}
	c.JSON(http.StatusCreated, doc)
}

// GET /documents/:id
func (h *DocumentHandler) GetDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	doc, err := h.Service.GetDocument(id, userID, roleID)
	if err != nil || doc == nil {
		if err != nil && err.Error() == "forbidden" {
			forbidden(c, "Forbidden")
			return
		}
		notFound(c, DocumentNotFound, "Document not found")
		return
	}
	c.JSON(http.StatusOK, doc)
}

// GET /documents/deal/:dealid
func (h *DocumentHandler) ListDocumentsByDeal(c *gin.Context) {
	dealID, err := strconv.ParseInt(c.Param("dealid"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid deal id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	docs, err := h.Service.ListDocumentsByDeal(dealID, userID, roleID)
	if err != nil {
		if err.Error() == "forbidden" {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Could not fetch documents")
		return
	}
	c.JSON(http.StatusOK, docs)
}

// DELETE /documents/:id
func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if err := h.Service.DeleteDocument(id, userID, roleID); err != nil {
		switch err.Error() {
		case "read-only role", "forbidden":
			forbidden(c, "Read-only role")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		}
		internalError(c, "Failed to delete document")
		return
	}
	c.Status(http.StatusNoContent)
}

// GET /documents
func (h *DocumentHandler) ListDocuments(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "100"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 100
	}
	offset := (page - 1) * size

	// доступ:
	// - Sales: общий список запрещаем (смотри по сделке /documents/deal/:dealid)
	// - Ops/Mgmt/Admin/Control: можно
	_, roleID := getUserAndRole(c)
	if roleID == authz.RoleSales {
		forbidden(c, "Forbidden for sales; use /documents/deal/{dealid}")
		return
	}

	docs, err := h.Service.ListDocuments(size, offset)
	if err != nil {
		internalError(c, "Could not fetch documents")
		return
	}
	c.JSON(http.StatusOK, docs)
}

// ===== Специальный сценарий =====
// POST /documents/create-from-lead
func (h *DocumentHandler) CreateDocumentFromLead(c *gin.Context) {
	var req struct {
		LeadID  int    `json:"lead_id"  binding:"required"`
		DocType string `json:"doc_type" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)

	doc, err := h.Service.CreateDocumentFromLead(req.LeadID, req.DocType, userID, roleID)
	if err != nil {
		switch err.Error() {
		case "lead not found":
			notFound(c, LeadNotFoundCode, "Lead not found")
			return
		case "deal not found":
			notFound(c, DealNotFoundCode, "Deal not found")
			return
		case "forbidden", "read-only role":
			forbidden(c, "Read-only role")
			return
		}
		internalError(c, "Failed to create document")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Документ успешно создан",
		"document": doc,
	})
}

// POST /documents/create-from-client
func (h *DocumentHandler) CreateDocumentFromClient(c *gin.Context) {
	var req createFromClientRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}

	userID, roleID := getUserAndRole(c)

	doc, err := h.Service.CreateDocumentFromClient(
		req.ClientID,
		req.DealID,
		req.DocType,
		userID,
		roleID,
		req.Extra,
	)
	if err != nil {
		var missingErr *services.MissingFieldsError
		if errors.As(err, &missingErr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          "missing required client fields for document",
				"missing_fields": missingErr.Fields,
			})
			return
		}
		switch err.Error() {
		case "client not found":
			notFound(c, ClientNotFoundCode, "Client not found")
			return
		case "deal not found":
			notFound(c, DealNotFoundCode, "Deal not found")
			return
		case "unsupported doc_type":
			writeError(c, http.StatusBadRequest, UnsupportedDocType, "Unsupported document type")
			return
		case "forbidden", "read-only role":
			forbidden(c, "Read-only role")
			return
		case "pdf generator not configured":
			internalError(c, "Failed to create document")
			return
		}
		internalError(c, "Failed to create document")
		return
	}

	c.JSON(http.StatusCreated, doc)
}

// ===== Статусные операции =====

// POST /documents/:id/submit
// Sales -> submit: draft -> under_review
func (h *DocumentHandler) Submit(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.Submit(id, userID, roleID); err != nil {
		switch err.Error() {
		case "read-only role":
			forbidden(c, "Read-only role")
			return
		case "forbidden":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		case "invalid status":
			writeError(c, http.StatusBadRequest, InvalidStatusCode, "Invalid status")
			return
		}
		internalError(c, "Failed to submit document")
		return
	}
	c.Status(http.StatusOK)
}

// POST /documents/:id/review
// Ops/Mgmt/Admin -> review: under_review -> approved | returned
func (h *DocumentHandler) Review(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var body struct {
		Action string `json:"action" binding:"required"` // "approve" | "return"
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.Review(id, body.Action, userID, roleID); err != nil {
		switch err.Error() {
		case "forbidden":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		case "invalid status", "bad action":
			writeError(c, http.StatusBadRequest, InvalidStatusCode, "Invalid status")
			return
		}
		internalError(c, "Failed to review document")
		return
	}
	c.Status(http.StatusOK)
}

// POST /documents/:id/sign
// Mgmt/Admin -> sign: approved|returned -> signed
func (h *DocumentHandler) Sign(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleAdminStaff {
		forbidden(c, "Forbidden")
		return
	}
	if err := h.Service.Sign(id, userID, roleID); err != nil {
		switch err.Error() {
		case "forbidden":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		case "invalid status":
			writeError(c, http.StatusBadRequest, InvalidStatusCode, "Invalid status")
			return
		}
		internalError(c, "Failed to sign document")
		return
	}
	c.Status(http.StatusOK)
}
func (h *DocumentHandler) ServeFile(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)

	abs, name, err := h.Service.ResolveFileForHTTP(id, userID, roleID, "original")
	if err != nil {
		switch err.Error() {
		case "not found", "file not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		case "forbidden":
			forbidden(c, "Forbidden")
			return
		case "bad filepath":
			badRequest(c, "Invalid file path")
			return
		}
		internalError(c, "Failed to fetch document")
		return
	}

	ext := strings.ToLower(filepath.Ext(name))
	ct := "application/octet-stream"
	switch ext {
	case ".pdf":
		ct = "application/pdf"
	case ".docx":
		ct = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		ct = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}

	c.Header("Content-Type", ct)
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, name))
	c.File(abs)
}

func (h *DocumentHandler) Download(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)

	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	variant := "original" // по умолчанию отдаём исходник (DOCX/XLSX), а не PDF
	switch format {
	case "", "original", "source":
		variant = "original"
	case "pdf", "docx", "xlsx":
		variant = format
	default:
		badRequest(c, "Invalid format")
		return
	}

	abs, name, err := h.Service.ResolveFileForHTTP(id, userID, roleID, variant)
	if err != nil {
		switch err.Error() {
		case "not found", "file not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		case "forbidden":
			forbidden(c, "Forbidden")
			return
		case "bad filepath":
			badRequest(c, "Invalid file path")
			return
		}
		internalError(c, "Failed to fetch document")
		return
	}

	ext := strings.ToLower(filepath.Ext(name))
	ct := mime.TypeByExtension(ext)
	if ct == "" {
		ct = "application/octet-stream"
	}

	c.Header("Content-Type", ct)
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(abs, name) // корректный attachment + filename
}
