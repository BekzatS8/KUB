package binotel

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://pbx.binotel.com/api/4.0"

// Client is an HTTP client for the Binotel REST API v4.0.
type Client struct {
	apiKey    string
	apiSecret string
	baseURL   string
	http      *http.Client
}

// NewClient returns a Binotel REST client.
// apiKey / apiSecret are the credentials from the Binotel dashboard.
func NewClient(apiKey, apiSecret string) *Client {
	return &Client{
		apiKey:    strings.TrimSpace(apiKey),
		apiSecret: strings.TrimSpace(apiSecret),
		baseURL:   defaultBaseURL,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// IsConfigured reports whether credentials are present.
func (c *Client) IsConfigured() bool {
	return c.apiKey != "" && c.apiSecret != ""
}

// sign returns MD5(key+secret) as required by Binotel API v4.0.
func (c *Client) sign() string {
	h := md5.New()
	h.Write([]byte(c.apiKey + c.apiSecret))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ── Request / response types ──────────────────────────────────────────────────

type makeCallRequest struct {
	Key            string `json:"key"`
	Secret         string `json:"secret"`
	Sign           string `json:"sign"`
	InternalNumber string `json:"internalNumber"`
	ExternalNumber string `json:"externalNumber"`
}

type binotelResp struct {
	Status  int             `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// MakeCallResult holds the call identifiers returned by Binotel.
type MakeCallResult struct {
	GeneralCallID string `json:"generalCallID"`
}

// MakeCall asks Binotel to initiate an outgoing call.
// internalNumber is the manager's Binotel extension (matches users.phone in CRM).
// externalNumber is the client/lead phone (digits only or E.164).
// Returns the Binotel generalCallID (may be empty if Binotel doesn't supply one).
func (c *Client) MakeCall(ctx context.Context, internalNumber, externalNumber string) (MakeCallResult, error) {
	if !c.IsConfigured() {
		return MakeCallResult{}, fmt.Errorf("binotel: api_key/api_secret not configured")
	}

	reqBody := makeCallRequest{
		Key:            c.apiKey,
		Secret:         c.apiSecret,
		Sign:           c.sign(),
		InternalNumber: internalNumber,
		ExternalNumber: externalNumber,
	}
	rawBody, err := json.Marshal(reqBody)
	if err != nil {
		return MakeCallResult{}, fmt.Errorf("binotel: marshal make-call request: %w", err)
	}

	url := c.baseURL + "/calls/make-call.json"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		return MakeCallResult{}, fmt.Errorf("binotel: build make-call request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return MakeCallResult{}, fmt.Errorf("binotel: make-call http: %w", err)
	}
	defer resp.Body.Close()

	respRaw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return MakeCallResult{}, fmt.Errorf("binotel: read make-call response: %w", err)
	}

	var br binotelResp
	if err := json.Unmarshal(respRaw, &br); err != nil {
		return MakeCallResult{}, fmt.Errorf("binotel: parse make-call response: %w (body=%s)", err, truncate(string(respRaw), 200))
	}

	if br.Status != 1 {
		msg := strings.TrimSpace(br.Message)
		if msg == "" {
			msg = "unknown binotel error"
		}
		return MakeCallResult{}, fmt.Errorf("binotel: make-call failed: %s", msg)
	}

	var result MakeCallResult
	if len(br.Data) > 0 {
		_ = json.Unmarshal(br.Data, &result)
	}
	return result, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
