package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type PublicDocumentSigningHandler struct {
	Service *services.PublicDocumentSigningService
}

func NewPublicDocumentSigningHandler(service *services.PublicDocumentSigningService) *PublicDocumentSigningHandler {
	return &PublicDocumentSigningHandler{Service: service}
}

func (h *PublicDocumentSigningHandler) GetDocument(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		badRequest(c, "Token required")
		return
	}
	tokenPrefix := h.Service.TokenPrefixForLog(token)
	tokenHashPrefix := h.Service.TokenHashPrefixForLog(token)
	log.Printf("[public-sign][get][request] token_prefix=%s token_hash_prefix=%s ip=%s", tokenPrefix, tokenHashPrefix, c.ClientIP())
	doc, expiresAt, err := h.Service.GetPublicDocument(c.Request.Context(), token)
	if err != nil {
		log.Printf("[public-sign][get][error] token_prefix=%s token_hash_prefix=%s err=%v", tokenPrefix, tokenHashPrefix, err)
		handlePublicDocError(c, err)
		return
	}
	log.Printf("[public-sign][get][success] token_prefix=%s token_hash_prefix=%s doc=%d", tokenPrefix, tokenHashPrefix, doc.ID)
	c.JSON(http.StatusOK, gin.H{"document": doc, "expires_at": expiresAt})
}

func (h *PublicDocumentSigningHandler) SignDocument(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		badRequest(c, "Token required")
		return
	}
	tokenPrefix := h.Service.TokenPrefixForLog(token)
	tokenHashPrefix := h.Service.TokenHashPrefixForLog(token)
	log.Printf("[public-sign][sign][request] token_prefix=%s token_hash_prefix=%s ip=%s", tokenPrefix, tokenHashPrefix, c.ClientIP())
	var payload services.PublicDocumentSignPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		badRequest(c, "Invalid request body")
		return
	}
	signedAt, eventID, docID, err := h.Service.SignPublicDocument(c.Request.Context(), token, payload, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		log.Printf("[public-sign][sign][error] token_prefix=%s token_hash_prefix=%s err=%v", tokenPrefix, tokenHashPrefix, err)
		handlePublicDocError(c, err)
		return
	}
	log.Printf("[public-sign][sign][success] token_prefix=%s token_hash_prefix=%s doc=%d event=%s", tokenPrefix, tokenHashPrefix, docID, eventID)
	c.JSON(http.StatusOK, gin.H{
		"status":          "ok",
		"document_id":     docID,
		"document_status": "signed",
		"signed_at":       signedAt,
		"event_id":        eventID,
	})
}

func handlePublicDocError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrPublicSignInvalidInput), errors.Is(err, services.ErrPublicSignInvalidToken):
		badRequest(c, "Invalid input")
	case errors.Is(err, repositories.ErrPublicLinkNotFound):
		notFound(c, DocumentNotFound, "Not found")
	case errors.Is(err, repositories.ErrPublicLinkExpired):
		gone(c, ExpiredCode, "Expired")
	case errors.Is(err, repositories.ErrPublicLinkUsed):
		conflict(c, ConflictCode, "Already used")
	case errors.Is(err, services.ErrPublicSignInvalidStatus):
		conflict(c, InvalidStatusCode, "Invalid status")
	default:
		internalError(c, "Failed to process request")
	}
}
