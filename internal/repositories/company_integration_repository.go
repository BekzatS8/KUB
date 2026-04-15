package repositories

import (
	"database/sql"
	"fmt"

	"turcompany/internal/models"
)

type CompanyIntegrationRepository interface {
	ListByCompanyID(companyID int) ([]models.CompanyIntegration, error)
	Create(integration *models.CompanyIntegration) error
	Update(integration *models.CompanyIntegration) error
	Delete(companyID int, integrationID int64) error
	GetByID(companyID int, integrationID int64) (*models.CompanyIntegration, error)
}

type companyIntegrationRepository struct {
	db *sql.DB
}

func NewCompanyIntegrationRepository(db *sql.DB) CompanyIntegrationRepository {
	return &companyIntegrationRepository{db: db}
}

func (r *companyIntegrationRepository) ListByCompanyID(companyID int) ([]models.CompanyIntegration, error) {
	const q = `
		SELECT id, company_id, integration_type, provider, title, external_account_id, phone, username,
		       meta_json::text, is_active, created_at, updated_at
		FROM company_integrations
		WHERE company_id = $1
		ORDER BY id DESC
	`
	rows, err := r.db.Query(q, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.CompanyIntegration, 0)
	for rows.Next() {
		integration, err := scanCompanyIntegration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *integration)
	}
	return out, rows.Err()
}

func (r *companyIntegrationRepository) Create(integration *models.CompanyIntegration) error {
	const q = `
		INSERT INTO company_integrations (
			company_id, integration_type, provider, title, external_account_id,
			phone, username, meta_json, is_active
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRow(
		q,
		integration.CompanyID,
		integration.IntegrationType,
		integration.Provider,
		integration.Title,
		integration.ExternalAccountID,
		integration.Phone,
		integration.Username,
		integration.MetaJSON,
		integration.IsActive,
	).Scan(&integration.ID, &integration.CreatedAt, &integration.UpdatedAt)
}

func (r *companyIntegrationRepository) Update(integration *models.CompanyIntegration) error {
	const q = `
		UPDATE company_integrations
		SET integration_type = $1,
		    provider = $2,
		    title = $3,
		    external_account_id = $4,
		    phone = $5,
		    username = $6,
		    meta_json = $7::jsonb,
		    is_active = $8,
		    updated_at = NOW()
		WHERE company_id = $9 AND id = $10
	`
	res, err := r.db.Exec(q,
		integration.IntegrationType,
		integration.Provider,
		integration.Title,
		integration.ExternalAccountID,
		integration.Phone,
		integration.Username,
		integration.MetaJSON,
		integration.IsActive,
		integration.CompanyID,
		integration.ID,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *companyIntegrationRepository) Delete(companyID int, integrationID int64) error {
	res, err := r.db.Exec(`DELETE FROM company_integrations WHERE company_id = $1 AND id = $2`, companyID, integrationID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *companyIntegrationRepository) GetByID(companyID int, integrationID int64) (*models.CompanyIntegration, error) {
	const q = `
		SELECT id, company_id, integration_type, provider, title, external_account_id, phone, username,
		       meta_json::text, is_active, created_at, updated_at
		FROM company_integrations
		WHERE company_id = $1 AND id = $2
	`
	integration, err := scanCompanyIntegration(r.db.QueryRow(q, companyID, integrationID))
	if err != nil {
		return nil, err
	}
	return integration, nil
}

type companyIntegrationScanner interface{ Scan(dest ...any) error }

func scanCompanyIntegration(scanner companyIntegrationScanner) (*models.CompanyIntegration, error) {
	integration := &models.CompanyIntegration{}
	var (
		provider sql.NullString
		external sql.NullString
		phone    sql.NullString
		username sql.NullString
		meta     sql.NullString
	)
	if err := scanner.Scan(
		&integration.ID,
		&integration.CompanyID,
		&integration.IntegrationType,
		&provider,
		&integration.Title,
		&external,
		&phone,
		&username,
		&meta,
		&integration.IsActive,
		&integration.CreatedAt,
		&integration.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if provider.Valid {
		integration.Provider = &provider.String
	}
	if external.Valid {
		integration.ExternalAccountID = &external.String
	}
	if phone.Valid {
		integration.Phone = &phone.String
	}
	if username.Valid {
		integration.Username = &username.String
	}
	if meta.Valid {
		integration.MetaJSON = &meta.String
	}
	return integration, nil
}

func ValidateIntegrationType(t string) error {
	switch t {
	case "whatsapp", "telegram", "instagram", "tiktok", "ip_telephony", "binotel":
		return nil
	default:
		return fmt.Errorf("invalid integration_type")
	}
}
