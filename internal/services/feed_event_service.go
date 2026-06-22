package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

var (
	ErrFeedEventNotFound        = errors.New("feed event not found")
	ErrFeedEventAlreadyResolved = errors.New("feed event already resolved")
)

// feedClientPatcher is the minimal interface the service needs to apply
// an approved client-edit payload.
type feedClientPatcher interface {
	Patch(id int, updates map[string]any, userID, roleID int) (*models.Client, error)
}

type FeedEventService struct {
	repo          *repositories.FeedEventRepository
	userRepo      repositories.UserRepository
	clientPatcher feedClientPatcher
}

func NewFeedEventService(
	repo *repositories.FeedEventRepository,
	userRepo repositories.UserRepository,
	clientPatcher feedClientPatcher,
) *FeedEventService {
	return &FeedEventService{repo: repo, userRepo: userRepo, clientPatcher: clientPatcher}
}

// Create stores a new pending feed event. The requester's display name is
// resolved from the user repo and stored for display without extra joins.
func (s *FeedEventService) Create(
	ctx context.Context,
	requesterID int,
	eventType string,
	payload json.RawMessage,
	resourceID *int,
) (*models.FeedEvent, error) {
	name := ""
	if s.userRepo != nil {
		if u, err := s.userRepo.GetByID(requesterID); err == nil && u != nil {
			name = strings.TrimSpace(u.FirstName + " " + u.LastName)
		}
	}

	e := &models.FeedEvent{
		EventType:     eventType,
		RequesterID:   requesterID,
		RequesterName: name,
		Payload:       payload,
		ResourceID:    resourceID,
	}
	if err := s.repo.Create(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

// List returns feed events visible to the caller:
//   - admin sees everything
//   - other roles see only their own events
func (s *FeedEventService) List(
	ctx context.Context,
	callerID, callerRoleID int,
	status string,
	limit, offset int,
) ([]*models.FeedEvent, error) {
	var requesterFilter *int
	if callerRoleID != authz.RoleSystemAdmin {
		requesterFilter = &callerID
	}
	events, err := s.repo.List(ctx, requesterFilter, status, limit, offset)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []*models.FeedEvent{}, nil
		}
		return nil, err
	}
	if events == nil {
		return []*models.FeedEvent{}, nil
	}
	return events, nil
}

// Approve marks the event as approved and, for client-edit events, applies
// the payload as a PATCH on the target client using admin credentials.
func (s *FeedEventService) Approve(ctx context.Context, eventID, reviewerID int) (*models.FeedEvent, error) {
	e, err := s.repo.GetByID(ctx, eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrFeedEventNotFound
		}
		return nil, err
	}
	if e.Status != models.FeedEventStatusPending {
		return nil, ErrFeedEventAlreadyResolved
	}

	// Apply the action before marking approved, so failures roll back the status.
	if err := s.applyEvent(ctx, e, reviewerID); err != nil {
		return nil, err
	}

	if err := s.repo.UpdateStatus(ctx, eventID, models.FeedEventStatusApproved, reviewerID, nil); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, eventID)
}

// Reject marks the event as rejected with an optional reason.
func (s *FeedEventService) Reject(ctx context.Context, eventID, reviewerID int, reason string) (*models.FeedEvent, error) {
	e, err := s.repo.GetByID(ctx, eventID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrFeedEventNotFound
		}
		return nil, err
	}
	if e.Status != models.FeedEventStatusPending {
		return nil, ErrFeedEventAlreadyResolved
	}

	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	if err := s.repo.UpdateStatus(ctx, eventID, models.FeedEventStatusRejected, reviewerID, reasonPtr); err != nil {
		return nil, err
	}
	return s.repo.GetByID(ctx, eventID)
}

// applyEvent executes the pending action: for pending_edit_client it patches
// the client record using the stored JSON payload.
func (s *FeedEventService) applyEvent(ctx context.Context, e *models.FeedEvent, reviewerID int) error {
	switch e.EventType {
	case models.FeedEventTypePendingEditClient:
		if s.clientPatcher == nil || e.ResourceID == nil {
			return errors.New("cannot apply client edit: missing client patcher or resource_id")
		}
		var updates map[string]any
		if err := json.Unmarshal(e.Payload, &updates); err != nil {
			return err
		}
		_, err := s.clientPatcher.Patch(*e.ResourceID, updates, reviewerID, authz.RoleSystemAdmin)
		return err

	default:
		// For event types not yet wired to an apply action (create_lead, etc.),
		// approve is a no-op that simply records the admin decision.
		return nil
	}
}
