package services

import (
	"errors"
	"strings"
	"testing"

	"turcompany/internal/models"
)

// При массовом одобрении в Ленте админ раньше видел только «Одобрено: 0, с
// ошибкой: 4» — причина не доходила ни до интерфейса, ни до логов. Проверяем,
// что технические ошибки применения превращаются в понятный текст.
func TestTranslateFeedApplyError(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"not found", "не найден"},
		{"forbidden", "недостаточно прав"},
		{"invalid status", "в другом статусе"},
		{"signer phone is required", "телефон"},
		{"signer email is required", "e-mail"},
		{"unsupported signing channel", "канал отправки"},
		{"send signing sms: context deadline exceeded", "время ожидания"},
	}
	for _, tc := range cases {
		got := translateFeedApplyError(errors.New(tc.in))
		if got == nil || !strings.Contains(got.Error(), tc.want) {
			t.Fatalf("translateFeedApplyError(%q) = %v, want text containing %q", tc.in, got, tc.want)
		}
	}
	if translateFeedApplyError(nil) != nil {
		t.Fatal("nil error must stay nil")
	}
	// Неизвестная ошибка должна пройти как есть, а не потеряться.
	orig := errors.New("boom: db is on fire")
	if got := translateFeedApplyError(orig); got.Error() != orig.Error() {
		t.Fatalf("unknown error must pass through, got %v", got)
	}
}

func TestFeedEventActionLabel(t *testing.T) {
	if label := feedEventActionLabel(models.FeedEventTypePendingSendDocument); !strings.Contains(label, "подпись") {
		t.Fatalf("send-document label should mention подпись, got %q", label)
	}
	if label := feedEventActionLabel("something_else"); label == "" {
		t.Fatal("unknown event type must still produce a label")
	}
}
