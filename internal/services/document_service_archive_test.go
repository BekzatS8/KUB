package services

import (
	"errors"
	"testing"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type docRepoArchiveStub struct {
	doc        *models.Document
	scope      repositories.ArchiveScope
	archived   bool
	unarchived bool
	deleted    bool
}

func (r *docRepoArchiveStub) Create(*models.Document) (int64, error) { return 1, nil }
func (r *docRepoArchiveStub) GetByID(id int64) (*models.Document, error) {
	if r.doc != nil && r.doc.ID == id && !r.doc.IsArchived {
		return r.doc, nil
	}
	return nil, nil
}
func (r *docRepoArchiveStub) GetByIDWithArchiveScope(id int64, scope repositories.ArchiveScope) (*models.Document, error) {
	r.scope = scope
	if r.doc != nil && r.doc.ID == id {
		return r.doc, nil
	}
	return nil, nil
}
func (r *docRepoArchiveStub) ListDocuments(int, int) ([]*models.Document, error) { return nil, nil }
func (r *docRepoArchiveStub) ListDocumentsWithArchiveScope(limit, offset int, scope repositories.ArchiveScope) ([]*models.Document, error) {
	r.scope = scope
	return []*models.Document{}, nil
}
func (r *docRepoArchiveStub) ListDocumentsByDeal(int64) ([]*models.Document, error) { return nil, nil }
func (r *docRepoArchiveStub) ListDocumentsByDealWithArchiveScope(int64, repositories.ArchiveScope) ([]*models.Document, error) {
	return []*models.Document{}, nil
}
func (r *docRepoArchiveStub) Delete(int64) error { r.deleted = true; return nil }
func (r *docRepoArchiveStub) Archive(int64, int, string) error {
	r.archived = true
	if r.doc != nil {
		r.doc.IsArchived = true
	}
	return nil
}
func (r *docRepoArchiveStub) Unarchive(int64) error {
	r.unarchived = true
	if r.doc != nil {
		r.doc.IsArchived = false
	}
	return nil
}
func (r *docRepoArchiveStub) UpdateStatus(int64, string) error          { return nil }
func (r *docRepoArchiveStub) MarkSigned(int64, string, time.Time) error { return nil }
func (r *docRepoArchiveStub) Update(*models.Document) error             { return nil }
func (r *docRepoArchiveStub) UpdateSigningMeta(int64, string, string, string, string) error {
	return nil
}

type dealRepoArchiveStub struct{ deal *models.Deals }

func (d *dealRepoArchiveStub) GetByID(id int) (*models.Deals, error) {
	if d.deal != nil && d.deal.ID == id {
		return d.deal, nil
	}
	return nil, errors.New("not found")
}
func (d *dealRepoArchiveStub) GetByLeadID(int) (*models.Deals, error)         { return nil, nil }
func (d *dealRepoArchiveStub) GetLatestByClientID(int) (*models.Deals, error) { return nil, nil }
func (d *dealRepoArchiveStub) GetLatestByClientRef(int, string) (*models.Deals, error) {
	return nil, nil
}

func TestDocumentArchiveAndDeletePermissions(t *testing.T) {
	docRepo := &docRepoArchiveStub{doc: &models.Document{ID: 1, DealID: 10, FilePath: "x.pdf"}}
	svc := &DocumentService{DocRepo: docRepo, DealRepo: &dealRepoArchiveStub{deal: &models.Deals{ID: 10, OwnerID: 200}}}

	if err := svc.DeleteDocument(1, 200, authz.RoleManagement); err == nil || err.Error() != "forbidden" {
		t.Fatalf("expected forbidden for non-admin delete")
	}
	if err := svc.ArchiveDocument(1, 200, authz.RoleManagement, "r"); err != nil {
		t.Fatalf("archive failed: %v", err)
	}
	if !docRepo.archived {
		t.Fatal("expected archive call")
	}
	if err := svc.UnarchiveDocument(1, 200, authz.RoleManagement); err != nil {
		t.Fatalf("unarchive failed: %v", err)
	}
	if !docRepo.unarchived {
		t.Fatal("expected unarchive call")
	}
}

func TestDocumentListUsesArchiveScope(t *testing.T) {
	docRepo := &docRepoArchiveStub{}
	svc := &DocumentService{DocRepo: docRepo}
	_, _ = svc.ListDocumentsWithArchiveScope(10, 0, repositories.ArchiveScopeArchivedOnly)
	if docRepo.scope != repositories.ArchiveScopeArchivedOnly {
		t.Fatalf("scope not forwarded")
	}
}
