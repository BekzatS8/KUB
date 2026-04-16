package services

import (
	"testing"
	"time"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type signerDocRepoStub struct {
	doc *models.Document
}

func (r *signerDocRepoStub) Create(doc *models.Document) (int64, error) { return 0, nil }
func (r *signerDocRepoStub) GetByID(id int64) (*models.Document, error) { return r.doc, nil }
func (r *signerDocRepoStub) GetByIDWithArchiveScope(id int64, scope repositories.ArchiveScope) (*models.Document, error) {
	return r.doc, nil
}
func (r *signerDocRepoStub) ListDocuments(limit, offset int) ([]*models.Document, error) {
	return nil, nil
}
func (r *signerDocRepoStub) ListDocumentsWithArchiveScope(limit, offset int, scope repositories.ArchiveScope) ([]*models.Document, error) {
	return nil, nil
}
func (r *signerDocRepoStub) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) {
	return nil, nil
}
func (r *signerDocRepoStub) ListDocumentsByDealWithArchiveScope(dealID int64, scope repositories.ArchiveScope) ([]*models.Document, error) {
	return nil, nil
}
func (r *signerDocRepoStub) Delete(id int64) error                                 { return nil }
func (r *signerDocRepoStub) Archive(id int64, archivedBy int, reason string) error { return nil }
func (r *signerDocRepoStub) Unarchive(id int64) error                              { return nil }
func (r *signerDocRepoStub) UpdateStatus(id int64, status string) error            { return nil }
func (r *signerDocRepoStub) MarkSigned(id int64, signedBy string, signedAt time.Time) error {
	return nil
}
func (r *signerDocRepoStub) Update(doc *models.Document) error { return nil }
func (r *signerDocRepoStub) UpdateSigningMeta(id int64, signMethod, signIP, signUserAgent, signMetadata string) error {
	return nil
}

type signerDealRepoStub struct {
	deal *models.Deals
}

func (r *signerDealRepoStub) GetByID(id int) (*models.Deals, error)         { return r.deal, nil }
func (r *signerDealRepoStub) GetByLeadID(leadID int) (*models.Deals, error) { return nil, nil }
func (r *signerDealRepoStub) GetLatestByClientID(clientID int) (*models.Deals, error) {
	return nil, nil
}
func (r *signerDealRepoStub) GetLatestByClientRef(clientID int, clientType string) (*models.Deals, error) {
	return nil, nil
}

type signerClientRepoStub struct {
	client *models.Client
}

func (r *signerClientRepoStub) GetByID(id int) (*models.Client, error) { return r.client, nil }

func newSignerResolutionService(client *models.Client) *DocumentService {
	return &DocumentService{
		DocRepo:    &signerDocRepoStub{doc: &models.Document{ID: 101, DealID: 202}},
		DealRepo:   &signerDealRepoStub{deal: &models.Deals{ID: 202, ClientID: 303}},
		ClientRepo: &signerClientRepoStub{client: client},
	}
}

func TestResolveSignerForSMS_AllowsMissingEmailWhenPhoneExists(t *testing.T) {
	svc := newSignerResolutionService(&models.Client{ID: 303, ClientType: models.ClientTypeIndividual, Phone: "+7 777 000 0000"})
	resolved, err := svc.ResolveSignerForSMS(101, 11, 1, SignerOverrides{})
	if err != nil {
		t.Fatalf("ResolveSignerForSMS returned error: %v", err)
	}
	if resolved.Phone == "" {
		t.Fatalf("expected phone to be resolved")
	}
	if resolved.Email != "" {
		t.Fatalf("expected empty email, got %q", resolved.Email)
	}
}

func TestResolveSignerForEmail_AllowsMissingPhoneWhenEmailExists(t *testing.T) {
	svc := newSignerResolutionService(&models.Client{ID: 303, ClientType: models.ClientTypeIndividual, Email: "client@example.com"})
	resolved, err := svc.ResolveSignerForEmail(101, 11, 1, SignerOverrides{})
	if err != nil {
		t.Fatalf("ResolveSignerForEmail returned error: %v", err)
	}
	if resolved.Email != "client@example.com" {
		t.Fatalf("expected client email, got %q", resolved.Email)
	}
	if resolved.Phone != "" {
		t.Fatalf("expected empty phone, got %q", resolved.Phone)
	}
}

func TestResolveSignerForSMS_ManualPhoneOverridesClientPhone(t *testing.T) {
	svc := newSignerResolutionService(&models.Client{ID: 303, ClientType: models.ClientTypeIndividual, Phone: "+7 701 111 1111"})
	resolved, err := svc.ResolveSignerForSMS(101, 11, 1, SignerOverrides{Phone: "+7 702 222 2222"})
	if err != nil {
		t.Fatalf("ResolveSignerForSMS returned error: %v", err)
	}
	if resolved.Phone != "77022222222" {
		t.Fatalf("expected manual phone to win, got %q", resolved.Phone)
	}
}

func TestResolveSignerForEmail_ManualEmailOverridesClientEmail(t *testing.T) {
	svc := newSignerResolutionService(&models.Client{ID: 303, ClientType: models.ClientTypeIndividual, Email: "client@example.com"})
	resolved, err := svc.ResolveSignerForEmail(101, 11, 1, SignerOverrides{Email: "manual@example.com"})
	if err != nil {
		t.Fatalf("ResolveSignerForEmail returned error: %v", err)
	}
	if resolved.Email != "manual@example.com" {
		t.Fatalf("expected manual email to win, got %q", resolved.Email)
	}
}

func TestGetSigningContactOptions_PrefersSMSWhenPhoneExists(t *testing.T) {
	svc := newSignerResolutionService(&models.Client{ID: 303, ClientType: models.ClientTypeIndividual, Phone: "+7 700 123 4567", Email: "client@example.com"})
	options, err := svc.GetSigningContactOptions(101, 11, 1)
	if err != nil {
		t.Fatalf("GetSigningContactOptions returned error: %v", err)
	}
	if options.PreferredChannel != "sms" {
		t.Fatalf("expected preferred sms, got %q", options.PreferredChannel)
	}
}

func TestGetSigningContactOptions_FallsBackToEmailWhenPhoneMissing(t *testing.T) {
	svc := newSignerResolutionService(&models.Client{ID: 303, ClientType: models.ClientTypeIndividual, Email: "client@example.com"})
	options, err := svc.GetSigningContactOptions(101, 11, 1)
	if err != nil {
		t.Fatalf("GetSigningContactOptions returned error: %v", err)
	}
	if options.PreferredChannel != "email" {
		t.Fatalf("expected preferred email, got %q", options.PreferredChannel)
	}
}
