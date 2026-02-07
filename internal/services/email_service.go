package services

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gopkg.in/gomail.v2"
)

type EmailService interface {
	SendWelcomeEmail(email, companyName string) error
	SendPasswordResetEmail(email, resetURL string) error
	SendVerificationCode(toEmail, code string, ttlMinutes int) error
	SendSigningConfirm(email string, data SigningEmailData) error
}

type emailService struct {
	dialer   *gomail.Dialer
	from     string
	fromName string
}

type SigningEmailData struct {
	DocumentID   int64
	DocumentType string
	Sender       string
	MagicLink    string
	ExpiresAt    time.Time
}

func NewEmailService(smtpHost string, smtpPort int, smtpUser, smtpPassword, fromEmail, fromName string) EmailService {
	dialer := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPassword)
	return &emailService{
		dialer:   dialer,
		from:     fromEmail,
		fromName: fromName,
	}
}

func (s *emailService) SendWelcomeEmail(email, companyName string) error {
	m := gomail.NewMessage()
	setFromHeader(m, s.from, s.fromName)
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Welcome to TurCompany!")

	body := fmt.Sprintf(`
		<h2>Welcome to TurCompany, %s!</h2>
		<p>Thank you for registering with us. We're excited to have you on board.</p>
		<p>Your account has been successfully created.</p>
		<p>Best regards,<br>The TurCompany Team</p>
	`, companyName)

	m.SetBody("text/html", body)

	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send welcome email: %w", err)
	}

	return nil
}
func (s *emailService) SendPasswordResetEmail(email, resetURL string) error {
	m := gomail.NewMessage()
	setFromHeader(m, s.from, s.fromName)
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Password reset request")

	body := fmt.Sprintf(`
                <h3>Password reset requested</h3>
                <p>We received a request to reset the password for your account.</p>
                <p>Use the following link to reset your password: <a href="%s">Reset password</a></p>
                <p>If the button doesn't work, copy and paste this URL into your browser: %s</p>
                <p>If you did not request this change, you can ignore this email.</p>
        `, resetURL, resetURL)

	m.SetBody("text/html", body)

	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	return nil
}

func (s *emailService) SendVerificationCode(toEmail, code string, ttlMinutes int) error {
	if shouldLogVerificationCode() {
		log.Printf("[DEV][email][verify] to=%s code=%s ttl=%d", toEmail, code, ttlMinutes)
	}

	m := gomail.NewMessage()
	setFromHeader(m, s.from, s.fromName)
	m.SetHeader("To", toEmail)
	m.SetHeader("Subject", "Код подтверждения регистрации")

	text := fmt.Sprintf("Ваш код: %s. Действует %d минут.", code, ttlMinutes)
	html := fmt.Sprintf(`<h3>Код подтверждения регистрации</h3><p>Ваш код: <strong>%s</strong>.</p><p>Действует %d минут.</p>`, code, ttlMinutes)

	m.SetBody("text/plain", text)
	m.AddAlternative("text/html", html)

	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send verification email: %w", err)
	}
	return nil
}

func (s *emailService) SendSigningConfirm(email string, data SigningEmailData) error {
	m := gomail.NewMessage()
	setFromHeader(m, s.from, s.fromName)
	m.SetHeader("To", email)
	m.SetHeader("Subject", fmt.Sprintf("Подписание документа №%d", data.DocumentID))

	sender := strings.TrimSpace(data.Sender)
	if sender == "" {
		sender = "TurCompany"
	}
	docTitle := strings.TrimSpace(data.DocumentType)
	if docTitle == "" {
		docTitle = "документ"
	}
	expiresAt := data.ExpiresAt
	ttlMinutes := int(time.Until(expiresAt).Minutes())
	if ttlMinutes < 1 {
		ttlMinutes = 1
	}

	text := fmt.Sprintf(
		"Отправитель: %s.\nДокумент: %s.\nОткрыть и подписать: %s\nСрок действия: %d минут.",
		sender,
		docTitle,
		data.MagicLink,
		ttlMinutes,
	)
	html := fmt.Sprintf(
		`<h3>Подписание документа №%d</h3><p>Отправитель: %s</p><p>Документ: %s</p><p><a href="%s" style="display:inline-block;padding:12px 18px;background-color:#1a73e8;color:#ffffff;text-decoration:none;border-radius:4px;">Открыть и подписать</a></p><p>Ссылка: <a href="%s">%s</a></p><p>Срок действия: %d минут.</p>`,
		data.DocumentID,
		sender,
		docTitle,
		data.MagicLink,
		data.MagicLink,
		data.MagicLink,
		ttlMinutes,
	)
	m.SetBody("text/plain", text)
	m.AddAlternative("text/html", html)

	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send signing confirm email: %w", err)
	}
	return nil
}

func setFromHeader(m *gomail.Message, from, fromName string) {
	if strings.TrimSpace(fromName) == "" {
		m.SetHeader("From", from)
		return
	}
	m.SetAddressHeader("From", from, fromName)
}

func shouldLogVerificationCode() bool {
	return strings.ToLower(os.Getenv("GIN_MODE")) != "release"
}
