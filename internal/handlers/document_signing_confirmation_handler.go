package handlers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/services"
)

const (
	SignConfirmExpiredCode     = "EXPIRED"
	SignConfirmTooManyAttempts = "TOO_MANY_ATTEMPTS"
	SignConfirmInvalidCode     = "INVALID_CODE"
	SignConfirmNotFoundCode    = "NOT_FOUND"
	SignConfirmAlreadyUsedCode = "ALREADY_USED"
)

type DocumentSigningConfirmationHandler struct {
	Service            *services.DocumentSigningConfirmationService
	DocumentSvc        *services.DocumentService
	SignSessionService *services.SignSessionService
}

func NewDocumentSigningConfirmationHandler(
	service *services.DocumentSigningConfirmationService,
	documentSvc *services.DocumentService,
	signSessionService *services.SignSessionService,
) *DocumentSigningConfirmationHandler {
	return &DocumentSigningConfirmationHandler{
		Service:            service,
		DocumentSvc:        documentSvc,
		SignSessionService: signSessionService,
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
	var body struct {
		Email          string `json:"email"`
		SignerFullName string `json:"signer_full_name"`
		SignerPosition string `json:"signer_position"`
		SignerPhone    string `json:"signer_phone"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		badRequest(c, "Invalid request body")
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

	signer, err := h.DocumentSvc.ResolveSigner(documentID, userID, roleID, services.SignerOverrides{
		Email:    body.Email,
		FullName: body.SignerFullName,
		Position: body.SignerPosition,
		Phone:    body.SignerPhone,
	})
	if err != nil {
		switch err.Error() {
		case "forbidden":
			forbidden(c, "Forbidden")
			return
		case "not found":
			notFound(c, DocumentNotFound, "Document not found")
			return
		default:
			badRequest(c, err.Error())
			return
		}
	}
	result, err := h.Service.StartSigning(c.Request.Context(), documentID, int64(userID), signer.Email)
	if err != nil {
		requestID := requestIDFromContext(c)
		wrapped := fmt.Errorf("start signing: %w", err)
		log.Printf("[sign][confirm][start][error] doc=%d email=%s user=%d role=%d request_id=%s err=%v",
			documentID, signer.Email, userID, roleID, requestID, wrapped)
		handleSignConfirmError(c, err)
		return
	}
	expiresAt := result.Channels[0].ExpiresAt
	c.JSON(http.StatusOK, gin.H{
		"status":     "pending",
		"expires_at": expiresAt,
	})
}

type emailConfirmRequest struct {
	Token string `json:"token" binding:"required"`
	Code  string `json:"code" binding:"required"`
}

func (h *DocumentSigningConfirmationHandler) ConfirmByEmailCode(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	requestID := requestIDFromContext(c)
	ip := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	userID, roleID := getUserAndRole(c)
	documentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		log.Printf("[sign][confirm][email][request][invalid_doc_id] raw_id=%q request_id=%s user=%d role=%d ip=%s ua=%q",
			c.Param("id"), requestID, userID, roleID, ip, userAgent)
		badRequest(c, "Invalid id")
		return
	}
	var body emailConfirmRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		log.Printf("[sign][confirm][email][request][invalid_body] doc=%d request_id=%s user=%d role=%d ip=%s ua=%q err=%v",
			documentID, requestID, userID, roleID, ip, userAgent, fmt.Errorf("bind json: %w", err))
		badRequest(c, "Invalid request body")
		return
	}
	token := services.NormalizeEmailConfirmTokenForLog(body.Token)
	tokenPrefix := redactPrefix(token, 8)
	tokenHashPrefix := redactPrefix(services.HashEmailConfirmTokenForLog(token, h.Service.TokenPepperForLog()), 8)
	codeLen := len(strings.TrimSpace(body.Code))
	log.Printf("[sign][confirm][email][request] doc=%d token_prefix=%s token_hash_prefix=%s code_len=%d request_id=%s user=%d role=%d ip=%s ua=%q",
		documentID, tokenPrefix, tokenHashPrefix, codeLen, requestID, userID, roleID, ip, userAgent)

	status, signerEmail, docHash, confirmation, err := h.Service.ConfirmByEmailToken(
		c.Request.Context(),
		documentID,
		body.Token,
		body.Code,
		ip,
		userAgent,
	)
	if err != nil {
		attempts := -1
		expiresAt := time.Time{}
		loggedDocID := documentID
		if confirmation != nil {
			loggedDocID = confirmation.DocumentID
			attempts = confirmation.Attempts
			expiresAt = confirmation.ExpiresAt
		}
		wrapped := fmt.Errorf("confirm signing by email token: %w", err)
		log.Printf("[sign][confirm][email][error] doc=%d token_prefix=%s token_hash_prefix=%s code_len=%d attempts=%d expires_at=%s user=%d role=%d request_id=%s ip=%s ua=%q err=%v",
			loggedDocID, tokenPrefix, tokenHashPrefix, codeLen, attempts, expiresAt.UTC().Format(time.RFC3339Nano), userID, roleID, requestID, ip, userAgent, wrapped)
		handleSignConfirmError(c, err)
		return
	}
	if h.SignSessionService == nil {
		internalError(c, "Service unavailable")
		return
	}
	token, session, err := h.SignSessionService.CreateEmailSession(c.Request.Context(), documentID, signerEmail, docHash)
	if err != nil {
		log.Printf("[sign][confirm][email][session_create][error] doc=%d token_prefix=%s token_hash_prefix=%s request_id=%s user=%d role=%d ip=%s ua=%q err=%v",
			documentID, tokenPrefix, tokenHashPrefix, requestID, userID, roleID, ip, userAgent, fmt.Errorf("create email session: %w", err))
		handleSignSessionCreateError(c, err)
		return
	}
	signURL := buildSignSessionURL(c, session.ID, token)
	log.Printf("[sign][confirm][email][success] doc=%d token_prefix=%s token_hash_prefix=%s status=%s session_id=%d request_id=%s user=%d role=%d ip=%s ua=%q",
		documentID, tokenPrefix, tokenHashPrefix, status, session.ID, requestID, userID, roleID, ip, userAgent)
	c.JSON(http.StatusOK, gin.H{
		"status":        status,
		"email_token":   services.NormalizeEmailConfirmTokenForLog(body.Token),
		"session_id":    session.ID,
		"session_token": token,
		"sign_url":      signURL,
		"expires_at":    session.ExpiresAt,
	})
}

func buildSignSessionURL(c *gin.Context, sessionID int64, token string) string {
	if c == nil {
		return ""
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwarded != "" {
		scheme = forwarded
	}
	host := strings.TrimSpace(c.Request.Host)
	if host == "" {
		return ""
	}
	queryToken := url.QueryEscape(token)
	return fmt.Sprintf("%s://%s/api/v1/sign/sessions/id/%d/page?token=%s", scheme, host, sessionID, queryToken)
}

func handleSignSessionCreateError(c *gin.Context, err error) {
	if err == nil {
		internalError(c, "Failed to create sign session")
		return
	}
	switch {
	case errors.Is(err, services.ErrSignSessionInvalidEmail):
		badRequest(c, "Invalid email")
	case errors.Is(err, services.ErrSignSessionAlreadySigned):
		conflict(c, ValidationFailed, "Document already signed by this email")
	case errors.Is(err, services.ErrSignSessionInvalidStatus):
		conflict(c, InvalidStatusCode, "Invalid status")
	case errors.Is(err, services.ErrSignSessionDocNotFound):
		notFound(c, DocumentNotFound, "Document not found")
	default:
		wrapped := fmt.Errorf("create sign session: %w", err)
		log.Printf("[sign][session][create][error] err=%v", wrapped)
		internalError(c, "Failed to create sign session")
	}
}

func (h *DocumentSigningConfirmationHandler) VerifyEmailToken(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	requestID := requestIDFromContext(c)
	ip := c.ClientIP()
	userAgent := c.GetHeader("User-Agent")
	userID, roleID := getUserAndRole(c)
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		log.Printf("[sign][confirm][email][validate][request][missing_token] request_id=%s user=%d role=%d ip=%s ua=%q",
			requestID, userID, roleID, ip, userAgent)
		badRequest(c, "Token required")
		return
	}
	normalized := services.NormalizeEmailConfirmTokenForLog(token)
	tokenPrefix := redactPrefix(normalized, 8)
	tokenHashPrefix := redactPrefix(services.HashEmailConfirmTokenForLog(normalized, h.Service.TokenPepperForLog()), 8)
	log.Printf("[sign][confirm][email][validate][request] token_prefix=%s token_hash_prefix=%s request_id=%s user=%d role=%d ip=%s ua=%q",
		tokenPrefix, tokenHashPrefix, requestID, userID, roleID, ip, userAgent)

	payload, err := h.Service.ValidateEmailToken(c.Request.Context(), token, ip, userAgent)
	if err != nil {
		confirmation, lookupErr := h.Service.LookupEmailConfirmationByToken(c.Request.Context(), token)
		if lookupErr != nil {
			log.Printf("[sign][confirm][email][validate][lookup][error] token_prefix=%s request_id=%s err=%v",
				tokenPrefix, requestID, fmt.Errorf("lookup email confirmation by token: %w", lookupErr))
		}
		attempts := -1
		expiresAt := time.Time{}
		docID := int64(0)
		if confirmation != nil {
			docID = confirmation.DocumentID
			attempts = confirmation.Attempts
			expiresAt = confirmation.ExpiresAt
		}
		wrapped := fmt.Errorf("validate signing email token: %w", err)
		log.Printf("[sign][confirm][email][validate][error] doc=%d token_prefix=%s token_hash_prefix=%s code_len=%d attempts=%d expires_at=%s user=%d role=%d request_id=%s ip=%s ua=%q err=%v",
			docID, tokenPrefix, tokenHashPrefix, 0, attempts, expiresAt.UTC().Format(time.RFC3339Nano), userID, roleID, requestID, ip, userAgent, wrapped)
		handleSignConfirmError(c, err)
		return
	}
	log.Printf("[sign][confirm][email][validate][success] doc=%d token_prefix=%s token_hash_prefix=%s status=%s request_id=%s user=%d role=%d ip=%s ua=%q",
		payload.Document.ID, tokenPrefix, tokenHashPrefix, payload.Document.Status, requestID, userID, roleID, ip, userAgent)
	c.JSON(http.StatusOK, payload)
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
	var emailStatus *services.SigningChannelStatus
	for _, channel := range channels {
		if channel.Channel == "email" {
			copy := channel
			emailStatus = &copy
			break
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"document": gin.H{
			"id":     doc.ID,
			"status": doc.Status,
		},
		"status":      statusOrDefault(emailStatus, "expired"),
		"expires_at":  expiresAtOrZero(emailStatus),
		"approved_at": approvedAtOrNil(emailStatus),
		"channels":    channels,
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
		writeError(c, http.StatusGone, SignConfirmExpiredCode, "Expired")
	case errors.Is(err, services.ErrSignConfirmTooManyTries):
		writeError(c, http.StatusTooManyRequests, SignConfirmTooManyAttempts, "Too many attempts")
	case errors.Is(err, services.ErrSignConfirmAlreadyUsed):
		writeError(c, http.StatusConflict, SignConfirmAlreadyUsedCode, "Already used")
	case errors.Is(err, services.ErrSignConfirmInvalidCode):
		writeError(c, http.StatusBadRequest, SignConfirmInvalidCode, "Invalid code")
	case errors.Is(err, services.ErrSignConfirmInvalidToken):
		writeError(c, http.StatusNotFound, SignConfirmNotFoundCode, "Not found")
	case errors.Is(err, services.ErrSignConfirmNotFound):
		writeError(c, http.StatusNotFound, SignConfirmNotFoundCode, "Not found")
	default:
		wrapped := fmt.Errorf("confirm signing: %w", err)
		log.Printf("[sign][confirm][error] err=%v", wrapped)
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

func statusOrDefault(status *services.SigningChannelStatus, fallback string) string {
	if status == nil || status.Status == "" {
		return fallback
	}
	return status.Status
}

func expiresAtOrZero(status *services.SigningChannelStatus) time.Time {
	if status == nil {
		return time.Time{}
	}
	return status.ExpiresAt
}

func approvedAtOrNil(status *services.SigningChannelStatus) *time.Time {
	if status == nil {
		return nil
	}
	return status.ApprovedAt
}
