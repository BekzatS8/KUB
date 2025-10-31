package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type DocumentHandler struct {
	Service *services.DocumentService
}

func NewDocumentHandler(service *services.DocumentService) *DocumentHandler {
	return &DocumentHandler{Service: service}
}

// ===== CRUD =====

// POST /documents
func (h *DocumentHandler) CreateDocument(c *gin.Context) {
	var doc models.Document
	if err := c.ShouldBindJSON(&doc); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, roleID := getUserAndRole(c)
	id, err := h.Service.CreateDocument(&doc, userID, roleID)
	if err != nil {
		status := http.StatusInternalServerError
		switch err.Error() {
		case "read-only role", "forbidden":
			status = http.StatusForbidden
		case "deal not found", "lead not found", "unsupported doc_type":
			status = http.StatusBadRequest
		case "pdf generator not configured":
			status = http.StatusInternalServerError
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

// GET /documents/:id
func (h *DocumentHandler) GetDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID, roleID := getUserAndRole(c)
	doc, err := h.Service.GetDocument(id, userID, roleID)
	if err != nil || doc == nil {
		code := http.StatusNotFound
		if err != nil && err.Error() == "forbidden" {
			code = http.StatusForbidden
		}
		c.JSON(code, gin.H{"error": "document not found"})
		return
	}
	c.JSON(http.StatusOK, doc)
}

// GET /documents/deal/:dealid
func (h *DocumentHandler) ListDocumentsByDeal(c *gin.Context) {
	dealID, err := strconv.ParseInt(c.Param("dealid"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deal id"})
		return
	}
	userID, roleID := getUserAndRole(c)
	docs, err := h.Service.ListDocumentsByDeal(dealID, userID, roleID)
	if err != nil {
		code := http.StatusInternalServerError
		if err.Error() == "forbidden" {
			code = http.StatusForbidden
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, docs)
}

// DELETE /documents/:id
func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.DeleteDocument(id, userID, roleID); err != nil {
		code := http.StatusInternalServerError
		switch err.Error() {
		case "read-only role", "forbidden":
			code = http.StatusForbidden
		case "not found":
			code = http.StatusNotFound
		}
		c.JSON(code, gin.H{"error": err.Error()})
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
	// - Ops/Mgmt/Admin/Audit: можно
	_, roleID := getUserAndRole(c)
	if roleID == authz.RoleSales {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden for sales; use /documents/deal/{dealid}"})
		return
	}

	docs, err := h.Service.ListDocuments(size, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not fetch documents"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, roleID := getUserAndRole(c)

	doc, err := h.Service.CreateDocumentFromLead(req.LeadID, req.DocType, userID, roleID)
	if err != nil {
		code := http.StatusInternalServerError
		switch err.Error() {
		case "lead not found", "deal not found":
			code = http.StatusBadRequest
		case "forbidden", "read-only role":
			code = http.StatusForbidden
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":  "Документ успешно создан",
		"document": doc,
	})
}

// ===== Статусные операции =====

// POST /documents/:id/submit
// Sales -> submit: draft -> under_review
func (h *DocumentHandler) Submit(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.Submit(id, userID, roleID); err != nil {
		code := http.StatusBadRequest
		switch err.Error() {
		case "read-only role", "forbidden":
			code = http.StatusForbidden
		case "not found":
			code = http.StatusNotFound
		case "invalid status":
			code = http.StatusBadRequest
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

// POST /documents/:id/review
// Ops/Mgmt/Admin -> review: under_review -> approved | returned
func (h *DocumentHandler) Review(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Action string `json:"action" binding:"required"` // "approve" | "return"
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.Review(id, body.Action, userID, roleID); err != nil {
		code := http.StatusBadRequest
		switch err.Error() {
		case "forbidden":
			code = http.StatusForbidden
		case "not found":
			code = http.StatusNotFound
		case "invalid status", "bad action":
			code = http.StatusBadRequest
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

// POST /documents/:id/sign
// Mgmt/Admin -> sign: approved|returned -> signed
func (h *DocumentHandler) Sign(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.Sign(id, userID, roleID); err != nil {
		code := http.StatusBadRequest
		switch err.Error() {
		case "forbidden":
			code = http.StatusForbidden
		case "not found":
			code = http.StatusNotFound
		case "invalid status":
			code = http.StatusBadRequest
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}
func (h *DocumentHandler) ServeFile(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID, roleID := getUserAndRole(c)

	abs, name, err := h.Service.ResolveFileForHTTP(id, userID, roleID, false)
	if err != nil {
		code := http.StatusInternalServerError
		switch err.Error() {
		case "not found", "file not found":
			code = http.StatusNotFound
		case "forbidden":
			code = http.StatusForbidden
		case "bad filepath":
			code = http.StatusBadRequest
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}

	// inline
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, name))
	c.File(abs)
}

func (h *DocumentHandler) Download(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID, roleID := getUserAndRole(c)

	abs, name, err := h.Service.ResolveFileForHTTP(id, userID, roleID, true)
	if err != nil {
		code := http.StatusInternalServerError
		switch err.Error() {
		case "not found", "file not found":
			code = http.StatusNotFound
		case "forbidden":
			code = http.StatusForbidden
		case "bad filepath":
			code = http.StatusBadRequest
		}
		c.JSON(code, gin.H{"error": err.Error()})
		return
	}

	// attachment
	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	c.File(abs)
}
