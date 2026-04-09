package handlers

import (
	"errors"
	"testing"

	"turcompany/internal/services"
)

func TestIsLeadConversionBadRequestError_BySentinel(t *testing.T) {
	if !isLeadConversionBadRequestError(services.ErrClientTypeRequired) {
		t.Fatal("expected ErrClientTypeRequired to be treated as bad request")
	}
	if !isLeadConversionBadRequestError(services.ErrClientTypeMismatch) {
		t.Fatal("expected ErrClientTypeMismatch to be treated as bad request")
	}
}

func TestIsLeadConversionBadRequestError_ByMessage(t *testing.T) {
	cases := []string{
		"invalid client_type",
		"lead is not in a convertible status",
		"client_type is required",
		"client_type does not match",
	}

	for _, message := range cases {
		if !isLeadConversionBadRequestError(errors.New(message)) {
			t.Fatalf("expected %q to be treated as bad request", message)
		}
	}
}

func TestIsLeadConversionBadRequestError_OtherError(t *testing.T) {
	if isLeadConversionBadRequestError(errors.New("unexpected db error")) {
		t.Fatal("did not expect unknown error to be treated as bad request")
	}
}

