package services

import (
	"testing"

	"turcompany/internal/models"
)

func TestValidateClientFieldsForDocType_VisaQuestionnaireNeedsIdDocs(t *testing.T) {
	client := &models.Client{LastName: "Ivanov", FirstName: "Ivan", IIN: "123456789012", Address: "Street", Phone: "+777"}
	missing := validateClientFieldsForDocType("visa_questionnaire", client)
	set := map[string]bool{}
	for _, f := range missing {
		set[f] = true
	}
	if !set["id_number"] || !set["passport_number"] {
		t.Fatalf("expected id_number+passport_number, got %v", missing)
	}
}

func TestValidateClientFieldsForDocType_ContractNeedsBaseFields(t *testing.T) {
	client := &models.Client{LastName: "Ivanov", FirstName: "Ivan", IIN: "123456789012"}
	missing := validateClientFieldsForDocType("contract_paid_full_ru", client)
	set := map[string]bool{}
	for _, f := range missing {
		set[f] = true
	}
	if !set["phone"] || !set["address"] {
		t.Fatalf("expected phone+address, got %v", missing)
	}
}
