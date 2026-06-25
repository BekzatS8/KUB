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
// an approved client-edit or client-delete request.
type feedClientPatcher interface {
	Patch(id int, updates map[string]any, userID, roleID int) (*models.Client, error)
	Delete(id int, userID, roleID int) error
}

// feedLeadUpdater / feedDealUpdater are the minimal interfaces needed to apply
// an approved lead/deal edit or delete request (executed with admin credentials).
type feedLeadUpdater interface {
	Update(lead *models.Leads, userID, roleID int) error
	Delete(id int, userID, roleID int) error
}

type feedDealUpdater interface {
	Update(deal *models.Deals, userID, roleID int) error
	Delete(id int, userID, roleID int) error
}

// feedDocumentCreator applies an approved ОКК/HR document request. On approval
// it runs with admin credentials so branch/scope checks pass:
//   - CreateDocumentFromClient — для pending_create_document
//   - DeleteDocument — для pending_delete_document (HR не может удалять документы
//     напрямую — заявка уходит админу, который удаляет своими правами).
type feedDocumentCreator interface {
	CreateDocumentFromClient(clientID int, clientType string, dealID int, docType string, userID, roleID int, extra map[string]string) (*models.Document, error)
	DeleteDocument(id int64, userID, roleID int) error
}

// feedCreateDocumentPayload is the JSON shape stored for a
// pending_create_document feed event (mirrors createDocumentFromClient).
type feedCreateDocumentPayload struct {
	ClientID   int               `json:"client_id"`
	ClientType string            `json:"client_type"`
	DealID     int               `json:"deal_id"`
	DocType    string            `json:"doc_type"`
	Extra      map[string]string `json:"extra"`
}

type FeedEventService struct {
	repo          *repositories.FeedEventRepository
	userRepo      repositories.UserRepository
	clientPatcher feedClientPatcher
	leadUpdater   feedLeadUpdater
	dealUpdater   feedDealUpdater
	docCreator    feedDocumentCreator
}

func NewFeedEventService(
	repo *repositories.FeedEventRepository,
	userRepo repositories.UserRepository,
	clientPatcher feedClientPatcher,
	leadUpdater feedLeadUpdater,
	dealUpdater feedDealUpdater,
	docCreator feedDocumentCreator,
) *FeedEventService {
	return &FeedEventService{
		repo:          repo,
		userRepo:      userRepo,
		clientPatcher: clientPatcher,
		leadUpdater:   leadUpdater,
		dealUpdater:   dealUpdater,
		docCreator:    docCreator,
	}
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

	case models.FeedEventTypePendingEditLead:
		if s.leadUpdater == nil || e.ResourceID == nil {
			return errors.New("cannot apply lead edit: missing lead updater or resource_id")
		}
		var lead models.Leads
		if err := json.Unmarshal(e.Payload, &lead); err != nil {
			return err
		}
		lead.ID = *e.ResourceID
		// Applied with admin credentials so scope/ownership checks pass.
		return s.leadUpdater.Update(&lead, reviewerID, authz.RoleSystemAdmin)

	case models.FeedEventTypePendingEditDeal:
		if s.dealUpdater == nil || e.ResourceID == nil {
			return errors.New("cannot apply deal edit: missing deal updater or resource_id")
		}
		var deal models.Deals
		if err := json.Unmarshal(e.Payload, &deal); err != nil {
			return err
		}
		deal.ID = *e.ResourceID
		return s.dealUpdater.Update(&deal, reviewerID, authz.RoleSystemAdmin)

	case models.FeedEventTypePendingDeleteClient:
		if s.clientPatcher == nil || e.ResourceID == nil {
			return errors.New("cannot apply client delete: missing client patcher or resource_id")
		}
		// Applied with admin credentials so scope/ownership checks pass.
		return s.clientPatcher.Delete(*e.ResourceID, reviewerID, authz.RoleSystemAdmin)

	case models.FeedEventTypePendingDeleteLead:
		if s.leadUpdater == nil || e.ResourceID == nil {
			return errors.New("cannot apply lead delete: missing lead updater or resource_id")
		}
		return s.leadUpdater.Delete(*e.ResourceID, reviewerID, authz.RoleSystemAdmin)

	case models.FeedEventTypePendingDeleteDeal:
		if s.dealUpdater == nil || e.ResourceID == nil {
			return errors.New("cannot apply deal delete: missing deal updater or resource_id")
		}
		return s.dealUpdater.Delete(*e.ResourceID, reviewerID, authz.RoleSystemAdmin)

	case models.FeedEventTypePendingCreateDocument:
		if s.docCreator == nil {
			return errors.New("cannot apply document create: missing document creator")
		}
		var p feedCreateDocumentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		// Created with admin credentials so branch/scope checks pass.
		_, err := s.docCreator.CreateDocumentFromClient(
			p.ClientID, p.ClientType, p.DealID, p.DocType,
			reviewerID, authz.RoleSystemAdmin, p.Extra,
		)
		return err

	case models.FeedEventTypePendingDeleteDocument:
		if s.docCreator == nil || e.ResourceID == nil {
			return errors.New("cannot apply document delete: missing document service or resource_id")
		}
		// Deleted with admin credentials (CanHardDeleteBusinessEntity).
		return s.docCreator.DeleteDocument(int64(*e.ResourceID), reviewerID, authz.RoleSystemAdmin)

	default:
		// For event types not wired to an apply action (create_lead, create_deal,
		// create_client — not used by the UI), approve simply records the decision.
		return nil
	}
}
