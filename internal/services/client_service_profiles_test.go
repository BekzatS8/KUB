package services

import (
	"testing"

	"turcompany/internal/models"
)

func TestValidateCreateRedFieldsIndividual(t *testing.T) {
	c := &models.Client{ClientType: models.ClientTypeIndividual, FirstName: "A", LastName: "B", Phone: "7700", Country: "KZ", TripPurpose: "tour"}
	if err := validateCreateRedFields(c); err == nil {
		t.Fatal("expected missing birth_date")
	}
}

func TestValidateCreateRedFieldsLegalProfileAware(t *testing.T) {
	c := &models.Client{ClientType: models.ClientTypeLegal, LegalProfile: &models.ClientLegalProfile{CompanyName: "ACME", BIN: "123", ContactPersonName: "John", ContactPersonPhone: "7700", LegalAddress: "addr"}}
	if err := validateCreateRedFields(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeAndValidateLegalUsesNestedProfileAsSource(t *testing.T) {
	svc := &ClientService{}
	c := &models.Client{
		ClientType: models.ClientTypeLegal,
		LegalProfile: &models.ClientLegalProfile{
			CompanyName:        "ACME LLP",
			BIN:                "123456789012",
			ContactPersonPhone: "+7 700 111 22 33",
			ContactPersonEmail: "sales@acme.test",
		},
	}
	if err := svc.normalizeAndValidate(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Name != "ACME LLP" || c.BinIin != "123456789012" {
		t.Fatalf("nested legal profile was not promoted into base fields: %#v", c)
	}
	if c.Phone != "77001112233" {
		t.Fatalf("phone not normalized from nested profile, got %q", c.Phone)
	}
}
