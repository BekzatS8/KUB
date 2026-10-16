// Package meta — вход через Facebook (приложение Meta компании) для
// подключения Instagram к Wazzup.
//
// Wazzup Tech Partner API создаёт канал Instagram только с готовыми данными
// (POST /v2/channels, credentials: username, page_id, inst_id, user_token) —
// своей ссылки входа для партнёра у него нет. Поэтому вход проводит CRM:
// администратор входит через Facebook, CRM получает токен пользователя и
// список страниц с привязанными бизнес-аккаунтами Instagram.
package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultGraphVersion = "v23.0"
	// DefaultScopes — права на страницы, бизнес-портфолио и Instagram, как у
	// входа в кабинете Wazzup: переписка, комментарии, маршрутизация сообщений.
	DefaultScopes = "pages_show_list,pages_read_engagement,pages_manage_metadata,pages_messaging," +
		"instagram_basic,instagram_manage_messages,instagram_manage_comments,business_management"
)

var ErrNotConfigured = errors.New("meta app is not configured")

type Config struct {
	AppID        string
	AppSecret    string
	GraphVersion string
	Scopes       string
	// RedirectURL — страница CRM, куда Facebook вернёт code. Должна совпадать
	// с «Valid OAuth Redirect URIs» в настройках приложения.
	RedirectURL string
}

type Client struct {
	cfg       Config
	http      *http.Client
	graphBase string
	webBase   string
}

func NewClient(cfg Config, timeout time.Duration) *Client {
	cfg.AppID = strings.TrimSpace(cfg.AppID)
	cfg.AppSecret = strings.TrimSpace(cfg.AppSecret)
	cfg.RedirectURL = strings.TrimSpace(cfg.RedirectURL)
	if v := strings.TrimSpace(cfg.GraphVersion); v != "" {
		cfg.GraphVersion = v
	} else {
		cfg.GraphVersion = DefaultGraphVersion
	}
	if s := strings.TrimSpace(cfg.Scopes); s != "" {
		cfg.Scopes = s
	} else {
		cfg.Scopes = DefaultScopes
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{
		cfg:       cfg,
		http:      &http.Client{Timeout: timeout},
		graphBase: "https://graph.facebook.com",
		webBase:   "https://www.facebook.com",
	}
}

// Configured — заданы App ID, секрет и адрес возврата.
func (c *Client) Configured() bool {
	return c != nil && c.cfg.AppID != "" && c.cfg.AppSecret != "" && c.cfg.RedirectURL != ""
}

// DialogURL — ссылка на окно входа Facebook.
func (c *Client) DialogURL(state string) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	q := url.Values{}
	q.Set("client_id", c.cfg.AppID)
	q.Set("redirect_uri", c.cfg.RedirectURL)
	q.Set("state", state)
	q.Set("scope", c.cfg.Scopes)
	q.Set("response_type", "code")
	return c.webBase + "/" + c.cfg.GraphVersion + "/dialog/oauth?" + q.Encode(), nil
}

// ExchangeCode меняет code на долгоживущий (≈60 дней) токен пользователя.
func (c *Client) ExchangeCode(ctx context.Context, code string) (string, error) {
	if !c.Configured() {
		return "", ErrNotConfigured
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", fmt.Errorf("facebook code is empty")
	}
	var short struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.get(ctx, "/oauth/access_token", url.Values{
		"client_id":     {c.cfg.AppID},
		"client_secret": {c.cfg.AppSecret},
		"redirect_uri":  {c.cfg.RedirectURL},
		"code":          {code},
	}, &short); err != nil {
		return "", err
	}
	if short.AccessToken == "" {
		return "", fmt.Errorf("facebook returned no access token")
	}
	var long struct {
		AccessToken string `json:"access_token"`
	}
	if err := c.get(ctx, "/oauth/access_token", url.Values{
		"grant_type":        {"fb_exchange_token"},
		"client_id":         {c.cfg.AppID},
		"client_secret":     {c.cfg.AppSecret},
		"fb_exchange_token": {short.AccessToken},
	}, &long); err != nil || long.AccessToken == "" {
		// Короткий токен тоже рабочий — Wazzup получит хотя бы его.
		return short.AccessToken, nil
	}
	return long.AccessToken, nil
}

// InstagramAccount — страница Facebook с привязанным бизнес-аккаунтом Instagram.
type InstagramAccount struct {
	PageID   string `json:"page_id"`
	PageName string `json:"page_name"`
	InstID   string `json:"inst_id"`
	Username string `json:"username"`
}

// InstagramAccounts — страницы пользователя, у которых есть бизнес-аккаунт
// Instagram. Страницы без Instagram пропускаются.
func (c *Client) InstagramAccounts(ctx context.Context, userToken string) ([]InstagramAccount, error) {
	out := []InstagramAccount{}
	params := url.Values{
		"fields":       {"id,name,instagram_business_account{id,username}"},
		"limit":        {"100"},
		"access_token": {userToken},
	}
	after := ""
	for page := 0; page < 10; page++ {
		if after != "" {
			params.Set("after", after)
		}
		var resp struct {
			Data []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				IG   *struct {
					ID       string `json:"id"`
					Username string `json:"username"`
				} `json:"instagram_business_account"`
			} `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := c.get(ctx, "/me/accounts", params, &resp); err != nil {
			return nil, err
		}
		for _, p := range resp.Data {
			if p.IG == nil || p.IG.ID == "" {
				continue
			}
			out = append(out, InstagramAccount{PageID: p.ID, PageName: p.Name, InstID: p.IG.ID, Username: p.IG.Username})
		}
		if resp.Paging.Next == "" || resp.Paging.Cursors.After == "" {
			break
		}
		after = resp.Paging.Cursors.After
	}
	return out, nil
}

// GraphError — отказ Graph API его же словами.
type GraphError struct {
	Status  int
	Message string
	Code    int
}

func (e *GraphError) Error() string {
	return fmt.Sprintf("facebook graph error status=%d code=%d: %s", e.Status, e.Code, e.Message)
}

func (c *Client) get(ctx context.Context, path string, params url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.graphBase+"/"+c.cfg.GraphVersion+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		// url.Error печатает адрес целиком, а в query — секрет и токен.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("facebook graph request %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Message string `json:"message"`
				Code    int    `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		return &GraphError{Status: resp.StatusCode, Message: msg, Code: e.Error.Code}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode facebook graph response: %w", err)
	}
	return nil
}
