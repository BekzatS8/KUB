package handlers

import (
	"testing"

	"turcompany/internal/models"
)

func TestBuildClientFromConvertWithClientRequest_LegalUsesLegalType(t *testing.T) {
	req := ConvertLeadWithClientRequest{
		ClientType:         models.ClientTypeLegal,
		CompanyName:        "ACME LLP",
		Bin:                "123456789012",
		LegalAddress:       "Almaty",
		ContactPersonName:  "John Doe",
		ContactPersonPhone: "+7 700 000 00 00",
		ContactPersonEmail: "john@example.com",
	}

	client, err := buildClientFromConvertWithClientRequest(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.ClientType != models.ClientTypeLegal {
		t.Fatalf("expected legal client type, got %q", client.ClientType)
	}
	if client.LegalProfile == nil {
		t.Fatal("expected legal profile to be populated")
	}
	if client.Name != "ACME LLP" {
		t.Fatalf("expected company name in base client name, got %q", client.Name)
	}
}

func TestBuildClientFromConvertWithClientRequest_InvalidTypeRejected(t *testing.T) {
	req := ConvertLeadWithClientRequest{
		ClientType: "partner",
		ClientName: "Bad Type Co",
	}

	client, err := buildClientFromConvertWithClientRequest(req)
	if err == nil {
		t.Fatalf("expected error, got client=%+v", client)
	}
}

func TestBuildClientFromConvertWithClientRequest_MissingTypeRejected(t *testing.T) {
	req := ConvertLeadWithClientRequest{
		ClientType: "",
		ClientName: "No Type Co",
	}

	client, err := buildClientFromConvertWithClientRequest(req)
	if err == nil {
		t.Fatalf("expected error, got client=%+v", client)
	}
}
