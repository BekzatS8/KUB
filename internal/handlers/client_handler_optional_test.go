package handlers

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildClientFromCreateRequestMapsOptionalFields(t *testing.T) {
	hasChildren := true
	height := int16(170)
	children := json.RawMessage(`[{"name":"Kid"}]`)
	driver := json.RawMessage(`["B"]`)
	birthDate := time.Date(1990, 1, 2, 0, 0, 0, 0, time.UTC)

	req := createClientRequest{
		Name:                    "Ivanov Ivan",
		Country:                 "KZ",
		TripPurpose:             "tourism",
		LastName:                "Ivanov",
		FirstName:               "Ivan",
		BirthDate:               "1990-01-02",
		Phone:                   "+7 777 000 00 00",
		PreviousLastName:        "Petrov",
		HasChildren:             &hasChildren,
		ChildrenList:            children,
		Height:                  &height,
		DriverLicenseCategories: driver,
	}

	client := buildClientFromCreateRequest(req, 7, &birthDate, nil, nil)
	if client.PreviousLastName != "Petrov" {
		t.Fatalf("expected PreviousLastName to be mapped")
	}
	if client.HasChildren == nil || !*client.HasChildren {
		t.Fatalf("expected HasChildren to be mapped")
	}
	if string(client.ChildrenList) != string(children) {
		t.Fatalf("expected ChildrenList to be mapped")
	}
	if client.Height == nil || *client.Height != 170 {
		t.Fatalf("expected Height to be mapped")
	}
	if string(client.DriverLicenseCategories) != string(driver) {
		t.Fatalf("expected DriverLicenseCategories to be mapped")
	}
}
