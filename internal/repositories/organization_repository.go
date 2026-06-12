package repositories

import (
	"database/sql"
	"turcompany/internal/models"
)

type OrganizationRepository interface {
	Get() (*models.Organization, error)
	Update(req *models.UpdateOrganizationRequest) (*models.Organization, error)
}

type organizationRepository struct{ db *sql.DB }

func NewOrganizationRepository(db *sql.DB) OrganizationRepository {
	return &organizationRepository{db: db}
}

func (r *organizationRepository) Get() (*models.Organization, error) {
	o := &models.Organization{}
	var legalName, bin, phone, email, address, website, whatsapp, telegram, instagram, tiktok, logoURL sql.NullString
	err := r.db.QueryRow(`
		SELECT id, name, legal_name, bin, phone, email, address, website,
		       whatsapp, telegram, instagram, tiktok, logo_url, created_at, updated_at
		FROM organization WHERE id = 1
	`).Scan(
		&o.ID, &o.Name, &legalName, &bin, &phone, &email, &address, &website,
		&whatsapp, &telegram, &instagram, &tiktok, &logoURL, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	o.LegalName = legalName.String
	o.BIN = bin.String
	o.Phone = phone.String
	o.Email = email.String
	o.Address = address.String
	o.Website = website.String
	o.WhatsApp = whatsapp.String
	o.Telegram = telegram.String
	o.Instagram = instagram.String
	o.TikTok = tiktok.String
	o.LogoURL = logoURL.String
	return o, nil
}

func (r *organizationRepository) Update(req *models.UpdateOrganizationRequest) (*models.Organization, error) {
	_, err := r.db.Exec(`
		UPDATE organization SET
			name       = COALESCE($1,  name),
			legal_name = COALESCE($2,  legal_name),
			bin        = COALESCE($3,  bin),
			phone      = COALESCE($4,  phone),
			email      = COALESCE($5,  email),
			address    = COALESCE($6,  address),
			website    = COALESCE($7,  website),
			whatsapp   = COALESCE($8,  whatsapp),
			telegram   = COALESCE($9,  telegram),
			instagram  = COALESCE($10, instagram),
			tiktok     = COALESCE($11, tiktok),
			logo_url   = COALESCE($12, logo_url),
			updated_at = NOW()
		WHERE id = 1
	`,
		nullableStringPtr(req.Name),
		nullableStringPtr(req.LegalName),
		nullableStringPtr(req.BIN),
		nullableStringPtr(req.Phone),
		nullableStringPtr(req.Email),
		nullableStringPtr(req.Address),
		nullableStringPtr(req.Website),
		nullableStringPtr(req.WhatsApp),
		nullableStringPtr(req.Telegram),
		nullableStringPtr(req.Instagram),
		nullableStringPtr(req.TikTok),
		nullableStringPtr(req.LogoURL),
	)
	if err != nil {
		return nil, err
	}
	return r.Get()
}

func nullableStringPtr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}
