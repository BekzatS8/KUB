package services

import (
	"testing"

	"turcompany/internal/models"
)

func TestValidateClientFieldsForDocTypePersonalDataConsent(t *testing.T) {
	client := &models.Client{
		LastName:  "Ivanov",
		FirstName: "Ivan",
		IIN:       "123456789012",
		Address:   "Street",
	}

	missing := validateClientFieldsForDocType("personal_data_consent", client)
	contains := map[string]bool{}
	for _, f := range missing {
		contains[f] = true
	}
	if !contains["id_number"] || !contains["passport_number"] {
		t.Fatalf("expected id_number and passport_number in missing, got %v", missing)
	}
}

func TestValidateClientFieldsForDocTypeContractFull(t *testing.T) {
	client := &models.Client{
		LastName:  "Ivanov",
		FirstName: "Ivan",
		IIN:       "123456789012",
	}

	missing := validateClientFieldsForDocType("contract_full", client)
	contains := map[string]bool{}
	for _, f := range missing {
		contains[f] = true
	}
	if !contains["phone"] || !contains["address"] {
		t.Fatalf("expected phone and address in missing, got %v", missing)
	}
}
