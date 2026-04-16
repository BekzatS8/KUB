package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
	"turcompany/internal/services"
)

type documentDealPaginationRepoStub struct {
	dealItems []*models.Document
	dealTotal int
}

func (s *documentDealPaginationRepoStub) Create(*models.Document) (int64, error) {
	return 0, errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) GetByID(int64) (*models.Document, error) {
	return nil, errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) GetByIDWithArchiveScope(int64, repositories.ArchiveScope) (*models.Document, error) {
	return nil, errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) ListDocuments(int, int) ([]*models.Document, error) {
	return nil, errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) ListDocumentsWithArchiveScope(int, int, repositories.ArchiveScope) ([]*models.Document, error) {
	return nil, errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) ListDocumentsByDeal(int64) ([]*models.Document, error) {
	return s.dealItems, nil
}
func (s *documentDealPaginationRepoStub) ListDocumentsByDealWithArchiveScope(int64, repositories.ArchiveScope) ([]*models.Document, error) {
	return s.dealItems, nil
}
func (s *documentDealPaginationRepoStub) Delete(int64) error { return errors.New("not implemented") }
func (s *documentDealPaginationRepoStub) Archive(int64, int, string) error {
	return errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) Unarchive(int64) error { return errors.New("not implemented") }
func (s *documentDealPaginationRepoStub) UpdateStatus(int64, string) error {
	return errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) MarkSigned(int64, string, time.Time) error {
	return errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) Update(*models.Document) error {
	return errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) UpdateSigningMeta(int64, string, string, string, string) error {
	return errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) ListDocumentsWithFilterAndArchiveScope(int, int, repositories.DocumentListFilter, repositories.ArchiveScope) ([]*models.Document, error) {
	return nil, errors.New("not implemented")
}
func (s *documentDealPaginationRepoStub) ListDocumentsByDealWithFilterAndArchiveScope(int64, repositories.DocumentListFilter, repositories.ArchiveScope) ([]*models.Document, error) {
	return s.dealItems, nil
}
func (s *documentDealPaginationRepoStub) CountDocumentsWithFilterAndArchiveScope(repositories.DocumentListFilter, repositories.ArchiveScope) (int, error) {
	return s.dealTotal, nil
}
func (s *documentDealPaginationRepoStub) ListDocumentsByDealWithFilterAndArchiveScopePaginated(int64, int, int, repositories.DocumentListFilter, repositories.ArchiveScope) ([]*models.Document, error) {
	return s.dealItems, nil
}

type documentDealPaginationDealRepoStub struct{}

func (s *documentDealPaginationDealRepoStub) GetByID(id int) (*models.Deals, error) {
	return &models.Deals{ID: id, OwnerID: 999}, nil
}
func (s *documentDealPaginationDealRepoStub) GetByLeadID(int) (*models.Deals, error) {
	return nil, errors.New("not implemented")
}
func (s *documentDealPaginationDealRepoStub) GetLatestByClientID(int) (*models.Deals, error) {
	return nil, errors.New("not implemented")
}
func (s *documentDealPaginationDealRepoStub) GetLatestByClientRef(int, string) (*models.Deals, error) {
	return nil, errors.New("not implemented")
}

func newDocumentDealPaginationRouter(docRepo services.DocumentRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewDocumentHandler(&services.DocumentService{
		DocRepo:    docRepo,
		DealRepo:   &documentDealPaginationDealRepoStub{},
		LeadRepo:   nil,
		ClientRepo: nil,
	})
	r.Use(func(c *gin.Context) {
		c.Set("user_id", 999)
		c.Set("role_id", authz.RoleOperations)
		c.Next()
	})
	r.GET("/documents/deal/:dealid", h.ListDocumentsByDeal)
	return r
}

func TestListDocumentsByDeal_PaginatedEmptyIncludesPagination(t *testing.T) {
	r := newDocumentDealPaginationRouter(&documentDealPaginationRepoStub{dealItems: nil, dealTotal: 0})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/documents/deal/12?paginate=true&page=1&size=15", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var got struct {
		Items      []json.RawMessage `json:"items"`
		Pagination *struct {
			Page      int  `json:"page"`
			Size      int  `json:"size"`
			Total     int  `json:"total"`
			HasNext   bool `json:"has_next"`
			HasPrev   bool `json:"has_prev"`
			TotalPage int  `json:"total_pages"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v body=%s", err, w.Body.String())
	}
	if got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("expected empty items array, got %#v", got.Items)
	}
	if got.Pagination == nil {
		t.Fatalf("expected pagination metadata, got nil")
	}
	if got.Pagination.Total != 0 || got.Pagination.Page != 1 || got.Pagination.Size != 15 || got.Pagination.HasNext || got.Pagination.HasPrev || got.Pagination.TotalPage != 0 {
		t.Fatalf("unexpected pagination payload: %+v", got.Pagination)
	}
}

func TestListDocumentsByDeal_LegacyModeStaysPlainArray(t *testing.T) {
	r := newDocumentDealPaginationRouter(&documentDealPaginationRepoStub{dealItems: []*models.Document{}, dealTotal: 0})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/documents/deal/12", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &arr); err != nil {
		t.Fatalf("expected legacy plain array response, got body=%s err=%v", w.Body.String(), err)
	}
	if len(arr) != 0 {
		t.Fatalf("expected empty plain array, got %d items", len(arr))
	}
}
