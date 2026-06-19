package repositories

import (
	"database/sql"
	"errors"

	"turcompany/internal/models"
)

type FunnelTransitionRuleRepository struct {
	db *sql.DB
}

func NewFunnelTransitionRuleRepository(db *sql.DB) *FunnelTransitionRuleRepository {
	return &FunnelTransitionRuleRepository{db: db}
}

const ftrCols = `r.id, r.name, r.from_funnel_id, r.from_stage_id, r.to_funnel_id, r.to_stage_id, r.is_active, r.created_at, r.updated_at`

func scanFTR(row interface{ Scan(...any) error }) (*models.FunnelTransitionRule, error) {
	r := &models.FunnelTransitionRule{}
	if err := row.Scan(
		&r.ID, &r.Name,
		&r.FromFunnelID, &r.FromStageID,
		&r.ToFunnelID, &r.ToStageID,
		&r.IsActive, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *FunnelTransitionRuleRepository) List() ([]*models.FunnelTransitionRule, error) {
	rows, err := r.db.Query(`
		SELECT `+ftrCols+`
		FROM funnel_transition_rules r
		ORDER BY r.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.FunnelTransitionRule
	for rows.Next() {
		rule, err := scanFTR(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

// ListEnriched returns all rules with funnel and stage names attached.
func (r *FunnelTransitionRuleRepository) ListEnriched() ([]*models.FunnelTransitionRule, error) {
	rows, err := r.db.Query(`
		SELECT
			r.id, r.name,
			r.from_funnel_id, r.from_stage_id,
			r.to_funnel_id, r.to_stage_id,
			r.is_active, r.created_at, r.updated_at,
			ff.name AS from_funnel_name,
			fs_from.name AS from_stage_name, fs_from.color AS from_stage_color,
			tf.name AS to_funnel_name,
			fs_to.name AS to_stage_name, fs_to.color AS to_stage_color
		FROM funnel_transition_rules r
		LEFT JOIN funnels ff ON ff.id = r.from_funnel_id
		LEFT JOIN funnel_stages fs_from ON fs_from.id = r.from_stage_id
		LEFT JOIN funnels tf ON tf.id = r.to_funnel_id
		LEFT JOIN funnel_stages fs_to ON fs_to.id = r.to_stage_id
		ORDER BY r.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.FunnelTransitionRule
	for rows.Next() {
		rule := &models.FunnelTransitionRule{}
		var fromFunnelName, fromStageName, fromStageColor string
		var toFunnelName, toStageName, toStageColor string
		if err := rows.Scan(
			&rule.ID, &rule.Name,
			&rule.FromFunnelID, &rule.FromStageID,
			&rule.ToFunnelID, &rule.ToStageID,
			&rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt,
			&fromFunnelName, &fromStageName, &fromStageColor,
			&toFunnelName, &toStageName, &toStageColor,
		); err != nil {
			return nil, err
		}
		rule.FromFunnel = &models.Funnel{ID: rule.FromFunnelID, Name: fromFunnelName}
		rule.FromStage = &models.FunnelStage{ID: rule.FromStageID, Name: fromStageName, Color: fromStageColor}
		rule.ToFunnel = &models.Funnel{ID: rule.ToFunnelID, Name: toFunnelName}
		rule.ToStage = &models.FunnelStage{ID: rule.ToStageID, Name: toStageName, Color: toStageColor}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *FunnelTransitionRuleRepository) GetByID(id int) (*models.FunnelTransitionRule, error) {
	rule, err := scanFTR(r.db.QueryRow(`SELECT `+ftrCols+` FROM funnel_transition_rules r WHERE r.id = $1`, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return rule, nil
}

// FindActiveByTrigger returns rules that fire when a deal enters (fromFunnelID, fromStageID).
func (r *FunnelTransitionRuleRepository) FindActiveByTrigger(fromFunnelID, fromStageID int) ([]*models.FunnelTransitionRule, error) {
	rows, err := r.db.Query(`
		SELECT `+ftrCols+`
		FROM funnel_transition_rules r
		WHERE r.from_funnel_id = $1
		  AND r.from_stage_id  = $2
		  AND r.is_active = TRUE
	`, fromFunnelID, fromStageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.FunnelTransitionRule
	for rows.Next() {
		rule, err := scanFTR(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rule)
	}
	return out, rows.Err()
}

func (r *FunnelTransitionRuleRepository) Create(rule *models.FunnelTransitionRule) error {
	return r.db.QueryRow(`
		INSERT INTO funnel_transition_rules (name, from_funnel_id, from_stage_id, to_funnel_id, to_stage_id, is_active)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, rule.Name, rule.FromFunnelID, rule.FromStageID, rule.ToFunnelID, rule.ToStageID, rule.IsActive).
		Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)
}

func (r *FunnelTransitionRuleRepository) Update(rule *models.FunnelTransitionRule) error {
	result, err := r.db.Exec(`
		UPDATE funnel_transition_rules
		SET name=$1, from_funnel_id=$2, from_stage_id=$3, to_funnel_id=$4, to_stage_id=$5, is_active=$6, updated_at=NOW()
		WHERE id=$7
	`, rule.Name, rule.FromFunnelID, rule.FromStageID, rule.ToFunnelID, rule.ToStageID, rule.IsActive, rule.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *FunnelTransitionRuleRepository) Delete(id int) error {
	result, err := r.db.Exec(`DELETE FROM funnel_transition_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
