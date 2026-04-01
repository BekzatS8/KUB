package wazzup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSendMessageInjectsBearerToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messageId":"m-1"}`))
	}))
	defer ts.Close()

	c := NewHTTPClient(ts.URL, 2*time.Second, 0, 10*time.Millisecond)
	resp, err := c.SendMessage(context.Background(), "secret-token", SendMessageRequest{ChatID: "77001112233", Text: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MessageID != "m-1" {
		t.Fatalf("unexpected message id: %q", resp.MessageID)
	}
}

func TestSendMessageProviderError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer ts.Close()

	c := NewHTTPClient(ts.URL, 2*time.Second, 0, 10*time.Millisecond)
	_, err := c.SendMessage(context.Background(), "secret-token", SendMessageRequest{ChatID: "7700", Text: "hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "secret-token") {
		t.Fatal("token leaked in error")
	}
}
