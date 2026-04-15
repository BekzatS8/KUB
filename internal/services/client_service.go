package services

import (
	"context"
	"database/sql"
	"errors"
	"net/mail"
	"strings"
	"time"

	"turcompany/internal/authz"
	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

type ClientService struct {
	Repo     *repositories.ClientRepository
	FileRepo *repositories.ClientFileRepository
}

var allowedEducationLevels = map[string]struct{}{
	"higher":            {},
	"secondary_special": {},
	"secondary":         {},
	"primary":           {},
	"incomplete_higher": {},
}

func NewClientService(repo *repositories.ClientRepository, fileRepo ...*repositories.ClientFileRepository) *ClientService {
	service := &ClientService{Repo: repo}
	if len(fileRepo) > 0 {
		service.FileRepo = fileRepo[0]
	}
	return service
}

type ClientProfilePayload struct {
	Client             *models.Client
	ClientRef          models.TypedClientRef
	MissingYellow      []string
	MissingContract    []string
	CompletenessType   string
	ContractReady      bool
	PhotoExists        bool
	PrimaryFileExists  map[string]bool
	PrimaryFileCatalog []string
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

func normalizeClientType(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return models.ClientTypeIndividual, nil
	}
	switch v {
	case models.ClientTypeIndividual, models.ClientTypeLegal:
		return v, nil
	default:
		return "", ErrInvalidClientType
	}
}

func normalizeRequiredClientType(value string) (string, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return "", ErrClientTypeRequired
	}
	return normalizeClientType(v)
}

func ensureClientTypeImmutable(storedType, requestedType string) (string, error) {
	stored, err := normalizeRequiredClientType(storedType)
	if err != nil {
		return "", err
	}
	requested, err := normalizeClientType(requestedType)
	if err != nil {
		return "", err
	}
	if requested != stored {
		return "", ErrClientTypeImmutable
	}
	return stored, nil
}

func (s *ClientService) authorizeWrite(roleID int) error {
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
	c.EducationLevel = trim(c.EducationLevel)
	c.Job = trim(c.Job)
	c.TripsLast5Years = trim(c.TripsLast5Years)
	c.RelativesInDestination = trim(c.RelativesInDestination)
	c.TrustedPerson = trim(c.TrustedPerson)
	c.Specialty = trim(c.Specialty)
	c.TrustedPersonPhone = normalizePhone(trim(c.TrustedPersonPhone))
	c.DriverLicenseNumber = trim(c.DriverLicenseNumber)
	c.EducationInstitutionName = trim(c.EducationInstitutionName)
	c.EducationInstitutionAddress = trim(c.EducationInstitutionAddress)
	c.Position = trim(c.Position)
	c.VisasReceived = trim(c.VisasReceived)
	c.VisaRefusals = trim(c.VisaRefusals)
	c.TherapistName = trim(c.TherapistName)
	c.ClinicName = trim(c.ClinicName)
	c.DiseasesLast3Years = trim(c.DiseasesLast3Years)
	c.AdditionalInfo = trim(c.AdditionalInfo)
	clientType, err := normalizeClientType(c.ClientType)
	if err != nil {
		return err
	}
	c.ClientType = clientType
	if c.ClientType == models.ClientTypeLegal {
		c.EducationLevel = ""
		normalizeLegalAliases(c)
	}
	if c.ClientType == models.ClientTypeIndividual {
		normalizeIndividualAliases(c)
		if c.EducationLevel != "" {
			if _, ok := allowedEducationLevels[c.EducationLevel]; !ok {
				return ErrInvalidEducationLevel
			}
		}
	}

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
	c.DisplayName = c.Name
	c.PrimaryPhone = c.Phone
	c.PrimaryEmail = c.Email

	return nil
}

func normalizeIndividualAliases(c *models.Client) {
	trim := strings.TrimSpace
	if c.IndividualProfile == nil {
		return
	}
	ip := c.IndividualProfile
	ip.LastName = trim(ip.LastName)
	ip.FirstName = trim(ip.FirstName)
	ip.MiddleName = trim(ip.MiddleName)
	ip.IIN = trim(ip.IIN)
	ip.IDNumber = trim(ip.IDNumber)
	ip.PassportSeries = trim(ip.PassportSeries)
	ip.PassportNumber = trim(ip.PassportNumber)
	ip.RegistrationAddress = trim(ip.RegistrationAddress)
	ip.ActualAddress = trim(ip.ActualAddress)
	ip.Country = trim(ip.Country)
	ip.TripPurpose = trim(ip.TripPurpose)
	ip.BirthPlace = trim(ip.BirthPlace)
	ip.Citizenship = trim(ip.Citizenship)
	ip.Sex = trim(ip.Sex)
	ip.MaritalStatus = trim(ip.MaritalStatus)
	ip.PreviousLastName = trim(ip.PreviousLastName)
	ip.SpouseName = trim(ip.SpouseName)
	ip.SpouseContacts = trim(ip.SpouseContacts)
	ip.Education = trim(ip.Education)
	ip.EducationLevel = trim(ip.EducationLevel)
	ip.Job = trim(ip.Job)
	ip.TripsLast5Years = trim(ip.TripsLast5Years)
	ip.RelativesInDestination = trim(ip.RelativesInDestination)
	ip.TrustedPerson = trim(ip.TrustedPerson)
	ip.Specialty = trim(ip.Specialty)
	ip.TrustedPersonPhone = normalizePhone(trim(ip.TrustedPersonPhone))
	ip.DriverLicenseNumber = trim(ip.DriverLicenseNumber)
	ip.EducationInstitutionName = trim(ip.EducationInstitutionName)
	ip.EducationInstitutionAddress = trim(ip.EducationInstitutionAddress)
	ip.Position = trim(ip.Position)
	ip.VisasReceived = trim(ip.VisasReceived)
	ip.VisaRefusals = trim(ip.VisaRefusals)
	ip.TherapistName = trim(ip.TherapistName)
	ip.ClinicName = trim(ip.ClinicName)
	ip.DiseasesLast3Years = trim(ip.DiseasesLast3Years)
	ip.AdditionalInfo = trim(ip.AdditionalInfo)

	if ip.LastName != "" {
		c.LastName = ip.LastName
	}
	if ip.FirstName != "" {
		c.FirstName = ip.FirstName
	}
	if ip.MiddleName != "" {
		c.MiddleName = ip.MiddleName
	}
	if ip.IIN != "" {
		c.IIN = ip.IIN
	}
	if ip.IDNumber != "" {
		c.IDNumber = ip.IDNumber
	}
	if ip.PassportSeries != "" {
		c.PassportSeries = ip.PassportSeries
	}
	if ip.PassportNumber != "" {
		c.PassportNumber = ip.PassportNumber
	}
	if ip.RegistrationAddress != "" {
		c.RegistrationAddress = ip.RegistrationAddress
	}
	if ip.ActualAddress != "" {
		c.ActualAddress = ip.ActualAddress
	}
	if ip.Country != "" {
		c.Country = ip.Country
	}
	if ip.TripPurpose != "" {
		c.TripPurpose = ip.TripPurpose
	}
	if ip.BirthDate != nil {
		c.BirthDate = ip.BirthDate
	}
	if ip.BirthPlace != "" {
		c.BirthPlace = ip.BirthPlace
	}
	if ip.Citizenship != "" {
		c.Citizenship = ip.Citizenship
	}
	if ip.Sex != "" {
		c.Sex = ip.Sex
	}
	if ip.MaritalStatus != "" {
		c.MaritalStatus = ip.MaritalStatus
	}
	if ip.PassportIssueDate != nil {
		c.PassportIssueDate = ip.PassportIssueDate
	}
	if ip.PassportExpireDate != nil {
		c.PassportExpireDate = ip.PassportExpireDate
	}
	if ip.DriverLicenseIssueDate != nil {
		c.DriverLicenseIssueDate = ip.DriverLicenseIssueDate
	}
	if ip.DriverLicenseExpireDate != nil {
		c.DriverLicenseExpireDate = ip.DriverLicenseExpireDate
	}
	if ip.PreviousLastName != "" {
		c.PreviousLastName = ip.PreviousLastName
	}
	if ip.SpouseName != "" {
		c.SpouseName = ip.SpouseName
	}
	if ip.SpouseContacts != "" {
		c.SpouseContacts = ip.SpouseContacts
	}
	if ip.HasChildren != nil {
		c.HasChildren = ip.HasChildren
	}
	if len(ip.ChildrenList) > 0 {
		c.ChildrenList = ip.ChildrenList
	}
	if ip.Education != "" {
		c.Education = ip.Education
	}
	if ip.EducationLevel != "" {
		c.EducationLevel = ip.EducationLevel
	}
	if ip.Job != "" {
		c.Job = ip.Job
	}
	if ip.TripsLast5Years != "" {
		c.TripsLast5Years = ip.TripsLast5Years
	}
	if ip.RelativesInDestination != "" {
		c.RelativesInDestination = ip.RelativesInDestination
	}
	if ip.TrustedPerson != "" {
		c.TrustedPerson = ip.TrustedPerson
	}
	if ip.Specialty != "" {
		c.Specialty = ip.Specialty
	}
	if ip.TrustedPersonPhone != "" {
		c.TrustedPersonPhone = ip.TrustedPersonPhone
	}
	if ip.DriverLicenseNumber != "" {
		c.DriverLicenseNumber = ip.DriverLicenseNumber
	}
	if ip.EducationInstitutionName != "" {
		c.EducationInstitutionName = ip.EducationInstitutionName
	}
	if ip.EducationInstitutionAddress != "" {
		c.EducationInstitutionAddress = ip.EducationInstitutionAddress
	}
	if ip.Position != "" {
		c.Position = ip.Position
	}
	if ip.VisasReceived != "" {
		c.VisasReceived = ip.VisasReceived
	}
	if ip.VisaRefusals != "" {
		c.VisaRefusals = ip.VisaRefusals
	}
	if ip.Height != nil {
		c.Height = ip.Height
	}
	if ip.Weight != nil {
		c.Weight = ip.Weight
	}
	if len(ip.DriverLicenseCategories) > 0 {
		c.DriverLicenseCategories = ip.DriverLicenseCategories
	}
	if ip.TherapistName != "" {
		c.TherapistName = ip.TherapistName
	}
	if ip.ClinicName != "" {
		c.ClinicName = ip.ClinicName
	}
	if ip.DiseasesLast3Years != "" {
		c.DiseasesLast3Years = ip.DiseasesLast3Years
	}
	if ip.AdditionalInfo != "" {
		c.AdditionalInfo = ip.AdditionalInfo
	}
}

func normalizeLegalAliases(c *models.Client) {
	trim := strings.TrimSpace
	if c.LegalProfile == nil {
		c.LegalProfile = &models.ClientLegalProfile{}
	}
	lp := c.LegalProfile

	lp.CompanyName = trim(lp.CompanyName)
	lp.BIN = trim(lp.BIN)
	lp.LegalForm = trim(lp.LegalForm)
	lp.DirectorFullName = trim(lp.DirectorFullName)
	lp.ContactPersonName = trim(lp.ContactPersonName)
	lp.ContactPersonPosition = trim(lp.ContactPersonPosition)
	lp.ContactPersonPhone = normalizePhone(trim(lp.ContactPersonPhone))
	lp.ContactPersonEmail = trim(lp.ContactPersonEmail)
	lp.LegalAddress = trim(lp.LegalAddress)
	lp.ActualAddress = trim(lp.ActualAddress)
	lp.BankName = trim(lp.BankName)
	lp.IBAN = trim(lp.IBAN)
	lp.BIK = trim(lp.BIK)
	lp.KBE = trim(lp.KBE)
	lp.TaxRegime = trim(lp.TaxRegime)
	lp.Website = trim(lp.Website)
	lp.Industry = trim(lp.Industry)
	lp.CompanySize = trim(lp.CompanySize)
	lp.AdditionalInfo = trim(lp.AdditionalInfo)

	if lp.CompanyName == "" {
		lp.CompanyName = c.Name
	} else {
		c.Name = lp.CompanyName
	}
	if lp.BIN == "" {
		lp.BIN = c.BinIin
	} else {
		c.BinIin = lp.BIN
	}
	if lp.ContactPersonPhone == "" {
		lp.ContactPersonPhone = c.Phone
	} else {
		c.Phone = lp.ContactPersonPhone
	}
	if lp.ContactPersonEmail == "" {
		lp.ContactPersonEmail = c.Email
	} else {
		c.Email = lp.ContactPersonEmail
	}
	if lp.LegalAddress == "" {
		lp.LegalAddress = c.Address
	} else {
		c.Address = lp.LegalAddress
	}
	if c.Address == "" {
		c.Address = lp.ActualAddress
	}
}

func mergeLegalProfile(base, patch *models.ClientLegalProfile) *models.ClientLegalProfile {
	if base == nil && patch == nil {
		return nil
	}
	if base == nil {
		cp := *patch
		return &cp
	}
	merged := *base
	if patch == nil {
		return &merged
	}
	if patch.CompanyName != "" {
		merged.CompanyName = patch.CompanyName
	}
	if patch.BIN != "" {
		merged.BIN = patch.BIN
	}
	if patch.LegalForm != "" {
		merged.LegalForm = patch.LegalForm
	}
	if patch.DirectorFullName != "" {
		merged.DirectorFullName = patch.DirectorFullName
	}
	if patch.ContactPersonName != "" {
		merged.ContactPersonName = patch.ContactPersonName
	}
	if patch.ContactPersonPosition != "" {
		merged.ContactPersonPosition = patch.ContactPersonPosition
	}
	if patch.ContactPersonPhone != "" {
		merged.ContactPersonPhone = patch.ContactPersonPhone
	}
	if patch.ContactPersonEmail != "" {
		merged.ContactPersonEmail = patch.ContactPersonEmail
	}
	if patch.LegalAddress != "" {
		merged.LegalAddress = patch.LegalAddress
	}
	if patch.ActualAddress != "" {
		merged.ActualAddress = patch.ActualAddress
	}
	if patch.BankName != "" {
		merged.BankName = patch.BankName
	}
	if patch.IBAN != "" {
		merged.IBAN = patch.IBAN
	}
	if patch.BIK != "" {
		merged.BIK = patch.BIK
	}
	if patch.KBE != "" {
		merged.KBE = patch.KBE
	}
	if patch.TaxRegime != "" {
		merged.TaxRegime = patch.TaxRegime
	}
	if patch.Website != "" {
		merged.Website = patch.Website
	}
	if patch.Industry != "" {
		merged.Industry = patch.Industry
	}
	if patch.CompanySize != "" {
		merged.CompanySize = patch.CompanySize
	}
	if patch.AdditionalInfo != "" {
		merged.AdditionalInfo = patch.AdditionalInfo
	}
	return &merged
}

func validateCreateRedFields(c *models.Client) error {
	missing := make([]string, 0)
	switch c.ClientType {
	case models.ClientTypeLegal:
		companyName := c.Name
		contactName := ""
		contactPhone := c.Phone
		if c.LegalProfile != nil {
			if strings.TrimSpace(c.LegalProfile.CompanyName) != "" {
				companyName = strings.TrimSpace(c.LegalProfile.CompanyName)
			}
			contactName = strings.TrimSpace(c.LegalProfile.ContactPersonName)
			if strings.TrimSpace(c.LegalProfile.ContactPersonPhone) != "" {
				contactPhone = strings.TrimSpace(c.LegalProfile.ContactPersonPhone)
			}
		}
		if companyName == "" {
			missing = append(missing, "company_name")
		}
		if c.BinIin == "" {
			if c.LegalProfile == nil || strings.TrimSpace(c.LegalProfile.BIN) == "" {
				missing = append(missing, "bin")
			}
		}
		if contactName == "" {
			missing = append(missing, "contact_person_name")
		}
		if contactPhone == "" {
			missing = append(missing, "contact_person_phone")
		}
		if c.Address == "" && (c.LegalProfile == nil || strings.TrimSpace(c.LegalProfile.LegalAddress) == "") {
			missing = append(missing, "legal_address")
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
	id, err := s.Repo.Create(c)
	if err != nil {
		return 0, mapClientDBError(err)
	}
	return id, nil
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
	stableClientType, err := ensureClientTypeImmutable(current.ClientType, c.ClientType)
	if err != nil {
		return err
	}
	c.ClientType = stableClientType
	if c.ClientType == models.ClientTypeLegal {
		c.LegalProfile = mergeLegalProfile(current.LegalProfile, c.LegalProfile)
	}
	if err := s.normalizeAndValidate(c); err != nil {
		return err
	}
	if err := s.ensureEmailUnique(c.Email, c.ID); err != nil {
		return err
	}
	err = s.Repo.Update(c)
	if err != nil {
		return mapClientDBError(err)
	}
	return nil
}

func (s *ClientService) GetByID(id int, userID, roleID int) (*models.Client, error) {
	client, err := s.Repo.GetByID(id)
	if err != nil || client == nil {
		return client, err
	}
	if roleID == authz.RoleSales && client.OwnerID != userID {
		return nil, ErrForbidden
	}
	return client, nil
}

func (s *ClientService) GetByIDWithArchiveScope(id int, userID, roleID int, scope repositories.ArchiveScope) (*models.Client, error) {
	client, err := s.Repo.GetByIDWithArchiveScope(id, scope)
	if err != nil || client == nil {
		return client, err
	}
	if roleID == authz.RoleSales && client.OwnerID != userID {
		return nil, ErrForbidden
	}
	return client, nil
}

func (s *ClientService) Delete(id int, userID, roleID int) error {
	if !authz.CanHardDeleteBusinessEntity(roleID) {
		return ErrForbidden
	}
	current, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
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
	if repositories.IsSQLState(err, repositories.SQLStateForeignKey) {
		return ErrClientInUse
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
		if repositories.IsSQLState(err, repositories.SQLStateUniqueViolation) {
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

func mapClientDBError(err error) error {
	if err == nil {
		return nil
	}
	if !repositories.IsSQLState(err, repositories.SQLStateUniqueViolation) {
		return err
	}
	switch repositories.ConstraintName(err) {
	case "clients_bin_iin_unique_idx":
		return ErrClientAlreadyExists
	case "client_individual_profiles_iin_uq":
		return ErrIndividualIINExists
	case "client_legal_profiles_bin_uq":
		return ErrLegalBINExists
	default:
		return ErrClientAlreadyExists
	}
}

func (s *ClientService) ListAll(limit, offset int, clientType string) ([]*models.Client, error) {
	return s.Repo.ListAllWithFilterAndArchiveScope(limit, offset, repositories.ClientListFilter{ClientType: clientType}, repositories.ArchiveScopeActiveOnly)
}

func (s *ClientService) ListMine(userID, limit, offset int, clientType string) ([]*models.Client, error) {
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(userID, limit, offset, repositories.ClientListFilter{ClientType: clientType}, repositories.ArchiveScopeActiveOnly)
}

func (s *ClientService) ListIndividualsForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	if err := s.validateClientListFilter(&filter); err != nil {
		return nil, err
	}
	return s.Repo.ListIndividualsWithArchiveScope(userID, filter.Query, limit, offset, scope)
}

func (s *ClientService) ListCompaniesForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	if err := s.validateClientListFilter(&filter); err != nil {
		return nil, err
	}
	return s.Repo.ListCompaniesWithArchiveScope(userID, filter.Query, limit, offset, scope)
}

func (s *ClientService) ListForRole(userID, roleID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	if roleID == authz.RoleSales {
		return nil, ErrForbidden
	}
	if err := s.validateClientListFilter(&filter); err != nil {
		return nil, err
	}
	return s.Repo.ListAllWithFilterAndArchiveScope(limit, offset, filter, scope)
}

func (s *ClientService) ListMineWithArchiveScope(userID, limit, offset int, filter repositories.ClientListFilter, scope repositories.ArchiveScope) ([]*models.Client, error) {
	if err := s.validateClientListFilter(&filter); err != nil {
		return nil, err
	}
	return s.Repo.ListByOwnerWithFilterAndArchiveScope(userID, limit, offset, filter, scope)
}

func (s *ClientService) validateClientListFilter(filter *repositories.ClientListFilter) error {
	if filter == nil {
		return nil
	}
	filter.ClientType = strings.ToLower(strings.TrimSpace(filter.ClientType))
	if filter.ClientType != "" {
		if _, err := normalizeClientType(filter.ClientType); err != nil {
			return err
		}
	}
	return nil
}

func (s *ClientService) ArchiveClient(id, userID, roleID int, reason string) error {
	if !authz.CanArchiveBusinessEntity(roleID) {
		return ErrForbidden
	}
	client, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil {
		return err
	}
	if client == nil {
		return ErrClientNotFound
	}
	if roleID == authz.RoleSales && client.OwnerID != userID {
		return ErrForbidden
	}
	if client.IsArchived {
		return nil
	}
	return s.Repo.Archive(id, userID, reason)
}

func (s *ClientService) UnarchiveClient(id, userID, roleID int) error {
	if !authz.CanArchiveBusinessEntity(roleID) {
		return ErrForbidden
	}
	client, err := s.Repo.GetByIDWithArchiveScope(id, repositories.ArchiveScopeAll)
	if err != nil {
		return err
	}
	if client == nil {
		return ErrClientNotFound
	}
	if roleID == authz.RoleSales && client.OwnerID != userID {
		return ErrForbidden
	}
	if !client.IsArchived {
		return ErrNotArchived
	}
	return s.Repo.Unarchive(id)
}

func (s *ClientService) GetMissingYellow(ctx context.Context, clientID, userID, roleID int) ([]string, error) {
	client, err := s.GetByID(clientID, userID, roleID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrClientNotFound
	}

	files, err := s.fetchPrimaryFileExists(ctx, clientID, client.ClientType)
	if err != nil {
		return nil, err
	}
	return missingYellowFields(client, files), nil
}

func missingYellowFields(client *models.Client, primaryFiles map[string]bool) []string {
	if client != nil && client.ClientType == models.ClientTypeLegal {
		return missingYellowFieldsLegal(client)
	}
	return missingYellowFieldsIndividual(client, primaryFiles["photo35x45"])
}

func missingYellowFieldsIndividual(client *models.Client, hasPhoto35x45 bool) []string {
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

func missingYellowFieldsLegal(client *models.Client) []string {
	if client == nil {
		return []string{"company_name", "bin", "legal_address", "contact_person_phone", "contact_person_email"}
	}
	missing := make([]string, 0)
	lp := client.LegalProfile
	companyName := strings.TrimSpace(client.Name)
	bin := strings.TrimSpace(client.BinIin)
	legalAddress := strings.TrimSpace(client.Address)
	actualAddress := strings.TrimSpace(client.ActualAddress)
	director := ""
	contactName := ""
	contactPhone := strings.TrimSpace(client.Phone)
	contactEmail := strings.TrimSpace(client.Email)
	signerPosition := ""
	if lp != nil {
		if v := strings.TrimSpace(lp.CompanyName); v != "" {
			companyName = v
		}
		if v := strings.TrimSpace(lp.BIN); v != "" {
			bin = v
		}
		if v := strings.TrimSpace(lp.LegalAddress); v != "" {
			legalAddress = v
		}
		if v := strings.TrimSpace(lp.ActualAddress); v != "" {
			actualAddress = v
		}
		director = strings.TrimSpace(lp.DirectorFullName)
		contactName = strings.TrimSpace(lp.ContactPersonName)
		if v := strings.TrimSpace(lp.ContactPersonPhone); v != "" {
			contactPhone = v
		}
		if v := strings.TrimSpace(lp.ContactPersonEmail); v != "" {
			contactEmail = v
		}
		signerPosition = strings.TrimSpace(lp.ContactPersonPosition)
	}
	if companyName == "" {
		missing = append(missing, "company_name")
	}
	if bin == "" {
		missing = append(missing, "bin")
	}
	if legalAddress == "" {
		missing = append(missing, "legal_address")
	}
	if actualAddress == "" {
		missing = append(missing, "actual_address")
	}
	if director == "" && contactName == "" {
		missing = append(missing, "director_full_name|contact_person_name")
	}
	if contactPhone == "" {
		missing = append(missing, "contact_person_phone")
	}
	if contactEmail == "" {
		missing = append(missing, "contact_person_email")
	}
	if signerPosition == "" {
		missing = append(missing, "signer_position")
	}
	return missing
}

func contractReadinessMissing(client *models.Client, primaryFiles map[string]bool) []string {
	if client == nil {
		return []string{"client"}
	}
	if client.ClientType != models.ClientTypeLegal {
		return []string{}
	}
	missing := append([]string{}, missingYellowFieldsLegal(client)...)
	lp := client.LegalProfile
	if lp == nil || strings.TrimSpace(lp.BankName) == "" {
		missing = append(missing, "bank_name")
	}
	if lp == nil || strings.TrimSpace(lp.IBAN) == "" {
		missing = append(missing, "iban")
	}
	if lp == nil || strings.TrimSpace(lp.BIK) == "" {
		missing = append(missing, "bik")
	}
	if lp == nil || strings.TrimSpace(lp.KBE) == "" {
		missing = append(missing, "kbe")
	}
	if !primaryFiles["charter"] {
		missing = append(missing, "file:charter")
	}
	if !primaryFiles["bin_certificate"] {
		missing = append(missing, "file:bin_certificate")
	}
	return missing
}

func (s *ClientService) fetchPrimaryFileExists(ctx context.Context, clientID int, clientType string) (map[string]bool, error) {
	files := make(map[string]bool)
	categories := allowedClientFileCategories(clientType)
	for _, category := range categories {
		files[category] = false
		if s.FileRepo == nil {
			continue
		}
		_, err := s.FileRepo.GetPrimaryByCategory(ctx, int64(clientID), category)
		if err == nil {
			files[category] = true
			continue
		}
		if !errors.Is(err, repositories.ErrClientFileNotFound) {
			return nil, err
		}
	}
	return files, nil
}

func (s *ClientService) GetProfile(ctx context.Context, clientID, userID, roleID int) (*ClientProfilePayload, error) {
	client, err := s.GetByID(clientID, userID, roleID)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, ErrClientNotFound
	}

	primaryFiles, err := s.fetchPrimaryFileExists(ctx, clientID, client.ClientType)
	if err != nil {
		return nil, err
	}
	missing := missingYellowFields(client, primaryFiles)
	missingContract := contractReadinessMissing(client, primaryFiles)

	return &ClientProfilePayload{
		Client:             client,
		ClientRef:          client.TypedRef(),
		MissingYellow:      missing,
		MissingContract:    missingContract,
		CompletenessType:   client.ClientType,
		ContractReady:      len(missingContract) == 0,
		PhotoExists:        primaryFiles["photo35x45"],
		PrimaryFileExists:  primaryFiles,
		PrimaryFileCatalog: allowedClientFileCategories(client.ClientType),
	}, nil
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
		ct, err = ensureClientTypeImmutable(current.ClientType, ct)
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
