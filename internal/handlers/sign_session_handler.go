package handlers

import (
	"errors"
	"fmt"
	"net/http"
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
		switch {
		case errors.Is(err, services.ErrSignSessionRateLimited):
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
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

func (h *SignSessionHandler) ServeSignPage(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if token == "" {
		c.String(http.StatusBadRequest, "Missing token")
		return
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.String(http.StatusOK, signPageHTML(token))
}

func signPageHTML(token string) string {
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
    <p>Введите код из сообщения WhatsApp, чтобы подтвердить подпись.</p>
    <label for="code">Код подтверждения</label>
    <input id="code" type="text" placeholder="123456" />
    <button id="submit">Подписать</button>
    <div class="message" id="message"></div>
  </div>
  <script>
    const token = %q;
    const message = document.getElementById("message");
    const button = document.getElementById("submit");
    const codeInput = document.getElementById("code");

    function setMessage(text, isError) {
      message.textContent = text;
      message.style.color = isError ? "#b42318" : "#0e7a0d";
    }

    button.addEventListener("click", async () => {
      const code = codeInput.value.trim();
      if (!code) {
        setMessage("Введите код.", true);
        return;
      }
      button.disabled = true;
      setMessage("Проверяем код...", false);
      try {
        const verifyResp = await fetch("/api/v1/sign/sessions/" + token + "/verify", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ code })
        });
        if (!verifyResp.ok) {
          const data = await verifyResp.json().catch(() => ({}));
          throw new Error(data.error || "Не удалось проверить код");
        }
        setMessage("Код подтвержден. Подписываем...", false);
        const signResp = await fetch("/api/v1/sign/sessions/" + token + "/sign", {
          method: "POST"
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
</html>`, token)
}
