package services

import (
	"errors"
	"testing"
	"time"

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

func TestNormalizeAndValidateIndividualUsesNestedProfileForNewFields(t *testing.T) {
	svc := &ClientService{}
	c := &models.Client{
		ClientType:  models.ClientTypeIndividual,
		LastName:    "Doe",
		FirstName:   "John",
		Phone:       "77001112233",
		Country:     "KZ",
		TripPurpose: "tour",
		BirthDate:   ptrTimeForClientProfileTest(),
		IndividualProfile: &models.ClientIndividualProfile{
			EducationLevel:              "  higher ",
			Specialty:                   "  Engineer ",
			TrustedPersonPhone:          "+7 701 000 11 22",
			DriverLicenseNumber:         " DL-77 ",
			DriverLicenseIssueDate:      ptrTimeForClientProfileTest(),
			DriverLicenseExpireDate:     ptrTimeForClientProfileTest(),
			EducationInstitutionName:    "  KBTU ",
			EducationInstitutionAddress: "  Almaty ",
			Position:                    " Lead ",
			VisasReceived:               " US ",
			VisaRefusals:                " none ",
		},
	}
	if err := svc.normalizeAndValidate(c); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Specialty != "Engineer" || c.Position != "Lead" {
		t.Fatalf("expected nested profile values promoted, got specialty=%q position=%q", c.Specialty, c.Position)
	}
	if c.EducationLevel != "higher" {
		t.Fatalf("expected trimmed education_level, got %q", c.EducationLevel)
	}
	if c.TrustedPersonPhone != "77010001122" {
		t.Fatalf("expected phone normalized from nested profile, got %q", c.TrustedPersonPhone)
	}
	if c.VisaRefusals != "none" {
		t.Fatalf("expected trimmed visa_refusals, got %q", c.VisaRefusals)
	}
	if c.DriverLicenseIssueDate == nil || c.DriverLicenseExpireDate == nil {
		t.Fatalf("expected driver license dates promoted from nested profile")
	}
}

func TestNormalizeAndValidateIndividualRejectsUnknownEducationLevel(t *testing.T) {
	svc := &ClientService{}
	c := &models.Client{
		ClientType:     models.ClientTypeIndividual,
		LastName:       "Doe",
		FirstName:      "John",
		Phone:          "77001112233",
		Country:        "KZ",
		TripPurpose:    "tour",
		BirthDate:      ptrTimeForClientProfileTest(),
		EducationLevel: "doctoral",
	}
	if err := svc.normalizeAndValidate(c); !errors.Is(err, ErrInvalidEducationLevel) {
		t.Fatalf("expected ErrInvalidEducationLevel, got %v", err)
	}
}

func ptrTimeForClientProfileTest() *time.Time {
	v := time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC)
	return &v
}
