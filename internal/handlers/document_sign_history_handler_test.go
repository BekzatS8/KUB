package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
)

// buildSignHistoryRouter returns a gin router wired with the sign-history logic
// backed entirely by in-memory data — no DB required.
func buildSignHistoryRouter(
	doc *models.Document,
	docErr error,
	sessions []*models.SignSession,
	confs []*models.SignatureConfirmation,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 1)
		c.Set("role_id", 90) // legal — sees all documents
		c.Next()
	})

	r.GET("/documents/:id/sign/history", func(c *gin.Context) {
		if docErr != nil {
			if docErr.Error() == "forbidden" {
				c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
			return
		}
		if doc == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		var events []SignHistoryEvent

		for _, s := range sessions {
			channel := ""
			switch {
			case s.PhoneE164 != "":
				channel = "sms"
			case s.SignerEmail != "":
				channel = "email"
			}
			events = append(events, SignHistoryEvent{Type: "session_created", At: s.CreatedAt, Channel: channel, Status: s.Status})
			if s.VerifiedAt != nil {
				events = append(events, SignHistoryEvent{Type: "session_verified", At: *s.VerifiedAt, Status: "verified"})
			}
			if s.SignedAt != nil {
				events = append(events, SignHistoryEvent{Type: "session_signed", At: *s.SignedAt, IP: s.SignedIP, UserAgent: s.SignedUserAgent, Status: "signed"})
			}
		}
		for _, conf := range confs {
			events = append(events, SignHistoryEvent{Type: "confirmation_sent", At: conf.CreatedAt, Channel: conf.Channel, Status: conf.Status})
			if conf.ApprovedAt != nil {
				events = append(events, SignHistoryEvent{Type: "confirmation_approved", At: *conf.ApprovedAt, Channel: conf.Channel, Status: "approved"})
			}
			if conf.RejectedAt != nil {
				events = append(events, SignHistoryEvent{Type: "confirmation_rejected", At: *conf.RejectedAt, Channel: conf.Channel, Status: "rejected"})
			}
		}
		if doc.SignedAt != nil {
			events = append(events, SignHistoryEvent{Type: "document_signed", At: *doc.SignedAt, By: doc.SignedBy, Method: doc.SignMethod, IP: doc.SignIP, UserAgent: doc.SignUserAgent, Status: "signed"})
		}

		sort.Slice(events, func(i, j int) bool { return events[i].At.Before(events[j].At) })
		if events == nil {
			events = []SignHistoryEvent{}
		}
		c.JSON(http.StatusOK, gin.H{
			"document": gin.H{"id": doc.ID, "status": doc.Status},
			"events":   events,
		})
	})
	return r
}

func TestSignHistory_EmptyWhenNoSigningData(t *testing.T) {
	doc := &models.Document{ID: 10, Status: "draft"}
	r := buildSignHistoryRouter(doc, nil, nil, nil)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/documents/10/sign/history", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	evts := body["events"].([]any)
	if len(evts) != 0 {
		t.Fatalf("expected 0 events for unsigned doc, got %d", len(evts))
	}
}

func TestSignHistory_SortedChronologically(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	t1 := t0.Add(2 * time.Minute)
	t2 := t0.Add(5 * time.Minute)
	t3 := t0.Add(10 * time.Minute)
	t4 := t0.Add(15 * time.Minute)

	signedAt := t3
	doc := &models.Document{
		ID:            20,
		Status:        "signed",
		SignedAt:      &t4,
		SignedBy:      "Иван Иванов",
		SignMethod:    "otp",
		SignIP:        "1.2.3.4",
		SignUserAgent: "Mozilla/5.0",
	}
	approvedAt := t2
	sessions := []*models.SignSession{
		{ID: 1, DocumentID: 20, SignerEmail: "ivan@example.com", Status: "signed", CreatedAt: t0, SignedAt: &signedAt, SignedIP: "1.2.3.4"},
	}
	confs := []*models.SignatureConfirmation{
		{ID: "c1", DocumentID: 20, Channel: "email", Status: "approved", CreatedAt: t1, ExpiresAt: t0.Add(30 * time.Minute), ApprovedAt: &approvedAt},
	}

	r := buildSignHistoryRouter(doc, nil, sessions, confs)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/documents/20/sign/history", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	evts := body["events"].([]any)
	// session_created(t0), confirmation_sent(t1), confirmation_approved(t2), session_signed(t3), document_signed(t4)
	if len(evts) != 5 {
		t.Fatalf("expected 5 events, got %d: %v", len(evts), evts)
	}
	expected := []string{"session_created", "confirmation_sent", "confirmation_approved", "session_signed", "document_signed"}
	for i, want := range expected {
		got := evts[i].(map[string]any)["type"].(string)
		if got != want {
			t.Errorf("event[%d]: want type %q, got %q", i, want, got)
		}
	}
}

func TestSignHistory_ForbiddenDoc(t *testing.T) {
	r := buildSignHistoryRouter(nil, errors.New("forbidden"), nil, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/documents/99/sign/history", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSignHistory_DocumentSignedFields(t *testing.T) {
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	doc := &models.Document{
		ID:            30,
		Status:        "signed",
		SignedAt:      &t0,
		SignedBy:      "Алим",
		SignMethod:    "otp",
		SignIP:        "10.0.0.1",
		SignUserAgent: "Chrome/100",
	}
	r := buildSignHistoryRouter(doc, nil, nil, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/documents/30/sign/history", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	evts := body["events"].([]any)
	if len(evts) != 1 {
		t.Fatalf("expected 1 event (document_signed), got %d", len(evts))
	}
	e := evts[0].(map[string]any)
	if e["type"] != "document_signed" {
		t.Errorf("expected document_signed, got %v", e["type"])
	}
	if e["by"] != "Алим" {
		t.Errorf("expected by=Алим, got %v", e["by"])
	}
	if e["ip"] != "10.0.0.1" {
		t.Errorf("expected ip=10.0.0.1, got %v", e["ip"])
	}
}
