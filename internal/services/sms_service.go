package services

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
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
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func buildOTPMessage(code string) string {
	return fmt.Sprintf("Код подтверждения: %s", code)
}

// ================== БЛОК: ДОКУМЕНТЫ ==================

func (s *SMS_Service) SendSMS(documentID int64, phone string, userID, roleID int) error {
	if s.DocSvc != nil {
		if err := s.DocSvc.EnsureSMSAllowed(documentID, userID, roleID); err != nil {
			return err
		}
	}
	code := s.generateCode()
	text := buildOTPMessage(code)

	// 🔥 DEV MODE: логируем код
	log.Printf("[DEV][SMS][DOC] document_id=%d phone=%s code=%s", documentID, phone, code)

	// если клиента нет или dry-run — не шлём SMS
	if s.Client != nil {
		if _, err := s.Client.SendSMS(phone, text); err != nil {
			return fmt.Errorf("mobizon error: %w", err)
		}
	}

	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash code: %w", err)
	}
	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = defaultVerificationTTL
	}
	now := s.now()
	rec := &models.SMSConfirmation{
		DocumentID:   documentID,
		Phone:        phone,
		CodeHash:     string(codeHash),
		SentAt:       now,
		ExpiresAt:    now.Add(ttl),
		Attempts:     0,
		Confirmed:    false,
		ConfirmedAt:  time.Time{},
		LastResendAt: &now,
		ResendCount:  0,
	}

	if _, err := s.Repo.Create(rec); err != nil {
		return fmt.Errorf("db error after SMS: %w", err)
	}

	return nil
}

func (s *SMS_Service) ResendSMS(documentID int64, phone string, userID, roleID int) error {
	if s.DocSvc != nil {
		if err := s.DocSvc.EnsureSMSAllowed(documentID, userID, roleID); err != nil {
			return err
		}
	}
	existing, err := s.Repo.GetLatestByDocumentID(documentID)
	if err != nil {
		return err
	}
	// если нет актуального — отправляем новый (нужен телефон)
	if existing == nil || existing.Confirmed || s.IsCodeExpired(existing.SentAt) {
		if phone == "" {
			return fmt.Errorf("phone required for first/expired resend")
		}
		return s.SendSMS(documentID, phone, userID, roleID)
	}
	now := s.now()
	if now.Sub(existing.SentAt) <= resendWindow && existing.ResendCount >= maxResendsPerWindow {
		return ErrResendThrottled
	}
	if phone == "" {
		phone = existing.Phone
	}
	code := s.generateCode()
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash code: %w", err)
	}
	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = defaultVerificationTTL
	}
	existing.Phone = phone
	existing.CodeHash = string(codeHash)
	existing.SentAt = now
	existing.ExpiresAt = now.Add(ttl)
	existing.Attempts = 0
	existing.Confirmed = false
	existing.ConfirmedAt = time.Time{}
	existing.LastResendAt = &now
	existing.ResendCount++

	if err := s.Repo.Update(existing); err != nil {
		return err
	}

	text := buildOTPMessage(code)
	if s.Client != nil {
		if _, err := s.Client.SendSMS(existing.Phone, text); err != nil {
			return fmt.Errorf("resend error: %w", err)
		}
	}
	log.Printf("[sms][doc][resend] doc_id=%d phone=%s", documentID, existing.Phone)
	return nil
}

func (s *SMS_Service) ConfirmCode(documentID int64, code string, userID, roleID int) (bool, error) {
	if s.DocSvc != nil {
		if err := s.DocSvc.EnsureSMSAllowed(documentID, userID, roleID); err != nil {
			return false, err
		}
	}
	rec, err := s.Repo.GetLatestByDocumentID(documentID)
	if err != nil {
		return false, err
	}
	// нет такой записи
	if rec == nil {
		return false, nil
	}
	// код истёк
	if s.IsCodeExpired(rec.SentAt) || s.now().After(rec.ExpiresAt) {
		return false, ErrCodeExpired
	}
	if rec.Attempts >= maxConfirmAttempts {
		return false, ErrTooManyAttempts
	}

	// уже подтверждён – считаем это УСПЕХОМ (идемпотентность)
	if rec.Confirmed {
		return true, nil
	}

	if err := bcrypt.CompareHashAndPassword([]byte(rec.CodeHash), []byte(code)); err != nil {
		rec.Attempts++
		if err := s.Repo.Update(rec); err != nil {
			return false, err
		}
		if rec.Attempts >= maxConfirmAttempts {
			return false, ErrTooManyAttempts
		}
		return false, ErrCodeInvalid
	}

	// первое успешное подтверждение
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

	log.Printf("[DEV][SMS][USER] user_id=%d phone=%s code=%s", userID, phone, code)

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

	text := buildOTPMessage(code)
	if _, err := s.Client.SendSMS(phone, text); err != nil {
		return fmt.Errorf("mobizon error: %w", err)
	}

	log.Printf("[sms][user][send] user_id=%d phone=%s", userID, phone)
	return nil
}

// ResendUserSMS — просто вызывает SendUserSMS (он уже проверяет троттлинг).
func (s *SMS_Service) ResendUserSMS(userID int) error {
	if s.UserSvc == nil {
		return fmt.Errorf("user service is nil")
	}
	user, err := s.UserSvc.GetUserByID(userID)
	if err != nil {
		return err
	}
	if user == nil || strings.TrimSpace(user.Phone) == "" {
		return fmt.Errorf("phone required")
	}
	return s.SendUserSMS(userID, user.Phone)
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
