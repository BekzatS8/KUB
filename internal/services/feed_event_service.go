package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	// Review применяет ревью документа (approve/return) — используется одобрением
	// feed-события pending_review_document правами администратора.
	Review(id int64, action string, userID, roleID int) error
	// GetDocument нужен, чтобы отличить «документа больше нет» и «документ уже
	// проверен» от настоящей ошибки применения заявки.
	GetDocument(id int64, userID, roleID int) (*models.Document, error)
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

// feedDocumentSender применяет одобренную отправку документа на подпись
// (pending_send_document): на approve документ реально уходит клиенту правами
// администратора.
type feedDocumentSender interface {
	StartSigningForDocument(ctx context.Context, docID int64, channel, manualPhone, manualEmail, signerFullName, signerPosition string, userID, roleID int) error
}

// feedSendDocumentPayload — JSON отправки на подпись, отложенной на одобрение.
type feedSendDocumentPayload struct {
	DocumentID     int64  `json:"document_id"`
	Channel        string `json:"channel"`
	ManualPhone    string `json:"manual_phone"`
	ManualEmail    string `json:"manual_email"`
	SignerFullName string `json:"signer_full_name"`
	SignerPosition string `json:"signer_position"`
}

type FeedEventService struct {
	repo          *repositories.FeedEventRepository
	userRepo      repositories.UserRepository
	clientPatcher feedClientPatcher
	leadUpdater   feedLeadUpdater
	dealUpdater   feedDealUpdater
	docCreator    feedDocumentCreator
	docSender     feedDocumentSender
}

func NewFeedEventService(
	repo *repositories.FeedEventRepository,
	userRepo repositories.UserRepository,
	clientPatcher feedClientPatcher,
	leadUpdater feedLeadUpdater,
	dealUpdater feedDealUpdater,
	docCreator feedDocumentCreator,
	docSender feedDocumentSender,
) *FeedEventService {
	return &FeedEventService{
		repo:          repo,
		userRepo:      userRepo,
		clientPatcher: clientPatcher,
		leadUpdater:   leadUpdater,
		dealUpdater:   dealUpdater,
		docCreator:    docCreator,
		docSender:     docSender,
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
	// Админ и руководство видят все заявки (для одобрения); остальные — только свои.
	var requesterFilter *int
	if callerRoleID != authz.RoleSystemAdmin && callerRoleID != authz.RoleManagement {
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
		resourceID := 0
		if e.ResourceID != nil {
			resourceID = *e.ResourceID
		}
		log.Printf("[feed][approve][failed] event_id=%d type=%s resource_id=%d requester_id=%d reviewer_id=%d err=%v",
			e.ID, e.EventType, resourceID, e.RequesterID, reviewerID, err)
		return nil, fmt.Errorf("%s: %w", feedEventActionLabel(e.EventType), translateFeedApplyError(err))
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

	case models.FeedEventTypePendingReviewDocument:
		if s.docCreator == nil || e.ResourceID == nil {
			return errors.New("cannot apply document review: missing document service or resource_id")
		}
		docID := int64(*e.ResourceID)
		// Ревью-approve правами администратора (under_review → approved).
		err := s.docCreator.Review(docID, "approve", reviewerID, authz.RoleSystemAdmin)
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "invalid status") {
			// Админ мог утвердить документ напрямую в карточке — заявка при этом
			// осталась висеть в Ленте и потом падала с «invalid status».
			// Считаем такую заявку уже выполненной, а не ошибочной.
			if doc, gerr := s.docCreator.GetDocument(docID, reviewerID, authz.RoleSystemAdmin); gerr == nil && doc != nil && doc.Status != "draft" && doc.Status != "under_review" {
				log.Printf("[feed][approve][already_applied] event_id=%d type=%s document_id=%d status=%s",
					e.ID, e.EventType, docID, doc.Status)
				return nil
			}
		}
		return err

	case models.FeedEventTypePendingSendDocument:
		if s.docSender == nil {
			return errors.New("cannot apply document send: missing document sender")
		}
		var p feedSendDocumentPayload
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.DocumentID == 0 && e.ResourceID != nil {
			p.DocumentID = int64(*e.ResourceID)
		}
		if p.DocumentID == 0 {
			return errors.New("cannot apply document send: empty document_id in payload")
		}
		if strings.TrimSpace(p.Channel) == "" {
			return errors.New("cannot apply document send: empty channel in payload")
		}
		// Отправка выполняется правами администратора, одобрившего заявку.
		return s.docSender.StartSigningForDocument(
			ctx, p.DocumentID, p.Channel, p.ManualPhone, p.ManualEmail,
			p.SignerFullName, p.SignerPosition, reviewerID, authz.RoleSystemAdmin,
		)

	default:
		// For event types not wired to an apply action (create_lead, create_deal,
		// create_client — not used by the UI), approve simply records the decision.
		return nil
	}
}

// feedEventActionLabel — человекочитаемое название действия для сообщения об
// ошибке одобрения: админ должен видеть, ЧТО именно не удалось применить.
func feedEventActionLabel(eventType string) string {
	switch eventType {
	case models.FeedEventTypePendingSendDocument:
		return "отправка документа на подпись"
	case models.FeedEventTypePendingReviewDocument:
		return "проверка документа"
	case models.FeedEventTypePendingCreateDocument:
		return "создание документа"
	case models.FeedEventTypePendingDeleteDocument:
		return "удаление документа"
	case models.FeedEventTypePendingEditClient:
		return "редактирование клиента"
	case models.FeedEventTypePendingDeleteClient:
		return "удаление клиента"
	case models.FeedEventTypePendingEditLead, models.FeedEventTypePendingDeleteLead:
		return "изменение лида"
	case models.FeedEventTypePendingEditDeal, models.FeedEventTypePendingDeleteDeal:
		return "изменение сделки"
	default:
		return "применение заявки"
	}
}

// translateFeedApplyError переводит технические ошибки применения заявки в
// понятный админу текст. Раньше в тост уходило английское «signer phone is
// required» или вовсе ничего (при массовом одобрении — только счётчик ошибок).
func translateFeedApplyError(err error) error {
	if err == nil {
		return nil
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "not found"):
		return errors.New("объект заявки не найден — возможно, он уже удалён")
	case strings.Contains(text, "forbidden"):
		return errors.New("недостаточно прав на применение заявки")
	case strings.Contains(text, "invalid status"):
		return errors.New("объект уже в другом статусе — действие выполнено или отменено ранее")
	case strings.Contains(text, "signer phone is required"):
		return errors.New("у клиента не указан телефон для отправки SMS — заполните его в карточке клиента")
	case strings.Contains(text, "signer email is required"):
		return errors.New("у клиента не указан e-mail для отправки — заполните его в карточке клиента")
	case strings.Contains(text, "sms sending is disabled") || strings.Contains(text, "sms sender is nil"):
		return errors.New("отправка SMS отключена в настройках сервера")
	case strings.Contains(text, "sms api key"):
		return errors.New("не настроен ключ SMS-провайдера")
	case strings.Contains(text, "verify base url"):
		return errors.New("не настроен адрес страницы подписания (sign verify base URL)")
	case strings.Contains(text, "unsupported signing channel"):
		return errors.New("некорректный канал отправки в заявке (ожидается SMS или e-mail)")
	case strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "timeout"):
		return errors.New("превышено время ожидания провайдера отправки — попробуйте ещё раз")
	default:
		return err
	}
}
