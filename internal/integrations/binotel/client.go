package binotel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// defaultBaseURL is the Binotel REST API v4.0 base.
// Every method is POSTed to {baseURL}/{method}.json with a JSON body
// containing at least {"key":..., "secret":...}. See the official PHP
// samples (BinotelApi.php): host "https://api.binotel.com/api/", version "4.0".
const defaultBaseURL = "https://api.binotel.com/api/4.0"

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
		http:      &http.Client{Timeout: 20 * time.Second},
	}
}

// IsConfigured reports whether credentials are present.
func (c *Client) IsConfigured() bool {
	return c.apiKey != "" && c.apiSecret != ""
}

// APIError represents a non-success Binotel API response.
type APIError struct {
	Status  string
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("binotel api error (status=%s code=%s)", e.Status, e.Code)
	}
	return fmt.Sprintf("binotel api error %s: %s", e.Code, e.Message)
}

// statusEnvelope is the common envelope Binotel wraps every response in.
type statusEnvelope struct {
	Status  string      `json:"status"` // "success" on success
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
}

// request POSTs params (plus key/secret) to {method}.json, verifies the
// response is HTTP 200 with status "success", and unmarshals it into out.
func (c *Client) request(ctx context.Context, method string, params map[string]interface{}, out interface{}) error {
	if !c.IsConfigured() {
		return fmt.Errorf("binotel: api_key/api_secret not configured")
	}
	if params == nil {
		params = map[string]interface{}{}
	}
	params["key"] = c.apiKey
	params["secret"] = c.apiSecret

	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("binotel: marshal %s request: %w", method, err)
	}

	url := c.baseURL + "/" + method + ".json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("binotel: build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("binotel: %s http: %w", method, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MB
	if err != nil {
		return fmt.Errorf("binotel: read %s response: %w", method, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("binotel: %s http status %d (body=%s)", method, resp.StatusCode, truncate(string(raw), 200))
	}

	var env statusEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("binotel: parse %s response: %w (body=%s)", method, err, truncate(string(raw), 200))
	}
	if !strings.EqualFold(env.Status, "success") {
		return &APIError{Status: env.Status, Code: stringify(env.Code), Message: env.Message}
	}

	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("binotel: decode %s payload: %w", method, err)
		}
	}
	return nil
}

// ── Call data type (stats/*) ──────────────────────────────────────────────────

// flexString unmarshals from a JSON string OR number (Binotel is inconsistent
// about whether ids come quoted).
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(strings.Trim(string(b), `"`))
	return nil
}

// flexInt unmarshals from a JSON number OR a quoted number (Binotel returns
// numeric fields as strings, e.g. "billsec":"0"). Unparseable values become 0
// so a single odd field never drops the whole call.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = 0
		return nil
	}
	s := strings.TrimSpace(strings.Trim(string(b), `"`))
	if s == "" {
		*f = 0
		return nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*f = flexInt(n)
		return nil
	}
	if fl, err := strconv.ParseFloat(s, 64); err == nil {
		*f = flexInt(int64(fl))
	}
	return nil
}

// Int returns the value as a plain int.
func (f flexInt) Int() int { return int(f) }

// Call mirrors one entry of the callDetails map returned by the stats methods.
// Binotel encodes numbers as strings, and empty objects as [] — hence the
// tolerant field types (flexString / flexInt / json.RawMessage).
type Call struct {
	CompanyID              flexString      `json:"companyID"`
	GeneralCallID          flexString      `json:"generalCallID"`
	StartTime              flexInt         `json:"startTime"`
	CallType               flexInt         `json:"callType"` // 0 = inbound, 1 = outbound
	InternalNumber         string          `json:"internalNumber"`
	InternalAdditionalData string          `json:"internalAdditionalData"`
	ExternalNumber         string          `json:"externalNumber"`
	Waitsec                flexInt         `json:"waitsec"`
	Billsec                flexInt         `json:"billsec"`
	Disposition            string          `json:"disposition"`
	IsNewCall              flexInt         `json:"isNewCall"`
	CustomerData           json.RawMessage `json:"customerData"`
	EmployeeData           json.RawMessage `json:"employeeData"`
	PbxNumberData          json.RawMessage `json:"pbxNumberData"`
}

// GeneralCallIDString returns the call id as a string.
func (c Call) GeneralCallIDString() string { return string(c.GeneralCallID) }

// statsCalls runs a stats method and leniently decodes the callDetails map:
// a single malformed entry is skipped rather than failing the whole batch.
func (c *Client) statsCalls(ctx context.Context, method string, params map[string]interface{}) (map[string]Call, error) {
	var out struct {
		CallDetails map[string]json.RawMessage `json:"callDetails"`
	}
	if err := c.request(ctx, method, params, &out); err != nil {
		return nil, err
	}
	res := make(map[string]Call, len(out.CallDetails))
	for k, raw := range out.CallDetails {
		var call Call
		if err := json.Unmarshal(raw, &call); err != nil {
			continue
		}
		res[k] = call
	}
	return res, nil
}

// ListCallsPerDay returns all incoming and outgoing calls for the given day
// (stats/list-of-calls-per-day). Pass the zero time for "today".
func (c *Client) ListCallsPerDay(ctx context.Context, day time.Time) (map[string]Call, error) {
	params := map[string]interface{}{}
	if !day.IsZero() {
		params["dayInTimestamp"] = day.Unix()
	}
	return c.statsCalls(ctx, "stats/list-of-calls-per-day", params)
}

// AllIncomingCallsSince returns incoming calls since the given unix timestamp
// (stats/all-incoming-calls-since, limit 1000).
func (c *Client) AllIncomingCallsSince(ctx context.Context, sinceUnix int64) (map[string]Call, error) {
	return c.statsCalls(ctx, "stats/all-incoming-calls-since", map[string]interface{}{"timestamp": sinceUnix})
}

// AllOutgoingCallsSince returns outgoing calls since the given unix timestamp
// (stats/all-outgoing-calls-since, limit 1000).
func (c *Client) AllOutgoingCallsSince(ctx context.Context, sinceUnix int64) (map[string]Call, error) {
	return c.statsCalls(ctx, "stats/all-outgoing-calls-since", map[string]interface{}{"timestamp": sinceUnix})
}

// ── Outgoing calls (calls/*) ──────────────────────────────────────────────────

// MakeCallResult holds the call identifiers returned by Binotel.
type MakeCallResult struct {
	GeneralCallID string `json:"generalCallID"`
}

// MakeCall asks Binotel to initiate a two-legged call between the manager's
// internal extension and an external number
// (calls/internal-number-to-external-number).
func (c *Client) MakeCall(ctx context.Context, internalNumber, externalNumber string) (MakeCallResult, error) {
	var out struct {
		GeneralCallID interface{} `json:"generalCallID"`
	}
	err := c.request(ctx, "calls/internal-number-to-external-number", map[string]interface{}{
		"internalNumber": internalNumber,
		"externalNumber": externalNumber,
	}, &out)
	if err != nil {
		return MakeCallResult{}, err
	}
	return MakeCallResult{GeneralCallID: stringify(out.GeneralCallID)}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func stringify(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return ""
	case string:
		return val
	case json.Number:
		return val.String()
	case float64:
		return fmt.Sprintf("%.0f", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
