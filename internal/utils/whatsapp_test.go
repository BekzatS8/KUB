package utils

import "testing"

func TestSanitizeE164Digits(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "plus7", input: "+7 777 123-45-67", want: "77771234567"},
		{name: "leading8", input: "87771234567", want: "77771234567"},
		{name: "parentheses", input: "7(777)1234567", want: "77771234567"},
		{name: "kz_mobile", input: "77071234567", want: "77071234567"},
		{name: "too_short", input: "123", wantErr: true},
		{name: "too_long", input: "1234567890123456", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeE164Digits(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestWhatsAppTemplateRequired(t *testing.T) {
	client := NewWhatsAppClient("token", "phone", "v21.0", "", "ru", false)
	_, err := client.SendSMS("+77771234567", "Код подтверждения: 123456")
	if err == nil {
		t.Fatal("expected error when template is missing")
	}
}

func TestWhatsAppInvalidPhone(t *testing.T) {
	client := NewWhatsAppClient("token", "phone", "v21.0", "template", "ru", true)
	_, err := client.SendSMS("123", "Код подтверждения: 123456")
	if err == nil {
		t.Fatal("expected error for invalid phone")
	}
}
