package services

import (
	"testing"

	"turcompany/internal/models"
)

func TestValidateCreateRedFieldsReturnsMissingFields(t *testing.T) {
	client := &models.Client{LastName: "Иванов", FirstName: "Иван"}

	err := validateCreateRedFields(client)
	if err == nil {
		t.Fatalf("expected error")
	}
	missingErr, ok := err.(*MissingFieldsError)
	if !ok {
		t.Fatalf("expected MissingFieldsError, got %T", err)
	}

	required := map[string]bool{"country": false, "trip_purpose": false, "birth_date": false, "phone": false}
	for _, f := range missingErr.Fields {
		if _, exists := required[f]; exists {
			required[f] = true
		}
	}
	for field, found := range required {
		if !found {
			t.Fatalf("expected missing field %q in %+v", field, missingErr.Fields)
		}
	}
}

func TestMissingYellowFieldsContainsPhotoWhenMissing(t *testing.T) {
	client := &models.Client{
		MiddleName:          "",
		BirthPlace:          "",
		Citizenship:         "",
		Sex:                 "",
		MaritalStatus:       "",
		IIN:                 "",
		IDNumber:            "",
		PassportNumber:      "",
		RegistrationAddress: "",
		ActualAddress:       "",
		Email:               "",
	}

	missing := missingYellowFields(client, false)
	found := false
	for _, f := range missing {
		if f == "photo35x45" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected photo35x45 in missing_yellow, got %+v", missing)
	}
}

func TestMissingYellowFieldsDoesNotContainPhotoWhenPresent(t *testing.T) {
	client := &models.Client{}
	missing := missingYellowFields(client, true)
	for _, f := range missing {
		if f == "photo35x45" {
			t.Fatalf("did not expect photo35x45 in missing_yellow when photo exists")
		}
	}
}
