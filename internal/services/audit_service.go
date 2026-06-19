package services

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"turcompany/internal/authz"
	"turcompany/internal/repositories"
)

type AuditService struct {
	repo *repositories.AuditRepository
}

func NewAuditService(repo *repositories.AuditRepository) *AuditService {
	return &AuditService{repo: repo}
}

type AuditEvent struct {
	ActorUserID *int
	ActorRoleID int
	Action      string
	EntityType  string
	EntityID    string
	IP          *string
	UserAgent   *string
	Meta        map[string]any
	// IsHidden — если true, запись видна только admin-у и самому актору.
	// Middleware автоматически выставляет true для RolePartner.
	IsHidden bool
}

func (s *AuditService) Log(ctx context.Context, e AuditEvent) {
	if s == nil || s.repo == nil || e.Action == "" {
		return
	}

	metaJSON := "{}"
	if e.Meta != nil {
		if b, err := json.Marshal(e.Meta); err == nil {
			metaJSON = string(b)
		}
	}

	// партнёрские действия автоматически скрыты от внутренних ролей
	hidden := e.IsHidden || e.ActorRoleID == authz.RolePartner

	if err := s.repo.Insert(ctx, e.ActorUserID, e.Action, e.EntityType, e.EntityID, e.IP, e.UserAgent, metaJSON, hidden); err != nil {
		if errors.Is(err, repositories.ErrAuditSchemaMissing) {
			log.Printf("[audit] schema mismatch: audit_logs table is missing; run migrations (action=%s)", e.Action)
			return
		}
		log.Printf("[audit] insert failed: %v (action=%s)", err, e.Action)
	}
}

type FeedEntry struct {
	ID          int64   `json:"id"`
	ActorUserID *int    `json:"actor_user_id"`
	Action      string  `json:"action"`
	EntityType  *string `json:"entity_type"`
	EntityID    *string `json:"entity_id"`
	Meta        string  `json:"meta"`
	IsHidden    bool    `json:"is_hidden"`
	CreatedAt   string  `json:"created_at"`
}

// ListFeed возвращает записи ленты с учётом прав роли.
//
//   - admin видит все записи (включая hidden)
//   - partner видит только свои записи (hidden + non-hidden от себя)
//   - остальные видят только non-hidden
func (s *AuditService) ListFeed(ctx context.Context, userID, roleID, limit, offset int) ([]*FeedEntry, error) {
	if s == nil || s.repo == nil {
		return nil, ErrForbidden
	}

	f := repositories.AuditListFilter{}
	switch roleID {
	case authz.RoleSystemAdmin:
		f.IncludeHidden = true
	case authz.RolePartner:
		// партнёр видит только свои записи (они всегда is_hidden=true)
		f.IncludeHidden = true
		f.ActorUserID = &userID
	default:
		f.IncludeHidden = false
	}

	rows, err := s.repo.List(ctx, limit, offset, f)
	if err != nil {
		if errors.Is(err, repositories.ErrAuditSchemaMissing) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]*FeedEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, &FeedEntry{
			ID:          r.ID,
			ActorUserID: r.ActorUserID,
			Action:      r.Action,
			EntityType:  r.EntityType,
			EntityID:    r.EntityID,
			Meta:        r.Meta,
			IsHidden:    r.IsHidden,
			CreatedAt:   r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return out, nil
}
