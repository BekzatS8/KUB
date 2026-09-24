package wazzup

import "testing"

// TestParsePartnerWebhookDialogMessage — входящее в личном диалоге.
// В v2 контакт лежит в recipient, а не в плоских полях верхнего уровня.
func TestParsePartnerWebhookDialogMessage(t *testing.T) {
	const payload = `{
      "event": "message.add",
      "data": [{
        "message_id": "a14325c0-97c0-43dd-917e-f0ad7460b6a9",
        "channel_id": "808e0654-d0f5-4926-a7fa-4783f1c05eeb",
        "direction": "inbound",
        "timestamp": 1762340622177,
        "status": "accepted",
        "crm_user_id": null,
        "recipient": {
          "chat_id": "77001112233",
          "chat_type": "whatsapp",
          "name": "Айдана",
          "username": null,
          "phone": "77001112233"
        },
        "text": "Здравствуйте, интересует виза"
      }],
      "meta": {"idempotency_key": "9ef07b18-e95f-4ba1-9981-63d95d8ed468", "timestamp": 1762340623}
    }`

	messages, ok := parsePartnerWebhook([]byte(payload))
	if !ok {
		t.Fatal("payload must be recognized as partner webhook")
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	m := messages[0]
	if m.MessageID != "a14325c0-97c0-43dd-917e-f0ad7460b6a9" {
		t.Errorf("unexpected message id %q", m.MessageID)
	}
	if m.ChatID != "77001112233" {
		t.Errorf("unexpected chat id %q", m.ChatID)
	}
	if m.ChatType != "whatsapp" {
		t.Errorf("unexpected chat type %q", m.ChatType)
	}
	if m.ContactName != "Айдана" {
		t.Errorf("unexpected contact name %q", m.ContactName)
	}
	if m.Text != "Здравствуйте, интересует виза" {
		t.Errorf("unexpected text %q", m.Text)
	}
	if isOutgoing(m) {
		t.Error("inbound message must not be treated as outgoing")
	}
	// timestamp в миллисекундах — parseUnixTimestamp обязан это распознать.
	if got := parseWebhookTime(m.Timestamp).Year(); got != 2025 {
		t.Errorf("unexpected parsed year %d (timestamp=%q)", got, m.Timestamp)
	}
}

// TestParsePartnerWebhookOutboundIsSkipped — главная ловушка миграции:
// v3 писал "out"/"outgoing", v2 пишет "outbound". Без этого ответы менеджеров
// считались бы входящими и плодили лиды.
func TestParsePartnerWebhookOutboundIsSkipped(t *testing.T) {
	const payload = `{"event":"message.add","data":[{
        "message_id":"m-1","channel_id":"c-1","direction":"outbound","timestamp":1762340622177,
        "recipient":{"chat_id":"77001112233","chat_type":"whatsapp"},"text":"Добрый день!"}],
      "meta":{"idempotency_key":"k","timestamp":1}}`

	messages, ok := parsePartnerWebhook([]byte(payload))
	if !ok || len(messages) != 1 {
		t.Fatalf("expected one parsed message, ok=%v n=%d", ok, len(messages))
	}
	if !isOutgoing(messages[0]) {
		t.Error("direction=outbound must be treated as outgoing")
	}
}

// TestParsePartnerWebhookNumericPhone — в групповых чатах провайдер шлёт phone
// числом, а не строкой; обычный string-тег на этом падал бы.
func TestParsePartnerWebhookNumericPhone(t *testing.T) {
	const payload = `{"event":"message.add","data":[{
        "message_id":"m-2","channel_id":"c-1","direction":"inbound","timestamp":1776953557938,
        "sender":{"chat_type":"telegram","chat_id":"221601332","name":"Test","phone":79111234567},
        "recipient":{"chat_id":"221601332","chat_type":"telegroup","name":"Group chat","phone":null},
        "text":"Hello"}],
      "meta":{"idempotency_key":"k","timestamp":1}}`

	messages, ok := parsePartnerWebhook([]byte(payload))
	if !ok {
		t.Fatal("payload must be recognized as partner webhook")
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
	if messages[0].ChatID != "221601332" {
		t.Errorf("unexpected chat id %q", messages[0].ChatID)
	}
	if messages[0].ContactName != "Group chat" {
		t.Errorf("unexpected contact name %q", messages[0].ContactName)
	}
}

// TestParsePartnerWebhookNonMessageEvent — статусы каналов и QR приходят в тот
// же эндпоинт; они валидны, но сообщений не несут.
func TestParsePartnerWebhookNonMessageEvent(t *testing.T) {
	const payload = `{"event":"channel.status_update","data":[{"channel_id":"c-1","status":"active","reason":null}],
      "meta":{"idempotency_key":"k","timestamp":1}}`

	messages, ok := parsePartnerWebhook([]byte(payload))
	if !ok {
		t.Fatal("channel event must be recognized as partner webhook")
	}
	if len(messages) != 0 {
		t.Fatalf("expected no messages, got %d", len(messages))
	}
}

// TestParsePartnerWebhookRejectsV3Payload — старый формат должен уходить
// прежнему парсеру, иначе аккаунт на v3 перестанет принимать входящие.
func TestParsePartnerWebhookRejectsV3Payload(t *testing.T) {
	const payload = `{"messages":[{"messageId":"m-1","chatType":"whatsapp","chatId":"77001112233","text":"hello"}]}`

	if _, ok := parsePartnerWebhook([]byte(payload)); ok {
		t.Fatal("v3 payload must not be treated as partner webhook")
	}
}
