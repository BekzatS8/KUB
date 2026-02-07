package services

import (
	"errors"
	"log"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"turcompany/internal/models"
)

type fakeUserVerificationRepo struct {
	records []*models.UserVerification
	nextID  int64
}

func newFakeUserVerificationRepo() *fakeUserVerificationRepo {
	return &fakeUserVerificationRepo{records: make([]*models.UserVerification, 0), nextID: 1}
}

func (r *fakeUserVerificationRepo) CountRecentSends(userID int, since time.Time) (int, error) {
	cnt := 0
	for _, v := range r.records {
		if v.UserID == userID && (v.SentAt.Equal(since) || v.SentAt.After(since)) {
			cnt++
		}
	}
	return cnt, nil
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

func (r *fakeUserVerificationRepo) IncrementAttempts(id int64) (int, error) {
	for _, v := range r.records {
		if v.ID == id {
			v.Attempts++
			return v.Attempts, nil
		}
	}
	return 0, errors.New("not found")
}

func (r *fakeUserVerificationRepo) ExpireNow(id int64) error {
	for _, v := range r.records {
		if v.ID == id {
			v.ExpiresAt = time.Now()
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeUserVerificationRepo) MarkConfirmed(id int64) error {
	for _, v := range r.records {
		if v.ID == id {
			v.Confirmed = true
			now := time.Now()
			v.ConfirmedAt = &now
			return nil
		}
	}
	return errors.New("not found")
}

func (r *fakeUserVerificationRepo) Update(v *models.UserVerification) error {
	for i, existing := range r.records {
		if existing.ID == v.ID {
			r.records[i] = v
			return nil
		}
	}
	return errors.New("not found")
}

type fakeEmailService struct {
	lastTo   string
	lastCode string
	lastTTL  int
	sent     int
}

func (f *fakeEmailService) SendWelcomeEmail(email, companyName string) error { return nil }
func (f *fakeEmailService) SendPasswordResetEmail(email, resetURL string) error {
	return nil
}
func (f *fakeEmailService) SendVerificationCode(toEmail, code string, ttlMinutes int) error {
	f.lastTo = toEmail
	f.lastCode = code
	f.lastTTL = ttlMinutes
	f.sent++
	log.Printf("[DEV][email][verify] to=%s code=%s ttl=%d", toEmail, code, ttlMinutes)
	return nil
}
func (f *fakeEmailService) SendSigningConfirm(email string, data SigningEmailData) error {
	return nil
}

type fakeUserService struct {
	verifiedUsers []int
	usersByEmail  map[string]*models.User
}

func (f *fakeUserService) VerifyUser(userID int) error {
	f.verifiedUsers = append(f.verifiedUsers, userID)
	return nil
}

func (f *fakeUserService) CreateUser(user *models.User) error { return nil }
func (f *fakeUserService) CreateUserWithPassword(user *models.User, plainPassword string) error {
	return nil
}
func (f *fakeUserService) GetUserByID(id int) (*models.User, error)            { return nil, nil }
func (f *fakeUserService) UpdateUser(user *models.User) error                  { return nil }
func (f *fakeUserService) DeleteUser(id int) error                             { return nil }
func (f *fakeUserService) ListUsers(limit, offset int) ([]*models.User, error) { return nil, nil }
func (f *fakeUserService) GetUserByEmail(email string) (*models.User, error) {
	if f.usersByEmail == nil {
		return nil, nil
	}
	return f.usersByEmail[email], nil
}
func (f *fakeUserService) GetUserCount() (int, error)                 { return 0, nil }
func (f *fakeUserService) GetUserCountByRole(roleID int) (int, error) { return 0, nil }
func (f *fakeUserService) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	return nil
}
func (f *fakeUserService) GetByRefreshToken(token string) (*models.User, error) { return nil, nil }
func (f *fakeUserService) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	return nil, nil
}

func TestUserVerificationService_Send(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	repo := newFakeUserVerificationRepo()
	emailSvc := &fakeEmailService{}
	svc := &UserVerificationService{
		Repo:     repo,
		EmailSvc: emailSvc,
		CodeTTL:  2 * time.Minute,
		now:      func() time.Time { return fixedNow },
	}

	if err := svc.Send(7, "user@example.com"); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	v, _ := repo.GetLatestByUserID(7)
	if v == nil {
		t.Fatalf("expected verification record")
	}
	if err := CompareVerificationCode(v.CodeHash, emailSvc.lastCode); err != nil {
		t.Errorf("code hash mismatch: %v", err)
	}
	expectedExp := fixedNow.Add(2 * time.Minute)
	if !v.ExpiresAt.Equal(expectedExp) {
		t.Errorf("ExpiresAt mismatch: got %s want %s", v.ExpiresAt, expectedExp)
	}
	if emailSvc.lastTo != "user@example.com" {
		t.Errorf("email recipient mismatch: got %s", emailSvc.lastTo)
	}
	if emailSvc.lastTTL != 2 {
		t.Errorf("ttl minutes mismatch: got %d", emailSvc.lastTTL)
	}
}

func TestUserVerificationService_Confirm(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	hash, _ := bcrypt.GenerateFromPassword([]byte("654321"), bcrypt.DefaultCost)
	repo := newFakeUserVerificationRepo()
	repo.records = append(repo.records, &models.UserVerification{
		ID: 1, UserID: 9, CodeHash: string(hash), SentAt: fixedNow, ExpiresAt: fixedNow.Add(5 * time.Minute), Attempts: 0,
	})
	userSvc := &fakeUserService{
		usersByEmail: map[string]*models.User{
			"person@example.com": {ID: 9, Email: "person@example.com"},
		},
	}

	svc := &UserVerificationService{
		Repo:    repo,
		UserSvc: userSvc,
		now:     func() time.Time { return fixedNow },
	}

	ok, err := svc.Confirm("person@example.com", "654321")
	if err != nil {
		t.Fatalf("Confirm returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected confirmation to succeed")
	}
	v, _ := repo.GetLatestByUserID(9)
	if !v.Confirmed {
		t.Errorf("expected verification to be marked confirmed")
	}
	if len(userSvc.verifiedUsers) != 1 || userSvc.verifiedUsers[0] != 9 {
		t.Errorf("expected user service to verify user 9")
	}
}

func TestUserVerificationService_ConfirmInvalidAndExpired(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	hash, _ := bcrypt.GenerateFromPassword([]byte("000111"), bcrypt.DefaultCost)
	repo := newFakeUserVerificationRepo()
	repo.records = append(repo.records, &models.UserVerification{
		ID: 1, UserID: 11, CodeHash: string(hash), SentAt: fixedNow.Add(-time.Minute), ExpiresAt: fixedNow.Add(time.Minute), Attempts: 0,
	})
	userSvc := &fakeUserService{
		usersByEmail: map[string]*models.User{
			"user@example.com": {ID: 11, Email: "user@example.com"},
		},
	}

	svc := &UserVerificationService{
		Repo:    repo,
		UserSvc: userSvc,
		now:     func() time.Time { return fixedNow },
	}

	ok, err := svc.Confirm("user@example.com", "999999")
	if !errors.Is(err, ErrCodeInvalid) {
		t.Fatalf("expected ErrCodeInvalid, got %v", err)
	}
	if ok {
		t.Fatalf("expected confirmation to fail for wrong code")
	}
	v, _ := repo.GetLatestByUserID(11)
	if v.Attempts != 1 {
		t.Errorf("expected attempts to increment, got %d", v.Attempts)
	}

	repo.records[0].ExpiresAt = fixedNow.Add(-time.Minute)
	ok, err = svc.Confirm("user@example.com", "000111")
	if !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("expected ErrCodeExpired, got %v", err)
	}
	if ok {
		t.Fatalf("expected expired confirmation to fail")
	}
}

func TestUserVerificationService_ResendCooldownAndMax(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	repo := newFakeUserVerificationRepo()
	repo.records = append(repo.records, &models.UserVerification{
		ID:           1,
		UserID:       7,
		CodeHash:     "hash",
		SentAt:       fixedNow.Add(-2 * time.Minute),
		ExpiresAt:    fixedNow.Add(2 * time.Minute),
		Attempts:     0,
		LastResendAt: ptrTime(fixedNow),
		ResendCount:  0,
	})
	userSvc := &fakeUserService{
		usersByEmail: map[string]*models.User{
			"user@example.com": {ID: 7, Email: "user@example.com"},
		},
	}
	emailSvc := &fakeEmailService{}
	svc := &UserVerificationService{
		Repo:     repo,
		UserSvc:  userSvc,
		EmailSvc: emailSvc,
		now:      func() time.Time { return fixedNow },
	}

	if err := svc.Resend("user@example.com"); !errors.Is(err, ErrResendThrottled) {
		t.Fatalf("expected ErrResendThrottled due to cooldown, got %v", err)
	}

	repo.records[0].LastResendAt = ptrTime(fixedNow.Add(-2 * time.Minute))
	repo.records[0].ResendCount = UserMaxResends
	if err := svc.Resend("user@example.com"); !errors.Is(err, ErrResendThrottled) {
		t.Fatalf("expected ErrResendThrottled due to max resends, got %v", err)
	}

	repo.records[0].ResendCount = UserMaxResends - 1
	if err := svc.Resend("user@example.com"); err != nil {
		t.Fatalf("expected resend to succeed, got %v", err)
	}
	if emailSvc.sent != 1 {
		t.Fatalf("expected verification email to be sent")
	}
	if repo.records[0].ResendCount != UserMaxResends {
		t.Fatalf("expected resend_count to increment")
	}
	if repo.records[0].LastResendAt == nil || !repo.records[0].LastResendAt.Equal(fixedNow) {
		t.Fatalf("expected last_resend_at to update")
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
