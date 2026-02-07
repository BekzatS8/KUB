package services

import (
	"testing"
)

func TestVerificationBcryptRoundtrip(t *testing.T) {
	code := "123456"
	hash, err := HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash verification code: %v", err)
	}
	if err := CompareVerificationCode(hash, code); err != nil {
		t.Fatalf("expected bcrypt compare to succeed: %v", err)
	}
}

func TestVerificationWrongCode(t *testing.T) {
	hash, err := HashVerificationCode("123456")
	if err != nil {
		t.Fatalf("hash verification code: %v", err)
	}
	if err := CompareVerificationCode(hash, "999999"); err == nil {
		t.Fatalf("expected compare to fail for wrong code")
	}
}

func TestVerificationLeadingZeros(t *testing.T) {
	code := "005730"
	hash, err := HashVerificationCode(code)
	if err != nil {
		t.Fatalf("hash verification code: %v", err)
	}
	if err := CompareVerificationCode(hash, code); err != nil {
		t.Fatalf("expected compare to succeed for leading zeros: %v", err)
	}
}
