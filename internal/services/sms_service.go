package services

import (
	"errors"
	"fmt"
	"log"
	"time"

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
	maxResendsPerWindow = 3
	resendWindow        = 10 * time.Minute
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
	Update(v *models.UserVerification) error
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

	Client  MobizonClient
	CodeTTL time.Duration // если 0 — возьмём DefaultVerificationTTL
	now     func() time.Time
}

func NewSMSService(
	docRepo SMSConfirmationRepo,
	client MobizonClient,
	docSvc *DocumentService,
	now func() time.Time,
) *SMS_Service {
	if now == nil {
		now = time.Now
	}
	return &SMS_Service{
		Repo:    docRepo,
		Client:  client,
		DocSvc:  docSvc,
		CodeTTL: DefaultVerificationTTL,
		now:     now,
	}
}

func buildOTPMessage(code string) string {
	return fmt.Sprintf("Код подтверждения: %s", code)
}

// ================== БЛОК: ДОКУМЕНТЫ ==================

func (s *SMS_Service) SendSMS(documentID int64, phone string, userID, roleID int) error {
	log.Printf("[sms][send][start] doc=%d user=%d role=%d phone=%q",
		documentID, userID, roleID, phone,
	)

	// 1️⃣ Проверка прав
	if s.DocSvc != nil {
		if err := s.DocSvc.EnsureSMSAllowed(documentID, userID, roleID); err != nil {
			log.Printf("[sms][send][forbidden] doc=%d err=%v", documentID, err)
			return err
		}
	}

	// 2️⃣ Нормализация телефона
	toDigits, err := utils.SanitizeE164Digits(phone)
	if err != nil {
		log.Printf("[sms][send][invalid_phone] doc=%d phone=%q err=%v",
			documentID, phone, err,
		)
		return fmt.Errorf("invalid phone: %w", err)
	}

	// 3️⃣ Генерация кода
	code := GenerateVerificationCode()
	text := fmt.Sprintf("Код подтверждения: %s", code)

	log.Printf("[sms][send][otp_generated] doc=%d phone=%s code=%s provider=%T",
		documentID, toDigits, code, s.Client,
	)

	// 4️⃣ Отправка через клиент (GreenAPI / WhatsApp / SMS)
	if s.Client != nil {
		log.Printf("[sms][send][provider_call] doc=%d phone=%s",
			documentID, toDigits)

		resp, err := s.Client.SendSMS(toDigits, text)
		if err != nil {
			log.Printf("[sms][send][provider_error] doc=%d phone=%s err=%v",
				documentID, toDigits, err,
			)
			return fmt.Errorf("sms provider error: %w", err)
		}

		log.Printf("[sms][send][provider_success] doc=%d message_id=%s",
			documentID, resp.Data.MessageID)
	} else {
		log.Printf("[sms][send][skip] doc=%d reason=client_is_nil", documentID)
	}

	// 5️⃣ Сохраняем в БД
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		log.Printf("[sms][send][hash_error] doc=%d err=%v", documentID, err)
		return err
	}

	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
	}

	now := s.now()

	rec := &models.SMSConfirmation{
		DocumentID:   documentID,
		Phone:        toDigits, // Сохраняем нормализованный номер
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
		log.Printf("[sms][send][db_error] doc=%d phone=%s err=%v",
			documentID, toDigits, err,
		)
		return fmt.Errorf("db error after SMS: %w", err)
	}

	log.Printf("[sms][send][success] doc=%d phone=%s", documentID, toDigits)
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
	code := GenerateVerificationCode()
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		return err
	}
	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
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
	if rec.Attempts >= MaxConfirmAttempts {
		return false, ErrTooManyAttempts
	}

	// уже подтверждён – считаем это УСПЕХОМ (идемпотентность)
	if rec.Confirmed {
		return true, nil
	}

	if err := CompareVerificationCode(rec.CodeHash, code); err != nil {
		rec.Attempts++
		if err := s.Repo.Update(rec); err != nil {
			return false, err
		}
		if rec.Attempts >= MaxConfirmAttempts {
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

// ================== ОБЩЕЕ ==================

func (s *SMS_Service) IsCodeExpired(sentAt time.Time) bool {
	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
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
