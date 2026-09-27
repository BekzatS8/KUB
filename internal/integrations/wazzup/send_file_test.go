package wazzup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Файл из хранилища уходит ссылкой: в v3 — contentUri без текста.
func TestHTTPClientSendsFileAsContentURI(t *testing.T) {
	var got map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = io.WriteString(w, `{"messageId":"m1"}`)
	}))
	defer ts.Close()
	c := NewHTTPClient(ts.URL, 2*time.Second, 0, 10*time.Millisecond)

	if _, err := c.SendMessage(context.Background(), "key", SendMessageRequest{
		ChannelID: "ch", ChatType: "whatsapp", ChatID: "77001112233",
		ContentURI: "https://api.kub/api/v1/drive/raw/tok", FileName: "a.pdf",
	}); err != nil {
		t.Fatal(err)
	}
	if got["contentUri"] != "https://api.kub/api/v1/drive/raw/tok" {
		t.Fatalf("contentUri must be sent, got %v", got)
	}
	if _, ok := got["text"]; ok && got["text"] != "" {
		t.Fatalf("file message must not carry text, got %v", got["text"])
	}
	if _, ok := got["FileName"]; ok {
		t.Fatal("file meta is for partner api only and must not leak into v3 payload")
	}
}

// В партнёрском API файл — attachment {url, name, mimetype, size}.
func TestPartnerClientSendsFileAsAttachment(t *testing.T) {
	var got map[string]any
	client, _, closeFn := newTestPartnerClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &got)
		_, _ = io.WriteString(w, `{"data":{"request_id":"r1"}}`)
	})
	defer closeFn()

	if _, err := client.SendMessage(context.Background(), "", SendMessageRequest{
		ChannelID: "ch", ChatType: "whatsapp", ChatID: "77001112233",
		ContentURI: "https://api.kub/raw/tok", FileName: "Договор.pdf", FileMime: "application/pdf", FileSize: 1234,
	}); err != nil {
		t.Fatal(err)
	}
	att, ok := got["attachment"].(map[string]any)
	if !ok || att["url"] != "https://api.kub/raw/tok" || att["name"] != "Договор.pdf" || att["mimetype"] != "application/pdf" || att["size"] != float64(1234) {
		t.Fatalf("attachment mismatch: %v", got)
	}
	if _, ok := got["text"]; ok {
		t.Fatalf("file message must not carry text, got %v", got)
	}
}
