package wazzup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// Подключение Instagram к аккаунту без своего кабинета (дочерний White Label).
//
// Встроенная форма добавления канала (/v2/iframe-links/channels) Instagram не
// поддерживает. По документации Tech Partner API канал создаётся методом
// POST /v2/channels с transport=instagram, а в ответе поле url — «url канала
// или url авторизации канала»: по нему администратор входит через Facebook и
// выбирает бизнес-аккаунт Instagram. Способ instAPI требует своего
// приложения Meta с токенами — его не используем.

// CreatedChannel — канал, созданный у провайдера.
type CreatedChannel struct {
	Channel Channel
	// AuthURL — ссылка авторизации канала (для Instagram — вход через Facebook).
	AuthURL string
	Raw     json.RawMessage
}

// channelCreator — клиенты, умеющие создавать канал через API.
type channelCreator interface {
	CreateChannel(ctx context.Context, transport string) (*CreatedChannel, error)
}

// CreateChannel создаёт канал без учётных данных (POST /v2/channels).
func (c *PartnerClient) CreateChannel(ctx context.Context, transport string) (*CreatedChannel, error) {
	transport = strings.TrimSpace(transport)
	if transport == "" {
		return nil, fmt.Errorf("transport is required")
	}
	body, err := c.doJSON(ctx, http.MethodPost, "/v2/channels", map[string]any{
		"transport":   transport,
		"credentials": map[string]any{},
	})
	if err != nil {
		return nil, err
	}
	return parseCreatedChannel(body)
}

func parseCreatedChannel(body []byte) (*CreatedChannel, error) {
	var root any
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("decode create channel response: %w", err)
	}
	items := extractChannelItems(root)
	if len(items) == 0 {
		// Один канал может прийти объектом, а не списком.
		if m, ok := root.(map[string]any); ok {
			if data, ok := m["data"].(map[string]any); ok {
				items = []map[string]any{data}
			} else if firstString(m, "channel_id", "id", "guid") != "" {
				items = []map[string]any{m}
			}
		}
	}
	out := &CreatedChannel{Raw: append(json.RawMessage(nil), body...)}
	if len(items) == 0 {
		return out, nil
	}
	item := items[0]
	out.Channel = mapChannel(item)
	out.AuthURL = channelAuthURL(item)
	return out, nil
}

// channelAuthURL ищет ссылку авторизации: url (v2), initUrl (v1, в details).
func channelAuthURL(item map[string]any) string {
	if u := firstString(item, "url", "auth_url", "init_url", "initUrl"); u != "" {
		return u
	}
	if details, ok := item["details"].(map[string]any); ok {
		return firstString(details, "initUrl", "init_url", "url")
	}
	return ""
}

// InstagramConnectResult — ответ CRM на «Подключить Instagram».
type InstagramConnectResult struct {
	Account   string `json:"account"`
	ChannelID string `json:"channel_id,omitempty"`
	State     string `json:"state,omitempty"`
	AuthURL   string `json:"auth_url,omitempty"`
	// ProviderResponse — ответ Wazzup, если ссылки в нём не нашлось: по нему
	// видно, что провайдер вернул вместо неё.
	ProviderResponse string `json:"provider_response,omitempty"`
}

// ChannelCreateError — провайдер отказал в создании канала; Detail — его
// объяснение для администратора.
type ChannelCreateError struct {
	Detail string
	Err    error
}

func (e *ChannelCreateError) Error() string { return "wazzup channel create failed: " + e.Detail }
func (e *ChannelCreateError) Unwrap() error { return ErrUpstream }

// ConnectInstagram создаёт канал Instagram в аккаунте без кабинета и отдаёт
// ссылку авторизации.
func (s *Service) ConnectInstagram(ctx context.Context) (*InstagramConnectResult, error) {
	var acc *AccountConfig
	for _, a := range s.accounts {
		if a.Partner {
			acc = a
			break
		}
	}
	if acc == nil {
		return nil, ErrAccountNotConfigured
	}
	integration, err := s.connection(ctx, acc)
	if err != nil {
		return nil, err
	}
	creator, ok := acc.Client.(channelCreator)
	if !ok {
		return nil, fmt.Errorf("%w: аккаунт не умеет создавать каналы через API", ErrBadRequest)
	}

	created, err := creator.CreateChannel(ctx, "instagram")
	if err != nil {
		log.Printf("integration=wazzup operation=instagram_connect status=failed account=%s err=%v", acc.Name, err)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, &ChannelCreateError{Detail: providerErrorDetail(err), Err: err}
	}
	log.Printf("integration=wazzup operation=instagram_connect status=ok account=%s channel=%s state=%s has_auth_url=%t",
		acc.Name, created.Channel.ID, created.Channel.Status, created.AuthURL != "")

	// Новый канал сразу попадает в справочник CRM (и получает роли).
	if _, syncErr := s.syncAccountChannels(ctx, acc, integration); syncErr != nil {
		log.Printf("integration=wazzup operation=instagram_connect_sync status=failed err=%v", syncErr)
	}

	res := &InstagramConnectResult{
		Account:   acc.Name,
		ChannelID: created.Channel.ID,
		State:     created.Channel.Status,
		AuthURL:   created.AuthURL,
	}
	if res.AuthURL == "" {
		raw := string(created.Raw)
		if len(raw) > 2000 {
			raw = raw[:2000] + "…"
		}
		res.ProviderResponse = raw
	}
	return res, nil
}

// providerErrorDetail достаёт из ошибки провайдера его объяснение (detail и
// errors из тела ответа), чтобы показать администратору.
func providerErrorDetail(err error) string {
	msg := err.Error()
	i := strings.Index(msg, "body=")
	if i < 0 {
		return msg
	}
	body := msg[i+len("body="):]
	var parsed struct {
		Title  string   `json:"title"`
		Detail string   `json:"detail"`
		Errors []string `json:"errors"`
	}
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return strings.TrimSpace(body)
	}
	parts := []string{}
	if parsed.Detail != "" {
		parts = append(parts, parsed.Detail)
	} else if parsed.Title != "" {
		parts = append(parts, parsed.Title)
	}
	if len(parsed.Errors) > 0 {
		parts = append(parts, strings.Join(parsed.Errors, "; "))
	}
	if len(parts) == 0 {
		return strings.TrimSpace(body)
	}
	return strings.Join(parts, ": ")
}
