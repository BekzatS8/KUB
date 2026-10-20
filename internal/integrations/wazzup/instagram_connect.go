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
	// AuthURL — «url канала или url авторизации канала» из ответа: для
	// Instagram — вход через приложение Wazzup в Facebook.
	AuthURL string
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
		out.AuthURL = channelAuthURL(items[0])
	}
	return out, nil
}

// channelAuthURL ищет ссылку авторизации канала: url (v2) или initUrl (v1).
func channelAuthURL(item map[string]any) string {
	if u := firstString(item, "url", "auth_url", "init_url", "initUrl"); u != "" {
		return u
	}
	if details, ok := item["details"].(map[string]any); ok {
		return firstString(details, "initUrl", "init_url", "url")
	}
	return ""
}

// rejectsInstagramCredentials — Wazzup не принимает данные входа в
// credentials («property … should not exist»): вопреки документации для
// transport=instagram он ждёт пустой объект и сам отдаёт ссылку авторизации.
func rejectsInstagramCredentials(err error) bool {
	return err != nil && strings.Contains(err.Error(), "should not exist")
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
	// AuthURL — Wazzup просит завершить подключение у себя (вход в Facebook
	// через его приложение).
	AuthURL string `json:"auth_url,omitempty"`
	// ProviderResponse — ответ Wazzup, если канал создан без ссылки и не
	// активен: по нему видно, чего он ждёт.
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
	if rejectsInstagramCredentials(err) {
		// Боевой API отвергает поля из документации — создаём канал с пустыми
		// credentials, Wazzup отдаёт свою ссылку авторизации.
		log.Printf("integration=wazzup operation=instagram_connect status=retry_empty_credentials account=%s err=%v", acc.Name, err)
		created, err = creator.CreateChannel(ctx, "instagram", nil)
	}
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

	res := &InstagramConnectResult{
		Account:   acc.Name,
		ChannelID: created.Channel.ID,
		State:     created.Channel.Status,
		Username:  creds.Username,
		AuthURL:   created.AuthURL,
	}
	if res.AuthURL == "" && !strings.EqualFold(res.State, "active") {
		raw := string(created.Raw)
		if len(raw) > 2000 {
			raw = raw[:2000] + "…"
		}
		res.ProviderResponse = raw
	}
	return res, nil
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
		Code   string            `json:"code"`
		Title  string            `json:"title"`
		Detail string            `json:"detail"`
		Errors []json.RawMessage `json:"errors"`
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
	// errors бывают строками и объектами {code, description}.
	items := []string{}
	for _, raw := range parsed.Errors {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			if str != "" && str != parsed.Detail {
				items = append(items, str)
			}
			continue
		}
		var obj struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		}
		if json.Unmarshal(raw, &obj) == nil && obj.Description != "" && obj.Description != parsed.Detail {
			items = append(items, obj.Description)
		}
	}
	if len(items) > 0 {
		parts = append(parts, strings.Join(items, "; "))
	}
	if parsed.Code != "" && len(parts) > 0 {
		parts[len(parts)-1] += " (" + parsed.Code + ")"
	}
	if len(parts) == 0 {
		return strings.TrimSpace(body)
	}
	return strings.Join(parts, ": ")
}
