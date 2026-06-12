package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

// SignHistoryEvent is one normalized entry in a document's signing timeline.
type SignHistoryEvent struct {
	Type      string    `json:"type"`
	At        time.Time `json:"at"`
	Channel   string    `json:"channel,omitempty"`
	By        string    `json:"by,omitempty"`
	Method    string    `json:"method,omitempty"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	Status    string    `json:"status,omitempty"`
}

// DocumentSignHistoryHandler provides read-only access to a document's signing history.
type DocumentSignHistoryHandler struct {
	DocSvc        *services.DocumentService
	Sessions      *repositories.SignSessionRepository
	Confirmations *repositories.SignatureConfirmationRepository
}

func NewDocumentSignHistoryHandler(
	docSvc *services.DocumentService,
	sessions *repositories.SignSessionRepository,
	confirmations *repositories.SignatureConfirmationRepository,
) *DocumentSignHistoryHandler {
	return &DocumentSignHistoryHandler{
		DocSvc:        docSvc,
		Sessions:      sessions,
		Confirmations: confirmations,
	}
}

// GetSignHistory assembles a chronological timeline of signing events for a document.
// Access is gated by the same document-level check as GET /documents/:id.
func (h *DocumentSignHistoryHandler) GetSignHistory(c *gin.Context) {
	docID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		badRequest(c, "Invalid id")
		return
	}
	userID, roleID := getUserAndRole(c)
	doc, err := h.DocSvc.GetDocument(docID, userID, roleID)
	if err != nil {
		switch err.Error() {
		case "forbidden":
			forbidden(c, "Forbidden")
		default:
			internalError(c, "Failed to fetch document")
		}
		return
	}
	if doc == nil {
		notFound(c, "DOCUMENT_NOT_FOUND", "Document not found")
		return
	}

	ctx := c.Request.Context()
	var events []SignHistoryEvent

	// Events from sign_sessions
	sessions, err := h.Sessions.ListByDocumentID(ctx, docID)
	if err != nil {
		internalError(c, "Failed to load sign sessions")
		return
	}
	for _, s := range sessions {
		channel := ""
		switch {
		case s.PhoneE164 != "":
			channel = "sms"
		case s.SignerEmail != "":
			channel = "email"
		}
		events = append(events, SignHistoryEvent{
			Type:    "session_created",
			At:      s.CreatedAt,
			Channel: channel,
			Status:  s.Status,
		})
		if s.VerifiedAt != nil {
			events = append(events, SignHistoryEvent{
				Type:   "session_verified",
				At:     *s.VerifiedAt,
				Status: "verified",
			})
		}
		if s.SignedAt != nil {
			events = append(events, SignHistoryEvent{
				Type:      "session_signed",
				At:        *s.SignedAt,
				IP:        s.SignedIP,
				UserAgent: s.SignedUserAgent,
				Status:    "signed",
			})
		}
	}

	// Events from signature_confirmations (only definitive terminal timestamps)
	confs, err := h.Confirmations.ListByDocumentID(ctx, docID)
	if err != nil {
		internalError(c, "Failed to load confirmations")
		return
	}
	for _, conf := range confs {
		events = append(events, SignHistoryEvent{
			Type:    "confirmation_sent",
			At:      conf.CreatedAt,
			Channel: conf.Channel,
			Status:  conf.Status,
		})
		if conf.ApprovedAt != nil {
			events = append(events, SignHistoryEvent{
				Type:    "confirmation_approved",
				At:      *conf.ApprovedAt,
				Channel: conf.Channel,
				Status:  "approved",
			})
		}
		if conf.RejectedAt != nil {
			events = append(events, SignHistoryEvent{
				Type:    "confirmation_rejected",
				At:      *conf.RejectedAt,
				Channel: conf.Channel,
				Status:  "rejected",
			})
		}
	}

	// Terminal event from the document itself (authoritative signed state)
	if doc.SignedAt != nil {
		events = append(events, SignHistoryEvent{
			Type:      "document_signed",
			At:        *doc.SignedAt,
			By:        doc.SignedBy,
			Method:    doc.SignMethod,
			IP:        doc.SignIP,
			UserAgent: doc.SignUserAgent,
			Status:    "signed",
		})
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].At.Before(events[j].At)
	})
	if events == nil {
		events = []SignHistoryEvent{}
	}

	c.JSON(http.StatusOK, gin.H{
		"document": gin.H{
			"id":     doc.ID,
			"status": doc.Status,
		},
		"events": events,
	})
}

