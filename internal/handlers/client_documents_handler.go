package handlers

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type ClientDocumentsHandler struct {
	docService *services.DocumentService
	clientRepo *repositories.ClientRepository
	docRepo    *repositories.DocumentRepository
}

func NewClientDocumentsHandler(
	docService *services.DocumentService,
	clientRepo *repositories.ClientRepository,
	docRepo *repositories.DocumentRepository,
) *ClientDocumentsHandler {
	return &ClientDocumentsHandler{
		docService: docService,
		clientRepo: clientRepo,
		docRepo:    docRepo,
	}
}

func (h *ClientDocumentsHandler) ensureClientAccess(c *gin.Context, clientID int) (*models.Client, bool) {
	client, err := h.clientRepo.GetByID(clientID)
	if err != nil || client == nil {
		if errors.Is(err, services.ErrForbidden) {
			forbidden(c, "У вас нет доступа к этому клиенту")
		} else {
			notFound(c, ClientNotFoundCode, "Клиент не найден")
		}
		return nil, false
	}
	return client, true
}

// GET /clients/:id/documents
func (h *ClientDocumentsHandler) ListDocuments(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}

	if client, ok := h.ensureClientAccess(c, clientID); !ok {
		return
	} else {
		_ = client // ensureClientAccess returns the client for use in CreateDocument
	}

	archiveScope, ok := archiveScopeFromQuery(c)
	if !ok {
		badRequest(c, "Некорректный archive scope")
		return
	}

	clientID64 := int64(clientID)
	userID, _ := getUserAndRole(c)

	filter := repositories.DocumentListFilter{
		ClientID: &clientID64,
		Status:   strings.TrimSpace(c.Query("status")),
		DocType:  strings.TrimSpace(c.Query("doc_type")),
		Query:    strings.TrimSpace(c.Query("q")),
		SortBy:   strings.TrimSpace(c.Query("sort")),
		Order:    strings.TrimSpace(c.Query("order")),
	}

	// Pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size

	hiddenUserID := userID
	filter.HiddenVisibilityUserID = &hiddenUserID

	total, err := h.docRepo.CountDocumentsWithFilterAndArchiveScope(filter, archiveScope)
	if err != nil {
		internalError(c, "Не удалось получить количество документов")
		return
	}

	docs, err := h.docRepo.ListDocumentsWithFilterAndArchiveScope(size, offset, filter, archiveScope)
	if err != nil {
		internalError(c, "Не удалось загрузить документы")
		return
	}
	if docs == nil {
		docs = []*models.Document{}
	}

	c.JSON(200, gin.H{
		"items": docs,
		"total": total,
		"page":  page,
		"size":  size,
	})
}

// POST /clients/:id/documents
func (h *ClientDocumentsHandler) CreateDocument(c *gin.Context) {
	clientID, err := strconv.Atoi(c.Param("id"))
	if err != nil || clientID <= 0 {
		badRequest(c, "Некорректный ID клиента")
		return
	}
	userID, roleID := getUserAndRole(c)

	client, ok := h.ensureClientAccess(c, clientID)
	if !ok {
		return
	}

	var req struct {
		DealID  *int64 `json:"deal_id"`
		DocType string `json:"doc_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Некорректные данные")
		return
	}
	req.DocType = strings.TrimSpace(req.DocType)
	if req.DocType == "" {
		badRequest(c, "Укажите тип документа")
		return
	}

	dealID := int(0)
	if req.DealID != nil {
		dealID = int(*req.DealID)
	}

	clientType := "individual"
	if client.ClientType != "" {
		clientType = client.ClientType
	}

	doc, err := h.docService.CreateDocumentFromClient(
		clientID,
		clientType,
		dealID,
		req.DocType,
		userID,
		roleID,
		nil,
	)
	if err != nil {
		h.handleDocServiceError(c, err)
		return
	}

	// Set client_id on the newly created document
	if doc != nil {
		clientID64 := int64(clientID)
		_ = h.docRepo.SetClientID(doc.ID, clientID64)
		doc.ClientID = &clientID64
	}

	c.JSON(201, doc)
}

func (h *ClientDocumentsHandler) handleDocServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrForbidden):
		forbidden(c, "У вас нет права создавать документы")
	case errors.Is(err, services.ErrReadOnly):
		forbidden(c, "Только просмотр")
	case errors.Is(err, services.ErrClientNotFound):
		notFound(c, ClientNotFoundCode, "Клиент не найден")
	case errors.Is(err, services.ErrDealNotFound):
		notFound(c, DealNotFoundCode, "Сделка не найдена")
	case strings.Contains(err.Error(), "invalid"):
		badRequest(c, fmt.Sprintf("Ошибка валидации: %v", err))
	default:
		internalError(c, "Не удалось создать документ")
	}
}

