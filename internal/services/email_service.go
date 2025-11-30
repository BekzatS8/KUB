package services

import (
	"fmt"
	"gopkg.in/gomail.v2"
)

type EmailService interface {
	SendWelcomeEmail(email, companyName string) error
	SendPasswordResetEmail(email, token string) error
}

type emailService struct {
	dialer *gomail.Dialer
	from   string
}

func NewEmailService(smtpHost string, smtpPort int, smtpUser, smtpPassword, fromEmail string) EmailService {
	dialer := gomail.NewDialer(smtpHost, smtpPort, smtpUser, smtpPassword)
	return &emailService{
		dialer: dialer,
		from:   fromEmail,
	}
}

func (s *emailService) SendWelcomeEmail(email, companyName string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
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

func (s *emailService) SendPasswordResetEmail(email, token string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.from)
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Password reset request")

	body := fmt.Sprintf(`
                <h3>Password reset requested</h3>
                <p>We received a request to reset the password for your account.</p>
                <p>Use the following token to reset your password: <strong>%s</strong></p>
                <p>If you did not request this change, you can ignore this email.</p>
        `, token)

	m.SetBody("text/html", body)

	if err := s.dialer.DialAndSend(m); err != nil {
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	return nil
}
