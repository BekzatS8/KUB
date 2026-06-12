package services

import (
	"context"
	"testing"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type docScopeUserRepoStub struct {
	user *models.User
}

func (r *docScopeUserRepoStub) Create(*models.User) error                   { return nil }
func (r *docScopeUserRepoStub) GetByID(int) (*models.User, error)           { return r.user, nil }
func (r *docScopeUserRepoStub) Update(*models.User) error                   { return nil }
func (r *docScopeUserRepoStub) Delete(int) error                            { return nil }
func (r *docScopeUserRepoStub) List(int, int) ([]*models.User, error)       { return nil, nil }
func (r *docScopeUserRepoStub) GetByEmail(string) (*models.User, error)     { return nil, nil }
func (r *docScopeUserRepoStub) GetAuthByEmail(string) (*models.User, error) { return nil, nil }
func (r *docScopeUserRepoStub) GetCount() (int, error)                      { return 0, nil }
func (r *docScopeUserRepoStub) GetCountByRole(int) (int, error)             { return 0, nil }
func (r *docScopeUserRepoStub) UpdatePassword(int, string) error            { return nil }
func (r *docScopeUserRepoStub) UpdateRefresh(int, string, time.Time) error  { return nil }
func (r *docScopeUserRepoStub) RotateRefresh(string, string, time.Time) (*models.User, error) {
	return nil, nil
}
func (r *docScopeUserRepoStub) ClearRefresh(int) error                         { return nil }
func (r *docScopeUserRepoStub) GetByRefreshToken(string) (*models.User, error) { return nil, nil }
func (r *docScopeUserRepoStub) VerifyUser(int) error                           { return nil }
func (r *docScopeUserRepoStub) UpdateTelegramLink(int, int64, bool) error      { return nil }
func (r *docScopeUserRepoStub) GetByIDSimple(int) (*models.User, error)                            { return nil, nil }
func (r *docScopeUserRepoStub) UpdateProfile(int, *models.User) error                              { return nil }
func (r *docScopeUserRepoStub) UpdateAvatar(int, string, string, string) error                     { return nil }
func (r *docScopeUserRepoStub) UpdateAvatarCrop(int, *float64, *float64, *float64, *float64) error { return nil }
func (r *docScopeUserRepoStub) DeleteAvatar(int) error                                             { return nil }
func (r *docScopeUserRepoStub) GetTelegramSettings(context.Context, int64) (int64, bool, error) {
	return 0, false, nil
}
func (r *docScopeUserRepoStub) GetByChatID(context.Context, int64) (*models.User, error) {
	return nil, nil
}
func (r *docScopeUserRepoStub) GetDepartmentIDByCode(string) (*int, error) { return nil, nil }

func TestResolveListBranchScope_ScopedRolesIgnoreRequestedBranch(t *testing.T) {
	branchID := 2
	svc := &DocumentService{UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}}
	requested := int64(9)

	got, err := svc.ResolveListBranchScope(100, authz.RoleVisa, &requested)
	if err != nil {
		t.Fatalf("visa ResolveListBranchScope failed: %v", err)
	}
	if got == nil || *got != 2 {
		t.Fatalf("visa must be scoped to own branch, got %+v", got)
	}

	got, err = svc.ResolveListBranchScope(100, authz.RoleControl, &requested)
	if err != nil {
		t.Fatalf("control ResolveListBranchScope failed: %v", err)
	}
	if got == nil || *got != 2 {
		t.Fatalf("control must be scoped to own branch, got %+v", got)
	}
}

func TestResolveListBranchScope_AdminRoleKeepsRequestedBranch(t *testing.T) {
	svc := &DocumentService{}
	requested := int64(12)
	got, err := svc.ResolveListBranchScope(1, authz.RoleSystemAdmin, &requested)
	if err != nil {
		t.Fatalf("ResolveListBranchScope failed: %v", err)
	}
	if got == nil || *got != 12 {
		t.Fatalf("expected requested branch 12, got %+v", got)
	}
}

// TestResolveListBranchScope_PartnerIsBranchScoped verifies that partner (role 70) is
// restricted to its own branch, same as visa/sales/control.
func TestResolveListBranchScope_PartnerIsBranchScoped(t *testing.T) {
	branchID := 5
	svc := &DocumentService{UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}}
	requested := int64(99)

	got, err := svc.ResolveListBranchScope(100, authz.RolePartner, &requested)
	if err != nil {
		t.Fatalf("partner ResolveListBranchScope failed: %v", err)
	}
	if got == nil || *got != int64(branchID) {
		t.Fatalf("partner must be scoped to own branch %d, got %+v", branchID, got)
	}
}

// TestResolveListBranchScope_HRAndLegalAreUnrestricted verifies that HR and Legal have
// no branch restriction (return nil branchScope like management/admin).
func TestResolveListBranchScope_HRAndLegalAreUnrestricted(t *testing.T) {
	svc := &DocumentService{}
	requested := int64(7)

	for _, roleID := range []int{authz.RoleHR, authz.RoleLegal} {
		got, err := svc.ResolveListBranchScope(1, roleID, &requested)
		if err != nil {
			t.Fatalf("role %d ResolveListBranchScope failed: %v", roleID, err)
		}
		// Unrestricted roles return the requested branch unchanged.
		if got == nil || *got != 7 {
			t.Fatalf("role %d must be unrestricted (keep requested branch 7), got %+v", roleID, got)
		}
	}
}

// TestBranchScopeForRole_UnknownRoleReturnsForbidden ensures unknown role IDs remain blocked.
func TestBranchScopeForRole_UnknownRoleReturnsForbidden(t *testing.T) {
	svc := &DocumentService{}
	_, err := svc.branchScopeForRole(1, 999)
	if err == nil {
		t.Error("unknown role should return ErrForbidden")
	}
}

// TestEnsureDealAccess_PartnerCanAccessOwnBranchDeal verifies partner can see a deal
// belonging to their own branch.
func TestEnsureDealAccess_PartnerCanAccessOwnBranchDeal(t *testing.T) {
	branchID := 3
	svc := &DocumentService{UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branchID}}}

	deal := &models.Deals{BranchID: &branchID}
	if err := svc.ensureDealAccess(deal, 100, authz.RolePartner); err != nil {
		t.Errorf("partner must access deal on own branch: %v", err)
	}
}

// TestEnsureDealAccess_PartnerCannotAccessForeignBranchDeal ensures partner is blocked
// from deals on a different branch.
func TestEnsureDealAccess_PartnerCannotAccessForeignBranchDeal(t *testing.T) {
	userBranch := 3
	dealBranch := 7
	svc := &DocumentService{UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &userBranch}}}

	deal := &models.Deals{BranchID: &dealBranch}
	if err := svc.ensureDealAccess(deal, 100, authz.RolePartner); err == nil {
		t.Error("partner must NOT access deal on foreign branch")
	}
}

// TestEnsureDealAccess_HRAndLegalHaveUnrestrictedDealAccess verifies HR and Legal can
// access deals across all branches (needed to work with HR/legal documents linked to any deal).
func TestEnsureDealAccess_HRAndLegalHaveUnrestrictedDealAccess(t *testing.T) {
	branchID := 9
	svc := &DocumentService{}
	deal := &models.Deals{BranchID: &branchID}

	for _, roleID := range []int{authz.RoleHR, authz.RoleLegal} {
		if err := svc.ensureDealAccess(deal, 1, roleID); err != nil {
			t.Errorf("role %d must have unrestricted deal access: %v", roleID, err)
		}
	}
}

// ─── stubs for QC document tests ───────────────────────────────────────────

type docRepoStub struct{ doc *models.Document }

func (r *docRepoStub) Create(*models.Document) (int64, error) { return 1, nil }
func (r *docRepoStub) GetByID(int64) (*models.Document, error) {
	return r.doc, nil
}
func (r *docRepoStub) GetByIDWithArchiveScope(id int64, _ repositories.ArchiveScope) (*models.Document, error) {
	return r.doc, nil
}
func (r *docRepoStub) ListDocuments(int, int) ([]*models.Document, error)      { return nil, nil }
func (r *docRepoStub) ListDocumentsWithArchiveScope(int, int, repositories.ArchiveScope) ([]*models.Document, error) {
	return nil, nil
}
func (r *docRepoStub) ListDocumentsByDeal(int64) ([]*models.Document, error) { return nil, nil }
func (r *docRepoStub) ListDocumentsByDealWithArchiveScope(int64, repositories.ArchiveScope) ([]*models.Document, error) {
	return nil, nil
}
func (r *docRepoStub) Delete(int64) error                                           { return nil }
func (r *docRepoStub) Archive(int64, int, string) error                             { return nil }
func (r *docRepoStub) Unarchive(int64) error                                        { return nil }
func (r *docRepoStub) UpdateStatus(int64, string) error                             { return nil }
func (r *docRepoStub) MarkSigned(int64, string, time.Time) error                    { return nil }
func (r *docRepoStub) Update(*models.Document) error                                { return nil }
func (r *docRepoStub) UpdateSigningMeta(int64, string, string, string, string) error { return nil }

type dealRepoStub struct{ deal *models.Deals }

func (r *dealRepoStub) GetByID(int) (*models.Deals, error)                    { return r.deal, nil }
func (r *dealRepoStub) GetByLeadID(int) (*models.Deals, error)                { return nil, nil }
func (r *dealRepoStub) GetLatestByClientID(int) (*models.Deals, error)        { return nil, nil }
func (r *dealRepoStub) GetLatestByClientRef(int, string) (*models.Deals, error) { return nil, nil }

// ─── quality_control: delete is always forbidden ────────────────────────────

// TestDeleteDocument_QCForbidden verifies that quality_control cannot hard-delete documents.
// Three guards: RequirePermission middleware (no documents.delete), handler CanHardDeleteBusinessEntity,
// and service CanHardDeleteBusinessEntity — all independently block QC.
func TestDeleteDocument_QCForbidden(t *testing.T) {
	svc := &DocumentService{} // permission check fires before any repo access
	err := svc.DeleteDocument(1, 1, authz.RoleControl)
	if err == nil {
		t.Fatal("quality_control must not hard-delete documents")
	}
}

// TestDeleteDocument_ManagementForbidden verifies management cannot hard-delete.
func TestDeleteDocument_ManagementForbidden(t *testing.T) {
	svc := &DocumentService{}
	if err := svc.DeleteDocument(1, 1, authz.RoleManagement); err == nil {
		t.Fatal("management must not hard-delete documents")
	}
}

// TestDeleteDocument_AdminAllowed verifies admin can delete (gets past permission guard).
func TestDeleteDocument_AdminAllowed(t *testing.T) {
	branch := 1
	svc := &DocumentService{
		DocRepo:  &docRepoStub{doc: &models.Document{}}, // empty file paths → deleteDocumentFiles no-ops
		DealRepo: &dealRepoStub{deal: &models.Deals{BranchID: &branch}},
	}
	if err := svc.DeleteDocument(1, 1, authz.RoleSystemAdmin); err != nil {
		t.Fatalf("admin must be able to delete documents, got: %v", err)
	}
}

// ─── quality_control: submit (documents.update) scope enforcement ────────────

// TestSubmit_QCOwnBranchAllowed verifies QC can submit a document linked to a deal on its own branch.
func TestSubmit_QCOwnBranchAllowed(t *testing.T) {
	branch := 3
	svc := &DocumentService{
		DocRepo:  &docRepoStub{doc: &models.Document{DealID: 1, Status: "draft"}},
		DealRepo: &dealRepoStub{deal: &models.Deals{BranchID: &branch}},
		UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branch}},
	}
	if err := svc.Submit(1, 100, authz.RoleControl); err != nil {
		t.Fatalf("QC must submit own-branch document, got: %v", err)
	}
}

// TestSubmit_QCForeignBranchForbidden verifies QC cannot submit a document from a foreign branch.
func TestSubmit_QCForeignBranchForbidden(t *testing.T) {
	qcBranch, dealBranch := 3, 7
	svc := &DocumentService{
		DocRepo:  &docRepoStub{doc: &models.Document{DealID: 1, Status: "draft"}},
		DealRepo: &dealRepoStub{deal: &models.Deals{BranchID: &dealBranch}},
		UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &qcBranch}},
	}
	if err := svc.Submit(1, 100, authz.RoleControl); err == nil {
		t.Fatal("QC must not submit document from foreign branch")
	}
}

// ─── quality_control: archive (documents.update) scope enforcement ───────────

// TestArchiveDocument_QCOwnBranchAllowed verifies QC can archive a document on its own branch.
func TestArchiveDocument_QCOwnBranchAllowed(t *testing.T) {
	branch := 3
	svc := &DocumentService{
		DocRepo:  &docRepoStub{doc: &models.Document{DealID: 1}},
		DealRepo: &dealRepoStub{deal: &models.Deals{BranchID: &branch}},
		UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &branch}},
	}
	if err := svc.ArchiveDocument(1, 100, authz.RoleControl, "reason"); err != nil {
		t.Fatalf("QC must archive own-branch document, got: %v", err)
	}
}

// TestArchiveDocument_QCForeignBranchForbidden verifies QC cannot archive a foreign-branch document.
func TestArchiveDocument_QCForeignBranchForbidden(t *testing.T) {
	qcBranch, dealBranch := 3, 7
	svc := &DocumentService{
		DocRepo:  &docRepoStub{doc: &models.Document{DealID: 1}},
		DealRepo: &dealRepoStub{deal: &models.Deals{BranchID: &dealBranch}},
		UserRepo: &docScopeUserRepoStub{user: &models.User{BranchID: &qcBranch}},
	}
	if err := svc.ArchiveDocument(1, 100, authz.RoleControl, "reason"); err == nil {
		t.Fatal("QC must not archive foreign-branch document")
	}
}
