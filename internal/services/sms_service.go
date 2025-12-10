package services

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"time"

	"golang.org/x/crypto/bcrypt"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/utils"
)

var (
	ErrResendThrottled = errors.New("resend throttled")
	ErrTooManyAttempts = errors.New("too many attempts")
	ErrCodeExpired     = errors.New("code expired")
	ErrCodeInvalid     = errors.New("code invalid")
)

// Настройки безопасности (можно вынести в конфиг при желании)
const (
	maxResendsPerWindow    = 3
	resendWindow           = 10 * time.Minute
	maxConfirmAttempts     = 5
	defaultVerificationTTL = 5 * time.Minute
)

type SMSConfirmationRepo interface {
	Create(sms *models.SMSConfirmation) (int64, error)
	GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error)
	GetByDocumentIDAndCode(documentID int64, code string) (*models.SMSConfirmation, error)
	Update(sms *models.SMSConfirmation) error
	DeleteByDocumentID(documentID int64) error
}

type UserVerificationRepo interface {
	CountRecentSends(userID int, since time.Time) (int, error)
	Create(userID int, codeHash string, sentAt, expiresAt time.Time) (int64, error)
	GetLatestByUserID(userID int) (*models.UserVerification, error)
	IncrementAttempts(id int64) (int, error)
	ExpireNow(id int64) error
	MarkConfirmed(id int64) error
}

type MobizonClient interface {
	SendSMS(to, code string) (*utils.SendSMSResponse, error)
}

var (
	_ SMSConfirmationRepo  = (*repositories.SMSConfirmationRepository)(nil)
	_ UserVerificationRepo = (*repositories.UserVerificationRepository)(nil)
	_ MobizonClient        = (*utils.Client)(nil)
)

type SMS_Service struct {
	// Документные SMS (как было)
	Repo   SMSConfirmationRepo
	DocSvc *DocumentService

	// Регистрация/верификация пользователя (НОВОЕ)
	VerifRepo UserVerificationRepo
	UserSvc   UserService

	Client  MobizonClient
	CodeTTL time.Duration // если 0 — возьмём defaultVerificationTTL
	now     func() time.Time
}

func NewSMSService(
	docRepo SMSConfirmationRepo,
	client MobizonClient,
	docSvc *DocumentService,
	verifRepo UserVerificationRepo,
	userSvc UserService,
	now func() time.Time,
) *SMS_Service {
	if now == nil {
		now = time.Now
	}
	return &SMS_Service{
		Repo:      docRepo,
		Client:    client,
		DocSvc:    docSvc,
		VerifRepo: verifRepo,
		UserSvc:   userSvc,
		CodeTTL:   defaultVerificationTTL,
		now:       now,
	}
}

// --- утилита генерации 6-значного кода ---
func (s *SMS_Service) generateCode() string {
	src := rand.NewSource(s.now().UnixNano())
	rnd := rand.New(src)
	return fmt.Sprintf("%06d", rnd.Intn(1000000))
}

// ================== БЛОК: ДОКУМЕНТЫ ==================

func (s *SMS_Service) SendSMS(documentID int64, phone string) error {
	code := s.generateCode()
	text := fmt.Sprintf("Код подтверждения: %s", code)

	resp, err := s.Client.SendSMS(phone, text)
	if err != nil {
		return fmt.Errorf("mobizon error: %w", err)
	}

	rec := &models.SMSConfirmation{
		DocumentID:  documentID,
		Phone:       phone,
		SMSCode:     code, // (можно тоже захэшировать позже)
		SentAt:      s.now(),
		Confirmed:   false,
		ConfirmedAt: time.Time{},
	}
	if _, err := s.Repo.Create(rec); err != nil {
		return fmt.Errorf("db error after SMS: %w", err)
	}

	log.Printf("[sms][doc][send] ok: doc_id=%d phone=%s code=%s messageID=%s", documentID, phone, code, resp.Data.MessageID)
	return nil
}

func (s *SMS_Service) ResendSMS(documentID int64, phone string) error {
	existing, err := s.Repo.GetLatestByDocumentID(documentID)
	if err != nil {
		return err
	}
	// если нет актуального — отправляем новый (нужен телефон)
	if existing == nil || existing.Confirmed || s.IsCodeExpired(existing.SentAt) {
		if phone == "" {
			return fmt.Errorf("phone required for first/expired resend")
		}
		return s.SendSMS(documentID, phone)
	}
	// переотправляем тот же код
	text := fmt.Sprintf("Код подтверждения: %s", existing.SMSCode)
	if _, err := s.Client.SendSMS(existing.Phone, text); err != nil {
		return fmt.Errorf("resend error: %w", err)
	}
	log.Printf("[sms][doc][resend] doc_id=%d phone=%s code=%s", documentID, existing.Phone, existing.SMSCode)
	return nil
}

func (s *SMS_Service) ConfirmCode(documentID int64, code string) (bool, error) {
	rec, err := s.Repo.GetByDocumentIDAndCode(documentID, code)
	if err != nil {
		return false, err
	}
	if rec == nil || rec.Confirmed || s.IsCodeExpired(rec.SentAt) {
		return false, nil
	}
	rec.Confirmed = true
	rec.ConfirmedAt = s.now()
	if err := s.Repo.Update(rec); err != nil {
		return false, err
	}

	if s.DocSvc != nil {
		if err := s.DocSvc.SignBySMS(documentID); err != nil {
			log.Printf("[sms][doc][confirm] document sign failed: doc_id=%d err=%v", documentID, err)
			return false, err
		}
	}

	return true, nil
}

// ================== БЛОК: ПОЛЬЗОВАТЕЛИ ==================

// SendUserSMS — отправляем новый код (каждый resend — новый код).
// Храним только bcrypt-хэш.
func (s *SMS_Service) SendUserSMS(userID int, phone string) error {
	if s.VerifRepo == nil {
		return fmt.Errorf("verification repo is nil")
	}

	// Троттлинг отправок: не чаще 3/10мин
	since := s.now().Add(-resendWindow)
	cnt, err := s.VerifRepo.CountRecentSends(userID, since)
	if err != nil {
		return err
	}
	if cnt >= maxResendsPerWindow {
		return ErrResendThrottled
	}

	code := s.generateCode()
	codeHashBytes, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("bcrypt generate: %w", err)
	}
	codeHash := string(codeHashBytes)

	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = defaultVerificationTTL
	}
	sentAt := s.now()
	expiresAt := sentAt.Add(ttl)

	// Сохраняем запись (attempts=0, confirmed=false)
	if _, err := s.VerifRepo.Create(userID, codeHash, sentAt, expiresAt); err != nil {
		return err
	}

	text := fmt.Sprintf("Код подтверждения: %s", code)
	if _, err := s.Client.SendSMS(phone, text); err != nil {
		return fmt.Errorf("mobizon error: %w", err)
	}

	log.Printf("[sms][user][send] user_id=%d phone=%s", userID, phone)
	return nil
}

// ResendUserSMS — просто вызывает SendUserSMS (он уже проверяет троттлинг).
func (s *SMS_Service) ResendUserSMS(userID int, phone string) error {
	if phone == "" {
		// На первом шаге регистрации телефон обязателен. Если resend из клиента приходит без телефона —
		// обычно телефон уже хранится у пользователя; но для простоты оставим как есть.
		return fmt.Errorf("phone required")
	}
	return s.SendUserSMS(userID, phone)
}

// ConfirmUserCode — сверяет с bcrypt-хэшем, считает попытки, TTL.
// При успехе проставляет is_verified у User.
func (s *SMS_Service) ConfirmUserCode(userID int, code string) (bool, error) {
	if s.VerifRepo == nil {
		return false, fmt.Errorf("verification repo is nil")
	}
	v, err := s.VerifRepo.GetLatestByUserID(userID)
	if err != nil {
		return false, err
	}
	if v == nil || v.Confirmed {
		return false, ErrCodeInvalid
	}
	if s.now().After(v.ExpiresAt) {
		return false, ErrCodeExpired
	}

	// сравниваем с bcrypt
	if err := bcrypt.CompareHashAndPassword([]byte(v.CodeHash), []byte(code)); err != nil {
		// неверный код => увеличиваем attempts
		attempts, incErr := s.VerifRepo.IncrementAttempts(v.ID)
		if incErr != nil {
			return false, incErr
		}
		if attempts >= maxConfirmAttempts {
			_ = s.VerifRepo.ExpireNow(v.ID)
			return false, ErrTooManyAttempts
		}
		return false, ErrCodeInvalid
	}

	// Успех
	if err := s.VerifRepo.MarkConfirmed(v.ID); err != nil {
		return false, err
	}
	if s.UserSvc != nil {
		if err := s.UserSvc.VerifyUser(userID); err != nil {
			return false, err
		}
	}
	log.Printf("[sms][user][confirm] OK user_id=%d", userID)
	return true, nil
}

// ================== ОБЩЕЕ ==================

func (s *SMS_Service) IsCodeExpired(sentAt time.Time) bool {
	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = defaultVerificationTTL
	}
	return s.now().After(sentAt.Add(ttl))
}

// Документные утилиты как были
func (s *SMS_Service) DeleteConfirmation(documentID int64) error {
	return s.Repo.DeleteByDocumentID(documentID)
}

func (s *SMS_Service) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	return s.Repo.GetLatestByDocumentID(documentID)
}
