package services

import (
	"errors"
	"strings"
	"testing"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/pdf"
)

type fakeDocumentRepo struct{ docs []*models.Document }

func (r *fakeDocumentRepo) Create(doc *models.Document) (int64, error) {
	id := int64(len(r.docs) + 1)
	copyDoc := *doc
	copyDoc.ID = id
	r.docs = append(r.docs, &copyDoc)
	return id, nil
}
func (r *fakeDocumentRepo) GetByID(id int64) (*models.Document, error) {
	for _, d := range r.docs {
		if d.ID == id {
			return d, nil
		}
	}
	return nil, errors.New("not found")
}
func (r *fakeDocumentRepo) ListDocuments(limit, offset int) ([]*models.Document, error) {
	return nil, nil
}
func (r *fakeDocumentRepo) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) {
	return nil, nil
}
func (r *fakeDocumentRepo) Delete(id int64) error                      { return nil }
func (r *fakeDocumentRepo) UpdateStatus(id int64, status string) error { return nil }
func (r *fakeDocumentRepo) MarkSigned(id int64, signedBy string, signedAt time.Time) error {
	return nil
}
func (r *fakeDocumentRepo) Update(doc *models.Document) error { return nil }
func (r *fakeDocumentRepo) UpdateSigningMeta(id int64, signMethod, signIP, signUserAgent, signMetadata string) error {
	return nil
}

type fakeLeadRepo struct{}

func (r *fakeLeadRepo) GetByID(id int) (*models.Leads, error) { return nil, nil }

type fakeDealRepo struct{ deals map[int]*models.Deals }

func (r *fakeDealRepo) GetByID(id int) (*models.Deals, error)         { return r.deals[id], nil }
func (r *fakeDealRepo) GetByLeadID(leadID int) (*models.Deals, error) { return nil, nil }
func (r *fakeDealRepo) GetLatestByClientID(clientID int) (*models.Deals, error) {
	for _, d := range r.deals {
		if d != nil && d.ClientID == clientID {
			return d, nil
		}
	}
	return nil, nil
}

type fakeClientRepo struct{ clients map[int]*models.Client }

func (r *fakeClientRepo) GetByID(id int) (*models.Client, error) { return r.clients[id], nil }

type fakePDFGen struct{}

func (g *fakePDFGen) GenerateContract(data pdf.ContractData) (string, error) {
	return "/pdf/test_contract.pdf", nil
}
func (g *fakePDFGen) GenerateInvoice(data pdf.InvoiceData) (string, error) {
	return "/pdf/test_invoice.pdf", nil
}
func (g *fakePDFGen) GenerateFromTemplate(templateName string, placeholders map[string]string, filename string) (string, error) {
	return "/pdf/test_template.pdf", nil
}

type fakeDocxGen struct {
	lastTemplate     string
	lastPlaceholders map[string]string
}

func (g *fakeDocxGen) GenerateDocxAndPDF(templateName string, placeholders map[string]string, baseFilename string) (string, string, error) {
	g.lastTemplate = templateName
	g.lastPlaceholders = map[string]string{}
	for k, v := range placeholders {
		g.lastPlaceholders[k] = v
	}
	return "/docx/" + baseFilename + ".docx", "/pdf/" + baseFilename + ".pdf", nil
}
func (g *fakeDocxGen) GeneratePDF(templateName string, placeholders map[string]string, baseFilename string) (string, error) {
	g.lastTemplate = templateName
	g.lastPlaceholders = map[string]string{}
	for k, v := range placeholders {
		g.lastPlaceholders[k] = v
	}
	return "/pdf/" + baseFilename + ".pdf", nil
}

type fakeXlsxGen struct{ lastTemplate string }

func (g *fakeXlsxGen) GenerateFromTemplate(templateName string, placeholders map[string]string, baseFilename string) (string, error) {
	g.lastTemplate = templateName
	return "/excel/" + baseFilename + ".xlsx", nil
}
func (g *fakeXlsxGen) GenerateFromTemplateAndPDF(templateName string, placeholders map[string]string, baseFilename string) (string, string, error) {
	g.lastTemplate = templateName
	return "/excel/" + baseFilename + ".xlsx", "/pdf/" + baseFilename + ".pdf", nil
}

func TestCreateDocumentFromClient_RegistryDocxAndXlsx(t *testing.T) {
	baseClient := &models.Client{ID: 1, FirstName: "Ivan", LastName: "Ivanov", Address: "Earth", IIN: "123456789012", Phone: "+77770000000", IDNumber: "ID-1", PassportNumber: "P-1"}
	deal := &models.Deals{ID: 10, ClientID: baseClient.ID, OwnerID: 99}

	t.Run("docx", func(t *testing.T) {
		repo := &fakeDocumentRepo{}
		docx := &fakeDocxGen{}
		svc := NewDocumentService(repo, &fakeLeadRepo{}, &fakeDealRepo{deals: map[int]*models.Deals{deal.ID: deal}}, &fakeClientRepo{clients: map[int]*models.Client{baseClient.ID: baseClient}}, "", "files", &fakePDFGen{}, docx, &fakeXlsxGen{})
		doc, err := svc.CreateDocumentFromClient(baseClient.ID, deal.ID, "contract_paid_full_ru", deal.OwnerID, authz.RoleOperations, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if doc.FilePathPdf == "" || doc.FilePathDocx == "" {
			t.Fatalf("expected pdf+docx paths")
		}
		if !strings.HasSuffix(docx.lastTemplate, ".docx") {
			t.Fatalf("unexpected template: %s", docx.lastTemplate)
		}
	})

	t.Run("xlsx", func(t *testing.T) {
		repo := &fakeDocumentRepo{}
		xlsx := &fakeXlsxGen{}
		svc := NewDocumentService(repo, &fakeLeadRepo{}, &fakeDealRepo{deals: map[int]*models.Deals{deal.ID: deal}}, &fakeClientRepo{clients: map[int]*models.Client{baseClient.ID: baseClient}}, "", "files", &fakePDFGen{}, &fakeDocxGen{}, xlsx)
		doc, err := svc.CreateDocumentFromClient(baseClient.ID, deal.ID, "avr_kub_group", deal.OwnerID, authz.RoleOperations, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !strings.HasPrefix(doc.FilePath, "/excel/") || !strings.HasPrefix(doc.FilePathPdf, "/pdf/") {
			t.Fatalf("unexpected paths: %+v", doc)
		}
		if xlsx.lastTemplate != "avr_kub_group.xlsx" {
			t.Fatalf("template mismatch: %s", xlsx.lastTemplate)
		}
	})
}

func TestCreateDocumentFromClient_MissingFieldsStructured(t *testing.T) {
	client := &models.Client{ID: 1, FirstName: "Ivan", LastName: "Ivanov", IIN: "123"}
	deal := &models.Deals{ID: 10, ClientID: client.ID, OwnerID: 99}
	svc := NewDocumentService(&fakeDocumentRepo{}, &fakeLeadRepo{}, &fakeDealRepo{deals: map[int]*models.Deals{deal.ID: deal}}, &fakeClientRepo{clients: map[int]*models.Client{client.ID: client}}, "", "files", &fakePDFGen{}, &fakeDocxGen{}, &fakeXlsxGen{})
	_, err := svc.CreateDocumentFromClient(client.ID, deal.ID, "pause_application", deal.OwnerID, authz.RoleOperations, nil)
	if err == nil {
		t.Fatalf("expected error")
	}
	m, ok := err.(*DocumentMissingFieldsError)
	if !ok {
		t.Fatalf("unexpected err: %T %v", err, err)
	}
	if m.Scope != "pause_application" || len(m.Fields) == 0 {
		t.Fatalf("unexpected payload: %+v", m)
	}
}

func TestMergeExtra_RequestOverridesOnlyNonEmpty(t *testing.T) {
	dealExtra := map[string]string{"reason_code": "R2", "REFUND_AMOUNT_NUM": "1000"}
	reqExtra := map[string]string{"reason_code": "R1", "REFUND_AMOUNT_NUM": "", "NEW_KEY": "abc"}
	m := mergeExtra(dealExtra, reqExtra)
	if m["reason_code"] != "R1" {
		t.Fatalf("reason_code = %s", m["reason_code"])
	}
	if m["REFUND_AMOUNT_NUM"] != "1000" {
		t.Fatalf("REFUND_AMOUNT_NUM = %s", m["REFUND_AMOUNT_NUM"])
	}
	if m["NEW_KEY"] != "abc" {
		t.Fatalf("NEW_KEY = %s", m["NEW_KEY"])
	}
}

func TestReasonCodeFallbackFromDealExtra(t *testing.T) {
	baseClient := &models.Client{ID: 1, FirstName: "Ivan", LastName: "Ivanov", Address: "Earth", IIN: "123456789012", Phone: "+77770000000"}
	deal := &models.Deals{ID: 10, ClientID: baseClient.ID, OwnerID: 99, Amount: 120000}
	repo := &fakeDocumentRepo{}
	docx := &fakeDocxGen{}
	svc := NewDocumentService(repo, &fakeLeadRepo{}, &fakeDealRepo{deals: map[int]*models.Deals{deal.ID: deal}}, &fakeClientRepo{clients: map[int]*models.Client{baseClient.ID: baseClient}}, "", "files", &fakePDFGen{}, docx, &fakeXlsxGen{})
	_, err := svc.CreateDocumentFromClient(baseClient.ID, deal.ID, "pause_application", deal.OwnerID, authz.RoleOperations, map[string]string{"reason_code": "R2"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestMergeExtra_EmptyRequestUsesDealExtra(t *testing.T) {
	dealExtra := map[string]string{"reason_code": "R2", "REFUND_AMOUNT_NUM": "1000"}
	m := mergeExtra(dealExtra, map[string]string{"reason_code": "   ", "REFUND_AMOUNT_NUM": ""})
	if m["reason_code"] != "R2" {
		t.Fatalf("reason_code = %s", m["reason_code"])
	}
	if m["REFUND_AMOUNT_NUM"] != "1000" {
		t.Fatalf("REFUND_AMOUNT_NUM = %s", m["REFUND_AMOUNT_NUM"])
	}
}

func TestCreateDocumentFromClient_RequiredFieldResolvedFromDealExtra(t *testing.T) {
	client := &models.Client{ID: 1, FirstName: "Ivan", LastName: "Ivanov", Address: "Earth", IIN: "123456789012", Phone: "+77770000000"}
	deal := &models.Deals{ID: 10, ClientID: client.ID, OwnerID: 99, ExtraJSON: `{"reason_code":"R3"}`}
	svc := NewDocumentService(&fakeDocumentRepo{}, &fakeLeadRepo{}, &fakeDealRepo{deals: map[int]*models.Deals{deal.ID: deal}}, &fakeClientRepo{clients: map[int]*models.Client{client.ID: client}}, "", "files", &fakePDFGen{}, &fakeDocxGen{}, &fakeXlsxGen{})
	_, err := svc.CreateDocumentFromClient(client.ID, deal.ID, "pause_application", deal.OwnerID, authz.RoleOperations, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestReasonCodeFallbackFromDealExtra_AutoCheckboxes(t *testing.T) {
	baseClient := &models.Client{ID: 1, FirstName: "Ivan", LastName: "Ivanov", Address: "Earth", IIN: "123456789012", Phone: "+77770000000"}
	deal := &models.Deals{ID: 10, ClientID: baseClient.ID, OwnerID: 99, Amount: 120000, ExtraJSON: `{"reason_code":"R3"}`}
	repo := &fakeDocumentRepo{}
	docx := &fakeDocxGen{}
	svc := NewDocumentService(repo, &fakeLeadRepo{}, &fakeDealRepo{deals: map[int]*models.Deals{deal.ID: deal}}, &fakeClientRepo{clients: map[int]*models.Client{baseClient.ID: baseClient}}, "", "files", &fakePDFGen{}, docx, &fakeXlsxGen{})
	_, err := svc.CreateDocumentFromClient(baseClient.ID, deal.ID, "cancel_appointment", deal.OwnerID, authz.RoleOperations, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if docx.lastPlaceholders["CANCEL_R3"] != "X" {
		t.Fatalf("expected CANCEL_R3 to be X, got %q", docx.lastPlaceholders["CANCEL_R3"])
	}
}

func TestReasonCodeExplicitCheckboxesAreUsedAsIs(t *testing.T) {
	baseClient := &models.Client{ID: 1, FirstName: "Ivan", LastName: "Ivanov", Address: "Earth", IIN: "123456789012", Phone: "+77770000000"}
	deal := &models.Deals{ID: 10, ClientID: baseClient.ID, OwnerID: 99, Amount: 120000, ExtraJSON: `{"reason_code":"R3"}`}
	repo := &fakeDocumentRepo{}
	docx := &fakeDocxGen{}
	svc := NewDocumentService(repo, &fakeLeadRepo{}, &fakeDealRepo{deals: map[int]*models.Deals{deal.ID: deal}}, &fakeClientRepo{clients: map[int]*models.Client{baseClient.ID: baseClient}}, "", "files", &fakePDFGen{}, docx, &fakeXlsxGen{})
	_, err := svc.CreateDocumentFromClient(baseClient.ID, deal.ID, "cancel_appointment", deal.OwnerID, authz.RoleOperations, map[string]string{"CANCEL_R5": "X"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if docx.lastPlaceholders["CANCEL_R5"] != "X" {
		t.Fatalf("expected CANCEL_R5 to be X, got %q", docx.lastPlaceholders["CANCEL_R5"])
	}
	if docx.lastPlaceholders["CANCEL_R3"] != "" {
		t.Fatalf("expected CANCEL_R3 to stay empty with explicit flags, got %q", docx.lastPlaceholders["CANCEL_R3"])
	}
}

func TestReasonCodeRefundSetsRefundAndDocsMarkCheckboxes(t *testing.T) {
	baseClient := &models.Client{ID: 1, FirstName: "Ivan", LastName: "Ivanov", Address: "Earth", IIN: "123456789012", Phone: "+77770000000"}
	deal := &models.Deals{ID: 10, ClientID: baseClient.ID, OwnerID: 99, Amount: 120000, ExtraJSON: `{"reason_code":"R2"}`}
	repo := &fakeDocumentRepo{}
	docx := &fakeDocxGen{}
	svc := NewDocumentService(repo, &fakeLeadRepo{}, &fakeDealRepo{deals: map[int]*models.Deals{deal.ID: deal}}, &fakeClientRepo{clients: map[int]*models.Client{baseClient.ID: baseClient}}, "", "files", &fakePDFGen{}, docx, &fakeXlsxGen{})
	_, err := svc.CreateDocumentFromClient(baseClient.ID, deal.ID, "refund_application", deal.OwnerID, authz.RoleOperations, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if docx.lastPlaceholders["REFUND_R2"] != "X" {
		t.Fatalf("expected REFUND_R2 to be X, got %q", docx.lastPlaceholders["REFUND_R2"])
	}
	if docx.lastPlaceholders["DOCS_MARK_2"] != "☑" {
		t.Fatalf("expected DOCS_MARK_2 to be ☑, got %q", docx.lastPlaceholders["DOCS_MARK_2"])
	}
	if docx.lastPlaceholders["DOCS_MARK_1"] != "☐" {
		t.Fatalf("expected DOCS_MARK_1 to be ☐, got %q", docx.lastPlaceholders["DOCS_MARK_1"])
	}
}
