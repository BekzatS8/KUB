package handlers

import (
	"testing"

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
