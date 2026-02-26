package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"turcompany/internal/services"
)

type SignSessionHandler struct {
	Service *services.SignSessionService
}

func NewSignSessionHandler(service *services.SignSessionService) *SignSessionHandler {
	return &SignSessionHandler{Service: service}
}

func (h *SignSessionHandler) Create(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"error": "Sign sessions via phone are deprecated"})
}

func (h *SignSessionHandler) CreateDeprecated(c *gin.Context) {
	var input struct {
		DocumentID int64  `json:"document_id" binding:"required"`
		Phone      string `json:"phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Invalid input")
		return
	}
	userID, roleID := getUserAndRole(c)

	token, signURL, session, err := h.Service.Create(c.Request.Context(), input.DocumentID, input.Phone, userID, roleID)
	if err != nil {
		requestID := requestIDFromContext(c)
		wrapped := fmt.Errorf("create sign session: %w", err)
		log.Printf("[sign][session][create][error] doc=%d phone=%s user=%d role=%d request_id=%s err=%v",
			input.DocumentID, input.Phone, userID, roleID, requestID, wrapped)
		switch {
		case errors.Is(err, services.ErrSignSessionInvalidPhone):
			badRequest(c, "Invalid phone")
		case errors.Is(err, services.ErrSignSessionDocNotFound):
			notFound(c, DocumentNotFound, "Document not found")
		case errors.Is(err, services.ErrSignSessionForbidden):
			forbidden(c, "Forbidden")
		case errors.Is(err, services.ErrSignSessionInvalidStatus):
			conflict(c, InvalidStatusCode, "Invalid status")
		case errors.Is(err, services.ErrSignSessionRateLimited):
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		case errors.Is(err, services.ErrSignSessionBaseURL):
			internalError(c, "Signing configuration error")
		case errors.Is(err, services.ErrSignSessionDelivery):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Signing delivery is unavailable"})
		case errors.Is(err, services.ErrSignDeliveryDisabled):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Signing delivery is disabled"})
		default:
			internalError(c, "Failed to create sign session")
		}
		return
	}

	response := gin.H{
		"status":     "sent",
		"expires_at": session.ExpiresAt,
	}
	if token != "" {
		response["token"] = token
	}
	if signURL != "" {
		response["sign_url"] = signURL
	}
	c.JSON(http.StatusOK, response)
}

func (h *SignSessionHandler) Verify(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"error": "Sign session verification via phone is deprecated"})
	return
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		badRequest(c, "Missing token")
		return
	}
	var input struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Invalid input")
		return
	}
	session, err := h.Service.Verify(c.Request.Context(), token, input.Code, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, services.ErrSignSessionNotFound):
			notFound(c, ValidationFailed, "Session not found")
		case errors.Is(err, services.ErrSignSessionExpired):
			c.JSON(http.StatusGone, gin.H{"error": "Session expired"})
		case errors.Is(err, services.ErrSignSessionAlreadySigned):
			c.JSON(http.StatusConflict, gin.H{"error": "Session already signed"})
		case errors.Is(err, services.ErrSignSessionTooManyTries):
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many attempts"})
		default:
			badRequest(c, "Invalid code")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":      "verified",
		"verified_at": session.VerifiedAt,
	})
}

func (h *SignSessionHandler) Sign(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"error": "Sign session signing via phone is deprecated"})
	return
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		badRequest(c, "Missing token")
		return
	}
	var input struct {
		Agree *bool `json:"agree"`
	}
	if err := c.ShouldBindJSON(&input); err != nil && err.Error() != "EOF" {
		badRequest(c, "Invalid input")
		return
	}
	if input.Agree != nil && !*input.Agree {
		badRequest(c, "Agreement required")
		return
	}
	session, err := h.Service.Sign(c.Request.Context(), token, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		switch {
		case errors.Is(err, services.ErrSignSessionNotFound):
			notFound(c, ValidationFailed, "Session not found")
		case errors.Is(err, services.ErrSignSessionExpired):
			c.JSON(http.StatusGone, gin.H{"error": "Session expired"})
		case errors.Is(err, services.ErrSignSessionAlreadySigned):
			c.JSON(http.StatusConflict, gin.H{"error": "Session already signed"})
		case errors.Is(err, services.ErrSignSessionNotVerified):
			badRequest(c, "Session not verified")
		default:
			internalError(c, "Failed to sign")
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "signed",
		"signed_at": session.SignedAt,
	})
}

func (h *SignSessionHandler) ServeSessionPage(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid session id")
		return
	}
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		badRequest(c, "Missing token")
		return
	}
	if _, err := h.Service.ValidateSessionForPage(c.Request.Context(), sessionID, token); err != nil {
		handleSignSessionTokenError(c, err)
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, signPageHTML(sessionID, token))
}

func (h *SignSessionHandler) SignByID(c *gin.Context) {
	if h.Service == nil {
		internalError(c, "Service unavailable")
		return
	}
	sessionID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid session id")
		return
	}
	var input struct {
		Token    string `json:"token" binding:"required"`
		Agree    *bool  `json:"agree"`
		FullName string `json:"full_name"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Invalid input")
		return
	}
	if input.Agree != nil && !*input.Agree {
		badRequest(c, "Agreement required")
		return
	}
	session, err := h.Service.SignByID(c.Request.Context(), sessionID, input.Token, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		handleSignSessionTokenError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":    "signed",
		"signed_at": session.SignedAt,
	})
}

func handleSignSessionTokenError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrSignSessionNotFound):
		notFound(c, ValidationFailed, "Session not found")
	case errors.Is(err, services.ErrSignSessionExpired):
		c.JSON(http.StatusGone, gin.H{"error": "Session expired"})
	case errors.Is(err, services.ErrSignSessionAlreadySigned):
		c.JSON(http.StatusConflict, gin.H{"error": "Session already signed"})
	case errors.Is(err, services.ErrSignSessionTooManyTries):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "Too many attempts"})
	case errors.Is(err, services.ErrSignSessionInvalidToken):
		badRequest(c, "Invalid token")
	case errors.Is(err, services.ErrSignSessionInvalidStatus):
		conflict(c, InvalidStatusCode, "Invalid status")
	case errors.Is(err, services.ErrSignSessionDocNotFound):
		notFound(c, DocumentNotFound, "Document not found")
	case errors.Is(err, services.ErrPDFCPUMissing):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "PDFCPU_MISSING"})
	case errors.Is(err, services.ErrDocumentChangedAfterOTP):
		c.JSON(http.StatusConflict, gin.H{"error": "DOCUMENT_CHANGED_AFTER_OTP"})
	default:
		internalError(c, "Failed to sign")
	}
}

func (h *SignSessionHandler) ServeSignPage(c *gin.Context) {
	c.String(http.StatusGone, "Sign session page by token is deprecated")
}

func signPageHTML(sessionID int64, token string) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Подписание документа</title>
  <style>
    body { font-family: Arial, sans-serif; background: #f7f7f7; padding: 32px; }
    .card { background: #fff; max-width: 420px; margin: 0 auto; padding: 24px; border-radius: 12px; box-shadow: 0 2px 10px rgba(0,0,0,0.08); }
    label { display: block; margin-bottom: 8px; }
    input { width: 100%%; padding: 10px; margin-bottom: 12px; border: 1px solid #ddd; border-radius: 8px; }
    button { width: 100%%; padding: 12px; border: none; border-radius: 8px; background: #0c66e4; color: #fff; font-weight: bold; cursor: pointer; }
    button:disabled { background: #b6c6e4; cursor: not-allowed; }
    .message { margin-top: 12px; font-size: 14px; }
  </style>
</head>
<body>
  <div class="card">
    <h2>Подписание документа</h2>
    <p>Нажмите кнопку, чтобы подтвердить подпись.</p>
    <button id="submit">Подписать</button>
    <div class="message" id="message"></div>
  </div>
  <script>
    const token = %q;
    const sessionId = %d;
    const message = document.getElementById("message");
    const button = document.getElementById("submit");

    function setMessage(text, isError) {
      message.textContent = text;
      message.style.color = isError ? "#b42318" : "#0e7a0d";
    }

    button.addEventListener("click", async () => {
      button.disabled = true;
      setMessage("Подписываем...", false);
      try {
        const signResp = await fetch("/api/v1/sign/sessions/id/" + sessionId + "/sign", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ token, agree: true })
        });
        if (!signResp.ok) {
          const data = await signResp.json().catch(() => ({}));
          throw new Error(data.error || "Не удалось подписать");
        }
        setMessage("Документ подписан.", false);
      } catch (err) {
        setMessage(err.message || "Ошибка", true);
      } finally {
        button.disabled = false;
      }
    });
  </script>
</body>
</html>`, token, sessionID)
}
