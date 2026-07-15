package handlers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
	"turcompany/internal/storage"
)

type DocumentHandler struct {
	Service *services.DocumentService
	store   storage.Storage
}

// createFromClientRequest — схема payload для POST /documents/create-from-client.
//
// Top-level JSON:
//
//	{
//	  "client_id": number,                // required, int, binding:"required"
//	  "client_type": "individual|legal", // required
//	  "deal_id": number,                  // optional, int, 0 => взять последнюю сделку клиента
//	  "doc_type": string,                 // required, binding:"required"
//	  "extra": {"KEY":"VALUE", ...}    // optional, object<string,string>
//	}
//
// Валидация в сервисе:
//   - Для всех doc_type обязательны базовые поля клиента и deal contract_number.
//   - Для legal клиента дополнительно проверяются company/signer поля и,
//     для contract-подобных шаблонов, банковские реквизиты.
//   - Дополнительно обязательные поля extra по doc_type:
//   - cancel_appointment: reason_code
//   - refund_application: reason_code
//   - pause_application: reason_code
//
// - Остальные extra-поля зависят от doc_type и перечислены в internal/services/document_registry.go (ExtraKeys).
type createFromClientRequest struct {
	ClientID   int               `json:"client_id" binding:"required"`
	ClientType string            `json:"client_type" binding:"required"`
	DealID     int               `json:"deal_id"` // можно 0, тогда возьмём последнюю сделку клиента
	DocType    string            `json:"doc_type" binding:"required"`
	Extra      map[string]string `json:"extra"` // сумма, причина и т.п.
}

const docsCreateFromClientDebugMaxBody = 8192

func docsDebugSchemaEnabled() bool {
	return strings.TrimSpace(os.Getenv("DOCS_DEBUG_SCHEMA")) == "1"
}

func readBodyForDebug(c *gin.Context) []byte {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	return body
}

func truncateForDebugLog(body []byte, limit int) string {
	if limit <= 0 || len(body) <= limit {
		return string(body)
	}
	return string(body[:limit]) + "...(truncated)"
}

func NewDocumentHandler(service *services.DocumentService, store storage.Storage) *DocumentHandler {
	return &DocumentHandler{Service: service, store: store}
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
	if roleID == authz.RoleHR {
		c.JSON(403, gin.H{"error": "document_generation_unavailable", "message": "Генерация документов для HR в разработке"})
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
	scope, ok := archiveScopeFromQuery(c)
	if !ok {
		badRequest(c, "Invalid archive filter")
		return
	}
	filter, err := documentListFilterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if isPaginatedMode(c) {
		page, size := normalizedPageAndSize(c)
		offset := offsetFromPage(page, size)
		docs, total, err := h.Service.ListDocumentsByDealWithFilterAndTotal(dealID, userID, roleID, size, offset, filter, scope)
		if err != nil {
			if err.Error() == "forbidden" {
				forbidden(c, "Forbidden")
				return
			}
			internalError(c, "Could not fetch documents")
			return
		}
		if docs == nil {
			docs = make([]*models.Document, 0)
		}
		c.JSON(http.StatusOK, models.PaginatedResponse[*models.Document]{Items: docs, Pagination: buildPaginationMeta(page, size, total)})
		return
	}

	docs, err := h.Service.ListDocumentsByDealWithFilter(dealID, userID, roleID, filter, scope)
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
	if !authz.CanHardDeleteBusinessEntity(roleID) {
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
	paginate := isPaginatedMode(c)
	page := 1
	size := 100
	if paginate {
		page, size = normalizedPageAndSize(c)
	} else {
		page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
		size, _ = strconv.Atoi(c.DefaultQuery("size", "100"))
		if page < 1 {
			page = 1
		}
		if size < 1 {
			size = 100
		}
	}
	offset := offsetFromPage(page, size)

	userID, roleID := getUserAndRole(c)

	scope, ok := archiveScopeFromQuery(c)
	if !ok {
		badRequest(c, "Invalid archive filter")
		return
	}
	filter, err := documentListFilterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	scopedBranchID, scopeErr := h.Service.ResolveListBranchScope(userID, roleID, filter.BranchID)
	if scopeErr != nil {
		forbidden(c, "Forbidden")
		return
	}
	filter.BranchID = scopedBranchID

	// Изоляция документов по отделам (ТЗ 04.07.2026, п.2.1): каждый отдел видит
	// только свои документы; sales/visa дополнительно видят документы по сделкам
	// (scope 'deal'). Админ и руководство — любой scope.
	//
	// ?scope= сужает выдачу внутри того, что роли и так доступно (табы
	// «Документы отдела» / «Документы клиентов»), но расширить её не может:
	// запрос чужого отдела — 403, а не молча весь список.
	requestedScope := filter.Scope
	if allowed := allowedDocumentScopes(roleID); allowed != nil {
		switch {
		case requestedScope == "":
			filter.Scope = ""
			filter.Scopes = allowed
		case slices.Contains(allowed, requestedScope):
			filter.Scope = requestedScope
			filter.Scopes = nil
		default:
			forbidden(c, "Forbidden scope for your role")
			return
		}
	} else {
		// админ/руководство: ?scope= как есть, без параметра — все документы
		filter.Scopes = nil
	}

	if roleID != authz.RoleSystemAdmin {
		uid := userID
		filter.HiddenVisibilityUserID = &uid
	}

	if paginate {
		docs, total, err := h.Service.ListDocumentsWithFilterAndArchiveScopeAndTotal(size, offset, filter, scope)
		if err != nil {
			internalError(c, "Could not fetch documents")
			return
		}
		c.JSON(http.StatusOK, models.PaginatedResponse[*models.Document]{Items: docs, Pagination: buildPaginationMeta(page, size, total)})
		return
	}

	docs, err := h.Service.ListDocumentsWithFilterAndArchiveScope(size, offset, filter, scope)
	if err != nil {
		internalError(c, "Could not fetch documents")
		return
	}
	c.JSON(http.StatusOK, docs)
}

func documentListFilterFromQuery(c *gin.Context) (repositories.DocumentListFilter, error) {
	filter := repositories.DocumentListFilter{
		Query:      strings.TrimSpace(c.Query("q")),
		Status:     strings.ToLower(strings.TrimSpace(c.Query("status"))),
		DocType:    strings.ToLower(strings.TrimSpace(c.Query("doc_type"))),
		ClientType: strings.ToLower(strings.TrimSpace(c.Query("client_type"))),
		SortBy:     strings.ToLower(strings.TrimSpace(c.Query("sort_by"))),
		Order:      strings.ToLower(strings.TrimSpace(c.Query("order"))),
		Scope:      strings.ToLower(strings.TrimSpace(c.Query("scope"))),
	}
	if filter.Scope != "" && !isAllowedDocumentScope(filter.Scope) {
		return repositories.DocumentListFilter{}, errors.New("Invalid scope")
	}
	if raw := strings.TrimSpace(c.Query("creator_role_id")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			return repositories.DocumentListFilter{}, errors.New("Invalid creator_role_id")
		}
		filter.CreatorRoleID = &v
	}
	if filter.Status != "" && !isAllowedDocumentStatus(filter.Status) {
		return repositories.DocumentListFilter{}, errors.New("Invalid status")
	}
	if raw := strings.TrimSpace(c.Query("deal_id")); raw != "" {
		dealID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || dealID <= 0 {
			return repositories.DocumentListFilter{}, errors.New("Invalid deal_id")
		}
		filter.DealID = &dealID
	}
	if raw := strings.TrimSpace(c.Query("client_id")); raw != "" {
		clientID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || clientID <= 0 {
			return repositories.DocumentListFilter{}, errors.New("Invalid client_id")
		}
		filter.ClientID = &clientID
	}
	if raw := strings.TrimSpace(c.Query("branch_id")); raw != "" {
		branchID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || branchID <= 0 {
			return repositories.DocumentListFilter{}, errors.New("Invalid branch_id")
		}
		filter.BranchID = &branchID
	}
	if filter.ClientType != "" && filter.ClientType != models.ClientTypeIndividual && filter.ClientType != models.ClientTypeLegal {
		return repositories.DocumentListFilter{}, errors.New("Invalid client_type")
	}
	if filter.SortBy != "" && filter.SortBy != "created_at" && filter.SortBy != "status" && filter.SortBy != "doc_type" && filter.SortBy != "name" {
		return repositories.DocumentListFilter{}, errors.New("Invalid sort_by")
	}
	if filter.Order != "" && filter.Order != "asc" && filter.Order != "desc" {
		return repositories.DocumentListFilter{}, errors.New("Invalid order")
	}
	return filter, nil
}

func isAllowedDocumentStatus(status string) bool {
	switch status {
	case "draft", "under_review", "approved", "returned", "sent_for_signature", "signed", "cancelled":
		return true
	default:
		return false
	}
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
		case "unsupported_doc_type_for_lead_use_create_from_client":
			writeError(c, http.StatusBadRequest, UnsupportedDocType, "Unsupported doc_type for lead path; use /documents/create-from-client for legal/templated contracts")
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
	var rawBody []byte
	if docsDebugSchemaEnabled() {
		rawBody = readBodyForDebug(c)
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		if docsDebugSchemaEnabled() {
			log.Printf("[docs:create-from-client] bind error: %s", err.Error())
			log.Printf("[docs:create-from-client] raw body (max %d bytes): %s", docsCreateFromClientDebugMaxBody, truncateForDebugLog(rawBody, docsCreateFromClientDebugMaxBody))
		}
		badRequest(c, "Invalid payload")
		return
	}

	userID, roleID := getUserAndRole(c)
	if roleID == authz.RoleHR {
		c.JSON(403, gin.H{"error": "document_generation_unavailable", "message": "Генерация документов для HR в разработке"})
		return
	}

	doc, err := h.Service.CreateDocumentFromClient(
		req.ClientID,
		req.ClientType,
		req.DealID,
		req.DocType,
		userID,
		roleID,
		req.Extra,
	)
	if err != nil {
		var missingErr *services.DocumentMissingFieldsError
		if errors.As(err, &missingErr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":          "missing_fields",
				"scope":          missingErr.Scope,
				"missing_fields": missingErr.Fields,
			})
			return
		}
		var unresolvedErr *services.DocumentUnresolvedPlaceholdersError
		if errors.As(err, &unresolvedErr) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":  "unresolved_placeholders",
				"fields": gin.H{"doc_type": unresolvedErr.DocType, "template_file": unresolvedErr.TemplateFile, "missing_keys": unresolvedErr.MissingKeys},
			})
			return
		}
		switch err.Error() {
		case "client not found":
			notFound(c, ClientNotFoundCode, "Client not found")
			return
		case "client_type is required", "client_type does not match stored client type":
			badRequest(c, err.Error())
			return
		case "invalid client_type: allowed values are individual, legal":
			badRequest(c, err.Error())
			return
		case "deal not found":
			notFound(c, DealNotFoundCode, "Deal not found")
			return
		case "deal does not belong to client":
			badRequest(c, err.Error())
			return
		case "unsupported doc_type":
			writeError(c, http.StatusBadRequest, UnsupportedDocType, "Unsupported document type")
			return
		case "forbidden", "read-only role":
			forbidden(c, "Read-only role")
			return
		case "template_not_found":
			writeError(c, http.StatusBadRequest, "template_not_found", "Template not found")
			return
		case "pdf_conversion_disabled":
			writeError(c, http.StatusBadRequest, "pdf_conversion_disabled", "PDF conversion is disabled")
			return
		case "pdf_conversion_failed":
			writeError(c, http.StatusInternalServerError, "pdf_conversion_failed", "PDF conversion failed")
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
	var body struct {
		SignedBy string `json:"signed_by"`
		SignedAt string `json:"signed_at"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && err.Error() != "EOF" {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	var signedAt *time.Time
	if strings.TrimSpace(body.SignedAt) != "" {
		t, perr := time.Parse(time.RFC3339, body.SignedAt)
		if perr != nil {
			badRequest(c, "Invalid signed_at")
			return
		}
		signedAt = &t
	}
	if err := h.Service.MarkDocumentSigned(id, body.SignedBy, signedAt, userID, roleID); err != nil {
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

type archiveDocumentRequest struct {
	Reason string `json:"reason"`
}

func (h *DocumentHandler) ArchiveDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var req archiveDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		badRequest(c, "Invalid payload")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.ArchiveDocument(id, userID, roleID, req.Reason); err != nil {
		switch err.Error() {
		case "forbidden", "read-only role":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		}
		internalError(c, "Failed to archive document")
		return
	}
	doc, _ := h.Service.GetDocumentWithArchiveScope(id, userID, roleID, repositories.ArchiveScopeAll)
	c.JSON(http.StatusOK, doc)
}

func (h *DocumentHandler) UnarchiveDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.UnarchiveDocument(id, userID, roleID); err != nil {
		switch err.Error() {
		case "forbidden", "read-only role":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		case "not archived":
			badRequest(c, "Document is not archived")
			return
		}
		internalError(c, "Failed to unarchive document")
		return
	}
	doc, _ := h.Service.GetDocument(id, userID, roleID)
	c.JSON(http.StatusOK, doc)
}

// POST /documents/:id/send-for-signature
func (h *DocumentHandler) SendForSignature(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.PrepareForSignature(id, userID, roleID); err != nil {
		switch err.Error() {
		case "forbidden", "read-only role":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		case "document must be approved before signature":
			writeError(c, http.StatusBadRequest, InvalidStatusCode, "Invalid status")
			return
		}
		internalError(c, "Failed to send document for signature")
		return
	}
	c.Status(http.StatusOK)
}

// GET /documents/types[?scope=visa]
//
// Шаблоны документов: по ним отдел генерирует документы клиентов. Без scope —
// весь реестр с проставленными отделами (нужно админу в Настройках), со scope —
// только шаблоны этого отдела (вкладка «Документы отдела»).
func (h *DocumentHandler) ListDocumentTypes(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	scope := strings.ToLower(strings.TrimSpace(c.Query("scope")))
	if scope != "" {
		if !isAllowedDocumentScope(scope) || scope == "deal" {
			badRequest(c, "scope must be a department scope")
			return
		}
		// чужой отдел не показываем — та же граница, что и у списка документов
		if allowed := allowedDocumentScopes(roleID); allowed != nil && !slices.Contains(allowed, scope) {
			forbidden(c, "Forbidden scope for your role")
			return
		}
	}
	specs, err := h.Service.ListDocumentTypesWithDepartments(c.Request.Context(), scope)
	if err != nil {
		internalError(c, "Could not fetch document types")
		return
	}
	c.JSON(http.StatusOK, specs)
}

type setTemplateDepartmentsRequest struct {
	Scopes []string `json:"scopes"`
}

// PUT /documents/types/:doc_type/departments — админ раскидывает шаблоны по
// отделам. Правка глобальная (шаблон становится виден всему отделу), поэтому
// только системный админ.
func (h *DocumentHandler) SetTemplateDepartments(c *gin.Context) {
	_, roleID := getUserAndRole(c)
	if !authz.CanManageSystem(roleID) {
		forbidden(c, "Forbidden")
		return
	}
	docType := strings.ToLower(strings.TrimSpace(c.Param("doc_type")))
	if docType == "" {
		badRequest(c, "Invalid doc_type")
		return
	}
	var req setTemplateDepartmentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Invalid payload")
		return
	}
	// пустой список допустим — шаблон убирается из всех отделов
	scopes := make([]string, 0, len(req.Scopes))
	for _, raw := range req.Scopes {
		s := strings.ToLower(strings.TrimSpace(raw))
		if !isAllowedDocumentScope(s) || s == "deal" {
			badRequest(c, "scope must be a department scope")
			return
		}
		if !slices.Contains(scopes, s) {
			scopes = append(scopes, s)
		}
	}
	if err := h.Service.SetTemplateDepartments(c.Request.Context(), docType, scopes); err != nil {
		if errors.Is(err, services.ErrDocumentTypeUnknown) {
			badRequest(c, "Неизвестный шаблон документа")
			return
		}
		internalError(c, "Could not save template departments")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
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
	h.serveDocumentFile(c, abs, name, false)
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
	h.serveDocumentFile(c, abs, name, true)
}

// RestoreDocument возвращает документ из корзины (ТЗ 04.07.2026, п.7.1; только админ).
func (h *DocumentHandler) RestoreDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.RestoreDocument(id, userID, roleID); err != nil {
		if err.Error() == "forbidden" {
			forbidden(c, "Forbidden")
			return
		}
		internalError(c, "Failed to restore document")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// PurgeDocument окончательно удаляет документ из корзины (только админ).
func (h *DocumentHandler) PurgeDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if err := h.Service.PurgeDocument(id, userID, roleID); err != nil {
		switch err.Error() {
		case "forbidden":
			forbidden(c, "Forbidden")
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
		default:
			internalError(c, "Failed to purge document")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// isAllowedDocumentScope reports whether the scope value is one of the known
// department scopes (ТЗ 04.07.2026, п.2.1).
func isAllowedDocumentScope(scope string) bool {
	switch scope {
	case "deal", "hr", "legal", "sales", "visa", "partner", "quality_control", "management":
		return true
	default:
		return false
	}
}

// allowedDocumentScopes returns the document scopes a role is allowed to list.
// nil means "no restriction": admin and management browse every department.
// The order matters for the UI — the department's own scope comes first.
func allowedDocumentScopes(roleID int) []string {
	switch roleID {
	case authz.RoleHR:
		return []string{"hr"}
	case authz.RoleLegal:
		return []string{"legal"}
	case authz.RoleControl:
		return []string{"quality_control"}
	case authz.RoleSales:
		return []string{"sales", "deal"}
	case authz.RoleVisa:
		return []string{"visa", "deal"}
	case authz.RolePartner:
		return []string{"partner"}
	default:
		return nil
	}
}

// documentScopeForRole returns the department scope owned by the role
// ("" = no own department scope).
func documentScopeForRole(roleID int) string {
	switch roleID {
	case authz.RoleHR:
		return "hr"
	case authz.RoleLegal:
		return "legal"
	case authz.RoleControl:
		return "quality_control"
	case authz.RoleSales:
		return "sales"
	case authz.RoleVisa:
		return "visa"
	case authz.RolePartner:
		return "partner"
	case authz.RoleManagement:
		return "management"
	default:
		return ""
	}
}

// POST /documents/upload-with-meta
func (h *DocumentHandler) UploadWithMeta(c *gin.Context) {
	userID, roleID := getUserAndRole(c)
	scope := c.PostForm("scope")
	if scope == "" {
		// без явного scope документ падает в отдел загрузившего
		scope = documentScopeForRole(roleID)
	}
	if !isAllowedDocumentScope(scope) || scope == "deal" {
		badRequest(c, "scope must be a department scope")
		return
	}
	// каждый отдел загружает только в свой scope; админ и руководство — в любой
	if roleID != authz.RoleSystemAdmin && roleID != authz.RoleManagement {
		if own := documentScopeForRole(roleID); own == "" || own != scope {
			forbidden(c, "Forbidden scope for your role")
			return
		}
	}
	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		badRequest(c, "title is required")
		return
	}
	description := strings.TrimSpace(c.PostForm("description"))
	var targetUserID *int64
	if raw := strings.TrimSpace(c.PostForm("target_user_id")); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			badRequest(c, "Invalid target_user_id")
			return
		}
		targetUserID = &v
	}
	file, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "file is required")
		return
	}
	ext := strings.ToLower(filepath.Ext(file.Filename))
	allowed := map[string]bool{".pdf": true, ".docx": true, ".xlsx": true, ".txt": true}
	if !allowed[ext] {
		badRequest(c, "Unsupported file type. Allowed: pdf, docx, xlsx, txt")
		return
	}
	if file.Size > 50<<20 {
		badRequest(c, "File too large. Max 50MB")
		return
	}
	doc, saveErr := h.Service.UploadDocumentWithMeta(scope, title, description, targetUserID, file, userID, roleID)
	if saveErr != nil {
		internalError(c, "Failed to upload document")
		return
	}
	c.JSON(http.StatusCreated, doc)
}

// serveDocumentFile serves a document from S3 when available, falling back to
// local disk for files generated by LibreOffice/docx/pdf generators that haven't
// been uploaded to S3 yet.
func (h *DocumentHandler) serveDocumentFile(c *gin.Context, key, name string, attachment bool) {
	disposition := "inline"
	if attachment {
		disposition = fmt.Sprintf(`attachment; filename="%s"`, name)
	}

	// Try S3 first.
	if h.store != nil {
		reader, _, err := h.store.Open(c.Request.Context(), key)
		if err == nil {
			defer reader.Close()
			c.Header("Content-Disposition", disposition)
			http.ServeContent(c.Writer, c.Request, name, time.Time{}, reader)
			return
		}
		// Not in S3 — fall back to local disk (generated file not yet uploaded).
		localPath := filepath.Join(h.Service.FilesRoot, filepath.FromSlash(key))
		if _, statErr := os.Stat(localPath); statErr == nil {
			c.Header("Content-Disposition", disposition)
			if attachment {
				c.FileAttachment(localPath, name)
			} else {
				c.File(localPath)
			}
			return
		}
		notFound(c, DocumentNotFound, "Document not found")
		return
	}

	// Local-only mode.
	c.Header("Content-Disposition", disposition)
	if attachment {
		c.FileAttachment(key, name)
	} else {
		c.File(key)
	}
}
