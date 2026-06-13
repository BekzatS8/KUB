package services

import (
	"testing"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// ─── isHiddenDocVisible unit tests ──────────────────────────────────────────

func TestIsHiddenDocVisible_AdminSeesHidden(t *testing.T) {
	doc := &models.Document{IsHidden: true, CreatedBy: intPtr(99)}
	if !isHiddenDocVisible(doc, 1, authz.RoleSystemAdmin) {
		t.Error("admin must see hidden documents")
	}
}

func TestIsHiddenDocVisible_AuthorSeesOwnHidden(t *testing.T) {
	doc := &models.Document{IsHidden: true, CreatedBy: intPtr(7)}
	if !isHiddenDocVisible(doc, 7, authz.RolePartner) {
		t.Error("partner-author must see their own hidden document")
	}
}

func TestIsHiddenDocVisible_ManagementCannotSeeHidden(t *testing.T) {
	doc := &models.Document{IsHidden: true, CreatedBy: intPtr(99)}
	if isHiddenDocVisible(doc, 1, authz.RoleManagement) {
		t.Error("management must NOT see hidden documents")
	}
}

func TestIsHiddenDocVisible_LegalCannotSeeHidden(t *testing.T) {
	doc := &models.Document{IsHidden: true, CreatedBy: intPtr(99)}
	if isHiddenDocVisible(doc, 1, authz.RoleLegal) {
		t.Error("legal must NOT see hidden documents")
	}
}

func TestIsHiddenDocVisible_SalesCannotSeeHidden(t *testing.T) {
	doc := &models.Document{IsHidden: true, CreatedBy: intPtr(99)}
	if isHiddenDocVisible(doc, 1, authz.RoleSales) {
		t.Error("sales must NOT see hidden documents")
	}
}

func TestIsHiddenDocVisible_PartnerCannotSeeOthersHidden(t *testing.T) {
	doc := &models.Document{IsHidden: true, CreatedBy: intPtr(99)}
	if isHiddenDocVisible(doc, 55, authz.RolePartner) {
		t.Error("partner must NOT see another partner's hidden document")
	}
}

func TestIsHiddenDocVisible_NonHiddenVisibleToAll(t *testing.T) {
	doc := &models.Document{IsHidden: false}
	for _, roleID := range []int{
		authz.RoleSystemAdmin, authz.RoleManagement, authz.RoleLegal,
		authz.RoleHR, authz.RolePartner, authz.RoleSales, authz.RoleVisa, authz.RoleControl,
	} {
		if !isHiddenDocVisible(doc, 1, roleID) {
			t.Errorf("role %d must see non-hidden documents (regression)", roleID)
		}
	}
}

// ─── GetDocument visibility via service ─────────────────────────────────────

// TestGetDocument_AdminSeesHiddenDoc verifies admin can retrieve a hidden document by ID.
func TestGetDocument_AdminSeesHiddenDoc(t *testing.T) {
	svc := &DocumentService{
		DocRepo: &docRepoStub{doc: &models.Document{ID: 1, DealID: 0, IsHidden: true, CreatedBy: intPtr(99)}},
	}
	doc, err := svc.GetDocument(1, 1, authz.RoleSystemAdmin)
	if err != nil || doc == nil {
		t.Fatalf("admin must see hidden doc, got err=%v", err)
	}
}

// TestGetDocument_PartnerAuthorSeesOwnHiddenDoc verifies the creating partner can read their doc.
func TestGetDocument_PartnerAuthorSeesOwnHiddenDoc(t *testing.T) {
	const partnerUserID = 7
	branch := 3
	svc := &DocumentService{
		DocRepo:  &docRepoStub{doc: &models.Document{ID: 1, DealID: 1, IsHidden: true, CreatedBy: intPtr(partnerUserID)}},
		DealRepo: &dealRepoStub{deal: &models.Deals{ID: 1, BranchID: &branch}},
		UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branch}},
	}
	doc, err := svc.GetDocument(1, partnerUserID, authz.RolePartner)
	if err != nil || doc == nil {
		t.Fatalf("partner-author must see own hidden doc, got err=%v", err)
	}
}

// TestGetDocument_ManagementCannotSeeHiddenDoc verifies management is blocked from hidden docs.
func TestGetDocument_ManagementCannotSeeHiddenDoc(t *testing.T) {
	svc := &DocumentService{
		DocRepo: &docRepoStub{doc: &models.Document{ID: 1, DealID: 0, IsHidden: true, CreatedBy: intPtr(99)}},
	}
	doc, err := svc.GetDocument(1, 1, authz.RoleManagement)
	if err == nil || doc != nil {
		t.Fatal("management must NOT see hidden document (expected forbidden error)")
	}
}

// TestGetDocument_LegalCannotSeeHiddenDoc verifies legal (scope=All) is blocked from hidden docs.
func TestGetDocument_LegalCannotSeeHiddenDoc(t *testing.T) {
	svc := &DocumentService{
		DocRepo: &docRepoStub{doc: &models.Document{ID: 1, DealID: 0, IsHidden: true, CreatedBy: intPtr(99)}},
	}
	doc, err := svc.GetDocument(1, 1, authz.RoleLegal)
	if err == nil || doc != nil {
		t.Fatal("legal must NOT see hidden document (expected forbidden error)")
	}
}

// TestGetDocument_HRCannotSeeHiddenDoc verifies HR (scope=All) is blocked from hidden docs.
func TestGetDocument_HRCannotSeeHiddenDoc(t *testing.T) {
	svc := &DocumentService{
		DocRepo: &docRepoStub{doc: &models.Document{ID: 1, DealID: 0, IsHidden: true, CreatedBy: intPtr(99)}},
	}
	doc, err := svc.GetDocument(1, 1, authz.RoleHR)
	if err == nil || doc != nil {
		t.Fatal("HR must NOT see hidden document (expected forbidden error)")
	}
}

// TestGetDocument_NonHiddenDocVisibleToManagement is a regression test: management
// must still see non-hidden documents as before.
func TestGetDocument_NonHiddenDocVisibleToManagement(t *testing.T) {
	svc := &DocumentService{
		DocRepo: &docRepoStub{doc: &models.Document{ID: 1, DealID: 0, IsHidden: false}},
	}
	doc, err := svc.GetDocument(1, 1, authz.RoleManagement)
	if err != nil || doc == nil {
		t.Fatalf("management must see non-hidden documents (regression): err=%v", err)
	}
}

// TestGetDocument_NonHiddenDocVisibleToLegal is a regression test for legal.
func TestGetDocument_NonHiddenDocVisibleToLegal(t *testing.T) {
	svc := &DocumentService{
		DocRepo: &docRepoStub{doc: &models.Document{ID: 1, DealID: 0, IsHidden: false}},
	}
	doc, err := svc.GetDocument(1, 1, authz.RoleLegal)
	if err != nil || doc == nil {
		t.Fatalf("legal must see non-hidden documents (regression): err=%v", err)
	}
}

// ─── CreateDocument sets is_hidden for partner ───────────────────────────────

type capturingDocRepo struct {
	docRepoStub
	captured *models.Document
}

func (r *capturingDocRepo) Create(doc *models.Document) (int64, error) {
	captured := *doc
	r.captured = &captured
	return 1, nil
}

// TestCreateDocument_PartnerSetsIsHidden verifies that when a partner creates a document
// is_hidden is set to true and created_by equals the partner's user ID.
func TestCreateDocument_PartnerSetsIsHidden(t *testing.T) {
	const partnerUserID = 7
	branch := 3
	repo := &capturingDocRepo{}
	svc := &DocumentService{
		DocRepo:  repo,
		DealRepo: &dealRepoStub{deal: &models.Deals{ID: 1, BranchID: &branch}},
		UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branch}},
	}
	doc := &models.Document{DealID: 1, DocType: "invoice", Status: "draft", FilePath: "test.pdf"}
	_, err := svc.CreateDocument(doc, partnerUserID, authz.RolePartner)
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	if !repo.captured.IsHidden {
		t.Error("document created by partner must have is_hidden=true")
	}
	if repo.captured.CreatedBy == nil || *repo.captured.CreatedBy != partnerUserID {
		t.Errorf("created_by must equal partner user ID %d, got %v", partnerUserID, repo.captured.CreatedBy)
	}
}

// TestCreateDocument_ManagementNotHidden verifies non-partner roles create non-hidden docs.
func TestCreateDocument_ManagementNotHidden(t *testing.T) {
	repo := &capturingDocRepo{}
	svc := &DocumentService{
		DocRepo:  repo,
		DealRepo: &dealRepoStub{deal: &models.Deals{ID: 1}},
	}
	doc := &models.Document{DealID: 1, DocType: "invoice", Status: "draft", FilePath: "test.pdf"}
	_, err := svc.CreateDocument(doc, 1, authz.RoleManagement)
	if err != nil {
		t.Fatalf("CreateDocument failed: %v", err)
	}
	if repo.captured.IsHidden {
		t.Error("document created by management must NOT be hidden")
	}
}

// ─── List: HiddenVisibilityUserID is injected for non-admin ─────────────────

// filterCapturingDocRepo is a documentFilterRepo that records the filter passed for listing.
type filterCapturingDocRepo struct {
	docRepoStub
	capturedFilter repositories.DocumentListFilter
}

func (r *filterCapturingDocRepo) ListDocumentsWithFilterAndArchiveScope(_ int, _ int, filter repositories.DocumentListFilter, _ repositories.ArchiveScope) ([]*models.Document, error) {
	r.capturedFilter = filter
	return nil, nil
}

func (r *filterCapturingDocRepo) ListDocumentsByDealWithFilterAndArchiveScope(_ int64, filter repositories.DocumentListFilter, _ repositories.ArchiveScope) ([]*models.Document, error) {
	r.capturedFilter = filter
	return nil, nil
}

func (r *filterCapturingDocRepo) CountDocumentsWithFilterAndArchiveScope(filter repositories.DocumentListFilter, _ repositories.ArchiveScope) (int, error) {
	return 0, nil
}

func (r *filterCapturingDocRepo) ListDocumentsByDealWithFilterAndArchiveScopePaginated(_ int64, _, _ int, filter repositories.DocumentListFilter, _ repositories.ArchiveScope) ([]*models.Document, error) {
	r.capturedFilter = filter
	return nil, nil
}

// TestListDocumentsByDealWithFilter_InjectsHiddenVisibilityForNonAdmin verifies that
// non-admin callers get HiddenVisibilityUserID injected into the filter.
func TestListDocumentsByDealWithFilter_InjectsHiddenVisibilityForNonAdmin(t *testing.T) {
	const mgmtUserID = 5
	branch := 1
	repo := &filterCapturingDocRepo{}
	svc := &DocumentService{
		DocRepo:  repo,
		DealRepo: &dealRepoStub{deal: &models.Deals{ID: 1, BranchID: &branch}},
	}

	_, _ = svc.ListDocumentsByDealWithFilter(1, mgmtUserID, authz.RoleManagement, repositories.DocumentListFilter{}, repositories.ArchiveScopeActiveOnly)

	if repo.capturedFilter.HiddenVisibilityUserID == nil {
		t.Fatal("management list must have HiddenVisibilityUserID set")
	}
	if *repo.capturedFilter.HiddenVisibilityUserID != mgmtUserID {
		t.Errorf("HiddenVisibilityUserID must equal caller's userID %d, got %d", mgmtUserID, *repo.capturedFilter.HiddenVisibilityUserID)
	}
}

// TestListDocumentsByDealWithFilter_AdminNoHiddenFilter verifies that admin gets no
// HiddenVisibilityUserID restriction (sees all documents including hidden ones).
func TestListDocumentsByDealWithFilter_AdminNoHiddenFilter(t *testing.T) {
	branch := 1
	repo := &filterCapturingDocRepo{}
	svc := &DocumentService{
		DocRepo:  repo,
		DealRepo: &dealRepoStub{deal: &models.Deals{ID: 1, BranchID: &branch}},
	}

	_, _ = svc.ListDocumentsByDealWithFilter(1, 1, authz.RoleSystemAdmin, repositories.DocumentListFilter{}, repositories.ArchiveScopeActiveOnly)

	if repo.capturedFilter.HiddenVisibilityUserID != nil {
		t.Error("admin list must NOT have HiddenVisibilityUserID set (admin sees all)")
	}
}

// ─── Permission: partner has no documents.send ───────────────────────────────

// TestPartnerHasNoDocumentsSendPermission confirms that after the permission removal
// the partner role cannot call send-for-signature endpoints.
func TestPartnerHasNoDocumentsSendPermission(t *testing.T) {
	if authz.HasPermission("partner", "documents.send") {
		t.Error("partner must NOT have documents.send permission")
	}
}

// TestAdminHasDocumentsSendPermission is a regression check: admin still can send.
func TestAdminHasDocumentsSendPermission(t *testing.T) {
	if !authz.HasPermission("admin", "documents.send") {
		t.Error("admin must still have documents.send permission")
	}
}
