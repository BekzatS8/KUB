package services

import (
	"context"
	"net/http"
	"net/http/httptest"
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
}
