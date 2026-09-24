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

type staticTokenProvider struct {
	token      string
	calls      int
	invalidate int
}

func (p *staticTokenProvider) AccessToken(context.Context) (string, error) {
	p.calls++
	return p.token, nil
}

func (p *staticTokenProvider) InvalidateToken() { p.invalidate++ }

func newTestPartnerClient(t *testing.T, handler http.HandlerFunc) (*PartnerClient, *staticTokenProvider, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	tokens := &staticTokenProvider{token: "child-token"}
	return NewPartnerClient(srv.URL, tokens, time.Second, 0, time.Millisecond), tokens, srv.Close
}

// TestPartnerClientListChannelsMapsRealPayload — ответ снят с боевого
// GET /v2/channels дочернего аккаунта: поля там channel_id/state/status, а не
// id/status как в v3.
func TestPartnerClientListChannelsMapsRealPayload(t *testing.T) {
	const body = `{"data":[{"channel_id":"808e0654-d0f5-4926-a7fa-4783f1c05eeb",` +
		`"transport":"whatsapp","state":"qridle","status":"disabled",` +
		`"phone":"77009252505","messenger_id":"77009252505","name":"77009252505"}],` +
		`"meta":{"timestamp":1790264995}}`

	client, tokens, closeFn := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/channels" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer child-token" {
			t.Errorf("unexpected auth header %q", got)
		}
		_, _ = io.WriteString(w, body)
	})
	defer closeFn()

	channels, err := client.ListChannels(context.Background(), "ignored-api-key")
	if err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if len(channels) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(channels))
	}
	ch := channels[0]
	if ch.ID != "808e0654-d0f5-4926-a7fa-4783f1c05eeb" {
		t.Errorf("unexpected channel id %q", ch.ID)
	}
	if ch.Transport != "whatsapp" {
		t.Errorf("unexpected transport %q", ch.Transport)
	}
	if ch.Phone != "77009252505" {
		t.Errorf("unexpected phone %q", ch.Phone)
	}
	if ch.Status != "disabled" {
		t.Errorf("unexpected status %q", ch.Status)
	}
	if tokens.calls == 0 {
		t.Error("token provider was never asked for a token")
	}
}

// TestPartnerClientPatchWebhooksReconciles проверяет, что подписки приводятся к
// нужному URL: чего нет — создаётся, у чего URL разъехался — правится.
func TestPartnerClientPatchWebhooksReconciles(t *testing.T) {
	var created, updated []map[string]any

	client, _, closeFn := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/webhooks":
			// message.add уже подписан, но на устаревший URL; остальных нет.
			_, _ = io.WriteString(w, `{"data":[{"id":"sub-1","url":"https://old.example/hook","event":"message.add"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v2/webhooks":
			var payload struct {
				Data []map[string]any `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			created = payload.Data
			_, _ = io.WriteString(w, `{"data":[]}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/v2/webhooks":
			var payload struct {
				Data []map[string]any `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			updated = payload.Data
			_, _ = io.WriteString(w, `{"data":[]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})
	defer closeFn()

	const hook = "https://crm.example/integrations/wazzup/webhook/tok"
	if err := client.PatchWebhooks(context.Background(), "", hook, ""); err != nil {
		t.Fatalf("PatchWebhooks: %v", err)
	}

	if len(updated) != 1 || updated[0]["id"] != "sub-1" || updated[0]["url"] != hook {
		t.Fatalf("expected message.add subscription to be repointed, got %v", updated)
	}
	// Всего событий 5, одно уже существовало → создать должны 4.
	if len(created) != len(partnerWebhookEvents)-1 {
		t.Fatalf("expected %d new subscriptions, got %d", len(partnerWebhookEvents)-1, len(created))
	}
	for _, item := range created {
		if item["url"] != hook {
			t.Errorf("subscription created with wrong url: %v", item)
		}
	}
}

// TestPartnerClientSendMessageBuildsV2Payload — у v2 получатель вложен в
// recipient, а поля названы в snake_case (в v3 всё было плоско и camelCase).
func TestPartnerClientSendMessageBuildsV2Payload(t *testing.T) {
	var got map[string]any

	client, _, closeFn := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/messages" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = io.WriteString(w, `{"data":{"request_id":"req-42"}}`)
	})
	defer closeFn()

	resp, err := client.SendMessage(context.Background(), "", SendMessageRequest{
		ChannelID: "chan-1",
		ChatType:  "whatsapp",
		ChatID:    "77001112233",
		Text:      "привет",
		CRMUserID: "crm-7",
	})
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if resp.MessageID != "req-42" {
		t.Errorf("expected request_id as message id, got %q", resp.MessageID)
	}
	if got["channel_id"] != "chan-1" || got["text"] != "привет" || got["crm_user_id"] != "crm-7" {
		t.Errorf("unexpected payload: %v", got)
	}
	recipient, ok := got["recipient"].(map[string]any)
	if !ok {
		t.Fatalf("recipient is missing: %v", got)
	}
	if recipient["chat_type"] != "whatsapp" || recipient["chat_id"] != "77001112233" {
		t.Errorf("unexpected recipient: %v", recipient)
	}
}

// TestPartnerClientSendMessageRequiresChannel — в v2 channel_id обязателен,
// иначе провайдер не знает, с какого номера писать.
func TestPartnerClientSendMessageRequiresChannel(t *testing.T) {
	client, _, closeFn := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("request must not be sent without channel id")
	})
	defer closeFn()

	_, err := client.SendMessage(context.Background(), "", SendMessageRequest{
		ChatType: "whatsapp",
		ChatID:   "77001112233",
		Text:     "hi",
	})
	if err == nil || !strings.Contains(err.Error(), "channelId") {
		t.Fatalf("expected channelId error, got %v", err)
	}
}

// TestPartnerClientRefreshesTokenOn401 — токен живёт час, но может быть отозван
// раньше; тогда кэш надо сбросить и повторить запрос.
func TestPartnerClientRefreshesTokenOn401(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"OAUTH_TOKEN_UNKNOWN_OR_REVOKED"}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer srv.Close()

	tokens := &staticTokenProvider{token: "child-token"}
	client := NewPartnerClient(srv.URL, tokens, time.Second, 1, time.Millisecond)

	if _, err := client.ListChannels(context.Background(), ""); err != nil {
		t.Fatalf("ListChannels: %v", err)
	}
	if tokens.invalidate != 1 {
		t.Errorf("expected token cache to be invalidated once, got %d", tokens.invalidate)
	}
	if attempts != 2 {
		t.Errorf("expected a retry after 401, got %d attempts", attempts)
	}
}
