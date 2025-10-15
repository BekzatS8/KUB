package services

import (
	"fmt"
	"log"
	"math/rand"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/utils"
)

type SMS_Service struct {
	Repo    *repositories.SMSConfirmationRepository
	Client  *utils.Client
	DocSvc  *DocumentService // Сервис работы с документами
	CodeTTL time.Duration    // Время действия кода подтверждения
}

func NewSMSService(repo *repositories.SMSConfirmationRepository, client *utils.Client, docSvc *DocumentService) *SMS_Service {
	return &SMS_Service{
		Repo:    repo,
		Client:  client,
		DocSvc:  docSvc,
		CodeTTL: 5 * time.Minute, // Дефолтное время жизни кода - 5 минут
	}
}

// generateCode — генерация случайного кода для подтверждения.
func (s *SMS_Service) generateCode() string {
	src := rand.NewSource(time.Now().UnixNano())
	rnd := rand.New(src)
	return fmt.Sprintf("%06d", rnd.Intn(1000000))
}

// SendSMS — отправка SMS с кодом подтверждения.
func (s *SMS_Service) SendSMS(documentID int64, phone string) error {
	// Генерация случайного кода
	code := s.generateCode()
	text := fmt.Sprintf("Код подтверждения: %s", code)

	// Отправка SMS через Mobizon
	resp, err := s.Client.SendSMS(phone, text)
	if err != nil {
		return fmt.Errorf("mobizon error: %w", err)
	}

	// Сохранение данных о подтверждении в базе данных
	sms := &models.SMSConfirmation{
		DocumentID:  documentID,
		Phone:       phone,
		SMSCode:     code,
		SentAt:      time.Now(),
		Confirmed:   false,
		ConfirmedAt: time.Time{},
	}

	log.Printf("[sms][send] save: doc_id=%d phone=%s code=%s", documentID, phone, code)
	if _, err := s.Repo.Create(sms); err != nil {
		return fmt.Errorf("db error after SMS: %w", err)
	}

	log.Printf("[sms][send] ok: doc_id=%d phone=%s code=%s messageID=%s", documentID, phone, code, resp.Data.MessageID)
	return nil
}

// ResendSMS — повторная отправка SMS (если код не подтвержден или срок действия истек).
func (s *SMS_Service) ResendSMS(documentID int64, phone string) error {
	existing, err := s.Repo.GetLatestByDocumentID(documentID)
	if err != nil {
		return err
	}

	// Если SMS еще не отправлено или код истек, отправляем новое сообщение
	if existing == nil || existing.Confirmed || s.IsCodeExpired(existing.SentAt) {
		if phone == "" {
			return fmt.Errorf("номер телефона обязателен при первом отправлении")
		}
		return s.SendSMS(documentID, phone)
	}

	// Если код все еще действителен, отправляем тот же код
	text := fmt.Sprintf("Код подтверждения: %s", existing.SMSCode)
	if _, err := s.Client.SendSMS(existing.Phone, text); err != nil {
		return fmt.Errorf("resend error: %w", err)
	}

	log.Printf("[sms][resend] doc_id=%d phone=%s code=%s", documentID, existing.Phone, existing.SMSCode)
	return nil
}

// ConfirmCode — подтверждение кода от пользователя.
func (s *SMS_Service) ConfirmCode(documentID int64, code string) (bool, error) {
	// Ищем сохраненное SMS-подтверждение в базе
	sms, err := s.Repo.GetByDocumentIDAndCode(documentID, code)
	if err != nil {
		return false, err
	}
	if sms == nil {
		log.Printf("[sms][confirm] not found doc_id=%d code=%s", documentID, code)
		return false, nil
	}
	if sms.Confirmed {
		log.Printf("[sms][confirm] already confirmed doc_id=%d", documentID)
		return false, nil
	}
	if s.IsCodeExpired(sms.SentAt) {
		log.Printf("[sms][confirm] expired doc_id=%d sent_at=%s", documentID, sms.SentAt.Format(time.RFC3339))
		return false, nil
	}

	// Подтверждаем код
	sms.Confirmed = true
	sms.ConfirmedAt = time.Now()
	if err := s.Repo.Update(sms); err != nil {
		return false, err
	}

	// Подписываем документ, если сервис для этого доступен
	if s.DocSvc != nil {
		if err := s.DocSvc.SignBySMS(documentID); err != nil {
			log.Printf("[sms][confirm] document sign failed doc_id=%d err=%v", documentID, err)
			return false, err
		}
		log.Printf("[sms][confirm] document signed doc_id=%d", documentID)
	}

	return true, nil
}

// IsCodeExpired — проверка, истек ли срок действия кода.
func (s *SMS_Service) IsCodeExpired(sentAt time.Time) bool {
	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = 5 * time.Minute // по умолчанию 5 минут
	}
	return time.Now().After(sentAt.Add(ttl))
}

// DeleteConfirmation — удаление подтверждения SMS.
func (s *SMS_Service) DeleteConfirmation(documentID int64) error {
	return s.Repo.DeleteByDocumentID(documentID)
}

// GetLatestByDocumentID — получение последнего SMS по ID документа.
func (s *SMS_Service) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	return s.Repo.GetLatestByDocumentID(documentID)
}
