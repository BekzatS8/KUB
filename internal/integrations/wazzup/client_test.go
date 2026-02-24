package wazzup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPClient_UpsertUsersThenCreateIframe(t *testing.T) {
	knownUsers := map[string]struct{}{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/users":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for /v3/users: %s", r.Method)
			}
			var users []UserUpsert
			if err := json.NewDecoder(r.Body).Decode(&users); err != nil {
				t.Fatalf("decode users: %v", err)
			}
			for _, u := range users {
				knownUsers[u.ID] = struct{}{}
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/v3/iframe":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for /v3/iframe: %s", r.Method)
			}
			var req CreateIframeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode iframe request: %v", err)
			}
			if _, ok := knownUsers[req.User.ID]; !ok {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"INVALID_USER"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"url":"https://example.test/iframe"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	c := &HTTPClient{baseURL: ts.URL, http: &http.Client{Timeout: 10 * time.Second}}
	ctx := context.Background()
	wazzupUserID := "kub-10-25"

	if err := c.UpsertUsers(ctx, "api-key", []UserUpsert{{ID: wazzupUserID, Name: "Test User"}}); err != nil {
		t.Fatalf("upsert users failed: %v", err)
	}

	url, err := c.CreateIframe(ctx, "api-key", CreateIframeRequest{
		User:  UserUpsert{ID: wazzupUserID, Name: "Test User"},
		Scope: "global",
	})
	if err != nil {
		t.Fatalf("create iframe failed: %v", err)
	}
	if strings.TrimSpace(url) != "https://example.test/iframe" {
		t.Fatalf("unexpected iframe url: %q", url)
	}
}
