package services

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/lib/pq"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type ClientService struct {
	Repo     *repositories.ClientRepository
	FileRepo *repositories.ClientFileRepository
}

func NewClientService(repo *repositories.ClientRepository, fileRepo ...*repositories.ClientFileRepository) *ClientService {
	service := &ClientService{Repo: repo}
	if len(fileRepo) > 0 {
		service.FileRepo = fileRepo[0]
	}
	return service
}

type ClientProfilePayload struct {
	Client        *models.Client
	MissingYellow []string
	PhotoExists   bool
}

type MissingFieldsError struct {
	Fields []string
}

func (e *MissingFieldsError) Error() string {
	return "missing required fields"
}

func (s *ClientService) NormalizeAndValidate(c *models.Client) error {
	return s.normalizeAndValidate(c)
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == "23505"
	}
	return false
}

func normalizeClientType(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return models.ClientTypeIndividual, nil
	}
	switch v {
	case models.ClientTypeIndividual, models.ClientTypeLegal:
		return v, nil
	default:
		return "", errors.New("invalid client_type: allowed values are individual, legal")
	}
}

func (s *ClientService) authorizeWrite(roleID int) error {
	if roleID == authz.RoleAdminStaff {
		return ErrForbidden
	}
	if authz.IsReadOnly(roleID) {
		return ErrReadOnly
	}
	return nil
}

// нормализация + базовая валидация
func (s *ClientService) normalizeAndValidate(c *models.Client) error {
	trim := func(s string) string { return strings.TrimSpace(s) }

	c.Name = trim(c.Name)
	c.LastName = trim(c.LastName)
	c.FirstName = trim(c.FirstName)
	c.MiddleName = trim(c.MiddleName)
	c.IIN = trim(c.IIN)
	c.BinIin = trim(c.BinIin)
	c.Phone = normalizePhone(trim(c.Phone))
	c.Email = trim(c.Email)
	c.Country = trim(c.Country)
	c.TripPurpose = trim(c.TripPurpose)
	c.BirthPlace = trim(c.BirthPlace)
	c.Citizenship = trim(c.Citizenship)
	c.Sex = trim(c.Sex)
	c.MaritalStatus = trim(c.MaritalStatus)
	c.PreviousLastName = trim(c.PreviousLastName)
	c.SpouseName = trim(c.SpouseName)
	c.SpouseContacts = trim(c.SpouseContacts)
	c.Education = trim(c.Education)
	c.Job = trim(c.Job)
	c.TripsLast5Years = trim(c.TripsLast5Years)
	c.RelativesInDestination = trim(c.RelativesInDestination)
	c.TrustedPerson = trim(c.TrustedPerson)
	c.TherapistName = trim(c.TherapistName)
	c.ClinicName = trim(c.ClinicName)
	c.DiseasesLast3Years = trim(c.DiseasesLast3Years)
	c.AdditionalInfo = trim(c.AdditionalInfo)

	clientType, err := normalizeClientType(c.ClientType)
	if err != nil {
		return err
	}
	c.ClientType = clientType

	// если Name пустой, но есть ФИО — собираем отображаемое имя
	if c.Name == "" && (c.LastName != "" || c.FirstName != "") {
		full := strings.TrimSpace(
			strings.Join([]string{c.LastName, c.FirstName, c.MiddleName}, " "),
		)
		c.Name = full
	}

	if c.Name == "" {
		return errors.New("name or (last_name + first_name) is required")
	}

	if c.IIN != "" && len(c.IIN) != 12 {
		return errors.New("iin must be 12 digits")
	}

	if c.Email != "" && !isValidEmail(c.Email) {
		return ErrInvalidEmail
	}

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}

	return nil
}

func validateCreateRedFields(c *models.Client) error {
	missing := make([]string, 0)
	switch c.ClientType {
	case models.ClientTypeLegal:
		if c.Name == "" {
			missing = append(missing, "name")
		}
		if c.BinIin == "" {
			missing = append(missing, "bin_iin")
		}
		if c.Phone == "" {
			missing = append(missing, "phone")
		}
	default:
		if c.Country == "" {
			missing = append(missing, "country")
		}
		if c.TripPurpose == "" {
			missing = append(missing, "trip_purpose")
		}
		if c.BirthDate == nil {
			missing = append(missing, "birth_date")
		}
		if c.Phone == "" {
			missing = append(missing, "phone")
		}
		if c.LastName == "" {
			missing = append(missing, "last_name")
		}
		if c.FirstName == "" {
			missing = append(missing, "first_name")
		}
	}
	if len(missing) > 0 {
		return &MissingFieldsError{Fields: missing}
	}
	return nil
}

func isValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	if email == "" || strings.Contains(email, "{{") || strings.Contains(email, "}}") || strings.Contains(email, " ") {
		return false
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return false
	}
	return addr.Address == email
}

func (s *ClientService) ensureEmailUnique(email string, excludeID int) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	existing, err := s.Repo.GetByEmail(email)
	if err != nil {
		return err
	}
	if existing != nil && existing.ID != excludeID {
		return ErrEmailAlreadyUsed
	}
	return nil
}

func normalizePhone(value string) string {
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (s *ClientService) Create(c *models.Client, userID, roleID int) (int64, error) {
	if err := s.authorizeWrite(roleID); err != nil {
		return 0, err
	}
	if roleID == authz.RoleSales {
		c.OwnerID = userID
	}
	if c.OwnerID == 0 {
		c.OwnerID = userID
	}
	if roleID == authz.RoleSales && c.OwnerID != userID {
		return 0, ErrForbidden
	}
	if err := s.normalizeAndValidate(c); err != nil {
		return 0, err
	}
	if err := validateCreateRedFields(c); err != nil {
		return 0, err
	}
	return s.Repo.Create(c)
}

func (s *ClientService) Update(c *models.Client, userID, roleID int) error {
	if err := s.authorizeWrite(roleID); err != nil {
		return err
	}
	current, err := s.Repo.GetByID(c.ID)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrClientNotFound
	}
	if roleID == authz.RoleSales && current.OwnerID != userID {
		return ErrForbidden
	}
	if roleID != authz.RoleManagement {
		c.OwnerID = current.OwnerID
	}
	if err := s.normalizeAndValidate(c); err != nil {
		return err
	}
	if err := s.ensureEmailUnique(c.Email, c.ID); err != nil {
		return err
	}
	return s.Repo.Update(c)
}

func (s *ClientService) GetByID(id int, userID, roleID int) (*models.Client, error) {
	if roleID == authz.RoleAdminStaff {
		return nil, ErrForbidden
	}
	client, err := s.Repo.GetByID(id)
	if err != nil || client == nil {
		return client, err
	}
	if roleID == authz.RoleSales && client.OwnerID != userID {
		return nil, ErrForbidden
	}
	return client, nil
}

func (s *ClientService) Delete(id int, userID, roleID int) error {
	if err := s.authorizeWrite(roleID); err != nil {
		return err
	}
	current, err := s.Repo.GetByID(id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrClientNotFound
	}
	if roleID == authz.RoleSales && current.OwnerID != userID {
		return ErrForbidden
	}
	err = s.Repo.Delete(id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrClientNotFound
	}
	return err
}

func (s *ClientService) GetOrCreateByBIN(bin string, fallback *models.Client) (*models.Client, error) {
	bin = strings.TrimSpace(bin)
	if bin != "" {
		existing, err := s.Repo.GetByBIN(bin)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	if fallback != nil {
		fallback.IIN = strings.TrimSpace(fallback.IIN)
		fallback.Phone = normalizePhone(strings.TrimSpace(fallback.Phone))
	}

	if fallback != nil && fallback.IIN != "" {
		existing, err := s.Repo.GetByIIN(fallback.IIN)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	if fallback != nil && fallback.IIN == "" && fallback.Phone != "" {
		existing, err := s.Repo.GetByPhone(fallback.Phone)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	if fallback == nil {
		return nil, errors.New("client data is required")
	}

	if err := s.normalizeAndValidate(fallback); err != nil {
		return nil, err
	}

	id, err := s.Repo.Create(fallback)
	if err != nil {
		if isUniqueViolation(err) {
			if fallback.BinIin != "" {
				existing, lookupErr := s.Repo.GetByBIN(fallback.BinIin)
				if lookupErr != nil {
					return nil, lookupErr
				}
				if existing != nil {
					return existing, nil
				}
			}
			if fallback.IIN != "" {
				existing, lookupErr := s.Repo.GetByIIN(fallback.IIN)
				if lookupErr != nil {
					return nil, lookupErr
				}
				if existing != nil {
					return existing, nil
				}
			}
		}
		return nil, err
	}
	fallback.ID = int(id)
	return fallback, nil
}

func (s *ClientService) ListAll(limit, offset int, clientType string) ([]*models.Client, error) {
	return s.Repo.ListAll(limit, offset, clientType)
}

func (s *ClientService) ListMine(userID, limit, offset int, clientType string) ([]*models.Client, error) {
	return s.Repo.ListByOwner(userID, limit, offset, clientType)
}

func (s *ClientService) ListIndividualsForRole(userID, roleID, limit, offset int, q string) ([]*models.Client, error) {
	if roleID == authz.RoleAdminStaff || roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	return s.Repo.ListIndividuals(userID, q, limit, offset)
}

func (s *ClientService) ListCompaniesForRole(userID, roleID, limit, offset int, q string) ([]*models.Client, error) {
	if roleID == authz.RoleAdminStaff || roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	return s.Repo.ListCompanies(userID, q, limit, offset)
}

func (s *ClientService) ListForRole(userID, roleID, limit, offset int, clientType string) ([]*models.Client, error) {
	if roleID == authz.RoleAdminStaff {
		return nil, ErrForbidden
	}
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	if strings.TrimSpace(clientType) != "" {
		if _, err := normalizeClientType(clientType); err != nil {
			return nil, err
		}
	}
	return s.Repo.ListAll(limit, offset, clientType)
}

func (s *ClientService) GetMissingYellow(ctx context.Context, clientID, userID, roleID int) ([]string, error) {
	client, err := s.GetByID(clientID, userID, roleID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrClientNotFound
	}

	hasPhoto := false
	if s.FileRepo != nil {
		_, err = s.FileRepo.GetPrimaryByCategory(ctx, int64(clientID), "photo35x45")
		if err != nil && !errors.Is(err, repositories.ErrClientFileNotFound) {
			return nil, err
		}
		hasPhoto = err == nil
	}

	return missingYellowFields(client, hasPhoto), nil
}

func missingYellowFields(client *models.Client, hasPhoto35x45 bool) []string {
	missing := make([]string, 0)
	if client.MiddleName == "" {
		missing = append(missing, "middle_name")
	}
	if client.BirthPlace == "" {
		missing = append(missing, "birth_place")
	}
	if client.Citizenship == "" {
		missing = append(missing, "citizenship")
	}
	if client.Sex == "" {
		missing = append(missing, "sex")
	}
	if client.MaritalStatus == "" {
		missing = append(missing, "marital_status")
	}
	if client.IIN == "" {
		missing = append(missing, "iin")
	}
	if client.IDNumber == "" {
		missing = append(missing, "id_number")
	}
	if client.PassportNumber == "" {
		missing = append(missing, "passport_number")
	}
	if client.PassportIssueDate == nil {
		missing = append(missing, "passport_issue_date")
	}
	if client.PassportExpireDate == nil {
		missing = append(missing, "passport_expire_date")
	}
	if client.RegistrationAddress == "" {
		missing = append(missing, "registration_address")
	}
	if client.ActualAddress == "" {
		missing = append(missing, "actual_address")
	}
	if client.Email == "" {
		missing = append(missing, "email")
	}
	if !hasPhoto35x45 {
		missing = append(missing, "photo35x45")
	}
	return missing
}

func (s *ClientService) GetProfile(ctx context.Context, clientID, userID, roleID int) (*ClientProfilePayload, error) {
	client, err := s.GetByID(clientID, userID, roleID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrClientNotFound
	}

	missing, err := s.GetMissingYellow(ctx, clientID, userID, roleID)
	if err != nil {
		return nil, err
	}

	photoExists := false
	if s.FileRepo != nil {
		_, err = s.FileRepo.GetPrimaryByCategory(ctx, int64(clientID), "photo35x45")
		if err == nil {
			photoExists = true
		} else if !errors.Is(err, repositories.ErrClientFileNotFound) {
			return nil, err
		}
	}

	return &ClientProfilePayload{Client: client, MissingYellow: missing, PhotoExists: photoExists}, nil
}

func (s *ClientService) Patch(id int, updates map[string]any, userID, roleID int) (*models.Client, error) {
	if err := s.authorizeWrite(roleID); err != nil {
		return nil, err
	}
	current, err := s.Repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrClientNotFound
	}
	if roleID == authz.RoleSales && current.OwnerID != userID {
		return nil, ErrForbidden
	}
	if v, ok := updates["email"]; ok {
		email, _ := v.(string)
		email = strings.TrimSpace(email)
		if email != "" && !isValidEmail(email) {
			return nil, ErrInvalidEmail
		}
		if err := s.ensureEmailUnique(email, id); err != nil {
			return nil, err
		}
		updates["email"] = email
	}
	if v, ok := updates["phone"]; ok {
		phone, _ := v.(string)
		updates["phone"] = normalizePhone(strings.TrimSpace(phone))
	}
	if v, ok := updates["client_type"]; ok {
		ct, _ := v.(string)
		ct, err = normalizeClientType(ct)
		if err != nil {
			return nil, err
		}
		updates["client_type"] = ct
	}
	if roleID != authz.RoleManagement {
		delete(updates, "owner_id")
	}
	if len(updates) == 0 {
		return current, nil
	}
	if err := s.Repo.UpdatePartial(id, updates); err != nil {
		return nil, err
	}
	return s.Repo.GetByID(id)
}
