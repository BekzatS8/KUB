package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPublicSigningUIRendersHTMLPage(t *testing.T) {
	w, body := renderSigningUIPage(t, "/sign/email/verify?token=abc123")

	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("unexpected content type: %s", ct)
	}
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
	if !strings.Contains(body, "const verifyURL = `/api/v1/sign/${channel}/verify?token=${encodeURIComponent(token)}&format=json`;") {
		t.Fatalf("expected public verify URL in ui script")
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

func TestPublicSigningUIRendersSMSChannelWithoutDoubleEscaping(t *testing.T) {
	_, body := renderSigningUIPage(t, "/sign/sms/verify?token=tok-123_ABC")

	if !strings.Contains(body, `const initialToken = "tok-123_ABC";`) {
		t.Fatalf("expected raw token JS literal, got line: %s", findScriptLine(body, "const initialToken"))
	}
	if !strings.Contains(body, `const channel = "sms" || 'email';`) {
		t.Fatalf("expected sms channel JS literal, got line: %s", findScriptLine(body, "const channel"))
	}
	assertNoDoubleEscapedJSValue(t, body, "tok-123_ABC")
	assertNoDoubleEscapedJSValue(t, body, "sms")
	if !strings.Contains(body, "const verifyURL = `/api/v1/sign/${channel}/verify?token=${encodeURIComponent(token)}&format=json`;") {
		t.Fatalf("expected public verify fetch URL to use channel interpolation")
	}
}

func TestPublicSigningUIRendersEmailChannelWithoutDoubleEscaping(t *testing.T) {
	_, body := renderSigningUIPage(t, "/sign/email/verify?token=email-token-456")

	if !strings.Contains(body, `const initialToken = "email-token-456";`) {
		t.Fatalf("expected raw token JS literal, got line: %s", findScriptLine(body, "const initialToken"))
	}
	if !strings.Contains(body, `const channel = "email" || 'email';`) {
		t.Fatalf("expected email channel JS literal, got line: %s", findScriptLine(body, "const channel"))
	}
	assertNoDoubleEscapedJSValue(t, body, "email-token-456")
	assertNoDoubleEscapedJSValue(t, body, "email")
	if !strings.Contains(body, "const verifyURL = `/api/v1/sign/${channel}/verify?token=${encodeURIComponent(token)}&format=json`;") {
		t.Fatalf("expected public verify fetch URL to use channel interpolation")
	}
}

func renderSigningUIPage(t *testing.T, target string) (*httptest.ResponseRecorder, string) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	h, err := NewPublicSigningUIHandler()
	if err != nil {
		t.Fatalf("NewPublicSigningUIHandler error: %v", err)
	}
	r := gin.New()
	r.GET("/sign/email/verify", h.ServeEmailVerifyPage)
	r.GET("/sign/sms/verify", h.ServeSMSVerifyPage)

	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: got=%d want=%d", w.Code, http.StatusOK)
	}
	return w, w.Body.String()
}

func assertNoDoubleEscapedJSValue(t *testing.T, body, value string) {
	t.Helper()

	if strings.Contains(body, `\"`+value+`\"`) ||
		strings.Contains(body, `\\"`+value+`\\"`) {
		t.Fatalf("expected no double-escaped JS value %q", value)
	}
}

func findScriptLine(body, needle string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
