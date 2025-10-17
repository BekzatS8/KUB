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

type SMS_Service struct {
	// Документные SMS (как было)
	Repo   *repositories.SMSConfirmationRepository
	DocSvc *DocumentService

	// Регистрация/верификация пользователя (НОВОЕ)
	VerifRepo *repositories.UserVerificationRepository
	UserSvc   UserService

	Client  *utils.Client
	CodeTTL time.Duration // если 0 — возьмём defaultVerificationTTL
}

func NewSMSService(
	docRepo *repositories.SMSConfirmationRepository,
	client *utils.Client,
	docSvc *DocumentService,
	verifRepo *repositories.UserVerificationRepository,
	userSvc UserService,
) *SMS_Service {
	return &SMS_Service{
		Repo:      docRepo,
		Client:    client,
		DocSvc:    docSvc,
		VerifRepo: verifRepo,
		UserSvc:   userSvc,
		CodeTTL:   defaultVerificationTTL,
	}
}

// --- утилита генерации 6-значного кода ---
func (s *SMS_Service) generateCode() string {
	src := rand.NewSource(time.Now().UnixNano())
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
		SentAt:      time.Now(),
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
	rec.ConfirmedAt = time.Now()
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
	since := time.Now().Add(-resendWindow)
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
	sentAt := time.Now()
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
	if time.Now().After(v.ExpiresAt) {
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
	return time.Now().After(sentAt.Add(ttl))
}

// Документные утилиты как были
func (s *SMS_Service) DeleteConfirmation(documentID int64) error {
	return s.Repo.DeleteByDocumentID(documentID)
}

func (s *SMS_Service) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	return s.Repo.GetLatestByDocumentID(documentID)
}
