package services

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/pdf"
)

type fakeDocumentRepo struct {
	docs []*models.Document
}

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

func (r *fakeDocumentRepo) Delete(id int64) error { return nil }

func (r *fakeDocumentRepo) UpdateStatus(id int64, status string) error { return nil }

type fakeLeadRepo struct{}

func (r *fakeLeadRepo) GetByID(id int) (*models.Leads, error) { return nil, nil }

type fakeDealRepo struct {
	deals map[int]*models.Deals
}

func (r *fakeDealRepo) GetByID(id int) (*models.Deals, error) {
	if r.deals == nil {
		return nil, nil
	}
	return r.deals[id], nil
}

func (r *fakeDealRepo) GetByLeadID(leadID int) (*models.Deals, error) { return nil, nil }

func (r *fakeDealRepo) GetLatestByClientID(clientID int) (*models.Deals, error) {
	for _, d := range r.deals {
		if d != nil && d.ClientID == clientID {
			return d, nil
		}
	}
	return nil, nil
}

type fakeClientRepo struct {
	clients map[int]*models.Client
}

func (r *fakeClientRepo) GetByID(id int) (*models.Client, error) {
	if r.clients == nil {
		return nil, nil
	}
	return r.clients[id], nil
}

type fakeSMSConfirmRepo struct{}

func (r *fakeSMSConfirmRepo) Create(sms *models.SMSConfirmation) (int64, error) { return 0, nil }
func (r *fakeSMSConfirmRepo) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	return nil, nil
}
func (r *fakeSMSConfirmRepo) GetByDocumentIDAndCode(documentID int64, code string) (*models.SMSConfirmation, error) {
	return nil, nil
}
func (r *fakeSMSConfirmRepo) Update(sms *models.SMSConfirmation) error { return nil }

type fakePDFGen struct {
	lastTemplate string
	lastFilename string
}

func (g *fakePDFGen) GenerateContract(data pdf.ContractData) (string, error) {
	g.lastTemplate = "contract"
	g.lastFilename = data.Filename
	return "/pdf/test_contract.pdf", nil
}

func (g *fakePDFGen) GenerateInvoice(data pdf.InvoiceData) (string, error) {
	g.lastTemplate = "invoice"
	g.lastFilename = data.Filename
	return "/pdf/test_invoice.pdf", nil
}

func (g *fakePDFGen) GenerateFromTemplate(templateName string, placeholders map[string]string, filename string) (string, error) {
	g.lastTemplate = templateName
	g.lastFilename = filename
	base := strings.TrimSuffix(templateName, filepath.Ext(templateName))
	return "/pdf/test_" + base + ".pdf", nil
}

type fakeDocxGen struct {
	lastTemplate string
	lastBase     string
}

func (g *fakeDocxGen) GenerateDocxAndPDF(templateName string, placeholders map[string]string, baseFilename string) (string, string, error) {
	g.lastTemplate = templateName
	g.lastBase = baseFilename
	return "/docx/" + baseFilename + ".docx", "/pdf/" + baseFilename + ".pdf", nil
}

func (g *fakeDocxGen) GeneratePDF(templateName string, placeholders map[string]string, baseFilename string) (string, error) {
	g.lastTemplate = templateName
	g.lastBase = baseFilename
	return "/pdf/" + baseFilename + ".pdf", nil
}

type fakeXlsxGen struct {
	lastTemplate string
	lastBase     string
}

func (g *fakeXlsxGen) GenerateFromTemplate(templateName string, placeholders map[string]string, baseFilename string) (string, error) {
	g.lastTemplate = templateName
	g.lastBase = baseFilename
	return "/excel/" + baseFilename + ".xlsx", nil
}

func TestCreateDocumentFromClient(t *testing.T) {
	t.Helper()

	baseClient := &models.Client{ID: 1, FirstName: "Ivan", LastName: "Ivanov", Address: "Earth"}
	deal := &models.Deals{ID: 10, ClientID: baseClient.ID, OwnerID: 99}

	tests := []struct {
		name            string
		docType         string
		setupGenerators func() (*fakePDFGen, *fakeDocxGen, *fakeXlsxGen)
		setupRepos      func(*fakeDocumentRepo, *fakeDealRepo, *fakeClientRepo)
		assert          func(t *testing.T, doc *models.Document, pdfGen *fakePDFGen, docxGen *fakeDocxGen, xlsxGen *fakeXlsxGen)
	}{
		{
			name:    "docx and pdf contract_full",
			docType: "contract_full",
			setupGenerators: func() (*fakePDFGen, *fakeDocxGen, *fakeXlsxGen) {
				return &fakePDFGen{}, &fakeDocxGen{}, &fakeXlsxGen{}
			},
			setupRepos: func(d *fakeDocumentRepo, deals *fakeDealRepo, clients *fakeClientRepo) {
				clients.clients = map[int]*models.Client{baseClient.ID: baseClient}
				deals.deals = map[int]*models.Deals{deal.ID: deal}
			},
			assert: func(t *testing.T, doc *models.Document, pdfGen *fakePDFGen, docxGen *fakeDocxGen, xlsxGen *fakeXlsxGen) {
				t.Helper()
				if doc.DocType != "contract_full" {
					t.Fatalf("DocType = %s, want contract_full", doc.DocType)
				}
				if doc.FilePath == "" || !strings.HasPrefix(doc.FilePath, "/pdf/") {
					t.Fatalf("FilePath not set to pdf path: %s", doc.FilePath)
				}
				if doc.FilePathPdf == "" {
					t.Fatalf("FilePathPdf is empty")
				}
				if doc.FilePathDocx == "" {
					t.Fatalf("FilePathDocx is empty")
				}
				if doc.DealID != int64(deal.ID) {
					t.Fatalf("DealID = %d, want %d", doc.DealID, deal.ID)
				}
				if docxGen.lastTemplate != "contract_full.docx" {
					t.Fatalf("docx template = %s, want contract_full.docx", docxGen.lastTemplate)
				}
			},
		},
		{
			name:    "txt fallback contract",
			docType: "contract",
			setupGenerators: func() (*fakePDFGen, *fakeDocxGen, *fakeXlsxGen) {
				return &fakePDFGen{}, &fakeDocxGen{}, &fakeXlsxGen{}
			},
			setupRepos: func(d *fakeDocumentRepo, deals *fakeDealRepo, clients *fakeClientRepo) {
				clients.clients = map[int]*models.Client{baseClient.ID: baseClient}
				deals.deals = map[int]*models.Deals{deal.ID: deal}
			},
			assert: func(t *testing.T, doc *models.Document, pdfGen *fakePDFGen, docxGen *fakeDocxGen, xlsxGen *fakeXlsxGen) {
				t.Helper()
				if doc.FilePath == "" || !strings.HasPrefix(doc.FilePath, "/pdf/test_contract") {
					t.Fatalf("expected pdf fallback path, got %s", doc.FilePath)
				}
				if doc.FilePathPdf != doc.FilePath {
					t.Fatalf("FilePathPdf mismatch: %s vs %s", doc.FilePathPdf, doc.FilePath)
				}
				if doc.FilePathDocx != "" {
					t.Fatalf("expected empty FilePathDocx, got %s", doc.FilePathDocx)
				}
				if pdfGen.lastTemplate != "contract_full.txt" {
					t.Fatalf("pdf template = %s, want contract_full.txt", pdfGen.lastTemplate)
				}
			},
		},
		{
			name:    "excel personal_data_excel",
			docType: "personal_data_excel",
			setupGenerators: func() (*fakePDFGen, *fakeDocxGen, *fakeXlsxGen) {
				return &fakePDFGen{}, &fakeDocxGen{}, &fakeXlsxGen{}
			},
			setupRepos: func(d *fakeDocumentRepo, deals *fakeDealRepo, clients *fakeClientRepo) {
				clients.clients = map[int]*models.Client{
					baseClient.ID: {
						ID:                  baseClient.ID,
						FirstName:           "Maria",
						LastName:            "Petrova",
						MiddleName:          "A",
						RegistrationAddress: "Main st",
						ActualAddress:       "Home",
						IIN:                 "123",
					},
				}
				deals.deals = map[int]*models.Deals{deal.ID: deal}
			},
			assert: func(t *testing.T, doc *models.Document, pdfGen *fakePDFGen, docxGen *fakeDocxGen, xlsxGen *fakeXlsxGen) {
				t.Helper()
				if doc.DocType != "personal_data_excel" {
					t.Fatalf("DocType = %s, want personal_data_excel", doc.DocType)
				}
				if doc.FilePath == "" || !strings.HasPrefix(doc.FilePath, "/excel/") {
					t.Fatalf("expected excel path, got %s", doc.FilePath)
				}
				if doc.FilePathPdf != "" || doc.FilePathDocx != "" {
					t.Fatalf("expected pdf/docx paths empty, got %s %s", doc.FilePathPdf, doc.FilePathDocx)
				}
				if doc.DealID != int64(deal.ID) {
					t.Fatalf("DealID = %d, want %d", doc.DealID, deal.ID)
				}
				if xlsxGen.lastTemplate != "personal_data.xlsx" {
					t.Fatalf("xlsx template = %s, want personal_data.xlsx", xlsxGen.lastTemplate)
				}
				if xlsxGen.lastBase == "" {
					t.Fatalf("expected non-empty base filename")
				}
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			docRepo := &fakeDocumentRepo{}
			dealRepo := &fakeDealRepo{}
			clientRepo := &fakeClientRepo{}
			pdfGen, docxGen, xlsxGen := tt.setupGenerators()

			tt.setupRepos(docRepo, dealRepo, clientRepo)

			svc := NewDocumentService(docRepo, &fakeLeadRepo{}, dealRepo, clientRepo, &fakeSMSConfirmRepo{}, "sign", "files", pdfGen, docxGen, xlsxGen)

			doc, err := svc.CreateDocumentFromClient(baseClient.ID, deal.ID, tt.docType, deal.OwnerID, authz.RoleOperations, nil)
			if err != nil {
				t.Fatalf("CreateDocumentFromClient returned error: %v", err)
			}
			if doc == nil {
				t.Fatalf("expected document, got nil")
			}
			tt.assert(t, doc, pdfGen, docxGen, xlsxGen)
		})
	}
}

func TestCreateDocumentFromClientErrors(t *testing.T) {
	t.Helper()

	baseClient := &models.Client{ID: 1}
	baseDeal := &models.Deals{ID: 2, ClientID: baseClient.ID, OwnerID: 1}

	newService := func(docRepo *fakeDocumentRepo, dealRepo *fakeDealRepo, clientRepo *fakeClientRepo) *DocumentService {
		return NewDocumentService(docRepo, &fakeLeadRepo{}, dealRepo, clientRepo, &fakeSMSConfirmRepo{}, "sign", "files", &fakePDFGen{}, &fakeDocxGen{}, &fakeXlsxGen{})
	}

	t.Run("no client", func(t *testing.T) {
		t.Helper()
		docRepo := &fakeDocumentRepo{}
		dealRepo := &fakeDealRepo{deals: map[int]*models.Deals{baseDeal.ID: baseDeal}}
		clientRepo := &fakeClientRepo{}

		svc := newService(docRepo, dealRepo, clientRepo)
		if _, err := svc.CreateDocumentFromClient(baseClient.ID, baseDeal.ID, "contract", baseDeal.OwnerID, authz.RoleOperations, nil); err == nil || err.Error() != "client not found" {
			t.Fatalf("expected client not found error, got %v", err)
		}
	})

	t.Run("no deal when needed", func(t *testing.T) {
		t.Helper()
		docRepo := &fakeDocumentRepo{}
		dealRepo := &fakeDealRepo{}
		clientRepo := &fakeClientRepo{clients: map[int]*models.Client{baseClient.ID: baseClient}}

		svc := newService(docRepo, dealRepo, clientRepo)
		if _, err := svc.CreateDocumentFromClient(baseClient.ID, 0, "contract", baseDeal.OwnerID, authz.RoleOperations, nil); err == nil || err.Error() != "deal not found" {
			t.Fatalf("expected deal not found error, got %v", err)
		}
	})

	t.Run("forbidden sales foreign deal", func(t *testing.T) {
		t.Helper()
		docRepo := &fakeDocumentRepo{}
		dealRepo := &fakeDealRepo{deals: map[int]*models.Deals{baseDeal.ID: {ID: baseDeal.ID, ClientID: baseClient.ID, OwnerID: 999}}}
		clientRepo := &fakeClientRepo{clients: map[int]*models.Client{baseClient.ID: baseClient}}

		svc := newService(docRepo, dealRepo, clientRepo)
		if _, err := svc.CreateDocumentFromClient(baseClient.ID, baseDeal.ID, "contract", 1, authz.RoleSales, nil); err == nil || err.Error() != "forbidden" {
			t.Fatalf("expected forbidden error, got %v", err)
		}
	})
}

func TestNewDocumentService(t *testing.T) {
	t.Helper()

	docRepo := &fakeDocumentRepo{}
	leadRepo := &fakeLeadRepo{}
	dealRepo := &fakeDealRepo{}
	clientRepo := &fakeClientRepo{}
	smsRepo := &fakeSMSConfirmRepo{}
	pdfGen := &fakePDFGen{}
	docxGen := &fakeDocxGen{}
	xlsxGen := &fakeXlsxGen{}

	svc := NewDocumentService(docRepo, leadRepo, dealRepo, clientRepo, smsRepo, "secret", "/files", pdfGen, docxGen, xlsxGen)

	if svc.DocRepo != docRepo {
		t.Fatalf("DocRepo not set")
	}
	if svc.LeadRepo != leadRepo {
		t.Fatalf("LeadRepo not set")
	}
	if svc.DealRepo != dealRepo {
		t.Fatalf("DealRepo not set")
	}
	if svc.ClientRepo != clientRepo {
		t.Fatalf("ClientRepo not set")
	}
	if svc.SMSRepo != smsRepo {
		t.Fatalf("SMSRepo not set")
	}
	if svc.PDFGen != pdfGen || svc.DocxGen != docxGen || svc.XlsxGen != xlsxGen {
		t.Fatalf("generators not set")
	}
	if svc.SignSecret != "secret" || svc.FilesRoot != "/files" {
		t.Fatalf("config fields not set")
	}
}
