package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
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

// Настройки безопасности
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

type SMSClient interface {
	SendSMS(to, code string) (*utils.SendSMSResponse, error)
}

// Удаляем дублирующиеся объявления интерфейсов
// Используем уже объявленные в document_service.go

var (
	_ SMSConfirmationRepo  = (*repositories.SMSConfirmationRepository)(nil)
	_ UserVerificationRepo = (*repositories.UserVerificationRepository)(nil)
)

// SignatureMetadata - метаданные для юридически значимой подписи
type SignatureMetadata struct {
	IPAddress       string    `json:"ip_address"`
	UserAgent       string    `json:"user_agent"`
	DeviceInfo      string    `json:"device_info"`
	Timestamp       time.Time `json:"timestamp"`
	DocumentID      int64     `json:"document_id"`
	ClientID        int       `json:"client_id"`
	ConsentShown    bool      `json:"consent_shown"`
	LegalText       string    `json:"legal_text"`
	SignatureMethod string    `json:"signature_method"`
}

type SMS_Service struct {
	Repo    SMSConfirmationRepo
	DocRepo DocumentRepo // Используем интерфейс из document_service.go
	Client  SMSClient
	CodeTTL time.Duration
	now     func() time.Time

	// Для юридической значимости
	ClientRepo ClientRepo
	DealRepo   DealRepo
}

func NewSMSService(
	docRepo SMSConfirmationRepo,
	client SMSClient,
	docRepo2 DocumentRepo, // Переименуем для ясности
	now func() time.Time,
) *SMS_Service {
	if now == nil {
		now = time.Now
	}
	return &SMS_Service{
		Repo:    docRepo,
		Client:  client,
		DocRepo: docRepo2,
		CodeTTL: DefaultVerificationTTL,
		now:     now,
	}
}

// SetAdditionalRepos устанавливает дополнительные репозитории для юридической подписи
func (s *SMS_Service) SetAdditionalRepos(clientRepo ClientRepo, dealRepo DealRepo) {
	s.ClientRepo = clientRepo
	s.DealRepo = dealRepo
}

// BuildLegalSignatureMessage строит юридически значимое сообщение
func (s *SMS_Service) BuildLegalSignatureMessage(documentID int64, code string) (string, error) {
	if s.ClientRepo == nil || s.DealRepo == nil {
		return fmt.Sprintf("Код подтверждения: %s", code), nil
	}

	doc, err := s.DocRepo.GetByID(documentID)
	if err != nil {
		return fmt.Sprintf("Код подтверждения: %s", code), nil
	}

	deal, err := s.DealRepo.GetByID(int(doc.DealID))
	if err != nil || deal == nil {
		return fmt.Sprintf("Код подтверждения: %s", code), nil
	}

	client, err := s.ClientRepo.GetByID(deal.ClientID)
	if err != nil || client == nil {
		return fmt.Sprintf("Код подтверждения: %s", code), nil
	}

	// Форматируем имя клиента
	clientName := client.Name
	if client.LastName != "" && client.FirstName != "" {
		clientName = fmt.Sprintf("%s %s", client.LastName, client.FirstName)
	}

	// Определяем название документа
	docNames := map[string]string{
		"contract":               "Договор оказания услуг",
		"contract_full":          "Договор оказания услуг (полная оплата)",
		"contract_50_50":         "Договор оказания услуг (50/50)",
		"personal_data_consent":  "Согласие на обработку персональных данных",
		"refund_application":     "Заявление на возврат средств",
		"pause_application":      "Заявление на приостановку услуг",
		"additional_agreement":   "Дополнительное соглашение",
		"refund_receipt_full":    "Акт возврата (полный)",
		"refund_receipt_partial": "Акт возврата (частичный)",
	}

	docName := docNames[doc.DocType]
	if docName == "" {
		docName = "Документ"
	}

	// Юридический текст
	legalText := `ВВОДЯ КОД ПОДТВЕРЖДЕНИЯ, ВЫ:
1. Подтверждаете ознакомление с документом «%s»
2. Соглашаетесь со всеми условиями документа
3. Признаете документ юридически значимым
4. Подтверждаете свою личность
5. Принимаете ответственность за последствия

КОД ДЕЙСТВИТЕЛЕН 10 МИНУТ`

	legalNotice := fmt.Sprintf(legalText, docName)

	// Строим полное сообщение
	message := fmt.Sprintf(
		"%s, здравствуйте!\n\n"+
			"Вам направлен на подписание документ: «%s»\n\n"+
			"КОД ПОДТВЕРЖДЕНИЯ: %s\n\n"+
			"%s\n\n"+
			"НИКОМУ НЕ СООБЩАЙТЕ ЭТОТ КОД!",
		clientName,
		docName,
		code,
		legalNotice,
	)

	return message, nil
}

// SendLegalSignature отправляет юридически значимый запрос на подпись
func (s *SMS_Service) SendLegalSignature(documentID int64, phone string, userID, roleID int) error {
	log.Printf("[signature][send][start] doc=%d user=%d role=%d phone=%q",
		documentID, userID, roleID, phone,
	)

	// 1. Валидация и нормализация телефона
	toDigits, err := utils.SanitizeE164Digits(phone)
	if err != nil {
		log.Printf("[signature][send][invalid_phone] doc=%d phone=%q err=%v",
			documentID, phone, err,
		)
		return fmt.Errorf("invalid phone: %w", err)
	}

	// 2. Генерация кода
	code := GenerateVerificationCode()

	// 3. Построение юридически значимого сообщения
	text, err := s.BuildLegalSignatureMessage(documentID, code)
	if err != nil {
		log.Printf("[signature][send][build_message_error] doc=%d err=%v", documentID, err)
		text = fmt.Sprintf("Код подтверждения для документа: %s", code)
	}

	log.Printf("[signature][send][message] doc=%d message_length=%d",
		documentID, len(text))

	// 4. Отправка через клиент
	if s.Client != nil {
		log.Printf("[signature][send][provider_call] doc=%d phone=%s",
			documentID, toDigits)

		resp, err := s.Client.SendSMS(toDigits, text)
		if err != nil {
			log.Printf("[signature][send][provider_error] doc=%d phone=%s err=%v",
				documentID, toDigits, err,
			)
			return fmt.Errorf("signature provider error: %w", err)
		}

		log.Printf("[signature][send][provider_success] doc=%d message_id=%s",
			documentID, resp.Data.MessageID)
	} else {
		log.Printf("[signature][send][skip] doc=%d reason=client_is_nil", documentID)
	}

	// 5. Хэш кода
	codeHash, err := HashVerificationCode(code)
	if err != nil {
		log.Printf("[signature][send][hash_error] doc=%d err=%v", documentID, err)
		return err
	}

	// 6. TTL
	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
	}

	now := s.now()

	rec := &models.SMSConfirmation{
		DocumentID:   documentID,
		Phone:        toDigits,
		CodeHash:     string(codeHash),
		SentAt:       now,
		ExpiresAt:    now.Add(ttl),
		Attempts:     0,
		Confirmed:    false,
		ConfirmedAt:  time.Time{},
		LastResendAt: &now,
		ResendCount:  0,
	}

	// 7. Сохранение в БД
	if _, err := s.Repo.Create(rec); err != nil {
		log.Printf("[signature][send][db_error] doc=%d phone=%s err=%v",
			documentID, toDigits, err,
		)
		return fmt.Errorf("db error after signature: %w", err)
	}

	// 8. Обновление статуса документа
	if err := s.DocRepo.UpdateStatus(documentID, "sent_for_signature"); err != nil {
		log.Printf("[signature][send][update_status_error] doc=%d err=%v",
			documentID, err)
		// Не прерываем процесс
	}

	log.Printf("[signature][send][success] doc=%d phone=%s", documentID, toDigits)
	return nil
}

// ConfirmCodeWithMetadata подтверждает код с сохранением метаданных для юридической значимости
func (s *SMS_Service) ConfirmCodeWithMetadata(documentID int64, code string, ip, userAgent string) (bool, error) {
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

	// уже подтверждён – считаем это УСПЕХОМ
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

	// Сохраняем метаданные подписи в документ
	if err := s.saveSignatureMetadata(documentID, ip, userAgent); err != nil {
		log.Printf("[signature][metadata_error] failed to save metadata: %v", err)
		// Не прерываем процесс из-за ошибки метаданных
	}

	// Обновляем статус документа на "signed"
	if err := s.DocRepo.UpdateStatus(documentID, "signed"); err != nil {
		log.Printf("[signature][status_error] failed to update status: %v", err)
		return false, err
	}

	return true, nil
}

// saveSignatureMetadata сохраняет метаданные юридически значимой подписи
func (s *SMS_Service) saveSignatureMetadata(documentID int64, ip, userAgent string) error {
	doc, err := s.DocRepo.GetByID(documentID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}

	// Получаем информацию о клиенте для метаданных
	var clientID int
	if s.DealRepo != nil {
		deal, err := s.DealRepo.GetByID(int(doc.DealID))
		if err == nil && deal != nil {
			clientID = deal.ClientID
		}
	}

	// Определяем устройство
	deviceInfo := "Unknown"
	if strings.Contains(userAgent, "Mobile") {
		if strings.Contains(userAgent, "iPhone") {
			deviceInfo = "iPhone"
		} else if strings.Contains(userAgent, "Android") {
			deviceInfo = "Android Phone"
		} else {
			deviceInfo = "Mobile Device"
		}
	} else if strings.Contains(userAgent, "Windows") {
		deviceInfo = "Windows PC"
	} else if strings.Contains(userAgent, "Mac") {
		deviceInfo = "Mac"
	} else if strings.Contains(userAgent, "Linux") {
		deviceInfo = "Linux PC"
	}

	// Юридический текст
	legalText := `Вводя код подтверждения, Вы:
1. Подтверждаете ознакомление с документом
2. Соглашаетесь со всеми условиями документа
3. Признаете документ юридически значимым
4. Подтверждаете свою личность
5. Несете ответственность за последствия`

	// Создаем метаданные
	metadata := SignatureMetadata{
		IPAddress:       ip,
		UserAgent:       userAgent,
		DeviceInfo:      deviceInfo,
		Timestamp:       s.now(),
		DocumentID:      documentID,
		ClientID:        clientID,
		ConsentShown:    true,
		LegalText:       legalText,
		SignatureMethod: "sms_otp",
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Обновляем документ
	doc.SignMethod = "sms_otp"
	doc.SignIP = ip
	doc.SignUserAgent = userAgent
	doc.SignMetadata = string(metadataJSON)
	now := s.now()
	doc.SignedAt = &now

	if err := s.DocRepo.Update(doc); err != nil {
		return fmt.Errorf("failed to update document with metadata: %w", err)
	}

	log.Printf("[signature][metadata_saved] doc=%d ip=%s device=%s",
		documentID, ip, deviceInfo)

	return nil
}

// ================== Остальные методы (оставляем как есть, но добавляем вызов нового метода) ==================

func buildOTPMessage(code string) string {
	return fmt.Sprintf("Код подтверждения: %s", code)
}

func (s *SMS_Service) SendSMS(documentID int64, phone string, userID, roleID int) error {
	// Используем новый метод юридической подписи
	return s.SendLegalSignature(documentID, phone, userID, roleID)
}

func (s *SMS_Service) ResendSMS(documentID int64, phone string, userID, roleID int) error {
	// Используем существующую логику, но с юридическим сообщением
	existing, err := s.Repo.GetLatestByDocumentID(documentID)
	if err != nil {
		return err
	}

	if existing == nil || existing.Confirmed || s.IsCodeExpired(existing.SentAt) {
		if phone == "" {
			return fmt.Errorf("phone required for first/expired resend")
		}
		return s.SendLegalSignature(documentID, phone, userID, roleID)
	}

	now := s.now()
	if now.Sub(existing.SentAt) <= resendWindow && existing.ResendCount >= maxResendsPerWindow {
		return ErrResendThrottled
	}

	if phone == "" {
		phone = existing.Phone
	}

	code := GenerateVerificationCode()
	text, err := s.BuildLegalSignatureMessage(documentID, code)
	if err != nil {
		text = buildOTPMessage(code)
	}

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

	if s.Client != nil {
		if _, err := s.Client.SendSMS(existing.Phone, text); err != nil {
			return fmt.Errorf("resend error: %w", err)
		}
	}

	log.Printf("[sms][doc][resend] doc_id=%d phone=%s", documentID, existing.Phone)
	return nil
}

// ConfirmCode - старая версия для совместимости
func (s *SMS_Service) ConfirmCode(documentID int64, code string, userID, roleID int) (bool, error) {
	// Используем новую версию без метаданных
	return s.ConfirmCodeWithMetadata(documentID, code, "", "")
}

func (s *SMS_Service) IsCodeExpired(sentAt time.Time) bool {
	ttl := s.CodeTTL
	if ttl <= 0 {
		ttl = DefaultVerificationTTL
	}
	return s.now().After(sentAt.Add(ttl))
}

func (s *SMS_Service) DeleteConfirmation(documentID int64) error {
	return s.Repo.DeleteByDocumentID(documentID)
}

func (s *SMS_Service) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	return s.Repo.GetLatestByDocumentID(documentID)
}
