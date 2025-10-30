package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type TelegramService struct {
	token   string
	baseURL string
	client  *http.Client
}
type tgReplyKeyboardMarkup struct {
	Keyboard        [][]tgKeyboardButton `json:"keyboard"`
	ResizeKeyboard  bool                 `json:"resize_keyboard"`
	OneTimeKeyboard bool                 `json:"one_time_keyboard"`
}
type tgKeyboardButton struct {
	Text string `json:"text"`
}

func NewTelegramService(botToken string) *TelegramService {
	return &TelegramService{
		token:   botToken,
		baseURL: fmt.Sprintf("https://api.telegram.org/bot%s", botToken),
		client:  &http.Client{},
	}
}

type tgResp struct {
	Ok          bool            `json:"ok"`
	Description string          `json:"description"`
	Result      json.RawMessage `json:"result"`
}

func (t *TelegramService) SendMessage(chatID int64, text string) error {
	if t == nil || t.token == "" || chatID == 0 {
		log.Printf("[tg][skip] token or chatID empty (token? %v chatID=%d)", t != nil && t.token != "", chatID)
		return nil
	}
	body := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
	}
	b, _ := json.Marshal(body)
	url := t.baseURL + "/sendMessage"
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[tg][send] url=%s chatID=%d text=%q", url, chatID, text)
	resp, err := t.client.Do(req)
	if err != nil {
		log.Printf("[tg][send][err] http: %v", err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[tg][send] http_status=%d body=%s", resp.StatusCode, string(respBody))

	var api tgResp
	_ = json.Unmarshal(respBody, &api)
	if resp.StatusCode != 200 || !api.Ok {
		return fmt.Errorf("telegram sendMessage failed: status=%d ok=%v desc=%s", resp.StatusCode, api.Ok, api.Description)
	}
	return nil
}

// NEW: отправка сообщения с обычной ReplyKeyboard (кнопки под строкой ввода)
func (t *TelegramService) SendReplyKeyboard(chatID int64, text string, keyboard [][]string) error {
	if t == nil || t.token == "" || chatID == 0 {
		log.Printf("[tg][skip] token or chatID empty (token? %v chatID=%d)", t != nil && t.token != "", chatID)
		return nil
	}

	// превращаем [][]string в tg-формат [][]map[string]any
	var kb [][]map[string]any
	for _, row := range keyboard {
		var r []map[string]any
		for _, label := range row {
			r = append(r, map[string]any{"text": label})
		}
		kb = append(kb, r)
	}
	rm := map[string]any{
		"keyboard":          kb,
		"resize_keyboard":   true,
		"one_time_keyboard": false,
	}

	body := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": true,
		"reply_markup":             rm,
	}
	b, _ := json.Marshal(body)
	url := t.baseURL + "/sendMessage"
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	log.Printf("[tg][send+kb] url=%s chatID=%d text=%q", url, chatID, text)
	resp, err := t.client.Do(req)
	if err != nil {
		log.Printf("[tg][send+kb][err] http: %v", err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[tg][send+kb] http_status=%d body=%s", resp.StatusCode, string(respBody))

	var api tgResp
	_ = json.Unmarshal(respBody, &api)
	if resp.StatusCode != 200 || !api.Ok {
		return fmt.Errorf("telegram sendMessage(with kb) failed: status=%d ok=%v desc=%s", resp.StatusCode, api.Ok, api.Description)
	}
	return nil
}

func (t *TelegramService) SetWebhook(url string) error {
	if t == nil || t.token == "" || url == "" {
		return nil
	}
	full := t.baseURL + "/setWebhook?url=" + url
	log.Printf("[tg][setWebhook] %s", full)
	req, _ := http.NewRequest("GET", full, nil)
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	log.Printf("[tg][setWebhook] status=%d body=%s", resp.StatusCode, string(b))
	return nil
}
