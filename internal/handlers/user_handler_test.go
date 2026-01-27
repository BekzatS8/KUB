package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"turcompany/internal/models"
	"turcompany/internal/services"
)

type fakeUserServiceUser struct {
	createdUser *models.User
}

func (f *fakeUserServiceUser) CreateUser(user *models.User) error { return nil }
func (f *fakeUserServiceUser) CreateUserWithPassword(user *models.User, plainPassword string) error {
	user.ID = 1
	f.createdUser = user
	return nil
}
func (f *fakeUserServiceUser) GetUserByID(id int) (*models.User, error)            { return nil, nil }
func (f *fakeUserServiceUser) UpdateUser(user *models.User) error                  { return nil }
func (f *fakeUserServiceUser) DeleteUser(id int) error                             { return nil }
func (f *fakeUserServiceUser) ListUsers(limit, offset int) ([]*models.User, error) { return nil, nil }
func (f *fakeUserServiceUser) GetUserByEmail(email string) (*models.User, error)   { return nil, nil }
func (f *fakeUserServiceUser) GetUserCount() (int, error)                          { return 0, nil }
func (f *fakeUserServiceUser) GetUserCountByRole(roleID int) (int, error)          { return 0, nil }
func (f *fakeUserServiceUser) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	return nil
}
func (f *fakeUserServiceUser) GetByRefreshToken(token string) (*models.User, error) { return nil, nil }
func (f *fakeUserServiceUser) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	return nil, nil
}
func (f *fakeUserServiceUser) VerifyUser(userID int) error { return nil }

type fakeEmailService struct {
	lastTo   string
	lastCode string
	lastTTL  int
}

func (f *fakeEmailService) SendWelcomeEmail(email, companyName string) error { return nil }
func (f *fakeEmailService) SendPasswordResetEmail(email, resetURL string) error {
	return nil
}
func (f *fakeEmailService) SendVerificationCode(toEmail, code string, ttlMinutes int) error {
	f.lastTo = toEmail
	f.lastCode = code
	f.lastTTL = ttlMinutes
	return nil
}

type fakeUserVerificationRepo struct {
	records []*models.UserVerification
	nextID  int64
}

func newFakeUserVerificationRepo() *fakeUserVerificationRepo {
	return &fakeUserVerificationRepo{records: make([]*models.UserVerification, 0), nextID: 1}
}

func (r *fakeUserVerificationRepo) CountRecentSends(userID int, since time.Time) (int, error) {
	return 0, nil
}

func (r *fakeUserVerificationRepo) Create(userID int, codeHash string, sentAt, expiresAt time.Time) (int64, error) {
	id := r.nextID
	r.nextID++
	rec := &models.UserVerification{
		ID:           id,
		UserID:       userID,
		CodeHash:     codeHash,
		SentAt:       sentAt,
		ExpiresAt:    expiresAt,
		Attempts:     0,
		LastResendAt: &sentAt,
		ResendCount:  0,
	}
	r.records = append(r.records, rec)
	return id, nil
}

func (r *fakeUserVerificationRepo) GetLatestByUserID(userID int) (*models.UserVerification, error) {
	for i := len(r.records) - 1; i >= 0; i-- {
		if r.records[i].UserID == userID {
			return r.records[i], nil
		}
	}
	return nil, nil
}

func (r *fakeUserVerificationRepo) IncrementAttempts(id int64) (int, error) { return 0, nil }
func (r *fakeUserVerificationRepo) ExpireNow(id int64) error                { return nil }
func (r *fakeUserVerificationRepo) MarkConfirmed(id int64) error            { return nil }
func (r *fakeUserVerificationRepo) Update(v *models.UserVerification) error { return nil }

func TestRegisterCreatesVerificationAndSendsEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	userSvc := &fakeUserServiceUser{}
	repo := newFakeUserVerificationRepo()
	emailSvc := &fakeEmailService{}
	verificationSvc := services.NewUserVerificationService(repo, nil, emailSvc, func() time.Time {
		return time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	})

	handler := NewUserHandler(userSvc, verificationSvc)
	router := gin.New()
	router.POST("/register", handler.Register)

	payload := map[string]interface{}{
		"company_name": "Acme",
		"bin_iin":      "123",
		"email":        "user@example.com",
		"password":     "secret123",
		"phone":        "+77770000000",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, "/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", w.Code)
	}
	if userSvc.createdUser == nil || userSvc.createdUser.ID != 1 {
		t.Fatalf("expected user to be created")
	}
	if len(repo.records) != 1 {
		t.Fatalf("expected verification record to be created")
	}
	if repo.records[0].CodeHash == "" {
		t.Fatalf("expected verification code to be hashed")
	}
	if err := services.CompareVerificationCode(repo.records[0].CodeHash, emailSvc.lastCode); err != nil {
		t.Fatalf("expected verification code to match hash: %v", err)
	}
	if emailSvc.lastTo != "user@example.com" {
		t.Fatalf("expected verification email to be sent")
	}
}
