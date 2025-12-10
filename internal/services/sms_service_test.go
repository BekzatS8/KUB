package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"turcompany/internal/models"
	"turcompany/internal/utils"
)

type fakeMobizonClient struct {
	lastTo   string
	lastText string
	err      error
}

func (f *fakeMobizonClient) SendSMS(to, code string) (*utils.SendSMSResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.lastTo = to
	f.lastText = code
	return &utils.SendSMSResponse{Code: 0}, nil
}

type fakeDocRepo struct {
	docs    map[int64]*models.Document
	updates map[int64]string
}

func newFakeDocRepo() *fakeDocRepo {
	return &fakeDocRepo{docs: make(map[int64]*models.Document), updates: make(map[int64]string)}
}

func (r *fakeDocRepo) Create(doc *models.Document) (int64, error) {
	r.docs[doc.ID] = doc
	return doc.ID, nil
}

func (r *fakeDocRepo) GetByID(id int64) (*models.Document, error) {
	if doc, ok := r.docs[id]; ok {
		return doc, nil
	}
	return nil, nil
}

func (r *fakeDocRepo) ListDocuments(limit, offset int) ([]*models.Document, error)  { return nil, nil }
func (r *fakeDocRepo) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) { return nil, nil }
func (r *fakeDocRepo) Delete(id int64) error                                        { delete(r.docs, id); return nil }

func (r *fakeDocRepo) UpdateStatus(id int64, status string) error {
	if doc, ok := r.docs[id]; ok {
		doc.Status = status
		r.updates[id] = status
		return nil
	}
	return errors.New("not found")
}

type fakeSMSRepo struct {
	records map[int64]*models.SMSConfirmation
	nextID  int64
}

func newFakeSMSRepo() *fakeSMSRepo {
	return &fakeSMSRepo{records: make(map[int64]*models.SMSConfirmation), nextID: 1}
}

func (r *fakeSMSRepo) Create(sms *models.SMSConfirmation) (int64, error) {
	id := r.nextID
	r.nextID++
	clone := *sms
	clone.ID = id
	r.records[sms.DocumentID] = &clone
	return id, nil
}

func (r *fakeSMSRepo) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	if rec, ok := r.records[documentID]; ok {
		return rec, nil
	}
	return nil, nil
}

func (r *fakeSMSRepo) GetByDocumentIDAndCode(documentID int64, code string) (*models.SMSConfirmation, error) {
	rec, ok := r.records[documentID]
	if !ok {
		return nil, nil
	}
	if rec.SMSCode != code {
		return nil, nil
	}
	return rec, nil
}

func (r *fakeSMSRepo) Update(sms *models.SMSConfirmation) error {
	if _, ok := r.records[sms.DocumentID]; !ok {
		return errors.New("not found")
	}
	r.records[sms.DocumentID] = sms
	return nil
}

func (r *fakeSMSRepo) DeleteByDocumentID(documentID int64) error {
	delete(r.records, documentID)
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
	rec := &models.UserVerification{ID: id, UserID: userID, CodeHash: codeHash, SentAt: sentAt, ExpiresAt: expiresAt, Attempts: 0}
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
			return nil
		}
	}
	return errors.New("not found")
}

type fakeUserService struct {
	verifiedUsers []int
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
func (f *fakeUserService) GetUserByEmail(email string) (*models.User, error)   { return nil, nil }
func (f *fakeUserService) GetUserCount() (int, error)                          { return 0, nil }
func (f *fakeUserService) GetUserCountByRole(roleID int) (int, error)          { return 0, nil }
func (f *fakeUserService) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	return nil
}
func (f *fakeUserService) GetByRefreshToken(token string) (*models.User, error) { return nil, nil }
func (f *fakeUserService) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	return nil, nil
}

func TestSMSService_SendSMS(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	repo := newFakeSMSRepo()
	mobizon := &fakeMobizonClient{}
	svc := &SMS_Service{
		Repo:    repo,
		Client:  mobizon,
		CodeTTL: 2 * time.Minute,
		now:     func() time.Time { return fixedNow },
	}

	if err := svc.SendSMS(10, "+123"); err != nil {
		t.Fatalf("SendSMS returned error: %v", err)
	}

	rec, _ := repo.GetLatestByDocumentID(10)
	if rec == nil {
		t.Fatalf("expected record to be stored")
	}
	if rec.SMSCode == "" {
		t.Errorf("expected SMS code to be generated")
	}
	if !rec.SentAt.Equal(fixedNow) {
		t.Errorf("SentAt mismatch: got %s want %s", rec.SentAt, fixedNow)
	}
	if mobizon.lastTo != "+123" {
		t.Errorf("mobizon recipient mismatch: got %s", mobizon.lastTo)
	}
	if !strings.Contains(mobizon.lastText, rec.SMSCode) {
		t.Errorf("mobizon text should contain code, got %q", mobizon.lastText)
	}
}

func TestSMSService_ConfirmCode(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	repo := newFakeSMSRepo()
	docRepo := newFakeDocRepo()
	docRepo.docs[5] = &models.Document{ID: 5, Status: "approved"}
	docSvc := &DocumentService{DocRepo: docRepo}
	repo.Create(&models.SMSConfirmation{DocumentID: 5, SMSCode: "123456", SentAt: fixedNow})

	svc := &SMS_Service{
		Repo:    repo,
		Client:  &fakeMobizonClient{},
		DocSvc:  docSvc,
		CodeTTL: 2 * time.Minute,
		now:     func() time.Time { return fixedNow },
	}

	ok, err := svc.ConfirmCode(5, "123456")
	if err != nil {
		t.Fatalf("ConfirmCode returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected confirmation to succeed")
	}

	rec, _ := repo.GetLatestByDocumentID(5)
	if !rec.Confirmed {
		t.Errorf("expected record to be marked confirmed")
	}
	if !rec.ConfirmedAt.Equal(fixedNow) {
		t.Errorf("ConfirmedAt mismatch: got %s want %s", rec.ConfirmedAt, fixedNow)
	}
	if status, ok := docRepo.updates[5]; !ok || status != "signed" {
		t.Errorf("expected document service to mark docID=5 signed, got %q", status)
	}
}

func TestSMSService_ConfirmCode_InvalidOrExpired(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	repo := newFakeSMSRepo()
	repo.Create(&models.SMSConfirmation{DocumentID: 6, SMSCode: "123456", SentAt: fixedNow})

	svc := &SMS_Service{
		Repo:    repo,
		Client:  &fakeMobizonClient{},
		CodeTTL: time.Minute,
		now:     func() time.Time { return fixedNow.Add(2 * time.Minute) },
	}

	ok, err := svc.ConfirmCode(6, "123456")
	if err != nil {
		t.Fatalf("ConfirmCode returned error: %v", err)
	}
	if ok {
		t.Fatalf("expected confirmation to fail due to expiration")
	}

	ok, err = svc.ConfirmCode(6, "wrong")
	if err != nil {
		t.Fatalf("ConfirmCode returned error for wrong code: %v", err)
	}
	if ok {
		t.Fatalf("expected confirmation to fail for wrong code")
	}
}

func TestSMSService_SendUserSMS(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	verifRepo := newFakeUserVerificationRepo()
	mobizon := &fakeMobizonClient{}
	svc := &SMS_Service{
		VerifRepo: verifRepo,
		Client:    mobizon,
		CodeTTL:   2 * time.Minute,
		now:       func() time.Time { return fixedNow },
	}

	if err := svc.SendUserSMS(7, "+111"); err != nil {
		t.Fatalf("SendUserSMS returned error: %v", err)
	}

	v, _ := verifRepo.GetLatestByUserID(7)
	if v == nil {
		t.Fatalf("expected verification record")
	}
	code := strings.TrimPrefix(mobizon.lastText, "Код подтверждения: ")
	if code == mobizon.lastText {
		t.Fatalf("could not extract code from message: %q", mobizon.lastText)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(v.CodeHash), []byte(code)); err != nil {
		t.Errorf("code hash mismatch: %v", err)
	}
	expectedExp := fixedNow.Add(2 * time.Minute)
	if !v.ExpiresAt.Equal(expectedExp) {
		t.Errorf("ExpiresAt mismatch: got %s want %s", v.ExpiresAt, expectedExp)
	}
	if mobizon.lastTo != "+111" {
		t.Errorf("mobizon recipient mismatch: got %s", mobizon.lastTo)
	}
}

func TestSMSService_ConfirmUserCode(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	hash, _ := bcrypt.GenerateFromPassword([]byte("654321"), bcrypt.DefaultCost)
	verifRepo := newFakeUserVerificationRepo()
	verifRepo.records = append(verifRepo.records, &models.UserVerification{
		ID: 1, UserID: 9, CodeHash: string(hash), SentAt: fixedNow, ExpiresAt: fixedNow.Add(5 * time.Minute), Attempts: 0,
	})
	userSvc := &fakeUserService{}

	svc := &SMS_Service{
		VerifRepo: verifRepo,
		UserSvc:   userSvc,
		Client:    &fakeMobizonClient{},
		now:       func() time.Time { return fixedNow },
	}

	ok, err := svc.ConfirmUserCode(9, "654321")
	if err != nil {
		t.Fatalf("ConfirmUserCode returned error: %v", err)
	}
	if !ok {
		t.Fatalf("expected confirmation to succeed")
	}
	v, _ := verifRepo.GetLatestByUserID(9)
	if !v.Confirmed {
		t.Errorf("expected verification to be marked confirmed")
	}
	if len(userSvc.verifiedUsers) != 1 || userSvc.verifiedUsers[0] != 9 {
		t.Errorf("expected user service to verify user 9")
	}
}

func TestSMSService_ConfirmUserCode_InvalidAndExpired(t *testing.T) {
	fixedNow := time.Date(2025, 12, 10, 10, 0, 0, 0, time.UTC)
	hash, _ := bcrypt.GenerateFromPassword([]byte("000111"), bcrypt.DefaultCost)
	verifRepo := newFakeUserVerificationRepo()
	verifRepo.records = append(verifRepo.records, &models.UserVerification{
		ID: 1, UserID: 11, CodeHash: string(hash), SentAt: fixedNow.Add(-time.Minute), ExpiresAt: fixedNow.Add(time.Minute), Attempts: 0,
	})

	svc := &SMS_Service{
		VerifRepo: verifRepo,
		Client:    &fakeMobizonClient{},
		now:       func() time.Time { return fixedNow },
	}

	ok, err := svc.ConfirmUserCode(11, "999999")
	if !errors.Is(err, ErrCodeInvalid) {
		t.Fatalf("expected ErrCodeInvalid, got %v", err)
	}
	if ok {
		t.Fatalf("expected confirmation to fail for wrong code")
	}
	v, _ := verifRepo.GetLatestByUserID(11)
	if v.Attempts != 1 {
		t.Errorf("expected attempts to increment, got %d", v.Attempts)
	}

	v.ExpiresAt = fixedNow.Add(-time.Minute)
	ok, err = svc.ConfirmUserCode(11, "000111")
	if !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("expected ErrCodeExpired, got %v", err)
	}
	if ok {
		t.Fatalf("expected expired confirmation to fail")
	}
}
