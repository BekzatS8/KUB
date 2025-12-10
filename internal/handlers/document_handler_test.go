package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/pdf"
	"turcompany/internal/services"
)

type fakeDocumentRepo struct {
	docs []*models.Document
}

func (r *fakeDocumentRepo) Create(doc *models.Document) (int64, error) {
	doc.ID = int64(len(r.docs) + 1)
	r.docs = append(r.docs, doc)
	return doc.ID, nil
}

func (r *fakeDocumentRepo) GetByID(id int64) (*models.Document, error) { return nil, nil }
func (r *fakeDocumentRepo) ListDocuments(limit, offset int) ([]*models.Document, error) {
	return nil, nil
}
func (r *fakeDocumentRepo) ListDocumentsByDeal(dealID int64) ([]*models.Document, error) {
	return nil, nil
}
func (r *fakeDocumentRepo) Delete(id int64) error                      { return nil }
func (r *fakeDocumentRepo) UpdateStatus(id int64, status string) error { return nil }

type fakeDealRepo struct {
	deals map[int]*models.Deals
}

func (r *fakeDealRepo) GetByID(id int) (*models.Deals, error) {
	if d, ok := r.deals[id]; ok {
		return d, nil
	}
	return nil, nil
}
func (r *fakeDealRepo) GetByLeadID(leadID int) (*models.Deals, error) { return nil, nil }
func (r *fakeDealRepo) GetLatestByClientID(clientID int) (*models.Deals, error) {
	for _, d := range r.deals {
		if d.ClientID == clientID {
			return d, nil
		}
	}
	return nil, nil
}

type fakeClientRepo struct {
	clients map[int]*models.Client
}

func (r *fakeClientRepo) GetByID(id int) (*models.Client, error) {
	if c, ok := r.clients[id]; ok {
		return c, nil
	}
	return nil, nil
}

type fakeSMSConfirmRepo struct{}

func (fakeSMSConfirmRepo) Create(sms *models.SMSConfirmation) (int64, error) { return 0, nil }
func (fakeSMSConfirmRepo) GetLatestByDocumentID(documentID int64) (*models.SMSConfirmation, error) {
	return nil, nil
}
func (fakeSMSConfirmRepo) GetByDocumentIDAndCode(documentID int64, code string) (*models.SMSConfirmation, error) {
	return nil, nil
}
func (fakeSMSConfirmRepo) Update(sms *models.SMSConfirmation) error { return nil }

type fakePDFGen struct {
	lastTemplate string
	lastFilename string
}

func (g *fakePDFGen) GenerateContract(data pdf.ContractData) (string, error) {
	g.lastTemplate = data.Filename
	g.lastFilename = data.Filename
	return "/pdf/test_contract.pdf", nil
}
func (g *fakePDFGen) GenerateInvoice(data pdf.InvoiceData) (string, error) {
	g.lastTemplate = data.Filename
	g.lastFilename = data.Filename
	return "/pdf/test_invoice.pdf", nil
}
func (g *fakePDFGen) GenerateFromTemplate(templateName string, placeholders map[string]string, filename string) (string, error) {
	g.lastTemplate = templateName
	g.lastFilename = filename
	return "/pdf/test_template.pdf", nil
}

type fakeDocxGen struct {
	lastTemplate string
	lastBase     string
}

func (g *fakeDocxGen) GenerateDocxAndPDF(templatePath string, placeholders map[string]string, baseFilename string) (string, string, error) {
	g.lastTemplate = templatePath
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

func withUser(roleID, userID int) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("role_id", roleID)
		c.Set("user_id", userID)
	}
}

func TestDocumentHandler_CreateDocumentFromClient_Success(t *testing.T) {
	docRepo := &fakeDocumentRepo{}
	dealRepo := &fakeDealRepo{deals: map[int]*models.Deals{2: {ID: 2, ClientID: 1, OwnerID: 10}}}
	clientRepo := &fakeClientRepo{clients: map[int]*models.Client{1: {ID: 1, Name: "Acme"}}}
	docxGen := &fakeDocxGen{}

	svc := services.NewDocumentService(docRepo, nil, dealRepo, clientRepo, fakeSMSConfirmRepo{}, "", "", &fakePDFGen{}, docxGen, &fakeXlsxGen{})
	handler := NewDocumentHandler(svc)

	r := gin.Default()
	r.Use(withUser(authz.RoleManagement, 10))
	r.POST("/documents/create-from-client", handler.CreateDocumentFromClient)

	payload := map[string]interface{}{
		"client_id": 1,
		"deal_id":   2,
		"doc_type":  "contract_full",
		"extra": map[string]string{
			"SOME_KEY": "VALUE",
		},
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/documents/create-from-client", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var doc models.Document
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if doc.DocType != "contract_full" {
		t.Errorf("doc_type = %s, want %s", doc.DocType, "contract_full")
	}
	if doc.FilePath == "" || doc.FilePathDocx == "" || doc.FilePathPdf == "" {
		t.Errorf("expected file paths to be populated, got %+v", doc)
	}
	if doc.DealID != 2 {
		t.Errorf("deal_id = %d, want %d", doc.DealID, 2)
	}
	if docxGen.lastTemplate == "" {
		t.Errorf("expected docx generator to be used")
	}
}

func TestDocumentHandler_CreateDocumentFromClient_ClientNotFound(t *testing.T) {
	docRepo := &fakeDocumentRepo{}
	dealRepo := &fakeDealRepo{deals: map[int]*models.Deals{2: {ID: 2, ClientID: 1}}}
	clientRepo := &fakeClientRepo{clients: map[int]*models.Client{}}

	svc := services.NewDocumentService(docRepo, nil, dealRepo, clientRepo, fakeSMSConfirmRepo{}, "", "", &fakePDFGen{}, &fakeDocxGen{}, &fakeXlsxGen{})
	handler := NewDocumentHandler(svc)

	r := gin.Default()
	r.Use(withUser(authz.RoleManagement, 10))
	r.POST("/documents/create-from-client", handler.CreateDocumentFromClient)

	payload := map[string]interface{}{
		"client_id": 1,
		"deal_id":   2,
		"doc_type":  "contract_full",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/documents/create-from-client", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var apiErr APIError
	if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if apiErr.ErrorCode == "" {
		t.Errorf("expected error_code to be set")
	}
	if apiErr.Message == "" {
		t.Errorf("expected message to be set")
	}
}

// ensure gin test mode for this file too
func TestGinModeIsTest_Document(t *testing.T) {
	if gin.Mode() != gin.TestMode {
		t.Fatalf("gin mode = %s, want %s", gin.Mode(), gin.TestMode)
	}
}
