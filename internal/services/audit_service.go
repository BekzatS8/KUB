package services

import (
	"context"
	"encoding/json"
	"log"

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
	Action      string
	EntityType  string
	EntityID    string
	IP          *string
	UserAgent   *string
	Meta        map[string]any
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

	if err := s.repo.Insert(ctx, e.ActorUserID, e.Action, e.EntityType, e.EntityID, e.IP, e.UserAgent, metaJSON); err != nil {
		log.Printf("[audit] insert failed: %v (action=%s)", err, e.Action)
	}
}
