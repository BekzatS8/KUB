package services

import (
	"fmt"
	"math/rand"
	"time"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/utils"
)

type SMS_Service struct {
	Repo   *repositories.SMSConfirmationRepository
	Client *utils.Client
}

func NewSMSService(repo *repositories.SMSConfirmationRepository, client *utils.Client) *SMS_Service {
	return &SMS_Service{Repo: repo, Client: client}
}

func (s *SMS_Service) generateCode() string {
	src := rand.NewSource(time.Now().UnixNano())
	rnd := rand.New(src)
	return fmt.Sprintf("%06d", rnd.Intn(1000000))
}

func (s *SMS_Service) SendSMS(documentID int64, phone string) error {
	code := s.generateCode()
	text := fmt.Sprintf("Код подтверждения: %s", code)

	resp, err := s.Client.SendSMS(phone, text)
	if err != nil {
		return fmt.Errorf("mobizon error: %w", err)
	}

	sms := &models.SMSConfirmation{
		DocumentID:  documentID,
		Phone:       phone,
		SMSCode:     code,
		SentAt:      time.Now(),
		Confirmed:   false,
		ConfirmedAt: time.Time{},
	}

	fmt.Printf("📦 Inserting SMS: doc_id=%d phone=%s code=%s\n", documentID, phone, code)
	_, err = s.Repo.Create(sms)
	if err != nil {
		return fmt.Errorf("db error after SMS: %w", err)
	}

	fmt.Printf("✅ SMS sent to %s with code %s [MobizonMessageID: %s]\n", phone, code, resp.Data.MessageID)
	return nil
}

func (s *SMS_Service) ResendSMS(documentID int64, phone string) error {
	existing, err := s.Repo.GetLatestByDocumentID(documentID)
	if err != nil {
		return err
	}

	if existing == nil {
		if phone == "" {
			return fmt.Errorf("номер телефона обязателен при первом отправлении")
		}
		return s.SendSMS(documentID, phone)
	}

	if existing.Confirmed || s.IsCodeExpired(existing.SentAt) {
		return s.SendSMS(documentID, existing.Phone)
	}

	text := fmt.Sprintf("Код подтверждения: %s", existing.SMSCode)
	_, err = s.Client.SendSMS(existing.Phone, text)
	if err != nil {
		return fmt.Errorf("resend error: %w", err)
	}

	fmt.Printf("🔁 Resent SMS to %s with existing code %s\n", existing.Phone, existing.SMSCode)
	return nil
}

func (s *SMS_Service) ConfirmCode(documentID int64, code string) (bool, error) {
	sms, err := s.Repo.GetByDocumentIDAndCode(documentID, code)
	if err != nil {
		return false, err
	}
	if sms == nil || sms.Confirmed || s.IsCodeExpired(sms.SentAt) {
		return false, nil
	}

	sms.Confirmed = true
	sms.ConfirmedAt = time.Now()
	return true, s.Repo.Update(sms)
}

func (s *SMS_Service) IsCodeExpired(sentAt time.Time) bool {
	return time.Now().After(sentAt.Add(5 * time.Minute))
}

func (s *SMS_Service) DeleteConfirmation(documentID int64) error {
	return s.Repo.DeleteByDocumentID(documentID)
}

func (s *SMS_Service) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	return s.Repo.GetLatestByDocumentID(documentID)
}
