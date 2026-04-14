package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"turcompany/internal/models"
)

func TestCollectMissingRedFieldsLegalFlatBackwardCompatible(t *testing.T) {
	req := createClientRequest{
		ClientType: models.ClientTypeLegal,
		Name:       "Acme",
		BinIin:     "123456789012",
		Phone:      "77001112233",
		Address:    "Almaty",
	}
	missing := collectMissingRedFields(req)
	if len(missing) != 1 || missing[0] != "contact_person_name" {
		t.Fatalf("expected only contact_person_name missing, got %v", missing)
	}
}

func TestCollectMissingRedFieldsLegalNested(t *testing.T) {
	req := createClientRequest{
		ClientType: models.ClientTypeLegal,
		LegalProfile: &models.ClientLegalProfile{
			CompanyName:        "Acme",
			BIN:                "123456789012",
			ContactPersonName:  "John",
			ContactPersonPhone: "77001112233",
			LegalAddress:       "Almaty",
		},
	}
	missing := collectMissingRedFields(req)
	if len(missing) != 0 {
		t.Fatalf("expected no missing fields, got %v", missing)
	}
}

func TestBuildClientFromCreateRequestIncludesNewIndividualFields(t *testing.T) {
	birth := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	req := createClientRequest{
		Name:                        "John Doe",
		ClientType:                  models.ClientTypeIndividual,
		Specialty:                   "QA",
		TrustedPersonPhone:          "+77010000000",
		DriverLicenseNumber:         "DL42",
		EducationInstitutionName:    "KBTU",
		EducationInstitutionAddress: "Almaty",
		Position:                    "Engineer",
		VisasReceived:               "US",
		VisaRefusals:                "None",
	}
	client := buildClientFromCreateRequest(req, 10, &birth, nil, nil)
	if client.Specialty != "QA" || client.VisasReceived != "US" || client.VisaRefusals != "None" {
		t.Fatalf("expected new individual fields propagated, got %#v", client)
	}
	if client.EducationInstitutionAddress != "Almaty" || client.TrustedPersonPhone != "+77010000000" {
		t.Fatalf("expected education/trusted person fields propagated, got %#v", client)
	}
}

func TestUpdateClientRequestSupportsNewFieldsPointers(t *testing.T) {
	raw := []byte(`{"specialty":"dev","trusted_person_phone":"+7701","driver_license_number":"AB","education_institution_name":"Uni","education_institution_address":"Addr","position":"Lead","visas_received":"US","visa_refusals":"No"}`)
	var req updateClientRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if req.Specialty == nil || req.VisaRefusals == nil || req.EducationInstitutionName == nil {
		t.Fatalf("expected pointers for new fields to be set: %#v", req)
	}
}
