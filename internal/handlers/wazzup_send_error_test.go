package handlers

import (
	"fmt"
	"strings"
	"testing"

	wz "turcompany/internal/integrations/wazzup"
)

// Отказ Wazzup при «Написать первым» должен дойти до пользователя его же
// словами, а не безликим «upstream error».
func TestSendErrorShowsProviderReason(t *testing.T) {
	provider := fmt.Errorf(`wazzup partner POST /v2/messages failed: status=400 body={"detail":"Channel is not active"}`)
	err := fmt.Errorf("%w: %v", wz.ErrUpstream, provider)
	if got := wz.ProviderErrorDetail(err); !strings.Contains(got, "Channel is not active") {
		t.Fatalf("provider reason lost: %q", got)
	}
}
