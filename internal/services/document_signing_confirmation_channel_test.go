package services

import (
	"context"
	"testing"
	"time"
)

type fakeSMSSender struct{}

func (s *fakeSMSSender) Send(_ context.Context, _ SMSMessage) (*SMSResult, error) {
	return &SMSResult{Provider: "test", ProviderMessageID: "1"}, nil
}

func TestStartSigningByChannel_RoutesToSMS(t *testing.T) {
	svc := NewDocumentSigningConfirmationService(
		&fakeConfirmRepo{},
		&fakeUserRepo{},
		&fakeDocLookup{},
		nil,
		nil,
		nil,
		DocumentSigningConfirmationConfig{SMSTTL: 15 * time.Minute, SMSVerifyBaseURL: "http://localhost:4000"},
		time.Now,
	)
	svc.SetSMSSender(&fakeSMSSender{})
	result, err := svc.StartSigningByChannel(context.Background(), "sms", 10, 20, "+77001234567", "")
	if err != nil {
		t.Fatalf("StartSigningByChannel sms error: %v", err)
	}
	if len(result.Channels) != 1 || result.Channels[0].Channel != "sms" {
		t.Fatalf("expected sms channel result, got %+v", result.Channels)
	}
}

func TestStartSigningByChannel_RoutesToEmail(t *testing.T) {
	svc := NewDocumentSigningConfirmationService(
		&fakeConfirmRepo{},
		&fakeUserRepo{},
		&fakeDocLookup{},
		nil,
		&fakeEmailSender{},
		nil,
		DocumentSigningConfirmationConfig{EmailTTL: 15 * time.Minute, EmailVerifyBaseURL: "http://localhost:4000"},
		time.Now,
	)
	result, err := svc.StartSigningByChannel(context.Background(), "email", 10, 20, "", "signer@example.com")
	if err != nil {
		t.Fatalf("StartSigningByChannel email error: %v", err)
	}
	if len(result.Channels) != 1 || result.Channels[0].Channel != "email" {
		t.Fatalf("expected email channel result, got %+v", result.Channels)
	}
}
