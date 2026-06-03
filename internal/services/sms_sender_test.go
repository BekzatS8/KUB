package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMobizonSMSClientDryRun(t *testing.T) {
	client := NewMobizonSMSClient(MobizonSMSConfig{Enabled: true, DryRun: true})
	res, err := client.Send(context.Background(), SMSMessage{To: "+77001234567", Text: "test"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if res == nil || res.ProviderMessageID != "dry-run" {
		t.Fatalf("unexpected dry-run result: %#v", res)
	}
}

func TestMobizonSMSClientSendsOfficialFormRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/service/message/sendsmsmessage" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("apiKey"); got != "secret-key" {
			t.Fatalf("apiKey query = %q", got)
		}
		if got := r.URL.Query().Get("output"); got != "json" {
			t.Fatalf("output query = %q", got)
		}
		if got := r.URL.Query().Get("api"); got != "v1" {
			t.Fatalf("api query = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization header should be empty, got %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/x-www-form-urlencoded") {
			t.Fatalf("Content-Type = %q", got)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if got := r.Form.Get("recipient"); got != "77001234567" {
			t.Fatalf("recipient = %q", got)
		}
		if got := r.Form.Get("text"); got != "test" {
			t.Fatalf("text = %q", got)
		}
		if got := r.Form.Get("from"); got != "KUB" {
			t.Fatalf("from = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"messageId":123}}`))
	}))
	defer ts.Close()

	client := NewMobizonSMSClient(MobizonSMSConfig{
		Enabled: true,
		APIKey:  "secret-key",
		BaseURL: ts.URL + "/service",
		From:    "KUB",
		Timeout: time.Second,
	})
	res, err := client.Send(context.Background(), SMSMessage{To: "+77001234567", Text: "test"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if res.ProviderMessageID != "123" {
		t.Fatalf("message id = %q", res.ProviderMessageID)
	}
}

func TestMobizonSMSClientMapsHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()
	client := NewMobizonSMSClient(MobizonSMSConfig{Enabled: true, APIKey: "k", BaseURL: ts.URL, Timeout: time.Second})
	_, err := client.Send(context.Background(), SMSMessage{To: "+77001234567", Text: "test"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrSMSProviderFailure) {
		t.Fatalf("expected provider failure, got %v", err)
	}
}

func TestMobizonSMSClientMapsProviderCodeError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"message":"bad recipient"}`))
	}))
	defer ts.Close()

	client := NewMobizonSMSClient(MobizonSMSConfig{Enabled: true, APIKey: "k", BaseURL: ts.URL, Timeout: time.Second})
	_, err := client.Send(context.Background(), SMSMessage{To: "77001234567", Text: "test"})
	if !errors.Is(err, ErrSMSProviderFailure) {
		t.Fatalf("expected provider failure, got %v", err)
	}
}

func TestMobizonSMSClientValidatesPhoneAndTextAndKey(t *testing.T) {
	client := NewMobizonSMSClient(MobizonSMSConfig{Enabled: true, APIKey: "k", DryRun: true})
	if _, err := client.Send(context.Background(), SMSMessage{To: "abc", Text: "test"}); !errors.Is(err, ErrSMSInvalidPhone) {
		t.Fatalf("expected invalid phone, got %v", err)
	}
	if _, err := client.Send(context.Background(), SMSMessage{To: "77001234567", Text: " "}); !errors.Is(err, ErrSMSEmptyText) {
		t.Fatalf("expected empty text, got %v", err)
	}

	client = NewMobizonSMSClient(MobizonSMSConfig{Enabled: true})
	if _, err := client.Send(context.Background(), SMSMessage{To: "77001234567", Text: "test"}); !errors.Is(err, ErrSMSAPIKeyMissing) {
		t.Fatalf("expected missing api key, got %v", err)
	}
}

func TestMobizonSMSClientMapsTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer ts.Close()

	client := NewMobizonSMSClient(MobizonSMSConfig{Enabled: true, APIKey: "k", BaseURL: ts.URL, Timeout: 10 * time.Millisecond})
	_, err := client.Send(context.Background(), SMSMessage{To: "77001234567", Text: "test"})
	if !errors.Is(err, ErrSMSTimeout) {
		t.Fatalf("expected timeout, got %v", err)
	}
}
