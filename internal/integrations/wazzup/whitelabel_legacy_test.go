package wazzup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Окно подключения Instagram — старое API партнёров (поддержка Wazzup):
// POST /iframe/generate-channels-link {transport: instAPI, channelId?},
// авторизация API-ключом дочернего аккаунта, ответ {link}.
func TestLegacyChannelsLink(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.Method+" "+r.URL.Path, r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		gotBody = nil
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = io.WriteString(w, `{"link":"https://app.wazzup24.com/1234/tech-partner/channel?token=t"}`)
	}))
	defer srv.Close()

	c := NewWhiteLabelClient(WhiteLabelConfig{BaseURL: srv.URL, APIKey: "child-key"}, 5*time.Second)
	link, err := c.LegacyChannelsLink(context.Background(), "instAPI", "35778b22")
	if err != nil {
		t.Fatal(err)
	}
	if link != "https://app.wazzup24.com/1234/tech-partner/channel?token=t" {
		t.Fatalf("unexpected link %q", link)
	}
	if gotPath != "POST /iframe/generate-channels-link" || gotAuth != "Bearer child-key" ||
		gotBody["transport"] != "instAPI" || gotBody["channelId"] != "35778b22" {
		t.Fatalf("unexpected request %s auth=%q body=%v", gotPath, gotAuth, gotBody)
	}

	// Новый канал — без channelId.
	if _, err := c.LegacyChannelsLink(context.Background(), "instAPI", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotBody["channelId"]; ok {
		t.Fatalf("new channel must not send channelId, got %v", gotBody)
	}
}

// Без API-ключа и без доступов White Label — понятная ошибка, без запроса.
func TestLegacyChannelsLinkNeedsAuth(t *testing.T) {
	c := NewWhiteLabelClient(WhiteLabelConfig{BaseURL: "http://127.0.0.1:1"}, time.Second)
	if _, err := c.LegacyChannelsLink(context.Background(), "instAPI", ""); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected not configured error, got %v", err)
	}
}
