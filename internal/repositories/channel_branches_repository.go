package repositories

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
)

// Канал мессенджера с несколькими филиалами (миграция 099, замечания
// заказчика 10.11.2026): основной филиал + дополнительные.

// leadSharedBranchCond — условие «лид виден филиалу $n через свой канал
// мессенджера»: чат лида пришёл с канала, у которого этот филиал основной или
// дополнительный. alias — псевдоним таблицы leads.
func leadSharedBranchCond(alias string, n int) string {
	return fmt.Sprintf(`EXISTS (
		SELECT 1 FROM chats sbc
		JOIN wazzup_channels sbw ON sbw.external_channel_id = sbc.external_channel_id
		LEFT JOIN wazzup_channel_branches sbb ON sbb.channel_id = sbw.id
		WHERE sbc.lead_ref_id = %s.id
		  AND (sbw.branch_id = $%d OR sbb.branch_id = $%d)
	)`, alias, n, n)
}

// ChannelBranchesRepository — дополнительные филиалы каналов.
type ChannelBranchesRepository struct {
	db *sql.DB
}

func NewChannelBranchesRepository(db *sql.DB) *ChannelBranchesRepository {
	return &ChannelBranchesRepository{db: db}
}

// ExtraBranches — дополнительные филиалы по каналам (channel id → филиалы).
func (r *ChannelBranchesRepository) ExtraBranches(ctx context.Context) (map[int64][]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT channel_id, branch_id FROM wazzup_channel_branches ORDER BY channel_id, branch_id`)
	if err != nil {
		return nil, fmt.Errorf("list channel branches: %w", err)
	}
	defer rows.Close()
	out := map[int64][]int{}
	for rows.Next() {
		var ch int64
		var b int
		if err := rows.Scan(&ch, &b); err != nil {
			return nil, err
		}
		out[ch] = append(out[ch], b)
	}
	return out, rows.Err()
}

// SetBranches сохраняет основной и дополнительные филиалы канала. Ничьи
// рабочие лиды канала переезжают в основной филиал: лид, который никто ещё не
// взял, должен лежать там, где канал работает (лиды visario.kz копились в
// филиале админа KUB Шымкент). Лиды, уже взятые сотрудником, остаются в своём
// филиале — их видимость шире за счёт дополнительных филиалов. Возвращает,
// сколько лидов перенесено.
func (r *ChannelBranchesRepository) SetBranches(ctx context.Context, channelID int64, primary *int, extras []int) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var primaryArg any
	if primary != nil {
		primaryArg = *primary
	}
	res, err := tx.ExecContext(ctx, `UPDATE wazzup_channels SET branch_id = $1::int, updated_at = NOW() WHERE id = $2`, primaryArg, channelID)
	if err != nil {
		return 0, fmt.Errorf("set channel primary branch: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wazzup_channel_branches WHERE channel_id = $1`, channelID); err != nil {
		return 0, err
	}
	clean := []int{}
	for _, b := range extras {
		if b > 0 && (primary == nil || b != *primary) {
			clean = append(clean, b)
		}
	}
	if len(clean) > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO wazzup_channel_branches (channel_id, branch_id)
			SELECT $1, unnest($2::int[]) ON CONFLICT DO NOTHING`, channelID, pq.Array(clean)); err != nil {
			return 0, fmt.Errorf("set channel extra branches: %w", err)
		}
	}
	var moved int64
	if primary != nil {
		res, err := tx.ExecContext(ctx, `
			UPDATE leads l
			SET branch_id = $2
			WHERE l.owner_id IS NULL
			  AND l.deleted_at IS NULL
			  AND l.is_archived = FALSE
			  AND l.branch_id IS DISTINCT FROM $2
			  AND l.id IN (
			      SELECT c.lead_ref_id FROM chats c
			      JOIN wazzup_channels w ON w.external_channel_id = c.external_channel_id
			      WHERE w.id = $1 AND c.lead_ref_id IS NOT NULL
			  )`, channelID, *primary)
		if err != nil {
			return 0, fmt.Errorf("move channel leads to primary branch: %w", err)
		}
		moved, _ = res.RowsAffected()
	}
	return moved, tx.Commit()
}

// LeadSharedBranches — все филиалы каналов, с которых пришли чаты лида.
func (r *LeadRepository) LeadSharedBranches(leadID int) []int {
	rows, err := r.db.Query(`
		SELECT DISTINCT b FROM (
			SELECT w.branch_id AS b FROM chats c
			JOIN wazzup_channels w ON w.external_channel_id = c.external_channel_id
			WHERE c.lead_ref_id = $1 AND w.branch_id IS NOT NULL
			UNION
			SELECT cb.branch_id FROM chats c
			JOIN wazzup_channels w ON w.external_channel_id = c.external_channel_id
			JOIN wazzup_channel_branches cb ON cb.channel_id = w.id
			WHERE c.lead_ref_id = $1
		) s`, leadID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []int{}
	for rows.Next() {
		var b int
		if rows.Scan(&b) == nil {
			out = append(out, b)
		}
	}
	return out
}
