package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Настройки CRM «ключ → значение» (миграция 101).
const (
	// SettingInboundToCallcenter — 'true': входящие лиды (мессенджеры, звонки)
	// сначала попадают в воронку колл-центра, оператор передаёт их в продажи.
	SettingInboundToCallcenter = "inbound_to_callcenter"
	// CallcenterFunnelCode — воронка колл-центра.
	CallcenterFunnelCode = "callcenter_default"
)

// inboundFunnelIDSQL — воронка нового входящего лида: колл-центр, если маршрут
// включён и воронка активна, иначе — воронка продаж, как раньше.
// channelDeptParam — параметр с отделом канала мессенджера ("" — не задан):
// лиды каналов чужих отделов (визовый, партнёрский) в колл-центр не идут.
func inboundFunnelIDSQL(channelDeptParam string) string {
	deptCond := ""
	if channelDeptParam != "" {
		deptCond = fmt.Sprintf(`
		        AND (%[1]s::int IS NULL OR %[1]s::int = (SELECT department_id FROM funnels WHERE code = 'sales_default'))`, channelDeptParam)
	}
	return `COALESCE(
		(SELECT cf.id FROM funnels cf
		  WHERE cf.code = '` + CallcenterFunnelCode + `' AND cf.is_active = TRUE
		    AND EXISTS (SELECT 1 FROM crm_settings cs WHERE cs.key = '` + SettingInboundToCallcenter + `' AND cs.value = 'true')` + deptCond + `),
		(SELECT sf.id FROM funnels sf WHERE sf.code = 'sales_default'))`
}

// inboundFirstStageSQL — первый активный этап воронки, заданной выражением.
func inboundFirstStageSQL(funnelExpr string) string {
	return `(SELECT fs.id FROM funnel_stages fs
		 WHERE fs.funnel_id = ` + funnelExpr + ` AND fs.is_active = TRUE
		 ORDER BY fs.position ASC, fs.id ASC LIMIT 1)`
}

type CRMSettingsRepository struct {
	db *sql.DB
}

func NewCRMSettingsRepository(db *sql.DB) *CRMSettingsRepository {
	return &CRMSettingsRepository{db: db}
}

func (r *CRMSettingsRepository) Get(ctx context.Context, key string) (string, error) {
	var v string
	err := r.db.QueryRowContext(ctx, `SELECT value FROM crm_settings WHERE key = $1`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// InboundRouting — состояние маршрута входящих в колл-центр и сколько
// работающих операторов (без них входящие разбирать некому).
type InboundRouting struct {
	ToCallcenter    bool `json:"to_callcenter"`
	FunnelExists    bool `json:"funnel_exists"`
	FunnelID        int  `json:"funnel_id,omitempty"`
	OperatorsActive int  `json:"operators_active"`
}

func (r *CRMSettingsRepository) InboundRouting(ctx context.Context, operatorRoleID int) (*InboundRouting, error) {
	out := &InboundRouting{}
	v, err := r.Get(ctx, SettingInboundToCallcenter)
	if err != nil {
		return nil, err
	}
	out.ToCallcenter = strings.EqualFold(strings.TrimSpace(v), "true")
	var fid sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT id FROM funnels WHERE code = $1`, CallcenterFunnelCode).Scan(&fid); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	out.FunnelExists = fid.Valid
	out.FunnelID = int(fid.Int64)
	if err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM users WHERE role_id = $1 AND COALESCE(is_active, TRUE)`,
		operatorRoleID).Scan(&out.OperatorsActive); err != nil {
		return nil, err
	}
	return out, nil
}

// SetInboundToCallcenter включает/выключает маршрут. При включении воронка
// колл-центра активируется (на доске появляется). При выключении воронка
// остаётся: в ней могут быть лиды, которые операторы ещё не передали.
func (r *CRMSettingsRepository) SetInboundToCallcenter(ctx context.Context, on bool, userID int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	value := "false"
	if on {
		value = "true"
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO crm_settings (key, value, updated_at, updated_by)
		VALUES ($1, $2, NOW(), NULLIF($3, 0))
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW(), updated_by = EXCLUDED.updated_by`,
		SettingInboundToCallcenter, value, userID); err != nil {
		return fmt.Errorf("save inbound routing: %w", err)
	}
	if on {
		res, err := tx.ExecContext(ctx, `UPDATE funnels SET is_active = TRUE, updated_at = NOW() WHERE code = $1`, CallcenterFunnelCode)
		if err != nil {
			return fmt.Errorf("activate callcenter funnel: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("callcenter funnel not found")
		}
	}
	return tx.Commit()
}

// FunnelDepartment — отдел, для которого можно завести воронку.
type FunnelDepartment struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

// FunnelDepartments — отделы с воронками: продажи, визовый, партнёрский и
// колл-центр. Раньше форма воронки брала отделы из уже существующих воронок —
// первую воронку колл-центра создать было нельзя.
func (r *CRMSettingsRepository) FunnelDepartments(ctx context.Context) ([]FunnelDepartment, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, code FROM departments
		WHERE is_active = TRUE AND code IN ('sales', 'visa', 'partner', 'operator')
		ORDER BY CASE code WHEN 'sales' THEN 1 WHEN 'visa' THEN 2 WHEN 'partner' THEN 3 ELSE 4 END, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FunnelDepartment{}
	for rows.Next() {
		var d FunnelDepartment
		if err := rows.Scan(&d.ID, &d.Name, &d.Code); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
