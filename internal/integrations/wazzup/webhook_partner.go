package wazzup

import (
	"encoding/json"
	"strconv"
	"strings"
)

// Разбор вебхуков Tech Partner API (v2).
//
// Формат принципиально отличается от User API v3:
//
//	v3: {"messages":[{"messageId":…,"chatId":…,"text":…}]}
//	v2: {"event":"message.add","data":[…],"meta":{"idempotency_key":…}}
//
// Поэтому входящее тело сначала пробуем разобрать как v2-конверт, и только
// если это не он — отдаём старому парсеру. Оба формата ходят в один и тот же
// эндпоинт /integrations/wazzup/webhook/:token, потому что аккаунт может быть
// переключён между драйверами без смены URL подписки.

// partnerWebhookEnvelope — общий конверт любого события v2.
type partnerWebhookEnvelope struct {
	Event string            `json:"event"`
	Data  []json.RawMessage `json:"data"`
	Meta  struct {
		IdempotencyKey string `json:"idempotency_key"`
		Timestamp      int64  `json:"timestamp"`
	} `json:"meta"`
}

// flexString читает поле, которое провайдер шлёт то строкой, то числом
// (например recipient.phone: "79111234567" против 79111234567).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" || trimmed == "null" {
		*f = ""
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

func (f flexString) String() string { return strings.TrimSpace(string(f)) }

// partnerChatRef — sender/recipient в событии message.add.
type partnerChatRef struct {
	ChatType flexString `json:"chat_type"`
	ChatID   flexString `json:"chat_id"`
	Username flexString `json:"username"`
	Phone    flexString `json:"phone"`
	Name     flexString `json:"name"`
}

// partnerMessageEvent — событие message.add.
type partnerMessageEvent struct {
	MessageID flexString      `json:"message_id"`
	ChannelID flexString      `json:"channel_id"`
	Direction string          `json:"direction"`
	Timestamp int64           `json:"timestamp"`
	Status    string          `json:"status"`
	Text      string          `json:"text"`
	Sender    *partnerChatRef `json:"sender"`
	Recipient *partnerChatRef `json:"recipient"`
}

// parsePartnerWebhook разбирает тело как вебхук v2.
//
// Второй результат false означает «это не v2» — вызывающий код должен
// попробовать формат v3. Сообщения возвращаются уже сконвертированными в
// webhookMessage, чтобы дальше работала общая логика создания лидов.
func parsePartnerWebhook(payload []byte) ([]webhookMessage, bool) {
	var env partnerWebhookEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return nil, false
	}
	event := strings.TrimSpace(env.Event)
	if event == "" {
		return nil, false
	}

	// Не message.add — событие валидное (статусы, каналы, QR), но сообщений в
	// нём нет. Возвращаем пустой список: вебхук будет подтверждён с 200.
	if event != "message.add" {
		return nil, true
	}

	messages := make([]webhookMessage, 0, len(env.Data))
	for _, raw := range env.Data {
		var ev partnerMessageEvent
		if err := json.Unmarshal(raw, &ev); err != nil {
			continue
		}
		messages = append(messages, ev.toWebhookMessage())
	}
	return messages, true
}

// toWebhookMessage приводит событие v2 к внутреннему представлению.
//
// recipient в v2 — это всегда чат: для входящего в диалоге там сидит клиент,
// с которым переписывается сотрудник. Именно его идентификаторы нужны для
// привязки к лиду, поэтому берём их, а sender используем только как запасной
// источник имени (он заполняется в групповых чатах).
func (e partnerMessageEvent) toWebhookMessage() webhookMessage {
	m := webhookMessage{
		MessageID: e.MessageID.String(),
		ChannelID: e.ChannelID.String(),
		Direction: strings.ToLower(strings.TrimSpace(e.Direction)),
		Text:      e.Text,
	}
	if e.Timestamp > 0 {
		// parseUnixTimestamp сам отличит миллисекунды от секунд.
		m.Timestamp = strconv.FormatInt(e.Timestamp, 10)
	}
	if r := e.Recipient; r != nil {
		m.ChatType = r.ChatType.String()
		m.ChatID = firstNonEmpty(r.ChatID.String(), r.Phone.String(), r.Username.String())
		m.Username = r.Username.String()
		m.ContactName = r.Name.String()
	}
	if s := e.Sender; s != nil {
		if m.ChatID == "" {
			m.ChatID = firstNonEmpty(s.ChatID.String(), s.Phone.String(), s.Username.String())
		}
		if m.ChatType == "" {
			m.ChatType = s.ChatType.String()
		}
		if m.ContactName == "" {
			m.ContactName = s.Name.String()
		}
		if m.AuthorName == "" {
			m.AuthorName = s.Name.String()
		}
	}
	return m
}
