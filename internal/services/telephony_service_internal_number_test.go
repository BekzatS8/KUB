package services

import (
	"context"
	"testing"

	"turcompany/internal/models"
)

type telephonyRepoStub struct {
	findClientByPhoneCalled bool
	findLeadByPhoneCalled   bool
	createLeadCalled        bool
	upsertCallCalled        bool
}

func (r *telephonyRepoStub) CreateCall(context.Context, *models.TelephonyCall) (int64, error) {
	return 0, nil
}

func (r *telephonyRepoStub) UpsertCall(context.Context, *models.TelephonyCall) (int64, bool, error) {
	r.upsertCallCalled = true
	return 99, true, nil
}

func (r *telephonyRepoStub) GetByID(context.Context, int64) (*models.TelephonyCallResponse, error) {
	return nil, nil
}

func (r *telephonyRepoStub) FindByExternalCallID(context.Context, string, string) (*models.TelephonyCall, error) {
	return nil, nil
}

func (r *telephonyRepoStub) List(context.Context, models.TelephonyCallListFilter) ([]*models.TelephonyCallResponse, int, error) {
	return nil, 0, nil
}

func (r *telephonyRepoStub) ListByClient(context.Context, int64, int, int) ([]*models.TelephonyCallResponse, int, error) {
	return nil, 0, nil
}

func (r *telephonyRepoStub) ListByLead(context.Context, int64, int, int) ([]*models.TelephonyCallResponse, int, error) {
	return nil, 0, nil
}

func (r *telephonyRepoStub) LinkToClient(context.Context, int64, int64) error { return nil }
func (r *telephonyRepoStub) LinkToLead(context.Context, int64, int64) error   { return nil }

func (r *telephonyRepoStub) FindClientByPhone(context.Context, string) (int64, error) {
	r.findClientByPhoneCalled = true
	return 0, nil
}

func (r *telephonyRepoStub) FindLeadByPhone(context.Context, string) (int64, error) {
	r.findLeadByPhoneCalled = true
	return 0, nil
}

func (r *telephonyRepoStub) CreateLeadFromCall(context.Context, string, string, *int, *int) (int64, error) {
	r.createLeadCalled = true
	return 0, nil
}

func (r *telephonyRepoStub) FindManagerByExtension(context.Context, string) (int, int, error) {
	return 0, 0, nil
}

func TestIngestCall_SkipsLeadCreationForInternalBinotelNumber(t *testing.T) {
	repo := &telephonyRepoStub{}
	svc := NewTelephonyService(repo, nil, "")

	phone := "+103"
	normalized := "103"
	call := &models.TelephonyCall{
		Provider:        "binotel",
		Direction:       models.CallDirectionInbound,
		Status:          models.CallStatusIncoming,
		Phone:           phone,
		NormalizedPhone: &normalized,
	}

	callID, isNew, err := svc.ingestCall(context.Background(), call)
	if err != nil {
		t.Fatalf("ingestCall error: %v", err)
	}
	if callID != 99 || !isNew {
		t.Fatalf("unexpected upsert result: id=%d isNew=%v", callID, isNew)
	}
	if !repo.findClientByPhoneCalled {
		t.Fatal("expected client lookup to run")
	}
	if !repo.findLeadByPhoneCalled {
		t.Fatal("expected lead lookup to run")
	}
	if repo.createLeadCalled {
		t.Fatal("internal Binotel number must not create a lead")
	}
	if call.LeadID != nil {
		t.Fatalf("expected LeadID to stay nil, got %v", *call.LeadID)
	}
	if !repo.upsertCallCalled {
		t.Fatal("expected call upsert to run")
	}
}
