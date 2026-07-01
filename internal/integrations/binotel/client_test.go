package binotel

import (
	"encoding/json"
	"testing"
)

// Real Binotel responses encode every numeric field as a string and use [] for
// empty objects. This guards the tolerant decoding so calls aren't silently
// dropped during sync.
func TestCallDetailsDecoding_StringEncodedFields(t *testing.T) {
	raw := []byte(`{
		"status":"success",
		"callDetails":{
			"6689229249":{
				"companyID":"61266",
				"generalCallID":"6689229249",
				"startTime":"1782901927",
				"callType":"0",
				"internalNumber":"808",
				"externalNumber":"77086897969",
				"waitsec":"1",
				"billsec":"12",
				"disposition":"ANSWER",
				"isNewCall":"0",
				"customerData":"",
				"employeeData":[],
				"pbxNumberData":{"number":"77008870708"}
			}
		}
	}`)

	var out struct {
		CallDetails map[string]json.RawMessage `json:"callDetails"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	entry := out.CallDetails["6689229249"]
	if len(entry) == 0 {
		t.Fatal("missing call entry")
	}
	var call Call
	if err := json.Unmarshal(entry, &call); err != nil {
		t.Fatalf("decode call (string-encoded fields must not fail): %v", err)
	}

	if got := call.GeneralCallIDString(); got != "6689229249" {
		t.Errorf("generalCallID = %q, want 6689229249", got)
	}
	if call.CallType.Int() != 0 {
		t.Errorf("callType = %d, want 0", call.CallType.Int())
	}
	if call.StartTime.Int() != 1782901927 {
		t.Errorf("startTime = %d, want 1782901927", call.StartTime.Int())
	}
	if call.Billsec.Int() != 12 {
		t.Errorf("billsec = %d, want 12", call.Billsec.Int())
	}
	if call.ExternalNumber != "77086897969" {
		t.Errorf("externalNumber = %q", call.ExternalNumber)
	}
	if call.Disposition != "ANSWER" {
		t.Errorf("disposition = %q", call.Disposition)
	}
}
