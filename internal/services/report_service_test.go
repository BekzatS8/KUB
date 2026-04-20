package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
)

type reportTestUserRepo struct {
	user *models.User
	err  error
}

func (r *reportTestUserRepo) Create(user *models.User) error { return nil }
func (r *reportTestUserRepo) GetByID(id int) (*models.User, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.user, nil
}
func (r *reportTestUserRepo) Update(user *models.User) error { return nil }
func (r *reportTestUserRepo) Delete(id int) error            { return nil }
func (r *reportTestUserRepo) List(limit, offset int) ([]*models.User, error) {
	return nil, nil
}
func (r *reportTestUserRepo) GetByEmail(email string) (*models.User, error) { return nil, nil }
func (r *reportTestUserRepo) GetCount() (int, error)                        { return 0, nil }
func (r *reportTestUserRepo) GetCountByRole(roleID int) (int, error)        { return 0, nil }
func (r *reportTestUserRepo) UpdatePassword(userID int, passwordHash string) error {
	return nil
}
func (r *reportTestUserRepo) UpdateRefresh(userID int, token string, expiresAt time.Time) error {
	return nil
}
func (r *reportTestUserRepo) RotateRefresh(oldToken, newToken string, newExpiresAt time.Time) (*models.User, error) {
	return nil, nil
}
func (r *reportTestUserRepo) ClearRefresh(userID int) error { return nil }
func (r *reportTestUserRepo) GetByRefreshToken(token string) (*models.User, error) {
	return nil, nil
}
func (r *reportTestUserRepo) VerifyUser(userID int) error { return nil }
func (r *reportTestUserRepo) UpdateTelegramLink(userID int, chatID int64, enable bool) error {
	return nil
}
func (r *reportTestUserRepo) GetByIDSimple(id int) (*models.User, error) { return nil, nil }
func (r *reportTestUserRepo) GetTelegramSettings(ctx context.Context, userID int64) (chatID int64, notify bool, err error) {
	return 0, false, nil
}
func (r *reportTestUserRepo) GetByChatID(ctx context.Context, chatID int64) (*models.User, error) {
	return nil, nil
}

func TestResolveFilters_SalesAndOperationsBoundToOwnBranch(t *testing.T) {
	branchID := 2
	svc := &ReportService{
		UserRepo: &reportTestUserRepo{user: &models.User{BranchID: &branchID}},
	}
	requested := 4

	ownerID, scopedBranchID, err := svc.resolveFilters(77, authz.RoleSales, &requested)
	if err != nil {
		t.Fatalf("sales resolve failed: %v", err)
	}
	if ownerID == nil || *ownerID != 77 {
		t.Fatalf("sales must be owner-scoped, got owner=%v", ownerID)
	}
	if scopedBranchID == nil || *scopedBranchID != branchID {
		t.Fatalf("sales must use own branch, got branch=%v", scopedBranchID)
	}

	ownerID, scopedBranchID, err = svc.resolveFilters(77, authz.RoleOperations, &requested)
	if err != nil {
		t.Fatalf("operations resolve failed: %v", err)
	}
	if ownerID != nil {
		t.Fatalf("operations must not be owner-scoped")
	}
	if scopedBranchID == nil || *scopedBranchID != branchID {
		t.Fatalf("operations must use own branch, got branch=%v", scopedBranchID)
	}
}

func TestResolveFilters_ControlBoundToOwnBranch(t *testing.T) {
	branchID := 7
	requested := 3
	svc := &ReportService{UserRepo: &reportTestUserRepo{user: &models.User{BranchID: &branchID}}}

	ownerID, scopedBranchID, err := svc.resolveFilters(1, authz.RoleControl, &requested)
	if err != nil {
		t.Fatalf("control resolve failed: %v", err)
	}
	if ownerID != nil {
		t.Fatalf("control must not be owner-scoped")
	}
	if scopedBranchID == nil || *scopedBranchID != branchID {
		t.Fatalf("control must be bound to own branch, got %v", scopedBranchID)
	}
}

func TestResolveFilters_ManagementAndAdminCanUseRequestedBranch(t *testing.T) {
	svc := &ReportService{}
	requested := 3
	for _, roleID := range []int{authz.RoleManagement, authz.RoleSystemAdmin} {
		ownerID, scopedBranchID, err := svc.resolveFilters(1, roleID, &requested)
		if err != nil {
			t.Fatalf("role=%d unexpected error: %v", roleID, err)
		}
		if ownerID != nil {
			t.Fatalf("role=%d must not be owner-scoped", roleID)
		}
		if scopedBranchID == nil || *scopedBranchID != requested {
			t.Fatalf("role=%d must keep requested branch filter, got %v", roleID, scopedBranchID)
		}
	}
}

func TestResolveFilters_DeniesWhenBranchContextMissing(t *testing.T) {
	svc := &ReportService{UserRepo: &reportTestUserRepo{err: errors.New("lookup failed")}}
	if _, _, err := svc.resolveFilters(1, authz.RoleSales, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("sales without branch context must be forbidden, got %v", err)
	}
	if _, _, err := svc.resolveFilters(1, authz.RoleOperations, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("operations without branch context must be forbidden, got %v", err)
	}
	if _, _, err := svc.resolveFilters(1, authz.RoleControl, nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("control without branch context must be forbidden, got %v", err)
	}
}
