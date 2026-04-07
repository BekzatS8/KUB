package services

import (
	"context"
	"testing"
	"time"

	"turcompany/internal/models"
)

type captureUserRepo struct {
	created *models.User
}

func (r *captureUserRepo) Create(user *models.User) error {
	cp := *user
	r.created = &cp
	user.ID = 42
	return nil
}
func (r *captureUserRepo) GetByID(int) (*models.User, error)          { return nil, nil }
func (r *captureUserRepo) Update(*models.User) error                  { return nil }
func (r *captureUserRepo) Delete(int) error                           { return nil }
func (r *captureUserRepo) List(int, int) ([]*models.User, error)      { return nil, nil }
func (r *captureUserRepo) GetByEmail(string) (*models.User, error)    { return nil, nil }
func (r *captureUserRepo) GetCount() (int, error)                     { return 0, nil }
func (r *captureUserRepo) GetCountByRole(int) (int, error)            { return 0, nil }
func (r *captureUserRepo) UpdatePassword(int, string) error           { return nil }
func (r *captureUserRepo) UpdateRefresh(int, string, time.Time) error { return nil }
func (r *captureUserRepo) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (r *captureUserRepo) ClearRefresh(int) error                         { return nil }
func (r *captureUserRepo) GetByRefreshToken(string) (*models.User, error) { return nil, nil }
func (r *captureUserRepo) VerifyUser(int) error                           { return nil }
func (r *captureUserRepo) UpdateTelegramLink(int, int64, bool) error      { return nil }
func (r *captureUserRepo) GetByIDSimple(int) (*models.User, error)        { return nil, nil }
func (r *captureUserRepo) GetTelegramSettings(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (r *captureUserRepo) GetByChatID(context.Context, int64) (*models.User, error) { return nil, nil }

type noopMailService struct{}

func (noopMailService) SendWelcomeEmail(string, string) error             { return nil }
func (noopMailService) SendPasswordResetEmail(string, string) error       { return nil }
func (noopMailService) SendVerificationCode(string, string, int) error    { return nil }
func (noopMailService) SendSigningConfirm(string, SigningEmailData) error { return nil }

func TestCreateUserWithPassword_DefaultUnverifiedKeepsLegacyBehavior(t *testing.T) {
	repo := &captureUserRepo{}
	auth := NewAuthService([]byte("01234567890123456789012345678901"), nil, 0, 0, nil)
	svc := NewUserService(repo, noopMailService{}, auth)

	u := &models.User{CompanyName: "Acme", Email: "u@example.com", RoleID: 10, Phone: "+7700"}
	if err := svc.CreateUserWithPassword(u, "Passw0rd"); err != nil {
		t.Fatalf("CreateUserWithPassword error: %v", err)
	}
	if repo.created == nil {
		t.Fatal("repo Create was not called")
	}
	if repo.created.IsVerified {
		t.Fatalf("expected IsVerified=false by default, got true")
	}
	if repo.created.VerifiedAt != nil {
		t.Fatalf("expected VerifiedAt=nil by default, got %v", repo.created.VerifiedAt)
	}
}

func TestCreateUserWithPassword_VerifiedUserGetsVerifiedAt(t *testing.T) {
	repo := &captureUserRepo{}
	auth := NewAuthService([]byte("01234567890123456789012345678901"), nil, 0, 0, nil)
	svc := NewUserService(repo, noopMailService{}, auth)

	u := &models.User{CompanyName: "Acme", Email: "u@example.com", RoleID: 10, Phone: "+7700", IsVerified: true}
	if err := svc.CreateUserWithPassword(u, "Passw0rd"); err != nil {
		t.Fatalf("CreateUserWithPassword error: %v", err)
	}
	if repo.created == nil {
		t.Fatal("repo Create was not called")
	}
	if !repo.created.IsVerified {
		t.Fatalf("expected IsVerified=true, got false")
	}
	if repo.created.VerifiedAt == nil {
		t.Fatal("expected VerifiedAt to be set for verified user")
	}
}
