package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

const (
	SignConfirmExpiredCode      = "EXPIRED"
	SignConfirmTooManyAttempts  = "TOO_MANY_ATTEMPTS"
	SignConfirmInvalidCode      = "INVALID_CODE"
	SignConfirmNotFoundCode     = "NOT_FOUND"
	signConfirmSuccessPath      = "/sign/confirm"
	signConfirmStatusQuery      = "status"
	signConfirmErrorQuery       = "error"
	signConfirmStatusSuccessVal = "success"
	signConfirmStatusFailVal    = "fail"
)

type DocumentSigningConfirmationHandler struct {
	Service      *services.DocumentSigningConfirmationService
	DocumentSvc  *services.DocumentService
	FrontendHost string
}

func NewDocumentSigningConfirmationHandler(
	service *services.DocumentSigningConfirmationService,
	documentSvc *services.DocumentService,
	frontendHost string,
) *DocumentSigningConfirmationHandler {
	return &DocumentSigningConfirmationHandler{
		Service:      service,
		DocumentSvc:  documentSvc,
		FrontendHost: strings.TrimRight(strings.TrimSpace(frontendHost), "/"),
	}
}

func (h *DocumentSigningConfirmationHandler) StartSigning(c *gin.Context) {
	if h.Service == nil || h.DocumentSvc == nil {
		internalError(c, "Service unavailable")
		return
	}
	documentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	if _, err := h.DocumentSvc.GetDocument(documentID, userID, roleID); err != nil {
		switch err.Error() {
		case "forbidden":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		}
		internalError(c, "Failed to fetch document")
		return
	}

	result, err := h.Service.StartSigning(c.Request.Context(), documentID, int64(userID))
	if err != nil {
		handleSignConfirmError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

type emailConfirmRequest struct {
	Code string `json:"code" binding:"required"`
}

func (h *DocumentSigningConfirmationHandler) ConfirmByEmailCode(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	documentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	var body emailConfirmRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		badRequest(c, "Invalid request body")
		return
	}
	userID, roleID := getUserAndRole(c)
	if h.DocumentSvc != nil {
		if _, err := h.DocumentSvc.GetDocument(documentID, userID, roleID); err != nil {
			switch err.Error() {
			case "forbidden":
				forbidden(c, "Forbidden")
				return
			case "not found":
				notFound(c, DocumentNotFound, "Document not found")
				return
			}
			internalError(c, "Failed to fetch document")
			return
		}
	}
	ip := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	confirmation, err := h.Service.ConfirmByEmailCode(
		c.Request.Context(),
		documentID,
		int64(userID),
		body.Code,
		ip,
		userAgent,
	)
	if err != nil {
		handleSignConfirmError(c, err)
		return
	}
	c.JSON(http.StatusOK, confirmation)
}

func (h *DocumentSigningConfirmationHandler) VerifyEmailToken(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		badRequest(c, "Token required")
		return
	}
	_, err := h.Service.ConfirmByEmailToken(c.Request.Context(), token)
	if err != nil {
		redirect := h.buildFrontendRedirect(signConfirmStatusFailVal, mapErrorCode(err))
		c.Redirect(http.StatusFound, redirect)
		return
	}
	redirect := h.buildFrontendRedirect(signConfirmStatusSuccessVal, "")
	c.Redirect(http.StatusFound, redirect)
}

func (h *DocumentSigningConfirmationHandler) Status(c *gin.Context) {
	if h.Service == nil || h.DocumentSvc == nil {
		internalError(c, "Service unavailable")
		return
	}
	documentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	doc, err := h.DocumentSvc.GetDocument(documentID, userID, roleID)
	if err != nil {
		switch err.Error() {
		case "forbidden":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		}
		internalError(c, "Failed to fetch document")
		return
	}

	channels, err := h.Service.GetStatus(c.Request.Context(), documentID, int64(userID))
	if err != nil {
		internalError(c, "Failed to fetch status")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"document": gin.H{
			"id":     doc.ID,
			"status": doc.Status,
		},
		"channels": channels,
	})
}

func (h *DocumentSigningConfirmationHandler) DebugLatest(c *gin.Context) {
	if gin.Mode() == gin.ReleaseMode {
		notFound(c, DocumentNotFound, "Not found")
		return
	}
	if h.Service == nil || !h.Service.DebugEnabled() {
		notFound(c, DocumentNotFound, "Not found")
		return
	}
	debugKey := h.Service.DebugKey()
	if debugKey != "" && c.GetHeader("X-Debug-Key") != debugKey {
		forbidden(c, "Forbidden")
		return
	}
	documentID, err := strconv.ParseInt(c.Query("document_id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid document_id")
		return
	}
	userID, _ := getUserAndRole(c)
	info, ok := h.Service.DebugLatest(documentID, int64(userID))
	if !ok {
		notFound(c, DocumentNotFound, "Not found")
		return
	}
	c.JSON(http.StatusOK, info)
}

func handleSignConfirmError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrSignConfirmExpired):
		writeError(c, http.StatusBadRequest, SignConfirmExpiredCode, "Expired")
	case errors.Is(err, services.ErrSignConfirmTooManyTries):
		writeError(c, http.StatusTooManyRequests, SignConfirmTooManyAttempts, "Too many attempts")
	case errors.Is(err, services.ErrSignConfirmInvalidCode), errors.Is(err, services.ErrSignConfirmInvalidToken):
		writeError(c, http.StatusBadRequest, SignConfirmInvalidCode, "Invalid code")
	case errors.Is(err, services.ErrSignConfirmNotFound):
		writeError(c, http.StatusNotFound, SignConfirmNotFoundCode, "Not found")
	default:
		internalError(c, "Failed to confirm signing")
	}
}

func mapErrorCode(err error) string {
	switch {
	case errors.Is(err, services.ErrSignConfirmExpired):
		return SignConfirmExpiredCode
	case errors.Is(err, services.ErrSignConfirmTooManyTries):
		return SignConfirmTooManyAttempts
	case errors.Is(err, services.ErrSignConfirmInvalidCode), errors.Is(err, services.ErrSignConfirmInvalidToken):
		return SignConfirmInvalidCode
	case errors.Is(err, services.ErrSignConfirmNotFound):
		return SignConfirmNotFoundCode
	default:
		return SignConfirmNotFoundCode
	}
}

func (h *DocumentSigningConfirmationHandler) buildFrontendRedirect(status, errCode string) string {
	base := h.FrontendHost
	if base == "" {
		return signConfirmSuccessPath
	}
	target, parseErr := url.Parse(base)
	if parseErr != nil {
		return signConfirmSuccessPath
	}
	target.Path = strings.TrimRight(target.Path, "/") + signConfirmSuccessPath
	query := target.Query()
	query.Set(signConfirmStatusQuery, status)
	if errCode != "" {
		query.Set(signConfirmErrorQuery, errCode)
	}
	target.RawQuery = query.Encode()
	return target.String()
}
