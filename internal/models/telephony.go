package models

import (
	"encoding/json"
	"time"
)

// TelephonyCall represents a single call record in the telephony_calls table.
type TelephonyCall struct {
	ID              int64           `json:"id"`
	Provider        string          `json:"provider"`
	ExternalCallID  *string         `json:"external_call_id,omitempty"`
	Direction       string          `json:"direction"`
	Status          string          `json:"status"`
	Phone           string          `json:"phone"`
	NormalizedPhone *string         `json:"normalized_phone,omitempty"`
	ClientID        *int64          `json:"client_id,omitempty"`
	LeadID          *int64          `json:"lead_id,omitempty"`
	ManagerID       *int            `json:"manager_id,omitempty"`
	BranchID        *int            `json:"branch_id,omitempty"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	AnsweredAt      *time.Time      `json:"answered_at,omitempty"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
	DurationSeconds *int            `json:"duration_seconds,omitempty"`
	RecordingURL    *string         `json:"recording_url,omitempty"`
	RawPayload      json.RawMessage `json:"raw_payload,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// Call direction constants.
const (
	CallDirectionInbound  = "inbound"
	CallDirectionOutbound = "outbound"
)

// Call status constants.
const (
	CallStatusIncoming  = "incoming"
	CallStatusOutgoing  = "outgoing"
	CallStatusMissed    = "missed"
	CallStatusAnswered  = "answered"
	CallStatusCompleted = "completed"
	CallStatusFailed    = "failed"
	CallStatusUnknown   = "unknown"
)

// BinotelWebhookPayload is a flexible container for any Binotel webhook event.
// Binotel sends different payloads for call_start, call_answer, call_end, etc.
// We capture everything in RawPayload and extract known fields on a best-effort basis.
type BinotelWebhookPayload struct {
	// General event type: call_start, call_answer, call_end, missed_call, etc.
	EventType string `json:"event_type"`

	// Call identifier from Binotel
	GeneralCallID string `json:"generalCallID"`
	CallID        string `json:"callID"`

	// Phone numbers
	InternalNumber string `json:"internalNumber"` // manager's extension
	ExternalNumber string `json:"externalNumber"` // caller/callee external number

	// Timestamps (Unix seconds as strings or ints — handled flexibly)
	StartTime interface{} `json:"startTime"`
	AnswTime  interface{} `json:"answTime"`
	EndTime   interface{} `json:"endTime"`

	// Duration in seconds
	Duration interface{} `json:"duration"`

	// Recording URL (may be available on call_end)
	RecordURL string `json:"recordUrl"`

	// Status/disposition: answered, noanswer, busy, failed, etc.
	Disposition string `json:"disposition"`

	// Direction hint from Binotel: 0=inbound, 1=outbound (or string)
	CallType interface{} `json:"callType"`

	// Employee/manager details
	EmployeeEmail string `json:"employeeEmail"`
	EmployeePhone string `json:"employeePhone"`
	EmployeeID    string `json:"employeeID"`
}

// TelephonyCallListFilter holds pagination and filter criteria for listing calls.
type TelephonyCallListFilter struct {
	ClientID  *int64
	LeadID    *int64
	ManagerID *int
	BranchID  *int
	Status    string
	Phone     string
	DateFrom  *time.Time
	DateTo    *time.Time
	Limit     int
	Offset    int
}

// TelephonyCallResponse is the API response for a single call (with optional joined names).
type TelephonyCallResponse struct {
	TelephonyCall
	ClientName  *string `json:"client_name,omitempty"`
	LeadTitle   *string `json:"lead_title,omitempty"`
	ManagerName *string `json:"manager_name,omitempty"`
}
