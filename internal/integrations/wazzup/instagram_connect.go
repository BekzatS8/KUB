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
// поддерживает. По документации Tech Partner API канал Instagram создаётся
// методом POST /v2/channels с transport=instagram и обязательными
// credentials: username, page_id, inst_id, user_token. Ссылки входа Wazzup
// партнёру не выдаёт — вход через Facebook проводит CRM своим приложением
// Meta (пакет integrations/meta) и передаёт сюда готовые данные.

// CreatedChannel — канал, созданный у провайдера.
type CreatedChannel struct {
	Channel Channel
	Raw     json.RawMessage
}

// channelCreator — клиенты, умеющие создавать канал через API.
type channelCreator interface {
	CreateChannel(ctx context.Context, transport string, credentials map[string]any) (*CreatedChannel, error)
}

// CreateChannel создаёт канал (POST /v2/channels). credentials = nil — пустой
// объект (WhatsApp, Telegram, WABA, MAX).
func (c *PartnerClient) CreateChannel(ctx context.Context, transport string, credentials map[string]any) (*CreatedChannel, error) {
	transport = strings.TrimSpace(transport)
	if transport == "" {
		return nil, fmt.Errorf("transport is required")
	}
	if credentials == nil {
		credentials = map[string]any{}
	}
	body, err := c.doJSON(ctx, http.MethodPost, "/v2/channels", map[string]any{
		"transport":   transport,
		"credentials": credentials,
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
	if len(items) > 0 {
		out.Channel = mapChannel(items[0])
	}
	return out, nil
}

// InstagramCredentials — данные из входа через Facebook для POST /v2/channels.
type InstagramCredentials struct {
	Username  string
	PageID    string
	InstID    string
	UserToken string
}

// InstagramConnectResult — ответ CRM на подключение Instagram.
type InstagramConnectResult struct {
	Account   string `json:"account"`
	ChannelID string `json:"channel_id,omitempty"`
	State     string `json:"state,omitempty"`
	Username  string `json:"username,omitempty"`
}

// ChannelCreateError — провайдер отказал в создании канала; Detail — его
// объяснение для администратора.
type ChannelCreateError struct {
	Detail string
	Err    error
}

func (e *ChannelCreateError) Error() string { return "wazzup channel create failed: " + e.Detail }
func (e *ChannelCreateError) Unwrap() error { return ErrUpstream }

// ConnectInstagram создаёт канал Instagram в аккаунте без кабинета по данным
// из входа через Facebook.
func (s *Service) ConnectInstagram(ctx context.Context, creds InstagramCredentials) (*InstagramConnectResult, error) {
	creds.Username = strings.TrimPrefix(strings.TrimSpace(creds.Username), "@")
	creds.PageID = strings.TrimSpace(creds.PageID)
	creds.InstID = strings.TrimSpace(creds.InstID)
	creds.UserToken = strings.TrimSpace(creds.UserToken)
	if creds.Username == "" || creds.PageID == "" || creds.InstID == "" || creds.UserToken == "" {
		return nil, fmt.Errorf("%w: нужны username, page_id, inst_id и user_token", ErrBadRequest)
	}
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

	created, err := creator.CreateChannel(ctx, "instagram", map[string]any{
		"username":   creds.Username,
		"page_id":    creds.PageID,
		"inst_id":    creds.InstID,
		"user_token": creds.UserToken,
	})
	if err != nil {
		log.Printf("integration=wazzup operation=instagram_connect status=failed account=%s username=%s page=%s err=%v",
			acc.Name, creds.Username, creds.PageID, err)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return nil, err
		}
		return nil, &ChannelCreateError{Detail: providerErrorDetail(err), Err: err}
	}
	log.Printf("integration=wazzup operation=instagram_connect status=ok account=%s channel=%s state=%s username=%s",
		acc.Name, created.Channel.ID, created.Channel.Status, creds.Username)

	// Новый канал сразу попадает в справочник CRM (и получает роли).
	if _, syncErr := s.syncAccountChannels(ctx, acc, integration); syncErr != nil {
		log.Printf("integration=wazzup operation=instagram_connect_sync status=failed err=%v", syncErr)
	}

	return &InstagramConnectResult{
		Account:   acc.Name,
		ChannelID: created.Channel.ID,
		State:     created.Channel.Status,
		Username:  creds.Username,
	}, nil
}

// ProviderErrorDetail — объяснение провайдера из ошибки (detail и errors из
// тела ответа), чтобы показать его в CRM, а не безликое «upstream error».
func ProviderErrorDetail(err error) string { return providerErrorDetail(err) }

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
