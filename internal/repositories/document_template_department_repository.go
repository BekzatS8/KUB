package repositories

import (
	"context"
	"database/sql"
	"fmt"
)

// DocumentTemplateDepartmentRepository — привязка шаблонов документов к отделам
// (обратная связь 14.07.2026). Сами шаблоны описаны в коде
// (services.documentTypeRegistry), а кому какой нужен — настройка админа.
// Один шаблон может принадлежать нескольким отделам.
type DocumentTemplateDepartmentRepository struct {
	db *sql.DB
}

func NewDocumentTemplateDepartmentRepository(db *sql.DB) *DocumentTemplateDepartmentRepository {
	return &DocumentTemplateDepartmentRepository{db: db}
}

// List возвращает карту doc_type → отделы. Шаблоны без привязки в карте
// отсутствуют — их не видит ни один отдел, пока админ не назначит.
func (r *DocumentTemplateDepartmentRepository) List(ctx context.Context) (map[string][]string, error) {
	const q = `SELECT doc_type, scope FROM document_template_departments ORDER BY doc_type, scope`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list template departments: %w", err)
	}
	defer rows.Close()

	out := map[string][]string{}
	for rows.Next() {
		var docType, scope string
		if err := rows.Scan(&docType, &scope); err != nil {
			return nil, err
		}
		out[docType] = append(out[docType], scope)
	}
	return out, rows.Err()
}

// ListDocTypesByScope возвращает шаблоны одного отдела.
func (r *DocumentTemplateDepartmentRepository) ListDocTypesByScope(ctx context.Context, scope string) ([]string, error) {
	const q = `SELECT doc_type FROM document_template_departments WHERE scope = $1 ORDER BY doc_type`
	rows, err := r.db.QueryContext(ctx, q, scope)
	if err != nil {
		return nil, fmt.Errorf("list template doc types by scope: %w", err)
	}
	defer rows.Close()

	out := []string{}
	for rows.Next() {
		var docType string
		if err := rows.Scan(&docType); err != nil {
			return nil, err
		}
		out = append(out, docType)
	}
	return out, rows.Err()
}

// SetForDocType заменяет набор отделов у шаблона. Пустой scopes убирает шаблон
// из всех отделов. Делается транзакцией: иначе при ошибке вставки шаблон
// остался бы вообще без отделов.
func (r *DocumentTemplateDepartmentRepository) SetForDocType(ctx context.Context, docType string, scopes []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("set template departments: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM document_template_departments WHERE doc_type = $1`, docType); err != nil {
		return fmt.Errorf("set template departments: %w", err)
	}
	for _, scope := range scopes {
		const q = `
			INSERT INTO document_template_departments (doc_type, scope, updated_at)
			VALUES ($1, $2, NOW())
			ON CONFLICT (doc_type, scope) DO UPDATE SET updated_at = NOW()
		`
		if _, err := tx.ExecContext(ctx, q, docType, scope); err != nil {
			return fmt.Errorf("set template departments: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("set template departments: %w", err)
	}
	return nil
}
