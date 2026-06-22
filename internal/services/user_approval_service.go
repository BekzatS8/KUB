package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

var ErrApprovalNotFound = errors.New("approval request not found")
var ErrApprovalAlreadyResolved = errors.New("approval request already resolved")

type UserApprovalService struct {
	repo        *repositories.UserApprovalRepository
	userService UserService
	authService AuthService
	audit       *AuditService
}

func NewUserApprovalService(
	repo *repositories.UserApprovalRepository,
	userService UserService,
	authService AuthService,
	audit *AuditService,
) *UserApprovalService {
	return &UserApprovalService{
		repo:        repo,
		userService: userService,
		authService: authService,
		audit:       audit,
	}
}

// RequestCreate создаёт заявку на добавление пользователя (от юриста).
// Пароль хешируется здесь же, чтобы не хранить plain-text.
func (s *UserApprovalService) RequestCreate(
	ctx context.Context,
	requesterID int,
	user *models.User,
	plainPassword string,
) (*models.UserApprovalRequest, error) {
	hash, err := s.authService.HashPassword(plainPassword)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	payload := models.UserApprovalCreatePayload{
		FirstName:    user.FirstName,
		LastName:     user.LastName,
		MiddleName:   user.MiddleName,
		Position:     user.Position,
		Email:        user.Email,
		Phone:        user.Phone,
		Address:      user.Address,
		ExtraInfo:    user.ExtraInfo,
		CompanyName:  user.CompanyName,
		BinIin:       user.BinIin,
		RoleID:       user.RoleID,
		BranchID:     user.BranchID,
		PasswordHash: hash,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}
	raw := json.RawMessage(data)

	req := &models.UserApprovalRequest{
		RequesterID: requesterID,
		Action:      models.ApprovalActionCreate,
		RequestData: &raw,
	}
	if err := s.repo.Create(ctx, req); err != nil {
		return nil, err
	}

	s.audit.Log(ctx, AuditEvent{
		ActorUserID: &requesterID,
		Action:      "users.create.requested",
		EntityType:  "user_approval_request",
		EntityID:    fmt.Sprintf("%d", req.ID),
		Meta: map[string]any{
			"request_id": req.ID,
			"email":      user.Email,
			"role_id":    user.RoleID,
		},
	})

	return req, nil
}

// RequestDelete создаёт заявку на удаление пользователя (от юриста).
func (s *UserApprovalService) RequestDelete(
	ctx context.Context,
	requesterID, targetUserID int,
) (*models.UserApprovalRequest, error) {
	target, err := s.userService.GetUserByID(targetUserID)
	if err != nil || target == nil {
		return nil, ErrNotFound
	}

	req := &models.UserApprovalRequest{
		RequesterID:  requesterID,
		Action:       models.ApprovalActionDelete,
		TargetUserID: &targetUserID,
	}
	if err := s.repo.Create(ctx, req); err != nil {
		return nil, err
	}

	s.audit.Log(ctx, AuditEvent{
		ActorUserID: &requesterID,
		Action:      "users.delete.requested",
		EntityType:  "user_approval_request",
		EntityID:    fmt.Sprintf("%d", req.ID),
		Meta: map[string]any{
			"request_id":     req.ID,
			"target_user_id": targetUserID,
		},
	})

	return req, nil
}

// Approve одобряет запрос: выполняет фактическое действие (создание/удаление пользователя).
func (s *UserApprovalService) Approve(ctx context.Context, requestID, reviewerID int) error {
	req, err := s.repo.GetByID(ctx, requestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrApprovalNotFound
		}
		return err
	}
	if req.Status != models.ApprovalStatusPending {
		return ErrApprovalAlreadyResolved
	}

	switch req.Action {
	case models.ApprovalActionCreate:
		if err := s.executeCreate(ctx, req); err != nil {
			return fmt.Errorf("execute create: %w", err)
		}
	case models.ApprovalActionDelete:
		if req.TargetUserID == nil {
			return fmt.Errorf("delete request has no target_user_id")
		}
		if err := s.userService.DeleteUser(*req.TargetUserID); err != nil {
			return fmt.Errorf("execute delete: %w", err)
		}
	}

	if err := s.repo.UpdateStatus(ctx, requestID, models.ApprovalStatusApproved, reviewerID); err != nil {
		return err
	}

	s.audit.Log(ctx, AuditEvent{
		ActorUserID: &reviewerID,
		Action:      fmt.Sprintf("users.%s.approved", req.Action),
		EntityType:  "user_approval_request",
		EntityID:    fmt.Sprintf("%d", requestID),
		Meta:        map[string]any{"request_id": requestID},
	})

	return nil
}

// Reject отклоняет запрос без каких-либо действий с пользователями.
func (s *UserApprovalService) Reject(ctx context.Context, requestID, reviewerID int) error {
	req, err := s.repo.GetByID(ctx, requestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrApprovalNotFound
		}
		return err
	}
	if req.Status != models.ApprovalStatusPending {
		return ErrApprovalAlreadyResolved
	}

	if err := s.repo.UpdateStatus(ctx, requestID, models.ApprovalStatusRejected, reviewerID); err != nil {
		return err
	}

	s.audit.Log(ctx, AuditEvent{
		ActorUserID: &reviewerID,
		Action:      fmt.Sprintf("users.%s.rejected", req.Action),
		EntityType:  "user_approval_request",
		EntityID:    fmt.Sprintf("%d", requestID),
		Meta:        map[string]any{"request_id": requestID},
	})

	return nil
}

func (s *UserApprovalService) ListPending(ctx context.Context, limit, offset int) ([]*models.UserApprovalRequest, error) {
	return s.repo.ListPending(ctx, limit, offset)
}

func (s *UserApprovalService) ListAll(ctx context.Context, limit, offset int) ([]*models.UserApprovalRequest, error) {
	return s.repo.ListAll(ctx, limit, offset)
}

func (s *UserApprovalService) executeCreate(ctx context.Context, req *models.UserApprovalRequest) error {
	if req.RequestData == nil {
		return fmt.Errorf("request_data is empty")
	}
	var payload models.UserApprovalCreatePayload
	if err := json.Unmarshal(*req.RequestData, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	user := &models.User{
		FirstName:    payload.FirstName,
		LastName:     payload.LastName,
		MiddleName:   payload.MiddleName,
		Position:     payload.Position,
		Email:        payload.Email,
		Phone:        payload.Phone,
		Address:      payload.Address,
		ExtraInfo:    payload.ExtraInfo,
		CompanyName:  payload.CompanyName,
		BinIin:       payload.BinIin,
		RoleID:       payload.RoleID,
		BranchID:     payload.BranchID,
		PasswordHash: payload.PasswordHash,
		IsVerified:   true,
		IsActive:     true,
		IsActiveSet:  true,
	}

	return s.userService.CreateUser(user)
}
