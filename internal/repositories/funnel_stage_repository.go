package repositories

import (
	"database/sql"
	"errors"
	"fmt"

	"turcompany/internal/models"
)

type FunnelStageRepository struct {
	db *sql.DB
}

func NewFunnelStageRepository(db *sql.DB) *FunnelStageRepository {
	return &FunnelStageRepository{db: db}
}

var ErrStageHasDeals = errors.New("stage has deals, target stage required to reassign")

func scanFunnelStage(scanner interface{ Scan(dest ...any) error }) (*models.FunnelStage, error) {
	s := &models.FunnelStage{}
	var description sql.NullString
	if err := scanner.Scan(
		&s.ID,
		&s.FunnelID,
		&s.Name,
		&s.Code,
		&s.Color,
		&s.Type,
		&s.Position,
		&s.Probability,
		&description,
		&s.IsActive,
		&s.CreatedAt,
		&s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	s.Description = description.String
	return s, nil
}

const funnelStageColumns = `id, funnel_id, name, code, color, type, position, probability, description, is_active, created_at, updated_at`

func (r *FunnelStageRepository) ListByFunnel(funnelID int) ([]*models.FunnelStage, error) {
	rows, err := r.db.Query(`SELECT `+funnelStageColumns+` FROM funnel_stages WHERE funnel_id = $1 ORDER BY position ASC, id ASC`, funnelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*models.FunnelStage{}
	for rows.Next() {
		s, err := scanFunnelStage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *FunnelStageRepository) GetByID(id int) (*models.FunnelStage, error) {
	s, err := scanFunnelStage(r.db.QueryRow(`SELECT `+funnelStageColumns+` FROM funnel_stages WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *FunnelStageRepository) Create(s *models.FunnelStage) error {
	if s.Position == 0 {
		var maxPos sql.NullInt64
		if err := r.db.QueryRow(`SELECT MAX(position) FROM funnel_stages WHERE funnel_id = $1`, s.FunnelID).Scan(&maxPos); err != nil {
			return err
		}
		s.Position = int(maxPos.Int64) + 10
	}
	return r.db.QueryRow(`
		INSERT INTO funnel_stages (funnel_id, name, code, color, type, position, probability, description, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, created_at, updated_at
	`, s.FunnelID, s.Name, s.Code, s.Color, s.Type, s.Position, s.Probability, s.Description, s.IsActive).
		Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func (r *FunnelStageRepository) Update(s *models.FunnelStage) error {
	result, err := r.db.Exec(`
		UPDATE funnel_stages
		SET name=$1, code=$2, color=$3, type=$4, probability=$5, description=$6, is_active=$7, updated_at=NOW()
		WHERE id=$8
	`, s.Name, s.Code, s.Color, s.Type, s.Probability, s.Description, s.IsActive, s.ID)
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

// Delete removes a stage. If deals still reference it, reassignToStageID must be
// provided (non-nil); all such deals are moved to that stage before deletion.
func (r *FunnelStageRepository) Delete(id int, reassignToStageID *int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var dealCount int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM deals WHERE stage_id = $1`, id).Scan(&dealCount); err != nil {
		return err
	}
	if dealCount > 0 {
		if reassignToStageID == nil {
			err = ErrStageHasDeals
			return err
		}
		if _, err = tx.Exec(`UPDATE deals SET stage_id = $1 WHERE stage_id = $2`, *reassignToStageID, id); err != nil {
			return err
		}
	}

	result, err := tx.Exec(`DELETE FROM funnel_stages WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		err = sql.ErrNoRows
		return err
	}
	return tx.Commit()
}

func (r *FunnelStageRepository) Reorder(funnelID int, ids []int) error {
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
		if _, err = tx.Exec(`UPDATE funnel_stages SET position=$1, updated_at=NOW() WHERE id=$2 AND funnel_id=$3`, (i+1)*10, id, funnelID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *FunnelStageRepository) Duplicate(id int) (*models.FunnelStage, error) {
	src, err := r.GetByID(id)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, sql.ErrNoRows
	}

	var maxPos sql.NullInt64
	if err := r.db.QueryRow(`SELECT MAX(position) FROM funnel_stages WHERE funnel_id = $1`, src.FunnelID).Scan(&maxPos); err != nil {
		return nil, err
	}

	copyName := src.Name + " (копия)"
	copyCode := src.Code
	for i := 1; ; i++ {
		var exists bool
		if err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM funnel_stages WHERE funnel_id=$1 AND code=$2)`, src.FunnelID, copyCode).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			break
		}
		copyCode = fmt.Sprintf("%s_copy%d", src.Code, i)
	}

	dup := &models.FunnelStage{
		FunnelID:    src.FunnelID,
		Name:        copyName,
		Code:        copyCode,
		Color:       src.Color,
		Type:        src.Type,
		Position:    int(maxPos.Int64) + 10,
		Probability: src.Probability,
		Description: src.Description,
		IsActive:    src.IsActive,
	}
	if err := r.Create(dup); err != nil {
		return nil, err
	}
	return dup, nil
}

// ListBoardDeals returns deals belonging to funnelID enriched with client and
// owner display names, for the kanban board. branchID/departmentID apply the
// caller's scope restrictions (nil = unrestricted).
func (r *FunnelStageRepository) ListBoardDeals(funnelID int, branchID, departmentID *int) ([]*models.FunnelBoardDeal, error) {
	// Include deals that belong to this funnel OR have no funnel yet (so they
	// appear in the "unassigned" column and can be dragged into a stage).
	where := []string{"(d.funnel_id = $1 OR d.funnel_id IS NULL)", "d.is_archived = FALSE"}
	args := []any{funnelID}

	if branchID != nil {
		args = append(args, *branchID)
		where = append(where, fmt.Sprintf("d.branch_id = $%d", len(args)))
	}
	if departmentID != nil {
		args = append(args, *departmentID)
		where = append(where, fmt.Sprintf("(d.department_id = $%d OR d.department_id IS NULL)", len(args)))
	}

	rows, err := r.db.Query(`
		SELECT
			d.id, d.lead_id, d.funnel_id, d.stage_id,
			d.client_id, COALESCE(c.client_type, ''),
			COALESCE(NULLIF(c.display_name, ''), c.name, '') AS client_name,
			d.owner_id, COALESCE(TRIM(CONCAT(u.first_name, ' ', u.last_name)), '') AS owner_name,
			d.branch_id, d.amount, d.currency, COALESCE(d.status, 'new'), d.created_at
		FROM deals d
		LEFT JOIN clients c ON c.id = d.client_id
		LEFT JOIN users u ON u.id = d.owner_id
		WHERE `+joinWhere(where)+`
		ORDER BY d.created_at DESC
	`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*models.FunnelBoardDeal{}
	for rows.Next() {
		d := &models.FunnelBoardDeal{}
		var funnelID, stageID, branchID sql.NullInt64
		if err := rows.Scan(
			&d.ID, &d.LeadID, &funnelID, &stageID,
			&d.ClientID, &d.ClientType, &d.ClientName,
			&d.OwnerID, &d.OwnerName,
			&branchID, &d.Amount, &d.Currency, &d.Status, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		if funnelID.Valid {
			v := int(funnelID.Int64)
			d.FunnelID = &v
		}
		if stageID.Valid {
			v := int(stageID.Int64)
			d.StageID = &v
		}
		if branchID.Valid {
			v := int(branchID.Int64)
			d.BranchID = &v
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *FunnelStageRepository) InsertHistory(h *models.DealStageHistory) error {
	return r.db.QueryRow(`
		INSERT INTO deal_stage_history (deal_id, from_stage_id, to_stage_id, changed_by, comment)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, created_at
	`, h.DealID, h.FromStageID, h.ToStageID, h.ChangedBy, h.Comment).Scan(&h.ID, &h.CreatedAt)
}

func (r *FunnelStageRepository) ListHistory(dealID int) ([]*models.DealStageHistory, error) {
	rows, err := r.db.Query(`
		SELECT
			h.id, h.deal_id, h.from_stage_id, h.to_stage_id, h.changed_by,
			COALESCE(TRIM(CONCAT(u.first_name, ' ', u.last_name)), '') AS changed_by_name,
			COALESCE(h.comment, ''), h.created_at,
			fs_from.name, fs_to.name
		FROM deal_stage_history h
		LEFT JOIN users u ON u.id = h.changed_by
		LEFT JOIN funnel_stages fs_from ON fs_from.id = h.from_stage_id
		LEFT JOIN funnel_stages fs_to ON fs_to.id = h.to_stage_id
		WHERE h.deal_id = $1
		ORDER BY h.created_at DESC
	`, dealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []*models.DealStageHistory{}
	for rows.Next() {
		h := &models.DealStageHistory{}
		var fromStageID, toStageID, changedBy sql.NullInt64
		var fromStageName, toStageName sql.NullString
		if err := rows.Scan(
			&h.ID, &h.DealID, &fromStageID, &toStageID, &changedBy,
			&h.ChangedByName, &h.Comment, &h.CreatedAt,
			&fromStageName, &toStageName,
		); err != nil {
			return nil, err
		}
		if fromStageID.Valid {
			v := int(fromStageID.Int64)
			h.FromStageID = &v
			h.FromStage = &models.FunnelStage{ID: v, Name: fromStageName.String}
		}
		if toStageID.Valid {
			v := int(toStageID.Int64)
			h.ToStageID = &v
			h.ToStage = &models.FunnelStage{ID: v, Name: toStageName.String}
		}
		if changedBy.Valid {
			v := int(changedBy.Int64)
			h.ChangedBy = &v
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func joinWhere(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += " AND "
		}
		out += p
	}
	return out
}
