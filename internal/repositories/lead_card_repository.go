package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"turcompany/internal/models"
)

// Карточка лида (ТЗ «доработки CRM КУБ», п.3): «Краткая информация», передача
// в отдел продаж, возврат в общую очередь, объединение лидов и история.

// ErrLeadMergeConflict — объединить эти лиды нельзя (тот же лид, удалён и т.п.).
var ErrLeadMergeConflict = errors.New("lead merge conflict")

// loadLeadCardExtras дочитывает поля карточки, которых нет в общем scanLead
// (он же используется списками): краткую информацию, передачу в продажи,
// названия воронки и этапа.
func (r *LeadRepository) loadLeadCardExtras(lead *models.Leads) {
	if lead == nil {
		return
	}
	var (
		fullName, country, visaType, city, age, comment sql.NullString
		handedAt                                        sql.NullTime
		handedBy                                        sql.NullInt64
		handedByName, funnelName, stageName             sql.NullString
	)
	err := r.db.QueryRow(`
		SELECT l.brief_full_name, l.brief_country, l.brief_visa_type, l.brief_city,
		       l.brief_age, l.brief_comment,
		       l.handed_to_sales_at, l.handed_to_sales_by,
		       NULLIF(TRIM(CONCAT_WS(' ', hu.last_name, hu.first_name)), ''),
		       f.name, fs.name
		FROM leads l
		LEFT JOIN users hu ON hu.id = l.handed_to_sales_by
		LEFT JOIN funnels f ON f.id = l.funnel_id
		LEFT JOIN funnel_stages fs ON fs.id = l.stage_id
		WHERE l.id = $1
	`, lead.ID).Scan(&fullName, &country, &visaType, &city, &age, &comment,
		&handedAt, &handedBy, &handedByName, &funnelName, &stageName)
	if err != nil {
		return
	}
	lead.BriefFullName = fullName.String
	lead.BriefCountry = country.String
	lead.BriefVisaType = visaType.String
	lead.BriefCity = city.String
	lead.BriefAge = age.String
	lead.BriefComment = comment.String
	if handedAt.Valid {
		t := handedAt.Time
		lead.HandedToSalesAt = &t
	}
	if handedBy.Valid {
		v := int(handedBy.Int64)
		lead.HandedToSalesBy = &v
	}
	lead.HandedToSalesByName = handedByName.String
	lead.FunnelName = funnelName.String
	lead.StageName = stageName.String
	lead.SharedBranchIDs = r.LeadSharedBranches(lead.ID)
}

// UpdateBrief сохраняет «Краткую информацию» и пишет в историю изменённые
// поля (кто и когда изменил) одной транзакцией.
func (r *LeadRepository) UpdateBrief(ctx context.Context, leadID, userID int, brief models.LeadBrief, changes []models.LeadHistoryEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		UPDATE leads
		SET brief_full_name = NULLIF($2, ''),
		    brief_country   = NULLIF($3, ''),
		    brief_visa_type = NULLIF($4, ''),
		    brief_city      = NULLIF($5, ''),
		    brief_age       = NULLIF($6, ''),
		    brief_comment   = NULLIF($7, '')
		WHERE id = $1
	`, leadID, brief.FullName, brief.Country, brief.VisaType, brief.City, brief.Age, brief.Comment); err != nil {
		return fmt.Errorf("update lead brief: %w", err)
	}
	if err := insertLeadHistoryTx(ctx, tx, leadID, userID, changes); err != nil {
		return err
	}
	return tx.Commit()
}

func insertLeadHistoryTx(ctx context.Context, tx *sql.Tx, leadID, userID int, entries []models.LeadHistoryEntry) error {
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO lead_history (lead_id, user_id, action, field, old_value, new_value, comment)
			VALUES ($1, NULLIF($2, 0), $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''))
		`, leadID, userID, e.Action, e.Field, e.OldValue, e.NewValue, e.Comment); err != nil {
			return fmt.Errorf("insert lead history: %w", err)
		}
	}
	return nil
}

// AddHistory пишет записи истории вне транзакции (смена этапа и т.п.).
func (r *LeadRepository) AddHistory(ctx context.Context, leadID, userID int, entries ...models.LeadHistoryEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := insertLeadHistoryTx(ctx, tx, leadID, userID, entries); err != nil {
		return err
	}
	return tx.Commit()
}

// ListHistory — история лида, новые сверху.
func (r *LeadRepository) ListHistory(ctx context.Context, leadID int) ([]models.LeadHistoryEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT h.id, h.lead_id, h.user_id,
		       COALESCE(NULLIF(TRIM(CONCAT_WS(' ', u.last_name, u.first_name)), ''), u.email, ''),
		       h.action, COALESCE(h.field, ''), COALESCE(h.old_value, ''), COALESCE(h.new_value, ''),
		       COALESCE(h.comment, ''), h.created_at
		FROM lead_history h
		LEFT JOIN users u ON u.id = h.user_id
		WHERE h.lead_id = $1
		ORDER BY h.created_at DESC, h.id DESC
		LIMIT 500
	`, leadID)
	if err != nil {
		return nil, fmt.Errorf("list lead history: %w", err)
	}
	defer rows.Close()
	out := []models.LeadHistoryEntry{}
	for rows.Next() {
		var e models.LeadHistoryEntry
		var uid sql.NullInt64
		if err := rows.Scan(&e.ID, &e.LeadID, &uid, &e.UserName, &e.Action, &e.Field, &e.OldValue, &e.NewValue, &e.Comment, &e.CreatedAt); err != nil {
			return nil, err
		}
		if uid.Valid {
			v := int(uid.Int64)
			e.UserID = &v
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// HandToSales — оператор передаёт лид в отдел продаж: лид встаёт на первый
// этап воронки продаж, ответственный снимается (лид попадает в общую очередь
// МОП), фиксируется кто и когда передал. Возвращает id воронки продаж.
func (r *LeadRepository) HandToSales(ctx context.Context, leadID, userID int) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var funnelID int
	err = tx.QueryRowContext(ctx, `
		UPDATE leads l
		SET funnel_id = f.id,
		    stage_id = (SELECT fs.id FROM funnel_stages fs
		                WHERE fs.funnel_id = f.id AND fs.is_active = TRUE
		                ORDER BY fs.position ASC, fs.id ASC LIMIT 1),
		    department_id = COALESCE(f.department_id, l.department_id),
		    owner_id = NULL,
		    status = CASE WHEN COALESCE(l.status, 'new') = 'cancelled' THEN 'new' ELSE COALESCE(l.status, 'new') END,
		    handed_to_sales_at = NOW(),
		    handed_to_sales_by = NULLIF($2, 0)
		FROM funnels f
		WHERE l.id = $1 AND f.code = 'sales_default'
		RETURNING f.id
	`, leadID, userID).Scan(&funnelID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, errors.New("sales funnel not found")
		}
		return 0, fmt.Errorf("hand lead to sales: %w", err)
	}
	if err := insertLeadHistoryTx(ctx, tx, leadID, userID, []models.LeadHistoryEntry{{Action: "handed_to_sales"}}); err != nil {
		return 0, err
	}
	return funnelID, tx.Commit()
}

// ReturnToQueue — «Вернуть лид»: ответственный снимается, лид уходит в общую
// очередь нераспределённых; причина пишется в историю.
func (r *LeadRepository) ReturnToQueue(ctx context.Context, leadID, userID, previousOwner int, previousOwnerName, reason string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE leads SET owner_id = NULL WHERE id = $1`, leadID); err != nil {
		return fmt.Errorf("return lead: %w", err)
	}
	entry := models.LeadHistoryEntry{Action: "returned", Field: "owner", OldValue: previousOwnerName, Comment: reason}
	if err := insertLeadHistoryTx(ctx, tx, leadID, userID, []models.LeadHistoryEntry{entry}); err != nil {
		return err
	}
	return tx.Commit()
}

// MergeLeads объединяет source в target (п.3.3: лид из Instagram с лидом из
// WhatsApp/Telegram и наоборот). Переписка, звонки, задачи и сделки source
// переезжают на target; пустые поля target заполняются из source (телефон,
// описание, краткая информация, ответственный, филиал); source уходит в
// корзину с пометкой «объединён с лидом #target».
func (r *LeadRepository) MergeLeads(ctx context.Context, targetID, sourceID, userID int) error {
	if targetID == sourceID || targetID <= 0 || sourceID <= 0 {
		return ErrLeadMergeConflict
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	type side struct {
		phone, title, description string
		owner, branch             sql.NullInt64
	}
	load := func(id int) (*side, error) {
		var s side
		var phone, title, description sql.NullString
		err := tx.QueryRowContext(ctx, `
			SELECT phone, title, description, owner_id, branch_id
			FROM leads WHERE id = $1 AND deleted_at IS NULL
			FOR UPDATE
		`, id).Scan(&phone, &title, &description, &s.owner, &s.branch)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrLeadMergeConflict
		}
		if err != nil {
			return nil, err
		}
		s.phone, s.title, s.description = strings.TrimSpace(phone.String), title.String, strings.TrimSpace(description.String)
		return &s, nil
	}
	target, err := load(targetID)
	if err != nil {
		return err
	}
	source, err := load(sourceID)
	if err != nil {
		return err
	}

	// Описание: дописываем то, чего нет у основного лида (второй телефон и
	// первое сообщение объединяемого лида), чтобы ничего не потерять.
	description := target.description
	extra := []string{}
	if source.phone != "" && target.phone != "" && digitsOnly(source.phone) != digitsOnly(target.phone) {
		extra = append(extra, "Доп. телефон: +"+digitsOnly(source.phone))
	}
	if source.description != "" && !strings.Contains(description, source.description) {
		extra = append(extra, fmt.Sprintf("Из лида #%d: %s", sourceID, source.description))
	}
	if len(extra) > 0 {
		if description != "" {
			description += "\n"
		}
		description += strings.Join(extra, "\n")
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE leads t
		SET phone           = COALESCE(NULLIF(t.phone, ''), s.phone),
		    description     = NULLIF($3, ''),
		    owner_id        = COALESCE(t.owner_id, s.owner_id),
		    branch_id       = COALESCE(t.branch_id, s.branch_id),
		    brief_full_name = COALESCE(NULLIF(t.brief_full_name, ''), s.brief_full_name),
		    brief_country   = COALESCE(NULLIF(t.brief_country, ''), s.brief_country),
		    brief_visa_type = COALESCE(NULLIF(t.brief_visa_type, ''), s.brief_visa_type),
		    brief_city      = COALESCE(NULLIF(t.brief_city, ''), s.brief_city),
		    brief_age       = COALESCE(NULLIF(t.brief_age, ''), s.brief_age),
		    brief_comment   = COALESCE(NULLIF(t.brief_comment, ''), s.brief_comment)
		FROM leads s
		WHERE t.id = $1 AND s.id = $2
	`, targetID, sourceID, description); err != nil {
		return fmt.Errorf("merge leads: update target: %w", err)
	}

	moves := []string{
		`UPDATE chats SET lead_ref_id = $1 WHERE lead_ref_id = $2`,
		`UPDATE telephony_calls SET lead_id = $1 WHERE lead_id = $2`,
		`UPDATE tasks SET entity_id = $1 WHERE entity_type = 'lead' AND entity_id = $2`,
		`UPDATE deals SET lead_id = $1 WHERE lead_id = $2`,
	}
	for _, q := range moves {
		if _, err := tx.ExecContext(ctx, q, targetID, sourceID); err != nil {
			return fmt.Errorf("merge leads: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE leads
		SET deleted_at = NOW(),
		    deleted_by = NULLIF($3, 0),
		    merged_into_id = $1,
		    archive_reason = $4
		WHERE id = $2
	`, targetID, sourceID, userID, fmt.Sprintf("Объединён с лидом #%d", targetID)); err != nil {
		return fmt.Errorf("merge leads: retire source: %w", err)
	}

	if err := insertLeadHistoryTx(ctx, tx, targetID, userID, []models.LeadHistoryEntry{{
		Action: "merged", NewValue: fmt.Sprintf("#%d %s", sourceID, source.title),
	}}); err != nil {
		return err
	}
	if err := insertLeadHistoryTx(ctx, tx, sourceID, userID, []models.LeadHistoryEntry{{
		Action: "merged_into", NewValue: fmt.Sprintf("#%d %s", targetID, target.title),
	}}); err != nil {
		return err
	}
	return tx.Commit()
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}


// FindActiveLeadByChat — последний открытый (не в архиве и не удалённый) лид
// переписки мессенджера: по привязке чата к лиду, а для WhatsApp ещё и по
// номеру телефона. Для кнопки «Профиль» в шапке чата (ТЗ «доработки CRM
// КУБ», п.4). 0 — лида нет.
func (r *LeadRepository) FindActiveLeadByChat(ctx context.Context, transport, chatID string) (int, error) {
	transport = strings.ToLower(strings.TrimSpace(transport))
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return 0, nil
	}
	phone := ""
	if transport == "" || transport == "whatsapp" {
		phone = digitsOnly(chatID)
		if len(phone) < 10 {
			phone = ""
		}
	}
	var id int
	err := r.db.QueryRowContext(ctx, `
		SELECT l.id
		FROM leads l
		WHERE l.deleted_at IS NULL
		  AND l.is_archived = FALSE
		  AND (
		        l.id IN (
		            SELECT c.lead_ref_id FROM chats c
		            WHERE c.external_provider = 'wazzup'
		              AND c.lead_ref_id IS NOT NULL
		              AND ($1 = '' OR c.external_transport = $1)
		              AND c.external_chat_id = $2
		        )
		     OR ($3 <> '' AND length(regexp_replace(COALESCE(l.phone, ''), '\D', '', 'g')) >= 10
		         AND RIGHT(regexp_replace(COALESCE(l.phone, ''), '\D', '', 'g'), 10) = RIGHT($3, 10))
		  )
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT 1
	`, transport, chatID, phone).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("find lead by chat: %w", err)
	}
	return id, nil
}
