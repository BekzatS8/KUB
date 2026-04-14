package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublicSigningUIRendersHTMLPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, err := NewPublicSigningUIHandler()
	if err != nil {
		t.Fatalf("NewPublicSigningUIHandler error: %v", err)
	}
	r := gin.New()
	r.GET("/sign/email/verify", h.ServeEmailVerifyPage)

	req := httptest.NewRequest(http.MethodGet, "/sign/email/verify?token=abc123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("unexpected content type: %s", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Подписание документа") {
		t.Fatalf("expected signing page title in body")
	}
	if strings.Contains(body, `"require_post_confirm"`) {
		t.Fatalf("expected no raw JSON in browser page")
	}
	if !strings.Contains(body, "Ссылка истекла. Попросите отправить новую ссылку.") {
		t.Fatalf("expected explicit expired-link message in ui script")
	}
	if !strings.Contains(body, "Ссылка недействительна или документ недоступен.") {
		t.Fatalf("expected explicit invalid-link message in ui script")
	}
	if !strings.Contains(body, "const verifyURL = `/api/v1/sign/email/verify?token=${encodeURIComponent(token)}&format=json`;") {
		t.Fatalf("expected absolute verify URL in ui script")
	}
	if !strings.Contains(body, "payload.document?.preview_url || ''") {
		t.Fatalf("expected preview_url usage in verify flow")
	}
	if !strings.Contains(body, "agreementCheckbox") {
		t.Fatalf("expected agreement checkbox block in embedded ui")
	}
	if !strings.Contains(body, "agree_terms: isAgreed") ||
		!strings.Contains(body, "confirm_document_read: isAgreed") ||
		!strings.Contains(body, "agreement_text_version: agreement?.version || ''") ||
		!strings.Contains(body, "document_hash_from_client: documentHashPreview") {
		t.Fatalf("expected full confirm payload fields in ui")
	}
	if strings.Contains(body, "body: JSON.stringify({ token, code })") {
		t.Fatalf("expected old shortened confirm payload to be removed")
	}
	if strings.Contains(body, "Предпросмотр файла в публичном режиме недоступен") {
		t.Fatalf("expected old permanent preview-unavailable text to be removed")
	}
	if strings.Contains(body, "${apiBase}/sign/email/verify") {
		t.Fatalf("expected no apiBase-based relative verify URL in ui script")
	}
}
