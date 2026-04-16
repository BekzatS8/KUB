package handlers

import (
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed templates/signing/email_verify.html
var signingUITemplatesFS embed.FS

type PublicSigningUIHandler struct {
	emailVerifyTemplate *template.Template
}

func NewPublicSigningUIHandler() (*PublicSigningUIHandler, error) {
	tpl, err := template.ParseFS(signingUITemplatesFS, "templates/signing/email_verify.html")
	if err != nil {
		return nil, err
	}
	return &PublicSigningUIHandler{emailVerifyTemplate: tpl}, nil
}

func (h *PublicSigningUIHandler) ServeEmailVerifyPage(c *gin.Context) {
	h.serveVerifyPage(c, "email")
}

func (h *PublicSigningUIHandler) ServeSMSVerifyPage(c *gin.Context) {
	h.serveVerifyPage(c, "sms")
}

func (h *PublicSigningUIHandler) serveVerifyPage(c *gin.Context, channel string) {
	if h == nil || h.emailVerifyTemplate == nil {
		internalError(c, "Service unavailable")
		return
	}
	token := strings.TrimSpace(c.Query("token"))
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Status(http.StatusOK)
	_ = h.emailVerifyTemplate.Execute(c.Writer, map[string]any{
		"Token":   token,
		"Channel": channel,
	})
}
