package services

import (
	"errors"
	"testing"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

type testDocumentRepo struct {
	created []*models.Document
}

func (r *testDocumentRepo) Create(doc *models.Document) (int64, error) {
	doc.ID = int64(len(r.created) + 1)
	r.created = append(r.created, doc)
	return doc.ID, nil
}
func (r *testDocumentRepo) GetByID(id int64) (*models.Document, error) { return nil, nil }
func (r *testDocumentRepo) ListDocuments(limit, offset int) ([]*models.Document, error) {
	return nil, nil
}
func (r *testDocumentRepo) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) {
	return nil, nil
}
func (r *testDocumentRepo) Delete(id int64) error                      { return nil }
func (r *testDocumentRepo) UpdateStatus(id int64, status string) error { return nil }
func (r *testDocumentRepo) MarkSigned(id int64, signedBy string, signedAt time.Time) error {
	return nil
}
func (r *testDocumentRepo) Update(doc *models.Document) error { return nil }
func (r *testDocumentRepo) UpdateSigningMeta(id int64, signMethod, signIP, signUserAgent, signMetadata string) error {
	return nil
}

type testLeadRepo struct{}

func (r *testLeadRepo) GetByID(id int) (*models.Leads, error) { return nil, nil }

type testDealRepo struct {
	byID               *models.Deals
	latestByClientRef  *models.Deals
	latestByClientRefE error
	lastClientID       int
	lastClientType     string
}

func (r *testDealRepo) GetByID(id int) (*models.Deals, error) { return r.byID, nil }
func (r *testDealRepo) GetByLeadID(leadID int) (*models.Deals, error) {
	return nil, nil
}
func (r *testDealRepo) GetLatestByClientID(clientID int) (*models.Deals, error) { return nil, nil }
func (r *testDealRepo) GetLatestByClientRef(clientID int, clientType string) (*models.Deals, error) {
	r.lastClientID = clientID
	r.lastClientType = clientType
	if r.latestByClientRefE != nil {
		return nil, r.latestByClientRefE
	}
	return r.latestByClientRef, nil
}

type testClientRepo struct {
	client *models.Client
}

func (r *testClientRepo) GetByID(id int) (*models.Client, error) { return r.client, nil }

type testDocxGen struct{}

func (g *testDocxGen) GenerateDocxAndPDF(templateName string, placeholders map[string]string, baseFilename string) (string, string, error) {
	return "/docx/test.docx", "/pdf/test.pdf", nil
}
func (g *testDocxGen) GeneratePDF(templateName string, placeholders map[string]string, baseFilename string) (string, error) {
	return "/pdf/test.pdf", nil
}

func TestCreateDocumentFromClient_RequiresClientType(t *testing.T) {
	svc := &DocumentService{DocxGen: &testDocxGen{}}
	_, err := svc.CreateDocumentFromClient(1, "", 0, "contract_free_ru", 1, authz.RoleManagement, nil)
	if !errors.Is(err, ErrClientTypeRequired) {
		t.Fatalf("expected ErrClientTypeRequired, got %v", err)
	}
}

func TestCreateDocumentFromClient_WrongClientTypeFails(t *testing.T) {
	svc := &DocumentService{
		ClientRepo: &testClientRepo{client: &models.Client{ID: 10, ClientType: models.ClientTypeIndividual}},
		DealRepo:   &testDealRepo{},
		DocxGen:    &testDocxGen{},
	}
	_, err := svc.CreateDocumentFromClient(10, models.ClientTypeLegal, 0, "contract_free_ru", 1, authz.RoleManagement, nil)
	if !errors.Is(err, ErrClientTypeMismatch) {
		t.Fatalf("expected ErrClientTypeMismatch, got %v", err)
	}
}

func TestCreateDocumentFromClient_InvalidClientTypeFails(t *testing.T) {
	svc := &DocumentService{
		ClientRepo: &testClientRepo{client: &models.Client{ID: 10, ClientType: models.ClientTypeIndividual}},
		DealRepo:   &testDealRepo{},
		DocxGen:    &testDocxGen{},
	}
	_, err := svc.CreateDocumentFromClient(10, "partner", 0, "contract_free_ru", 1, authz.RoleManagement, nil)
	if err == nil {
		t.Fatal("expected error for invalid client_type")
	}
}

func TestCreateDocumentFromClient_UsesLatestDealByTypedRefWhenDealIDOmitted(t *testing.T) {
	dealRepo := &testDealRepo{
		latestByClientRef: &models.Deals{
			ID:         99,
			ClientID:   10,
			ClientType: models.ClientTypeIndividual,
			OwnerID:    1,
			Amount:     1500,
			Currency:   "USD",
			Status:     "won",
			CreatedAt:  time.Now(),
		},
	}
	docRepo := &testDocumentRepo{}
	svc := &DocumentService{
		DocRepo:    docRepo,
		DealRepo:   dealRepo,
		ClientRepo: &testClientRepo{client: &models.Client{ID: 10, ClientType: models.ClientTypeIndividual, Name: "Jane Doe", BinIin: "123456789012", Address: "Almaty", Phone: "77000000000"}},
		DocxGen:    &testDocxGen{},
	}

	doc, err := svc.CreateDocumentFromClient(10, models.ClientTypeIndividual, 0, "contract_free_ru", 1, authz.RoleManagement, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc == nil || doc.ID == 0 {
		t.Fatalf("expected created document, got %#v", doc)
	}
	if dealRepo.lastClientID != 10 || dealRepo.lastClientType != models.ClientTypeIndividual {
		t.Fatalf("expected typed ref lookup (10, individual), got (%d, %q)", dealRepo.lastClientID, dealRepo.lastClientType)
	}
}
