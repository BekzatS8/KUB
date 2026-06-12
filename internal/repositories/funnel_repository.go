package repositories

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"turcompany/internal/models"
)

type FunnelRepository struct {
	db *sql.DB
}

type FunnelListFilter struct {
	DepartmentCode string
	BranchID       *int
	ActiveOnly     bool
}

type LeadFunnelAccess struct {
	ID       int
	OwnerID  int
	BranchID *int
}

func NewFunnelRepository(db *sql.DB) *FunnelRepository {
	return &FunnelRepository{db: db}
}

func scanFunnel(scanner interface{ Scan(dest ...any) error }) (*models.Funnel, error) {
	f := &models.Funnel{}
	dept := &models.Department{}
	var branchID sql.NullInt64
	var branchName, branchCode sql.NullString
	var branchActive sql.NullBool
	var createdBy sql.NullInt64
	if err := scanner.Scan(
		&f.ID,
		&f.Name,
		&f.Code,
		&f.DepartmentID,
		&dept.Name,
		&dept.Code,
		&dept.IsActive,
		&branchID,
		&branchName,
		&branchCode,
		&branchActive,
		&f.IsActive,
		&f.SortOrder,
		&createdBy,
		&f.CreatedAt,
		&f.UpdatedAt,
	); err != nil {
		return nil, err
	}
	dept.ID = f.DepartmentID
	f.Department = dept
	if branchID.Valid {
		id := int(branchID.Int64)
		f.BranchID = &id
		f.Branch = &models.Branch{
			ID:       id,
			Name:     branchName.String,
			Code:     branchCode.String,
			IsActive: !branchActive.Valid || branchActive.Bool,
		}
	}
	if createdBy.Valid {
		id := int(createdBy.Int64)
		f.CreatedBy = &id
	}
	return f, nil
}

func (r *FunnelRepository) List(filter FunnelListFilter) ([]*models.Funnel, error) {
	where := []string{"1=1"}
	args := []any{}
	if filter.ActiveOnly {
		where = append(where, "f.is_active = TRUE")
	}
	if filter.DepartmentCode != "" {
		args = append(args, filter.DepartmentCode)
		where = append(where, fmt.Sprintf("d.code = $%d", len(args)))
	}
	if filter.BranchID != nil {
		args = append(args, *filter.BranchID)
		where = append(where, fmt.Sprintf("(f.branch_id = $%d OR f.branch_id IS NULL)", len(args)))
	}

	rows, err := r.db.Query(`
		SELECT
			f.id, f.name, f.code, f.department_id,
			d.name, d.code, d.is_active,
			f.branch_id, b.name, b.code, b.is_active,
			f.is_active, f.sort_order, f.created_by, f.created_at, f.updated_at
		FROM funnels f
		JOIN departments d ON d.id = f.department_id
		LEFT JOIN branches b ON b.id = f.branch_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY f.sort_order ASC, f.id ASC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*models.Funnel{}
	for rows.Next() {
		f, err := scanFunnel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (r *FunnelRepository) GetByID(id int) (*models.Funnel, error) {
	f, err := scanFunnel(r.db.QueryRow(`
		SELECT
			f.id, f.name, f.code, f.department_id,
			d.name, d.code, d.is_active,
			f.branch_id, b.name, b.code, b.is_active,
			f.is_active, f.sort_order, f.created_by, f.created_at, f.updated_at
		FROM funnels f
		JOIN departments d ON d.id = f.department_id
		LEFT JOIN branches b ON b.id = f.branch_id
		WHERE f.id = $1
	`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return f, nil
}

func (r *FunnelRepository) Create(f *models.Funnel) error {
	return r.db.QueryRow(`
		INSERT INTO funnels (name, code, department_id, branch_id, is_active, sort_order, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id, created_at, updated_at
	`, f.Name, f.Code, f.DepartmentID, f.BranchID, f.IsActive, f.SortOrder, f.CreatedBy).
		Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

func (r *FunnelRepository) Update(f *models.Funnel) error {
	result, err := r.db.Exec(`
		UPDATE funnels
		SET name=$1, code=$2, department_id=$3, branch_id=$4, is_active=$5, sort_order=$6, updated_at=NOW()
		WHERE id=$7
	`, f.Name, f.Code, f.DepartmentID, f.BranchID, f.IsActive, f.SortOrder, f.ID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *FunnelRepository) Delete(id int) error {
	result, err := r.db.Exec(`DELETE FROM funnels WHERE id=$1`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *FunnelRepository) Reorder(ids []int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	for i, id := range ids {
		if _, err = tx.Exec(`UPDATE funnels SET sort_order=$1, updated_at=NOW() WHERE id=$2`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *FunnelRepository) GetLeadFunnelAccess(leadID int) (*LeadFunnelAccess, error) {
	access := &LeadFunnelAccess{}
	var branchID sql.NullInt64
	err := r.db.QueryRow(`
		SELECT id, owner_id, branch_id
		FROM leads
		WHERE id = $1 AND is_archived = FALSE
	`, leadID).Scan(&access.ID, &access.OwnerID, &branchID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if branchID.Valid {
		id := int(branchID.Int64)
		access.BranchID = &id
	}
	return access, nil
}

func (r *FunnelRepository) MoveLeadToFunnel(leadID, funnelID int) error {
	result, err := r.db.Exec(`UPDATE leads SET funnel_id=$1, department_id=(SELECT department_id FROM funnels WHERE id=$1) WHERE id=$2`, funnelID, leadID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
